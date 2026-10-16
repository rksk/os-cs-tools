// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Regression test for a real, reported bug (digiops-cs#3319): a case's
// best/most-likely/worst-case fix-ETA, correctly set via Set Fix ETA and
// correctly readable on the case's own detail page (GetCaseByID), always
// showed empty on a dashboard list widget backed by SearchCases -- because
// SearchCaseView.BestCaseFixEta/MostLikelyFixEta/WorstCaseFixEta were never
// actually selected by SearchCases' own query on this data source, even
// though the struct field existed and GetCaseByID already read the
// identical work_item columns. Runs against a real Postgres. Skipped
// without CASE_STATS_TEST_DSN.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run SearchCasesFixEtaIntegration

package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// seedSearchFixEtaFixture inserts a fresh, Postgres-generated id (gen_random_uuid(),
// read back via RETURNING -- the same convention every other seeded-fixture
// integration test in this package uses) and a run-unique number/wso2Id/
// created_by, rather than a fixed literal id: two runs against the same shared
// staging database (CodeRabbit flagged this) must never delete or collide with
// each other's fixture.
func seedSearchFixEtaFixture(t *testing.T, pool *pgxpool.Pool) (caseID, createdBy string) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	createdByUser := "sfe-fixture-creator-" + suffix + "@test.local"

	// best_case_eta/most_likely_eta/worst_case_eta live on work_item itself,
	// not "case" -- confirmed directly against the real schema (\d work_item);
	// GetCaseByID's own query already reads them as wi.best_case_eta etc.
	now := time.Now().UTC()
	if err := scoped.QueryRow(ctx,
		`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type,
		                        best_case_eta, most_likely_eta, worst_case_eta)
		 VALUES (gen_random_uuid(), $1, $1, $2, $2, $3, $4, 'search fix-eta integration test fixture', 'CASE',
		         '2026-11-01', '2026-11-05', '2026-11-10')
		 RETURNING id`,
		now, createdByUser, "SFE-TEST-"+suffix, "SFE-TEST-WSO2-"+suffix).Scan(&caseID); err != nil {
		t.Fatalf("seed work_item: %v", err)
	}
	t.Cleanup(func() {
		_, _ = scoped.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, caseID)
	})
	if _, err := scoped.Exec(ctx, `INSERT INTO "case" (id, state) VALUES ($1, 'OPEN')`, caseID); err != nil {
		t.Fatalf("seed case: %v", err)
	}
	return caseID, createdByUser
}

// TestSearchCasesFixEtaIntegration_SearchReturnsTheSameEtaValuesAsGetCaseByID
// proves the two reads agree: a case's fix-ETA dates, once set, show up
// identically whether read via GetCaseByID (the case detail page) or
// SearchCases (a dashboard list widget) -- the exact disagreement the
// reported bug exhibited.
func TestSearchCasesFixEtaIntegration_SearchReturnsTheSameEtaValuesAsGetCaseByID(t *testing.T) {
	pool := caseStatsPool(t)
	sfeCaseID, sfeCreatedByUser := seedSearchFixEtaFixture(t, pool)
	repo := repository.NewCaseRepository(repository.NewScoped(pool))
	scope := repository.SearchScope{Unrestricted: true, ViewerEmail: "sfe-viewer@wso2.com"}
	ctx := repository.WithCallerIdentity(context.Background(), scope)

	detail, err := repo.GetCaseByID(ctx, sfeCaseID, scope)
	if err != nil {
		t.Fatalf("GetCaseByID: %v", err)
	}
	if detail.BestCaseFixEta == nil || *detail.BestCaseFixEta != "2026-11-01" {
		t.Fatalf("GetCaseByID BestCaseFixEta = %v, want \"2026-11-01\"", detail.BestCaseFixEta)
	}
	if detail.MostLikelyFixEta == nil || *detail.MostLikelyFixEta != "2026-11-05" {
		t.Fatalf("GetCaseByID MostLikelyFixEta = %v, want \"2026-11-05\"", detail.MostLikelyFixEta)
	}
	if detail.WorstCaseFixEta == nil || *detail.WorstCaseFixEta != "2026-11-10" {
		t.Fatalf("GetCaseByID WorstCaseFixEta = %v, want \"2026-11-10\"", detail.WorstCaseFixEta)
	}

	req := domain.SearchCasesRequest{
		Parsed:     domain.ParsedCaseFilters{CreatedBy: []string{sfeCreatedByUser}},
		SortBy:     domain.CaseSort{Field: domain.CaseSortFieldCreatedOn, Order: domain.CaseSortOrderDesc},
		Pagination: domain.Pagination{Limit: 5},
	}
	results, total, err := repo.SearchCases(ctx, req, scope)
	if err != nil {
		t.Fatalf("SearchCases: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("SearchCases returned %d result(s) (total=%d), want exactly 1", len(results), total)
	}

	got := results[0]
	if got.ID != sfeCaseID {
		t.Fatalf("SearchCases returned case %q, want %q", got.ID, sfeCaseID)
	}
	if got.BestCaseFixEta == nil || *got.BestCaseFixEta != "2026-11-01" {
		t.Errorf("SearchCases BestCaseFixEta = %v, want \"2026-11-01\" (matching GetCaseByID)", got.BestCaseFixEta)
	}
	if got.MostLikelyFixEta == nil || *got.MostLikelyFixEta != "2026-11-05" {
		t.Errorf("SearchCases MostLikelyFixEta = %v, want \"2026-11-05\" (matching GetCaseByID)", got.MostLikelyFixEta)
	}
	if got.WorstCaseFixEta == nil || *got.WorstCaseFixEta != "2026-11-10" {
		t.Errorf("SearchCases WorstCaseFixEta = %v, want \"2026-11-10\" (matching GetCaseByID)", got.WorstCaseFixEta)
	}
}

// TestSearchCasesFixEtaIntegration_UnsetEtaReadsAsNil proves a case with no
// fix-ETA set at all comes back with all three fields nil, not a zero-value
// date or an error -- the ordinary "never set" state most cases are in.
func TestSearchCasesFixEtaIntegration_UnsetEtaReadsAsNil(t *testing.T) {
	pool := caseStatsPool(t)
	ctx := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	noEtaCreatedBy := "sfe-no-eta-fixture-creator-" + suffix + "@test.local"

	var noEtaCaseID string
	now := time.Now().UTC()
	if err := scoped.QueryRow(ctx,
		`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type)
		 VALUES (gen_random_uuid(), $1, $1, $2, $2, $3, $4, 'no fix-eta fixture', 'CASE')
		 RETURNING id`,
		now, noEtaCreatedBy, "SFE-TEST-NOETA-"+suffix, "SFE-TEST-WSO2-NOETA-"+suffix).Scan(&noEtaCaseID); err != nil {
		t.Fatalf("seed work_item: %v", err)
	}
	t.Cleanup(func() {
		_, _ = scoped.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, noEtaCaseID)
	})
	if _, err := scoped.Exec(ctx, `INSERT INTO "case" (id, state) VALUES ($1, 'OPEN')`, noEtaCaseID); err != nil {
		t.Fatalf("seed case: %v", err)
	}

	repo := repository.NewCaseRepository(repository.NewScoped(pool))
	scope := repository.SearchScope{Unrestricted: true, ViewerEmail: "sfe-viewer@wso2.com"}
	searchCtx := repository.WithCallerIdentity(context.Background(), scope)

	req := domain.SearchCasesRequest{
		Parsed:     domain.ParsedCaseFilters{CreatedBy: []string{noEtaCreatedBy}},
		SortBy:     domain.CaseSort{Field: domain.CaseSortFieldCreatedOn, Order: domain.CaseSortOrderDesc},
		Pagination: domain.Pagination{Limit: 5},
	}
	results, total, err := repo.SearchCases(searchCtx, req, scope)
	if err != nil {
		t.Fatalf("SearchCases: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("SearchCases returned %d result(s) (total=%d), want exactly 1", len(results), total)
	}

	got := results[0]
	if got.BestCaseFixEta != nil {
		t.Errorf("BestCaseFixEta = %v, want nil for a case with no fix-ETA set", *got.BestCaseFixEta)
	}
	if got.MostLikelyFixEta != nil {
		t.Errorf("MostLikelyFixEta = %v, want nil for a case with no fix-ETA set", *got.MostLikelyFixEta)
	}
	if got.WorstCaseFixEta != nil {
		t.Errorf("WorstCaseFixEta = %v, want nil for a case with no fix-ETA set", *got.WorstCaseFixEta)
	}
}
