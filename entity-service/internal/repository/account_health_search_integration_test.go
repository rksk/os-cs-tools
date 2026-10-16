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

// Exercises SearchAccounts' customer-health filters and summary against a real
// Postgres with every migration applied. Skipped without CASE_STATS_TEST_DSN, so
// an ordinary `go test ./...` stays hermetic.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run AccountHealthSearch

package repository_test

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const ahsTag = "ahs-test"

// Fixture accounts, newest created first (the search orders created_on DESC).
//
//	recent   Customer, activated 250 days ago, one project without a go-live
//	         date -> go-live failure; recent case and escalation; an open
//	         migration engagement 200 days old (delayed and abandoned); an EOL
//	         product on a deactivated deployment; AT_RISK; CRE team Alpha.
//	live     Customer, activated in 2020 -> go-live success; nothing else;
//	         HEALTHY; a deactivated deployed product named "Retired".
//	pending  Customer, activated 100 days ago, project without a go-live date
//	         -> pending (3 months); an open migration 90 days old (delayed
//	         only; an engagement also counts as a recent case), a closed
//	         migration 300 days old, a case 400 days old; no health rows.
//	golive   Customer, activated 280 days ago but its project has a go-live
//	         date -> success; a service request 5 days old; an escalation 120
//	         days old; two projects, TO_BE_REVIEWED and HEALTHY; SRE team Alpha.
//	prospect Prospect, no activation date; must never match lifeCycle=Customer.
const (
	ahsAccRecent   = "ac000000-0000-0000-0000-000000000005"
	ahsAccLive     = "ac000000-0000-0000-0000-000000000004"
	ahsAccPending  = "ac000000-0000-0000-0000-000000000003"
	ahsAccGoLive   = "ac000000-0000-0000-0000-000000000002"
	ahsAccProspect = "ac000000-0000-0000-0000-000000000001"
)

func seedAccountHealthFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)

	cleanup := func() {
		for _, q := range []string{
			`DELETE FROM project_health_status WHERE account_id IN (SELECT id FROM account WHERE created_by = '` + ahsTag + `')`,
			`DELETE FROM work_item WHERE created_by = '` + ahsTag + `'`,
			`DELETE FROM deployed_product WHERE created_by = '` + ahsTag + `'`,
			`DELETE FROM deployment WHERE created_by = '` + ahsTag + `'`,
			`DELETE FROM product WHERE created_by = '` + ahsTag + `'`,
			`DELETE FROM project WHERE created_by = '` + ahsTag + `'`,
			`DELETE FROM account WHERE created_by = '` + ahsTag + `'`,
			`DELETE FROM "group" WHERE created_by = '` + ahsTag + `'`,
		} {
			_, _ = scoped.Exec(ctx, q)
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := scoped.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.70s): %v", sql, err)
		}
	}

	exec(`INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name) VALUES
		('ac100000-0000-0000-0000-000000000001', now(), now(), $1, $1, 'AHS Team Alpha')`, ahsTag)

	acct := func(id, num, lifeCycle, activation, subRegion string, offsetMin int, cre, sre any) {
		exec(`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id, life_cycle, activation_date, sub_region, cre_team_id, sre_team_id)
		      VALUES ($1, now() - make_interval(mins => $2), now(), $3, $3, $4, $5, $6, $7, `+activation+`, $8, $9, $10)`,
			id, offsetMin, ahsTag, "AHS Account "+num, "AHS-"+num, "sf-ahs-"+num, lifeCycle, subRegion, cre, sre)
	}
	team := "ac100000-0000-0000-0000-000000000001"
	acct(ahsAccRecent, "RECENT", "Customer", `(now() - interval '250 days')::date`, "AHS-SR1", 1, team, nil)
	acct(ahsAccLive, "LIVE", "Customer", `DATE '2020-01-01'`, "AHS-SR2", 2, nil, nil)
	acct(ahsAccPending, "PENDING", "Customer", `(now() - interval '100 days')::date`, "AHS-SR2", 3, nil, nil)
	acct(ahsAccGoLive, "GOLIVE", "Customer", `(now() - interval '280 days')::date`, "AHS-SR1", 4, nil, team)
	acct(ahsAccProspect, "PROSPECT", "Prospect", `NULL`, "AHS-SR1", 5, nil, nil)

	proj := func(id, key, account string, goLive bool) {
		goLiveSQL := "NULL"
		if goLive {
			goLiveSQL = "(now() - interval '1 month')::date"
		}
		exec(`INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, account_id, onboarding_go_live_date)
		      VALUES ($1, now(), now(), $2, $2, $3, $4, $5, `+goLiveSQL+`)`, id, ahsTag, key, "sf-"+key, account)
	}
	pRecent := "ac200000-0000-0000-0000-000000000005"
	pLive := "ac200000-0000-0000-0000-000000000004"
	pPending := "ac200000-0000-0000-0000-000000000003"
	pGoLive1 := "ac200000-0000-0000-0000-000000000021"
	pGoLive2 := "ac200000-0000-0000-0000-000000000022"
	pProspect := "ac200000-0000-0000-0000-000000000001"
	proj(pRecent, "AHSPROJ5", ahsAccRecent, false)
	proj(pLive, "AHSPROJ4", ahsAccLive, false)
	proj(pPending, "AHSPROJ3", ahsAccPending, false)
	proj(pGoLive1, "AHSPROJ21", ahsAccGoLive, true)
	proj(pGoLive2, "AHSPROJ22", ahsAccGoLive, true)
	proj(pProspect, "AHSPROJ1", ahsAccProspect, false)

	health := func(id, project, account, status string) {
		exec(`INSERT INTO project_health_status (id, project_id, account_id, status) VALUES ($1, $2, $3, $4::project_health_status_enum)`,
			id, project, account, status)
	}
	health("ac300000-0000-0000-0000-000000000001", pRecent, ahsAccRecent, "AT_RISK")
	health("ac300000-0000-0000-0000-000000000002", pLive, ahsAccLive, "HEALTHY")
	health("ac300000-0000-0000-0000-000000000003", pGoLive1, ahsAccGoLive, "TO_BE_REVIEWED")
	health("ac300000-0000-0000-0000-000000000004", pGoLive2, ahsAccGoLive, "HEALTHY")

	// Products, versions, deployments.
	exec(`INSERT INTO product (id, created_on, updated_on, created_by, updated_by, manufacturer, category, name, code)
	      VALUES ('ac400000-0000-0000-0000-000000000001', now(), now(), $1, $1, 'WSO2', 'SOFTWARE', 'AHS Gateway', 'ahsgw'),
	             ('ac400000-0000-0000-0000-000000000002', now(), now(), $1, $1, 'WSO2', 'SOFTWARE', 'AHS Retired', 'ahsret')`, ahsTag)
	exec(`INSERT INTO product_version (id, created_on, updated_on, created_by, updated_by, version, product_id, current_support_status, release_date, support_eol_date)
	      VALUES ('ac500000-0000-0000-0000-000000000001', now(), now(), $1, $1, '1.0.0', 'ac400000-0000-0000-0000-000000000001', 'DISCONTINUED', DATE '2018-01-01', DATE '2020-01-01'),
	             ('ac500000-0000-0000-0000-000000000002', now(), now(), $1, $1, '9.9.9', 'ac400000-0000-0000-0000-000000000002', 'DISCONTINUED', DATE '2018-01-01', DATE '2020-01-01')`, ahsTag)
	exec(`INSERT INTO deployment (id, created_on, updated_on, created_by, updated_by, number, name, is_active, project_id) VALUES
		('ac600000-0000-0000-0000-000000000001', now(), now(), $1, $1, 'AHSDEP01', 'ahs dep recent', false, $2),
		('ac600000-0000-0000-0000-000000000002', now(), now(), $1, $1, 'AHSDEP02', 'ahs dep live', true, $3)`, ahsTag, pRecent, pLive)
	// recent: EOL gateway on the deactivated deployment (counts). live: an EOL
	// version too, but the deployed product is deactivated (does not count for
	// the EOL flag or the product filter).
	exec(`INSERT INTO deployed_product (id, created_on, updated_on, created_by, updated_by, number, active, project_id, deployment_id, product_id, version_id) VALUES
		('ac700000-0000-0000-0000-000000000001', now(), now(), $1, $1, 'AHSDP01', true, $2, 'ac600000-0000-0000-0000-000000000001', 'ac400000-0000-0000-0000-000000000001', 'ac500000-0000-0000-0000-000000000001'),
		('ac700000-0000-0000-0000-000000000002', now(), now(), $1, $1, 'AHSDP02', false, $3, 'ac600000-0000-0000-0000-000000000002', 'ac400000-0000-0000-0000-000000000002', 'ac500000-0000-0000-0000-000000000002')`,
		ahsTag, pRecent, pLive)

	// Work items.
	n := 0
	workItem := func(account, project, wiType string, ageDays int) string {
		n++
		id := "ac800000-0000-0000-0000-0000000000" + twoDigits(n)
		exec(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, project_id, account_id)
		      VALUES ($1, now() - make_interval(days => $2), now(), $3, $3, $4, $5, 'seeded', $6::work_item_type_enum, $7, $8)`,
			id, ageDays, ahsTag, "AHSCS"+twoDigits(n), "AHS-WI-"+twoDigits(n), wiType, project, account)
		return id
	}
	caseRow := func(account, project string, ageDays int) string {
		id := workItem(account, project, "CASE", ageDays)
		exec(`INSERT INTO "case" (id, state, severity) VALUES ($1, 'OPEN'::case_state_enum, 'S3'::case_severity_enum)`, id)
		return id
	}
	engagement := func(account, project, state string, ageDays int) {
		id := workItem(account, project, "ENGAGEMENT", ageDays)
		exec(`INSERT INTO engagement (id, state, type) VALUES ($1, $2::engagement_state_enum, 'MIGRATION'::engagement_type_enum)`, id, state)
	}
	escalation := func(workItemID string, ageDays int) {
		exec(`INSERT INTO case_escalation (id, created_on, updated_on, created_by, updated_by, work_item_id, current_level)
		      VALUES (gen_random_uuid(), now() - make_interval(days => $1), now(), $2, $2, $3, 'EL1')`, ageDays, ahsTag, workItemID)
	}

	recentCase := caseRow(ahsAccRecent, pRecent, 10)
	escalation(recentCase, 30)
	engagement(ahsAccRecent, pRecent, "OPEN", 200)

	engagement(ahsAccPending, pPending, "OPEN", 90)
	engagement(ahsAccPending, pPending, "CLOSED", 300)
	caseRow(ahsAccPending, pPending, 400)

	sr := workItem(ahsAccGoLive, pGoLive1, "SERVICE_REQUEST", 5)
	exec(`INSERT INTO service_request (id, state) VALUES ($1, 'OPEN'::service_request_state_enum)`, sr)
	oldCase := caseRow(ahsAccGoLive, pGoLive1, 200)
	escalation(oldCase, 120)
}

func twoDigits(n int) string {
	return string([]byte{byte('0' + n/10), byte('0' + n%10)})
}

func ahsSearch(t *testing.T, repo repository.AccountRepository, f domain.SearchAccountsFilters, limit, offset int) ([]repository.AccountRow, int) {
	t.Helper()
	f.SearchQuery = "AHS Account"
	if f.LifeCycle == "" {
		f.LifeCycle = "Customer"
	}
	f.HasProject = true
	rows, total, err := repo.SearchAccounts(repository.WithSystemIdentity(context.Background()), domain.SearchAccountsRequest{
		Pagination: domain.Pagination{Limit: limit, Offset: offset},
		Filters:    f,
	})
	if err != nil {
		t.Fatalf("SearchAccounts: %v", err)
	}
	return rows, total
}

func ahsNames(rows []repository.AccountRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Number
	}
	return out
}

func TestAccountHealthSearchIntegration(t *testing.T) {
	pool := caseStatsPool(t)
	seedAccountHealthFixture(t, pool)
	repo := repository.NewAccountRepository(repository.NewScoped(pool))

	t.Run("NoNewFilterLeavesRowsWithoutHealth", func(t *testing.T) {
		rows, total := ahsSearch(t, repo, domain.SearchAccountsFilters{}, 50, 0)
		if total != 4 || len(rows) != 4 {
			t.Fatalf("total=%d rows=%d, want 4/4 (prospect excluded)", total, len(rows))
		}
		for _, r := range rows {
			if r.Health != nil {
				t.Errorf("account %s carries health without a request for it", r.Number)
			}
		}
	})

	t.Run("SummaryFlagsPerAccount", func(t *testing.T) {
		rows, total := ahsSearch(t, repo, domain.SearchAccountsFilters{IncludeHealthSummary: true}, 50, 0)
		if total != 4 {
			t.Fatalf("total=%d, want 4", total)
		}
		want := []string{"AHS-RECENT", "AHS-LIVE", "AHS-PENDING", "AHS-GOLIVE"}
		if got := ahsNames(rows); !reflect.DeepEqual(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
		byNum := map[string]repository.AccountHealthRow{}
		for _, r := range rows {
			if r.Health == nil {
				t.Fatalf("account %s has no health", r.Number)
			}
			byNum[r.Number] = *r.Health
		}
		three := 3
		cases := map[string]repository.AccountHealthRow{
			"AHS-RECENT":  {GoLiveState: "failure", GoLiveMonths: intPtr(8), HasRecentCases: true, HasEolProduct: true, HasAbandonedMigrations: true, HasMigrationDelays: true, HasRecentEscalations: true, HealthStatus: "at_risk"},
			"AHS-LIVE":    {GoLiveState: "success", GoLiveMonths: byNum["AHS-LIVE"].GoLiveMonths, HealthStatus: "healthy"},
			"AHS-PENDING": {GoLiveState: "pending", GoLiveMonths: &three, HasRecentCases: true, HasMigrationDelays: true, HealthStatus: "to_be_reviewed"},
			"AHS-GOLIVE":  {GoLiveState: "success", GoLiveMonths: intPtr(9), HasRecentCases: true, HealthStatus: "to_be_reviewed"},
		}
		if byNum["AHS-LIVE"].GoLiveMonths == nil {
			t.Errorf("AHS-LIVE: months since a 2020 activation must be set")
		}
		for num, w := range cases {
			if !reflect.DeepEqual(byNum[num], w) {
				t.Errorf("%s: got %+v (months %v), want %+v (months %v)", num, byNum[num], derefInt(byNum[num].GoLiveMonths), w, derefInt(w.GoLiveMonths))
			}
		}
	})

	filterCases := []struct {
		name    string
		filters domain.SearchAccountsFilters
		want    []string
	}{
		{"risk hasNoGoLive", domain.SearchAccountsFilters{Risks: []string{"hasNoGoLive"}}, []string{"AHS-RECENT"}},
		{"risk hasGoneLive", domain.SearchAccountsFilters{Risks: []string{"hasGoneLive"}}, []string{"AHS-LIVE", "AHS-GOLIVE"}},
		{"risks are OR-ed", domain.SearchAccountsFilters{Risks: []string{"hasEolProduct", "hasMigrationDelays"}}, []string{"AHS-RECENT", "AHS-PENDING"}},
		{"risk hasAbandonedMigrations", domain.SearchAccountsFilters{Risks: []string{"hasAbandonedMigrations"}}, []string{"AHS-RECENT"}},
		{"risk hasRecentEscalations", domain.SearchAccountsFilters{Risks: []string{"hasRecentEscalations"}}, []string{"AHS-RECENT"}},
		{"risk hasRecentCases", domain.SearchAccountsFilters{Risks: []string{"hasRecentCases"}}, []string{"AHS-RECENT", "AHS-PENDING", "AHS-GOLIVE"}},
		{"risk noSupportCases6mo", domain.SearchAccountsFilters{Risks: []string{"noSupportCases6mo"}}, []string{"AHS-LIVE"}},
		{"blank risks are no filter", domain.SearchAccountsFilters{Risks: []string{" ", ""}}, []string{"AHS-RECENT", "AHS-LIVE", "AHS-PENDING", "AHS-GOLIVE"}},
		{"unknown risk matches nothing", domain.SearchAccountsFilters{Risks: []string{"bogus"}}, nil},
		{"health at_risk", domain.SearchAccountsFilters{HealthStatus: "at_risk"}, []string{"AHS-RECENT"}},
		{"health healthy", domain.SearchAccountsFilters{HealthStatus: "healthy"}, []string{"AHS-LIVE"}},
		{"health to_be_reviewed needs a row", domain.SearchAccountsFilters{HealthStatus: "to_be_reviewed"}, []string{"AHS-GOLIVE"}},
		{"unknown health matches nothing", domain.SearchAccountsFilters{HealthStatus: "AT_RISK"}, nil},
		{"risk and health combine with AND", domain.SearchAccountsFilters{Risks: []string{"hasGoneLive"}, HealthStatus: "healthy"}, []string{"AHS-LIVE"}},
		{"product is case-insensitive, deactivated deployment counts", domain.SearchAccountsFilters{Product: "ahs GATEWAY"}, []string{"AHS-RECENT"}},
		{"deactivated deployed product does not count", domain.SearchAccountsFilters{Product: "AHS Retired"}, nil},
		{"abtTeam matches CRE or SRE", domain.SearchAccountsFilters{AbtTeam: "ahs team alpha"}, []string{"AHS-RECENT", "AHS-GOLIVE"}},
		{"subRegion is exact", domain.SearchAccountsFilters{SubRegion: []string{"AHS-SR2"}}, []string{"AHS-LIVE", "AHS-PENDING"}},
		{"subRegion any of", domain.SearchAccountsFilters{SubRegion: []string{"AHS-SR1", "AHS-SR2"}}, []string{"AHS-RECENT", "AHS-LIVE", "AHS-PENDING", "AHS-GOLIVE"}},
		{"plain filters combine with risks", domain.SearchAccountsFilters{SubRegion: []string{"AHS-SR1"}, Risks: []string{"hasGoneLive"}}, []string{"AHS-GOLIVE"}},
	}
	for _, tc := range filterCases {
		t.Run(tc.name, func(t *testing.T) {
			rows, total := ahsSearch(t, repo, tc.filters, 50, 0)
			got := ahsNames(rows)
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if len(got) == 0 && len(want) == 0 {
				got, want = nil, nil
			}
			if !reflect.DeepEqual(got, want) || total != len(want) {
				t.Errorf("got %v (total %d), want %v", got, total, want)
			}
		})
	}

	t.Run("ProspectNeverMatchesCustomerLifeCycle", func(t *testing.T) {
		rows, _ := ahsSearch(t, repo, domain.SearchAccountsFilters{Risks: []string{"hasNoGoLive", "hasGoneLive", "noSupportCases6mo"}}, 50, 0)
		for _, r := range rows {
			if r.Number == "AHS-PROSPECT" {
				t.Errorf("prospect account matched a Customer-scoped search")
			}
		}
	})

	t.Run("PaginationAppliesAfterFilteringAndTotalIsFilteredCount", func(t *testing.T) {
		f := domain.SearchAccountsFilters{Risks: []string{"hasGoneLive"}}
		rows, total := ahsSearch(t, repo, f, 1, 0)
		if total != 2 || len(rows) != 1 || rows[0].Number != "AHS-LIVE" {
			t.Fatalf("page 1: total=%d rows=%v, want total 2, [AHS-LIVE]", total, ahsNames(rows))
		}
		rows, total = ahsSearch(t, repo, f, 1, 1)
		if total != 2 || len(rows) != 1 || rows[0].Number != "AHS-GOLIVE" || rows[0].Health == nil {
			t.Fatalf("page 2: total=%d rows=%v, want total 2, [AHS-GOLIVE] with health", total, ahsNames(rows))
		}
	})

	t.Run("OffsetPastTheEndStillReportsTotal", func(t *testing.T) {
		rows, total := ahsSearch(t, repo, domain.SearchAccountsFilters{Risks: []string{"hasGoneLive"}}, 10, 40)
		if len(rows) != 0 || total != 2 {
			t.Errorf("rows=%v total=%d, want none and total 2", ahsNames(rows), total)
		}
		rows, total = ahsSearch(t, repo, domain.SearchAccountsFilters{IncludeHealthSummary: true}, 10, 40)
		if len(rows) != 0 || total != 4 {
			t.Errorf("summary-only: rows=%v total=%d, want none and total 4", ahsNames(rows), total)
		}
	})

	t.Run("SummaryOnlyPagesBeforeComputingFlags", func(t *testing.T) {
		rows, total := ahsSearch(t, repo, domain.SearchAccountsFilters{IncludeHealthSummary: true}, 2, 2)
		if total != 4 || !reflect.DeepEqual(ahsNames(rows), []string{"AHS-PENDING", "AHS-GOLIVE"}) {
			t.Fatalf("total=%d rows=%v", total, ahsNames(rows))
		}
		if rows[0].Health == nil || rows[0].Health.GoLiveState != "pending" {
			t.Errorf("pending account health = %+v", rows[0].Health)
		}
	})
}

func intPtr(i int) *int { return &i }

func derefInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
