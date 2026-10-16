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

// Exercises GetAccountHealthDetail against a real Postgres with every migration
// applied. Skipped without CASE_STATS_TEST_DSN, so an ordinary `go test ./...`
// stays hermetic.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run AccountHealthDetail

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const ahdTag = "ahd-test"

func ahdID(kind string, n int) string {
	return fmt.Sprintf("ad%s00000-0000-0000-0000-%012d", kind, n)
}

// Fixture accounts.
//
//	main    activated 100 days ago (pending, 3 months). Three projects, newest
//	        first: busy (no go-live date), live (has one), empty.
//	other   activated in 2020, one project with no go-live date (legacy success),
//	        a case and an escalation that must never show up under main.
//	late    activated 250 days ago, one project without go-live date (failure).
//	bare    no projects at all.
var (
	ahdAccMain  = ahdID("0", 1)
	ahdAccOther = ahdID("0", 2)
	ahdAccLate  = ahdID("0", 3)
	ahdAccBare  = ahdID("0", 4)

	ahdProjBusy  = ahdID("1", 1)
	ahdProjLive  = ahdID("1", 2)
	ahdProjEmpty = ahdID("1", 3)
	ahdProjOther = ahdID("1", 4)
	ahdProjLate  = ahdID("1", 5)

	ahdDepA = ahdID("2", 1) // busy, active, newest
	ahdDepB = ahdID("2", 2) // busy, inactive, runs EOL products
	ahdDepC = ahdID("2", 3) // busy, active, runs EOL products
	ahdDepD = ahdID("2", 4) // live, active, nothing EOL
	ahdDepX = ahdID("2", 5) // other project

	ahdProdGW  = ahdID("3", 1)
	ahdProdESB = ahdID("3", 2)
	ahdProdNew = ahdID("3", 3)

	ahdVerGW1  = ahdID("4", 1) // EOL 2020-01-01
	ahdVerGW2  = ahdID("4", 2) // EOL 2021-06-30, a second version of the same product
	ahdVerESB  = ahdID("4", 3) // EOL 2019-03-01
	ahdVerNew  = ahdID("4", 4) // EOL far in the future
	ahdVerNone = ahdID("4", 5) // no EOL date
)

type ahdFixture struct {
	// ids of the work items, by the name the expectations use
	wi map[string]string
}

func seedAccountHealthDetailFixture(t *testing.T, pool *pgxpool.Pool) ahdFixture {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)

	cleanup := func() {
		for _, q := range []string{
			`DELETE FROM work_item WHERE created_by = '` + ahdTag + `'`,
			`DELETE FROM deployed_product WHERE created_by = '` + ahdTag + `'`,
			`DELETE FROM deployment WHERE created_by = '` + ahdTag + `'`,
			`DELETE FROM product_version WHERE created_by = '` + ahdTag + `'`,
			`DELETE FROM product WHERE created_by = '` + ahdTag + `'`,
			`DELETE FROM project WHERE created_by = '` + ahdTag + `'`,
			`DELETE FROM account WHERE created_by = '` + ahdTag + `'`,
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

	acct := func(id, num, activation string) {
		exec(`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id, life_cycle, activation_date)
		      VALUES ($1, now(), now(), $2, $2, $3, $4, $5, 'Customer', `+activation+`)`,
			id, ahdTag, "AHD Account "+num, "AHD-"+num, "sf-ahd-"+num)
	}
	acct(ahdAccMain, "MAIN", `(now() - interval '100 days')::date`)
	acct(ahdAccOther, "OTHER", `DATE '2020-01-01'`)
	acct(ahdAccLate, "LATE", `(now() - interval '250 days')::date`)
	acct(ahdAccBare, "BARE", `NULL`)

	proj := func(id, key, name, account string, goLive bool, ageMin int) {
		goLiveSQL := "NULL"
		if goLive {
			goLiveSQL = "(now() - interval '1 month')::date"
		}
		exec(`INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, name, account_id, onboarding_go_live_date)
		      VALUES ($1, now() - make_interval(mins => $6), now(), $2, $2, $3, $4, $7, $5, `+goLiveSQL+`)`,
			id, ahdTag, key, "sf-"+key, account, ageMin, name)
	}
	proj(ahdProjBusy, "AHDBUSY", "Busy project", ahdAccMain, false, 1)
	proj(ahdProjLive, "AHDLIVE", "Live project", ahdAccMain, true, 2)
	proj(ahdProjEmpty, "AHDEMPTY", "Empty project", ahdAccMain, false, 3)
	proj(ahdProjOther, "AHDOTHER", "Other project", ahdAccOther, false, 1)
	proj(ahdProjLate, "AHDLATE", "Late project", ahdAccLate, false, 1)

	// Products and versions.
	exec(`INSERT INTO product (id, created_on, updated_on, created_by, updated_by, manufacturer, category, name, code) VALUES
		($2, now(), now(), $1, $1, 'WSO2', 'SOFTWARE', 'AHD Gateway', 'ahdgw'),
		($3, now(), now(), $1, $1, 'WSO2', 'SOFTWARE', 'AHD Bus', 'ahdesb'),
		($4, now(), now(), $1, $1, 'WSO2', 'SOFTWARE', 'AHD Fresh', 'ahdnew')`, ahdTag, ahdProdGW, ahdProdESB, ahdProdNew)
	version := func(id, product, ver, eol string) {
		exec(`INSERT INTO product_version (id, created_on, updated_on, created_by, updated_by, version, product_id, current_support_status, release_date, support_eol_date)
		      VALUES ($1, now(), now(), $2, $2, $3, $4, 'DISCONTINUED', DATE '2015-01-01', `+eol+`)`, id, ahdTag, ver, product)
	}
	version(ahdVerGW1, ahdProdGW, "1.0.0", `DATE '2020-01-01'`)
	version(ahdVerGW2, ahdProdGW, "2.0.0", `DATE '2021-06-30'`)
	version(ahdVerESB, ahdProdESB, "5.0.0", `DATE '2019-03-01'`)
	version(ahdVerNew, ahdProdNew, "9.0.0", `DATE '2099-01-01'`)
	version(ahdVerNone, ahdProdNew, "9.1.0", `NULL`)

	// Deployments. A is the newest and active; B is deactivated.
	dep := func(id, num, name, project string, active bool, ageMin int) {
		exec(`INSERT INTO deployment (id, created_on, updated_on, created_by, updated_by, number, name, is_active, project_id)
		      VALUES ($1, now() - make_interval(mins => $6), now(), $2, $2, $3, $4, $5, $7)`,
			id, ahdTag, num, name, active, ageMin, project)
	}
	dep(ahdDepA, "AHDDEP01", "dep a", ahdProjBusy, true, 1)
	dep(ahdDepB, "AHDDEP02", "dep b", ahdProjBusy, false, 2)
	dep(ahdDepC, "AHDDEP03", "dep c", ahdProjBusy, true, 3)
	dep(ahdDepD, "AHDDEP04", "dep d", ahdProjLive, true, 1)
	dep(ahdDepX, "AHDDEP05", "dep x", ahdProjOther, true, 1)

	// Deployed products, with the age (minutes) that orders the EOL list.
	n := 0
	dp := func(project, deployment, product, ver string, active any, ageMin int) {
		n++
		exec(`INSERT INTO deployed_product (id, created_on, updated_on, created_by, updated_by, number, active, project_id, deployment_id, product_id, version_id)
		      VALUES ($1, now() - make_interval(mins => $2), now(), $3, $3, $4, $5, $6, $7, $8, $9)`,
			ahdID("5", n), ageMin, ahdTag, fmt.Sprintf("AHDDP%02d", n), active, project, deployment, product, ver)
	}
	dp(ahdProjBusy, ahdDepC, ahdProdGW, ahdVerGW1, true, 1)   // EOL, newest: ESB-less first entry is gateway 1.0.0
	dp(ahdProjBusy, ahdDepB, ahdProdESB, ahdVerESB, nil, 2)   // EOL, NULL active counts
	dp(ahdProjBusy, ahdDepB, ahdProdGW, ahdVerGW1, true, 3)   // same version, another deployment
	dp(ahdProjBusy, ahdDepC, ahdProdGW, ahdVerGW1, true, 4)   // same version, same deployment again: listed once
	dp(ahdProjBusy, ahdDepA, ahdProdGW, ahdVerGW2, true, 5)   // second EOL version
	dp(ahdProjBusy, ahdDepA, ahdProdNew, ahdVerNew, true, 6)  // not EOL yet
	dp(ahdProjBusy, ahdDepA, ahdProdNew, ahdVerNone, true, 7) // no EOL date
	dp(ahdProjBusy, ahdDepA, ahdProdESB, ahdVerESB, false, 8) // deactivated: excluded
	dp(ahdProjLive, ahdDepD, ahdProdNew, ahdVerNew, true, 1)  // live project has no EOL
	dp(ahdProjOther, ahdDepX, ahdProdESB, ahdVerESB, true, 1) // other account's EOL

	// Work items.
	fx := ahdFixture{wi: map[string]string{}}
	w := 0
	workItem := func(name, account, project, wiType string, ageDays int) string {
		w++
		id := ahdID("8", w)
		fx.wi[name] = id
		exec(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, project_id, account_id)
		      VALUES ($1, now() - make_interval(days => $2), now(), $3, $3, $4, $5, 'seeded', $6::work_item_type_enum, $7, $8)`,
			id, ageDays, ahdTag, name, "AHD-WI-"+name, wiType, project, account)
		return id
	}
	caseRow := func(name, account, project, severity string, ageDays int) {
		id := workItem(name, account, project, "CASE", ageDays)
		exec(`INSERT INTO "case" (id, state, severity) VALUES ($1, 'OPEN'::case_state_enum, $2::case_severity_enum)`, id, severity)
	}
	engagement := func(name, account, project, engType, state string, ageDays int) {
		id := workItem(name, account, project, "ENGAGEMENT", ageDays)
		exec(`INSERT INTO engagement (id, state, type) VALUES ($1, $2::engagement_state_enum, $3::engagement_type_enum)`, id, state, engType)
	}
	e := 0
	escalation := func(workItemID string, ageDays int) string {
		e++
		id := ahdID("9", e)
		exec(`INSERT INTO case_escalation (id, created_on, updated_on, created_by, updated_by, work_item_id, current_level)
		      VALUES ($1, now() - make_interval(days => $2), now(), $3, $3, $4, 'EL1')`, id, ageDays, ahdTag, workItemID)
		return id
	}

	// busy project: recent cases, newest first: critA 5d, medA 10d, critB 15d,
	// srA 20d (no severity), eng90 90d and eng30 30d are engagements.
	caseRow("critA", ahdAccMain, ahdProjBusy, "S1", 5)
	caseRow("medA", ahdAccMain, ahdProjBusy, "S3", 10)
	caseRow("critB", ahdAccMain, ahdProjBusy, "S1", 15)
	sr := workItem("srA", ahdAccMain, ahdProjBusy, "SERVICE_REQUEST", 20)
	exec(`INSERT INTO service_request (id, state) VALUES ($1, 'OPEN'::service_request_state_enum)`, sr)
	caseRow("oldCase", ahdAccMain, ahdProjBusy, "S2", 400)
	workItem("announce", ahdAccMain, ahdProjBusy, "ANNOUNCEMENT", 3) // never a recent case
	engagement("eng30", ahdAccMain, ahdProjBusy, "MIGRATION", "OPEN", 30)
	engagement("eng90", ahdAccMain, ahdProjBusy, "MIGRATION", "OPEN", 90)
	engagement("eng200", ahdAccMain, ahdProjBusy, "MIGRATION", "OPEN", 200)
	engagement("engClosed", ahdAccMain, ahdProjBusy, "MIGRATION", "CLOSED", 300)
	engagement("engOther", ahdAccMain, ahdProjBusy, "FOLLOW_UP", "OPEN", 300)

	// live project: one case and a closed migration; an escalation on it.
	caseRow("liveCase", ahdAccMain, ahdProjLive, "S4", 40)

	// escalations: on critA (30d), on the announcement (10d: counts, newest),
	// on oldCase (400d old case, 200d old escalation), on the live case (60d).
	escCrit := escalation(fx.wi["critA"], 30)
	escAnn := escalation(fx.wi["announce"], 10)
	escOld := escalation(fx.wi["oldCase"], 200)
	escLive := escalation(fx.wi["liveCase"], 60)
	fx.wi["escCrit"], fx.wi["escAnn"], fx.wi["escOld"], fx.wi["escLive"] = escCrit, escAnn, escOld, escLive

	// other account: a case, an open old migration and an escalation.
	caseRow("otherCase", ahdAccOther, ahdProjOther, "S1", 5)
	engagement("otherMig", ahdAccOther, ahdProjOther, "MIGRATION", "OPEN", 250)
	escalation(fx.wi["otherCase"], 5)
	// A work item of the main account sitting in the other account's project is
	// not a case of any project of main.
	caseRow("stray", ahdAccMain, ahdProjOther, "S1", 2)

	return fx
}

func TestAccountHealthDetailIntegration(t *testing.T) {
	pool := caseStatsPool(t)
	fx := seedAccountHealthDetailFixture(t, pool)
	repo := repository.NewAccountRepository(repository.NewScoped(pool))
	ctx := repository.WithSystemIdentity(context.Background())

	get := func(t *testing.T, id string) repository.AccountHealthDetailRow {
		t.Helper()
		row, err := repo.GetAccountHealthDetail(ctx, id)
		if err != nil {
			t.Fatalf("GetAccountHealthDetail(%s): %v", id, err)
		}
		return row
	}
	ref := func(name string) domain.HealthCaseInfo {
		return domain.HealthCaseInfo{ID: fx.wi[name], Number: name}
	}
	link := func(name string) domain.HealthCaseLink {
		return domain.HealthCaseLink{ID: fx.wi[name], Number: name}
	}
	escLink := func(esc, caseName string) domain.HealthCaseLink {
		return domain.HealthCaseLink{EscalationID: fx.wi[esc], Number: caseName, EscalationNumber: caseName}
	}

	t.Run("UnknownAccountIsNotFound", func(t *testing.T) {
		_, err := repo.GetAccountHealthDetail(ctx, "ad000000-0000-0000-0000-0000000000ff")
		var nf *apierror.NotFoundError
		if !errors.As(err, &nf) {
			t.Fatalf("err = %v, want NotFoundError", err)
		}
	})

	t.Run("AccountWithoutProjectsHasAnEmptyList", func(t *testing.T) {
		row := get(t, ahdAccBare)
		if row.AccountName != "AHD Account BARE" || len(row.Projects) != 0 || row.ActivationDate != nil || row.GoLiveMonths != nil {
			t.Fatalf("row = %+v", row)
		}
	})

	main := get(t, ahdAccMain)

	t.Run("AccountLevelFields", func(t *testing.T) {
		if main.AccountName != "AHD Account MAIN" {
			t.Errorf("name = %q", main.AccountName)
		}
		if main.ActivationDate == nil || main.GoLiveMonths == nil || *main.GoLiveMonths != 3 {
			t.Errorf("activation = %v months = %v, want a date and 3 months", main.ActivationDate, main.GoLiveMonths)
		}
	})

	t.Run("ProjectsNewestFirstWithGoLiveState", func(t *testing.T) {
		var got [][2]string
		for _, p := range main.Projects {
			got = append(got, [2]string{p.Detail.ID, p.GoLiveState})
		}
		want := [][2]string{{ahdProjBusy, "pending"}, {ahdProjLive, "success"}, {ahdProjEmpty, "pending"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("projects = %v, want %v", got, want)
		}
	})

	busy, live, empty := main.Projects[0].Detail, main.Projects[1].Detail, main.Projects[2].Detail

	t.Run("RecentCasesGroupedByPriorityInFirstSeenOrder", func(t *testing.T) {
		// Newest first: critA, medA, critB, srA, eng30, eng90 (engagements and the
		// service request are case-like; the announcement, the 400 day old case
		// and the 200 day old engagement are not recent cases).
		want := []domain.HealthCasePriorityGroup{
			{Priority: "critical", Count: 2, Cases: []domain.HealthCaseInfo{ref("critA"), ref("critB")}},
			{Priority: "medium", Count: 1, Cases: []domain.HealthCaseInfo{ref("medA")}},
			{Priority: "Unspecified", Count: 3, Cases: []domain.HealthCaseInfo{ref("srA"), ref("eng30"), ref("eng90")}},
		}
		if !busy.HasRecentCases || busy.TotalRecentCases != 6 {
			t.Errorf("has=%v total=%d, want true/6", busy.HasRecentCases, busy.TotalRecentCases)
		}
		if !reflect.DeepEqual(busy.DetailedRecentCases, want) {
			t.Errorf("groups = %+v\nwant     %+v", busy.DetailedRecentCases, want)
		}
	})

	t.Run("MigrationsDelayedAndAbandoned", func(t *testing.T) {
		if !busy.HasMigrationDelays || !reflect.DeepEqual(busy.DetailedMigrationDelays, []domain.HealthCaseLink{link("eng90"), link("eng200")}) {
			t.Errorf("delays = %v %+v", busy.HasMigrationDelays, busy.DetailedMigrationDelays)
		}
		if !busy.HasAbandonedCases || !reflect.DeepEqual(busy.DetailedAbandonedCases, []domain.HealthCaseLink{link("eng200")}) {
			t.Errorf("abandoned = %v %+v", busy.HasAbandonedCases, busy.DetailedAbandonedCases)
		}
	})

	t.Run("EscalationsNewestFirstIncludingAnnouncements", func(t *testing.T) {
		want := []domain.HealthCaseLink{escLink("escAnn", "announce"), escLink("escCrit", "critA"), escLink("escOld", "oldCase")}
		if !busy.HasEscalatedCases || !reflect.DeepEqual(busy.DetailedEscalatedCases, want) {
			t.Errorf("escalations = %v %+v\nwant %+v", busy.HasEscalatedCases, busy.DetailedEscalatedCases, want)
		}
	})

	t.Run("OnlyActiveDeploymentsNewestFirst", func(t *testing.T) {
		want := []domain.HealthDeploymentRef{{ID: ahdDepA, Name: "dep a"}, {ID: ahdDepC, Name: "dep c"}}
		if !reflect.DeepEqual(busy.Deployments, want) {
			t.Errorf("deployments = %+v, want %+v", busy.Deployments, want)
		}
	})

	t.Run("EolSoftwareOnePerVersionFirstSeenOrder", func(t *testing.T) {
		// Newest deployed product first: gateway 1.0.0 (dep c), bus 5.0.0 (dep b,
		// NULL active), then the same gateway version again on dep b and dep c
		// (dep c listed once), then gateway 2.0.0 on dep a. The not-yet-EOL, the
		// undated and the deactivated products are left out, deactivated
		// deployment b is not.
		want := []domain.HealthEolProduct{
			{Name: "AHD Gateway 1.0.0", EolDate: "2020-01-01T00:00:00Z", Deployments: []domain.HealthDeploymentRef{{ID: ahdDepC, Name: "dep c"}, {ID: ahdDepB, Name: "dep b"}}},
			{Name: "AHD Bus 5.0.0", EolDate: "2019-03-01T00:00:00Z", Deployments: []domain.HealthDeploymentRef{{ID: ahdDepB, Name: "dep b"}}},
			{Name: "AHD Gateway 2.0.0", EolDate: "2021-06-30T00:00:00Z", Deployments: []domain.HealthDeploymentRef{{ID: ahdDepA, Name: "dep a"}}},
		}
		if !busy.IsUsingEolProduct || !reflect.DeepEqual(busy.SoftwareModel, want) {
			t.Errorf("software = %v %+v\nwant %+v", busy.IsUsingEolProduct, busy.SoftwareModel, want)
		}
	})

	t.Run("QuietProjectsHaveEmptyArraysNotNull", func(t *testing.T) {
		for _, d := range []domain.ProjectHealthDetail{empty} {
			if d.HasRecentCases || d.HasAbandonedCases || d.IsUsingEolProduct || d.HasMigrationDelays || d.HasEscalatedCases || d.TotalRecentCases != 0 {
				t.Errorf("flags set on the empty project: %+v", d)
			}
			if d.DetailedRecentCases == nil || d.DetailedAbandonedCases == nil || d.SoftwareModel == nil ||
				d.Deployments == nil || d.DetailedMigrationDelays == nil || d.DetailedEscalatedCases == nil {
				t.Errorf("a list is nil on the empty project: %+v", d)
			}
		}
	})

	t.Run("LiveProjectOnlyItsOwnItems", func(t *testing.T) {
		wantGroups := []domain.HealthCasePriorityGroup{{Priority: "low", Count: 1, Cases: []domain.HealthCaseInfo{ref("liveCase")}}}
		if !reflect.DeepEqual(live.DetailedRecentCases, wantGroups) || live.TotalRecentCases != 1 {
			t.Errorf("recent = %+v", live.DetailedRecentCases)
		}
		if live.IsUsingEolProduct || len(live.SoftwareModel) != 0 {
			t.Errorf("EOL on live project: %+v", live.SoftwareModel)
		}
		if !reflect.DeepEqual(live.Deployments, []domain.HealthDeploymentRef{{ID: ahdDepD, Name: "dep d"}}) {
			t.Errorf("deployments = %+v", live.Deployments)
		}
		if !reflect.DeepEqual(live.DetailedEscalatedCases, []domain.HealthCaseLink{escLink("escLive", "liveCase")}) {
			t.Errorf("escalations = %+v", live.DetailedEscalatedCases)
		}
		if live.HasMigrationDelays || live.HasAbandonedCases {
			t.Errorf("migration flags on live project")
		}
	})

	t.Run("OtherAccountsItemsNeverLeakIn", func(t *testing.T) {
		for _, p := range main.Projects {
			for _, g := range p.Detail.DetailedRecentCases {
				for _, c := range g.Cases {
					if c.Number == "otherCase" || c.Number == "stray" {
						t.Errorf("%s leaked into project %s", c.Number, p.Detail.ID)
					}
				}
			}
		}
	})

	t.Run("GoLiveStatesOfOtherAccounts", func(t *testing.T) {
		other := get(t, ahdAccOther)
		if len(other.Projects) != 1 || other.Projects[0].GoLiveState != "success" {
			t.Errorf("legacy activation: %+v", other.Projects)
		}
		if !other.Projects[0].Detail.HasAbandonedCases || !other.Projects[0].Detail.HasRecentCases {
			t.Errorf("other project detail = %+v", other.Projects[0].Detail)
		}
		late := get(t, ahdAccLate)
		if len(late.Projects) != 1 || late.Projects[0].GoLiveState != "failure" || late.GoLiveMonths == nil || *late.GoLiveMonths != 8 {
			t.Errorf("late = %+v (months %v)", late.Projects, derefInt(late.GoLiveMonths))
		}
	})
}
