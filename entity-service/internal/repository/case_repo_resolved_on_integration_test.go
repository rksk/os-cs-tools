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

// Regression test for a real, reported bug: resolved_on was never written by
// a case state change, so every report that counts cases resolved in a window
// (the dashboard's "Resolved Today" tile) read zero. CaseRepository.UpdateCase
// now stamps it in the same UPDATE as the state:
//
//   - into solution_proposed: NOW(), unless the case was waiting_on_wso2 or is
//     already solution_proposed (then the existing value is kept)
//   - into closed: the existing value, else NOW()
//   - into reopened: NULL
//   - any other state: the existing value is kept
//
// Covers the case table and each of the other case-like types (each has its
// own UPDATE statement). Runs against a real Postgres. Skipped without
// CASE_STATS_TEST_DSN.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run ResolvedOnIntegration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

type resolvedOnType struct {
	workItemType string
	table        string
	idSuffix     string
}

var resolvedOnTypes = []resolvedOnType{
	{"CASE", `"case"`, "1"},
	{"ENGAGEMENT", "engagement", "2"},
	{"SERVICE_REQUEST", "service_request", "3"},
	{"SECURITY_REPORT_ANALYSIS", "security_report_analysis", "4"},
	{"ANNOUNCEMENT", "announcement", "5"},
}

func resolvedOnID(suffix string) string {
	return "92000000-0000-0000-0000-00000000000" + suffix
}

func seedResolvedOnFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)
	cleanup := func() {
		for _, ty := range resolvedOnTypes {
			_, _ = scoped.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, resolvedOnID(ty.idSuffix))
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC()
	for _, ty := range resolvedOnTypes {
		if _, err := scoped.Exec(ctx, `INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type)
			VALUES ($1, $2, $2, 'test', 'test', $3, $4, 'resolved-on integration test fixture', $5)`,
			resolvedOnID(ty.idSuffix), now, "RO-TEST-000"+ty.idSuffix, "RO-TEST-WSO2-000"+ty.idSuffix, ty.workItemType); err != nil {
			t.Fatalf("seed work_item %s: %v", ty.workItemType, err)
		}
		if _, err := scoped.Exec(ctx, `INSERT INTO `+ty.table+` (id) VALUES ($1)`, resolvedOnID(ty.idSuffix)); err != nil {
			t.Fatalf("seed %s: %v", ty.table, err)
		}
	}
}

// resolvedOnStep puts the row into fromState holding fromResolved (bypassing
// UpdateCase), applies one UpdateCase to toState, and checks resolved_on.
type resolvedOnStep struct {
	name         string
	from, to     string
	fromResolved bool // row starts with the old timestamp
	// want: "keep" the old value (nil when it started nil), "set" a fresh
	// timestamp after the test started, "clear" back to NULL.
	want string
}

func TestResolvedOnIntegration_Transitions(t *testing.T) {
	pool := caseStatsPool(t)
	seedResolvedOnFixture(t, pool)
	repo := repository.NewCaseRepository(repository.NewScoped(pool))
	ctx := repository.WithSystemIdentity(context.Background())

	steps := []resolvedOnStep{
		{"work_in_progress to solution_proposed sets it", "WORK_IN_PROGRESS", "SOLUTION_PROPOSED", false, "set"},
		{"awaiting_info to solution_proposed restamps", "AWAITING_INFO", "SOLUTION_PROPOSED", true, "set"},
		{"waiting_on_wso2 to solution_proposed keeps it", "WAITING_ON_WSO2", "SOLUTION_PROPOSED", true, "keep"},
		{"solution_proposed to solution_proposed is idempotent", "SOLUTION_PROPOSED", "SOLUTION_PROPOSED", true, "keep"},
		{"solution_proposed to closed keeps the proposal time", "SOLUTION_PROPOSED", "CLOSED", true, "keep"},
		{"work_in_progress to closed sets it", "WORK_IN_PROGRESS", "CLOSED", false, "set"},
		{"closed to closed is idempotent", "CLOSED", "CLOSED", true, "keep"},
		{"solution_proposed to waiting_on_wso2 keeps it", "SOLUTION_PROPOSED", "WAITING_ON_WSO2", true, "keep"},
		{"solution_proposed to work_in_progress keeps it", "SOLUTION_PROPOSED", "WORK_IN_PROGRESS", true, "keep"},
		{"solution_proposed to awaiting_info keeps it", "SOLUTION_PROPOSED", "AWAITING_INFO", true, "keep"},
		{"solution_proposed to reopened clears it", "SOLUTION_PROPOSED", "REOPENED", true, "clear"},
		{"closed to reopened clears it", "CLOSED", "REOPENED", true, "clear"},
		{"open to work_in_progress leaves it empty", "OPEN", "WORK_IN_PROGRESS", false, "keep"},
		{"reopened to open leaves it empty", "REOPENED", "OPEN", false, "keep"},
		{"null state to solution_proposed sets it", "", "SOLUTION_PROPOSED", false, "set"},
	}
	// Announcements only have open/closed.
	announcementSteps := []resolvedOnStep{
		{"open to closed sets it", "OPEN", "CLOSED", false, "set"},
		{"closed to closed is idempotent", "CLOSED", "CLOSED", true, "keep"},
		{"open to open leaves it empty", "OPEN", "OPEN", false, "keep"},
		{"closed to open clears it", "CLOSED", "OPEN", true, "clear"},
	}

	for _, ty := range resolvedOnTypes {
		ty := ty
		list := steps
		if ty.workItemType == "ANNOUNCEMENT" {
			list = announcementSteps
		}
		for _, st := range list {
			st := st
			t.Run(ty.workItemType+"/"+st.name, func(t *testing.T) {
				id := resolvedOnID(ty.idSuffix)
				fromState := st.from
				if ty.workItemType == "ANNOUNCEMENT" && fromState == "CLOSED" {
					fromState = "CLOSE"
				}
				var old *time.Time
				if st.fromResolved {
					o := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
					old = &o
				}
				var fromArg *string
				if fromState != "" {
					fromArg = &fromState
				}
				if _, err := repository.NewScoped(pool).Exec(ctx, `UPDATE `+ty.table+` SET state = $2::TEXT::`+stateEnum(ty.table)+`, resolved_on = $3 WHERE id = $1`, id, fromArg, old); err != nil {
					t.Fatalf("precondition: %v", err)
				}

				started := time.Now().UTC().Add(-time.Minute)
				to := domain.CaseState(lower(st.to))
				c, _, _, err := repo.UpdateCase(ctx, domain.UpdateCaseRequest{ID: id, State: &to}, nil)
				if err != nil {
					t.Fatalf("UpdateCase(%s): %v", st.to, err)
				}

				var stored *time.Time
				if err := repository.NewScoped(pool).QueryRow(ctx, `SELECT resolved_on FROM `+ty.table+` WHERE id = $1`, id).Scan(&stored); err != nil {
					t.Fatalf("read back: %v", err)
				}
				switch st.want {
				case "keep":
					assertSameTime(t, "stored resolved_on", stored, old)
					assertSameTime(t, "returned ResolvedOn", c.ResolvedOn, old)
				case "clear":
					assertSameTime(t, "stored resolved_on", stored, nil)
					assertSameTime(t, "returned ResolvedOn", c.ResolvedOn, nil)
				case "set":
					if stored == nil || stored.Before(started) {
						t.Fatalf("stored resolved_on = %v, want a fresh timestamp after %v", stored, started)
					}
					assertSameTime(t, "returned ResolvedOn", c.ResolvedOn, stored)
				}
			})
		}
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func stateEnum(table string) string {
	switch table {
	case `"case"`:
		return "case_state_enum"
	case "announcement":
		return "announcement_state_enum"
	}
	return table + "_state_enum"
}

func assertSameTime(t *testing.T, what string, got, want *time.Time) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Errorf("%s = %v, want %v", what, got, want)
		}
		return
	}
	if !got.Equal(*want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}
