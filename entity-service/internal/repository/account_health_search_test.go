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

package repository

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

func TestBuildAccountSearchWhere_NewFiltersAreAbsentByDefault(t *testing.T) {
	where, args, next := buildAccountSearchWhere(domain.SearchAccountsFilters{})
	if where != "WHERE a.deleted_on IS NULL" || len(args) != 0 || next != 1 {
		t.Fatalf("empty filters: where=%q args=%v next=%d", where, args, next)
	}
	// Existing filters render exactly as before.
	where, args, next = buildAccountSearchWhere(domain.SearchAccountsFilters{LifeCycle: "Customer", HasProject: true})
	want := "WHERE a.deleted_on IS NULL AND a.life_cycle = $1 AND EXISTS (SELECT 1 FROM project p WHERE p.account_id = a.id)"
	if where != want || !reflect.DeepEqual(args, []any{"Customer"}) || next != 2 {
		t.Fatalf("lifeCycle/hasProject: where=%q args=%v next=%d", where, args, next)
	}
}

func TestBuildAccountSearchWhere_NewFilters(t *testing.T) {
	where, args, next := buildAccountSearchWhere(domain.SearchAccountsFilters{
		LifeCycle: "Customer",
		SubRegion: []string{"ASIA", "SAARC"},
		AbtTeam:   "  Team Nova ",
		Product:   " Gateway ",
	})
	for _, frag := range []string{
		"a.life_cycle = $1",
		"a.sub_region = ANY($2::text[])",
		"(lower(cre.name) = lower($3) OR lower(sre.name) = lower($3))",
		"lower(hpr.name) = lower($4)",
		"(hdp.active IS NULL OR hdp.active = TRUE)",
	} {
		if !strings.Contains(where, frag) {
			t.Errorf("where missing %q:\n%s", frag, where)
		}
	}
	wantArgs := []any{"Customer", []string{"ASIA", "SAARC"}, "Team Nova", "Gateway"}
	if !reflect.DeepEqual(args, wantArgs) || next != 5 {
		t.Errorf("args=%v next=%d, want %v 5", args, next, wantArgs)
	}
}

func TestBuildAccountSearchWhere_BlankTeamAndProductAreNoFilter(t *testing.T) {
	where, args, next := buildAccountSearchWhere(domain.SearchAccountsFilters{AbtTeam: "  ", Product: "\t"})
	if where != "WHERE a.deleted_on IS NULL" || len(args) != 0 || next != 1 {
		t.Fatalf("where=%q args=%v next=%d", where, args, next)
	}
}

func TestAccountSearchNeedsHealth(t *testing.T) {
	cases := []struct {
		name string
		f    domain.SearchAccountsFilters
		want bool
	}{
		{"none", domain.SearchAccountsFilters{LifeCycle: "Customer", SubRegion: []string{"ASIA"}, Product: "x", AbtTeam: "y"}, false},
		{"summary", domain.SearchAccountsFilters{IncludeHealthSummary: true}, true},
		{"risks", domain.SearchAccountsFilters{Risks: []string{"hasEolProduct"}}, true},
		{"blank risks", domain.SearchAccountsFilters{Risks: []string{" "}}, false},
		{"health status", domain.SearchAccountsFilters{HealthStatus: "healthy"}, true},
	}
	for _, tc := range cases {
		if got := accountSearchNeedsHealth(tc.f); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAccountHealthFilterPredicate(t *testing.T) {
	cases := []struct {
		name string
		f    domain.SearchAccountsFilters
		want string
	}{
		{"none", domain.SearchAccountsFilters{IncludeHealthSummary: true}, ""},
		{"one risk", domain.SearchAccountsFilters{Risks: []string{"hasNoGoLive"}}, "(fl.go_live_state = 'failure')"},
		{"risks OR, blanks dropped", domain.SearchAccountsFilters{Risks: []string{" hasEolProduct ", "", "noSupportCases6mo"}}, "(fl.has_eol_product OR NOT fl.has_recent_cases)"},
		{"unknown risk matches nothing", domain.SearchAccountsFilters{Risks: []string{"bogus"}}, "FALSE"},
		{"known and unknown risk", domain.SearchAccountsFilters{Risks: []string{"bogus", "hasGoneLive"}}, "(fl.go_live_state = 'success')"},
		{"at_risk", domain.SearchAccountsFilters{HealthStatus: "at_risk"}, "fl.hs_at_risk"},
		{"healthy", domain.SearchAccountsFilters{HealthStatus: "healthy"}, "fl.hs_all_healthy"},
		{"to_be_reviewed needs rows", domain.SearchAccountsFilters{HealthStatus: "to_be_reviewed"}, "(fl.hs_has_rows AND NOT fl.hs_at_risk AND NOT fl.hs_all_healthy)"},
		{"unknown status matches nothing", domain.SearchAccountsFilters{HealthStatus: "At_Risk"}, "FALSE"},
		{"risk AND status", domain.SearchAccountsFilters{Risks: []string{"hasRecentCases"}, HealthStatus: "healthy"}, "(fl.has_recent_cases) AND fl.hs_all_healthy"},
	}
	for _, tc := range cases {
		if got := accountHealthFilterPredicate(tc.f); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestAccountRiskPredicatesCoverEveryDocumentedKey(t *testing.T) {
	for _, key := range []string{
		domain.AccountRiskNoGoLive, domain.AccountRiskGoneLive, domain.AccountRiskRecentCases,
		domain.AccountRiskNoSupportCases6mo, domain.AccountRiskEolProduct, domain.AccountRiskAbandonedMigrations,
		domain.AccountRiskMigrationDelays, domain.AccountRiskRecentEscalations,
	} {
		if _, ok := accountRiskPredicates[key]; !ok {
			t.Errorf("no predicate for risk key %q", key)
		}
	}
	if len(accountRiskPredicates) != 8 {
		t.Errorf("%d risk predicates, want 8", len(accountRiskPredicates))
	}
}

func TestBuildAccountHealthSearchSQL_Shape(t *testing.T) {
	where, _, next := buildAccountSearchWhere(domain.SearchAccountsFilters{LifeCycle: "Customer", HasProject: true})

	// A flag filter computes flags for every candidate before pagination, and
	// never over the page alone.
	data, count := buildAccountHealthSearchSQL(domain.SearchAccountsFilters{Risks: []string{"hasEolProduct"}}, where, next)
	if !strings.Contains(data, "FROM base s") || strings.Contains(data, "FROM psrc s") {
		t.Errorf("filtered query must compute flags over base:\n%s", data)
	}
	if !strings.Contains(data, "JOIN fl ON fl.id = b.id WHERE (fl.has_eol_product)") {
		t.Errorf("risk predicate must be applied in matched, before the page:\n%s", data)
	}
	if strings.Index(data, "matched AS") > strings.Index(data, "page AS") {
		t.Errorf("matched must precede page")
	}
	if !strings.Contains(data, "LIMIT $2 OFFSET $3") || strings.Contains(count, "LIMIT") {
		t.Errorf("pagination placeholders wrong:\n%s\n---\n%s", data, count)
	}
	if !strings.Contains(data, "ORDER BY m.created_on DESC, m.id") {
		t.Errorf("page must order by created_on DESC, id")
	}

	// A summary-only request pages first and computes flags for the page.
	data, count = buildAccountHealthSearchSQL(domain.SearchAccountsFilters{IncludeHealthSummary: true}, where, next)
	if !strings.Contains(data, "FROM psrc s") || strings.Contains(data, "FROM base s") {
		t.Errorf("summary-only query must compute flags over the page:\n%s", data)
	}
	if strings.Contains(count, "fl0") {
		t.Errorf("summary-only count must not compute flags:\n%s", count)
	}
}
