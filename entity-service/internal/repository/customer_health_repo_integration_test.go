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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// Regression test for a bug where MarkProjectHealthy left a project's
// previously OPEN project_risk row untouched -- reachable whenever "Mark
// Healthy" is invoked without first going through CloseProjectRisk (e.g. via
// the csm-portal-backend's own "to be reviewed" display, which could show
// that option for a project that was genuinely still AT_RISK underneath, due
// to an unrelated sys_id-format bug on the caller side). The orphaned OPEN
// row then made every later OpenProjectRisk call on the same project fail
// forever with "project already has an open risk", even though the project's
// health status read back as HEALTHY with no open risk. Skipped unless
// ENTITY_TEST_DATABASE_URL is set (see account_repo_salesforce_integration_test.go
// for the setup).
func newCustomerHealthIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ENTITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENTITY_TEST_DATABASE_URL is not set; skipping the live-database tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestCustomerHealthRepoIntegration_MarkProjectHealthyClosesAnOrphanedOpenRisk(t *testing.T) {
	pool := newCustomerHealthIntegrationPool(t)
	ctx := context.Background()

	accountID := "6cb00000-0000-4000-8000-000000000001"
	projectID := "6cb00000-0000-4000-8000-000000000002"

	cleanup := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM project_risk WHERE project_id = $1`, projectID); err != nil {
			t.Fatalf("clean project_risk: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM project_health_status WHERE project_id = $1`, projectID); err != nil {
			t.Fatalf("clean project_health_status: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM project WHERE id = $1`, projectID); err != nil {
			t.Fatalf("clean project: %v", err)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, accountID); err != nil {
			t.Fatalf("clean account: %v", err)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now()
	if _, err := pool.Exec(ctx, `
		INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id)
		VALUES ($1, $2, $2, 'test', 'test', 'Orphaned Risk Test Account', 'ACCT-ORPHRISK-1', 'SFORPHRISK1')`,
		accountID, now); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, name, account_id)
		VALUES ($1, $2, $2, 'test', 'test', 'ORPHRISK-PROJ-1', 'SFORPHRISKPROJ1', 'Orphaned Risk Test Project', $3)`,
		projectID, now, accountID); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	repo := NewCustomerHealthRepository(pool)

	risk, err := repo.OpenProjectRisk(ctx, projectID, "a risk nobody closes through the normal flow", "opener@example.com")
	if err != nil {
		t.Fatalf("OpenProjectRisk: %v", err)
	}

	// Simulate "Mark Healthy" being invoked directly, without first calling
	// CloseProjectRisk on the still-open risk -- the exact path the bug
	// reached.
	if _, err := repo.MarkProjectHealthy(ctx, projectID, "reviewer@example.com", nil); err != nil {
		t.Fatalf("MarkProjectHealthy: %v", err)
	}

	statuses, err := repo.GetAccountProjectHealthStatuses(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountProjectHealthStatuses: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected exactly one project health status, got %d", len(statuses))
	}
	got := statuses[0]
	if got.HealthStatus.Status != "HEALTHY" {
		t.Errorf("expected health status HEALTHY, got %q", got.HealthStatus.Status)
	}
	if got.OpenRisk != nil {
		t.Errorf("expected no open risk after MarkProjectHealthy, got %+v", got.OpenRisk)
	}

	var riskStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM project_risk WHERE id = $1`, risk.ID).Scan(&riskStatus); err != nil {
		t.Fatalf("query risk status: %v", err)
	}
	if riskStatus != "CLOSED" {
		t.Errorf("expected the previously open risk to be CLOSED, got %q", riskStatus)
	}

	// The real-world symptom: a subsequent Mark At Risk must now succeed,
	// instead of failing forever with "project already has an open risk".
	if _, err := repo.OpenProjectRisk(ctx, projectID, "a genuinely new risk", "opener2@example.com"); err != nil {
		t.Fatalf("OpenProjectRisk after MarkProjectHealthy should succeed, got: %v", err)
	}
}

// Regression test for a real, reported bug: filtering the Customer Health
// dashboard by risk status also returned accounts whose displayed status was
// healthy or to-be-reviewed. GetAccountsByHealthStatus used to query
// project_risk directly for AT_RISK ("status = 'OPEN'") instead of the same
// project_health_status-based aggregate GetBatchAccountHealthSummaries uses
// for display -- the two could disagree for an account whose project_risk
// row predates the synchronized writes OpenProjectRisk/CloseProjectRisk/
// MarkProjectHealthy perform, or that was written some other way. It also
// had no real case for TO_BE_REVIEWED at all (always returned no accounts),
// even though the service layer's own validProjectHealthStatusValue already
// accepted it. Both are fixed by aggregating project_health_status the exact
// way overallStatus does for a single account.
func TestCustomerHealthRepoIntegration_GetAccountsByHealthStatus(t *testing.T) {
	pool := newCustomerHealthIntegrationPool(t)
	ctx := context.Background()

	// accountAtRisk/projectAtRisk: a genuine open risk, via the normal write
	// path -- must appear under AT_RISK only.
	accountAtRisk := "6cb00000-0000-4000-8000-0000000000a1"
	projectAtRisk := "6cb00000-0000-4000-8000-0000000000a2"
	// accountHealthy/projectHealthy: marked healthy, via the normal write
	// path -- must appear under HEALTHY only.
	accountHealthy := "6cb00000-0000-4000-8000-0000000000b1"
	projectHealthy := "6cb00000-0000-4000-8000-0000000000b2"
	// accountToBeReviewed/projectToBeReviewed: a risk opened then closed,
	// which resets the project's health status to TO_BE_REVIEWED (see
	// closeProjectRiskTx) -- must appear under TO_BE_REVIEWED only. This is
	// the status the repo used to always report zero accounts for.
	accountToBeReviewed := "6cb00000-0000-4000-8000-0000000000c1"
	projectToBeReviewed := "6cb00000-0000-4000-8000-0000000000c2"
	// accountLegacyMismatch/projectLegacyMismatch: marked healthy through the
	// normal write path, THEN given a raw, directly-inserted OPEN
	// project_risk row that bypasses OpenProjectRisk entirely -- reproducing
	// the exact disagreement the reported bug hit (an OPEN project_risk row
	// with no matching AT_RISK project_health_status row). Must NOT appear
	// under AT_RISK: the fix reads project_health_status only, so this
	// account's displayed status (HEALTHY) and its filter membership must
	// agree.
	accountLegacyMismatch := "6cb00000-0000-4000-8000-0000000000d1"
	projectLegacyMismatch := "6cb00000-0000-4000-8000-0000000000d2"

	accounts := []string{accountAtRisk, accountHealthy, accountToBeReviewed, accountLegacyMismatch}
	projects := []string{projectAtRisk, projectHealthy, projectToBeReviewed, projectLegacyMismatch}

	cleanup := func() {
		for _, projectID := range projects {
			if _, err := pool.Exec(ctx, `DELETE FROM project_risk WHERE project_id = $1`, projectID); err != nil {
				t.Fatalf("clean project_risk: %v", err)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM project_health_status WHERE project_id = $1`, projectID); err != nil {
				t.Fatalf("clean project_health_status: %v", err)
			}
			if _, err := pool.Exec(ctx, `DELETE FROM project WHERE id = $1`, projectID); err != nil {
				t.Fatalf("clean project: %v", err)
			}
		}
		for _, accountID := range accounts {
			if _, err := pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, accountID); err != nil {
				t.Fatalf("clean account: %v", err)
			}
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now()
	for i, accountID := range accounts {
		if _, err := pool.Exec(ctx, `
			INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id)
			VALUES ($1, $2, $2, 'test', 'test', $3, $4, $5)`,
			accountID, now,
			fmt.Sprintf("GetAccountsByHealthStatus Test Account %d", i),
			fmt.Sprintf("ACCT-GABHS-%d", i),
			fmt.Sprintf("SFGABHS%d", i)); err != nil {
			t.Fatalf("insert account %d: %v", i, err)
		}
	}
	for i, projectID := range projects {
		if _, err := pool.Exec(ctx, `
			INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, name, account_id)
			VALUES ($1, $2, $2, 'test', 'test', $3, $4, $5, $6)`,
			projectID, now,
			fmt.Sprintf("GABHS-PROJ-%d", i),
			fmt.Sprintf("SFGABHSPROJ%d", i),
			fmt.Sprintf("GetAccountsByHealthStatus Test Project %d", i),
			accounts[i]); err != nil {
			t.Fatalf("insert project %d: %v", i, err)
		}
	}

	repo := NewCustomerHealthRepository(pool)

	if _, err := repo.OpenProjectRisk(ctx, projectAtRisk, "a genuine open risk", "opener@example.com"); err != nil {
		t.Fatalf("OpenProjectRisk: %v", err)
	}

	if _, err := repo.MarkProjectHealthy(ctx, projectHealthy, "reviewer@example.com", nil); err != nil {
		t.Fatalf("MarkProjectHealthy: %v", err)
	}

	reviewedRisk, err := repo.OpenProjectRisk(ctx, projectToBeReviewed, "a risk about to be closed", "opener@example.com")
	if err != nil {
		t.Fatalf("OpenProjectRisk (to-be-reviewed setup): %v", err)
	}
	if _, err := repo.CloseProjectRisk(ctx, reviewedRisk.ID, "resolved", "closer@example.com"); err != nil {
		t.Fatalf("CloseProjectRisk: %v", err)
	}

	if _, err := repo.MarkProjectHealthy(ctx, projectLegacyMismatch, "reviewer@example.com", nil); err != nil {
		t.Fatalf("MarkProjectHealthy (legacy-mismatch setup): %v", err)
	}
	// Simulate a project_risk row that never went through OpenProjectRisk --
	// e.g. data written before the synchronized-write logic existed, or by
	// some other path. project_health_status is deliberately left at HEALTHY
	// from the MarkProjectHealthy call above, disagreeing with this row.
	if _, err := pool.Exec(ctx, `
		INSERT INTO project_risk (id, project_id, account_id, status, opened_comment, opened_by_email, opened_on)
		VALUES (gen_random_uuid(), $1, $2, 'OPEN', 'inserted directly, bypassing OpenProjectRisk', 'legacy@example.com', NOW())`,
		projectLegacyMismatch, accountLegacyMismatch); err != nil {
		t.Fatalf("insert mismatched project_risk row: %v", err)
	}

	atRisk, err := repo.GetAccountsByHealthStatus(ctx, domain.ProjectHealthStatusAtRisk)
	if err != nil {
		t.Fatalf("GetAccountsByHealthStatus(AT_RISK): %v", err)
	}
	healthy, err := repo.GetAccountsByHealthStatus(ctx, domain.ProjectHealthStatusHealthy)
	if err != nil {
		t.Fatalf("GetAccountsByHealthStatus(HEALTHY): %v", err)
	}
	toBeReviewed, err := repo.GetAccountsByHealthStatus(ctx, domain.ProjectHealthStatusToBeReviewed)
	if err != nil {
		t.Fatalf("GetAccountsByHealthStatus(TO_BE_REVIEWED): %v", err)
	}

	if !slices.Contains(atRisk, accountAtRisk) {
		t.Errorf("AT_RISK should include the genuinely at-risk account, got %v", atRisk)
	}
	if slices.Contains(atRisk, accountHealthy) || slices.Contains(atRisk, accountToBeReviewed) {
		t.Errorf("AT_RISK should not include the healthy or to-be-reviewed accounts, got %v", atRisk)
	}
	if slices.Contains(atRisk, accountLegacyMismatch) {
		t.Errorf("AT_RISK should not include an account whose displayed status is HEALTHY just because an unrelated project_risk row is OPEN -- the exact reported bug; got %v", atRisk)
	}

	if !slices.Contains(healthy, accountHealthy) {
		t.Errorf("HEALTHY should include the genuinely healthy account, got %v", healthy)
	}
	if !slices.Contains(healthy, accountLegacyMismatch) {
		t.Errorf("HEALTHY should include the legacy-mismatch account, since its project_health_status row (the source of truth) says HEALTHY, got %v", healthy)
	}
	if slices.Contains(healthy, accountAtRisk) || slices.Contains(healthy, accountToBeReviewed) {
		t.Errorf("HEALTHY should not include the at-risk or to-be-reviewed accounts, got %v", healthy)
	}

	if !slices.Contains(toBeReviewed, accountToBeReviewed) {
		t.Errorf("TO_BE_REVIEWED should include the account whose risk was closed (resetting it to TO_BE_REVIEWED) -- this previously always returned zero accounts; got %v", toBeReviewed)
	}
	if slices.Contains(toBeReviewed, accountAtRisk) || slices.Contains(toBeReviewed, accountHealthy) || slices.Contains(toBeReviewed, accountLegacyMismatch) {
		t.Errorf("TO_BE_REVIEWED should not include the at-risk, healthy, or legacy-mismatch accounts, got %v", toBeReviewed)
	}
}
