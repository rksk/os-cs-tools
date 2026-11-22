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

// Closing a case on the plain Postgres path end to end on a real database:
// the real caseService over the real CaseRepository, an internal caller
// closing a case through UpdateCase, and the row it leaves behind. Rejected
// closes are proved by reading the row back unchanged, not by the error
// alone. Skipped unless ENTITY_TEST_DATABASE_URL is set.
//
//	ENTITY_TEST_DATABASE_URL=postgres://... go test ./internal/service/ -run CloseValidationIntegration

package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const (
	ccAccountID  = "c1ce0000-0000-4000-8000-000000000001"
	ccProjectID  = "c1ce0000-0000-4000-8000-000000000003"
	ccStaffID    = "c1ce0000-0000-4000-8000-000000000005"
	ccStaffEmail = "cc-staff@test.local"
	ccCaseIDs    = `('c1ce0000-0000-4000-8000-0000000000a1','c1ce0000-0000-4000-8000-0000000000a2','c1ce0000-0000-4000-8000-0000000000a3','c1ce0000-0000-4000-8000-0000000000a4','c1ce0000-0000-4000-8000-0000000000a5','c1ce0000-0000-4000-8000-0000000000a6','c1ce0000-0000-4000-8000-0000000000b1')`
)

// seedCloseValidationFixture inserts a project, a staff user and n plain
// cases in Work in Progress (ids ...a1 to ...a<n>), plus one service request
// (...b1) in Work in Progress, and removes them all again afterwards.
func seedCloseValidationFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)

	cleanup := func() {
		for _, stmt := range []string{
			`DELETE FROM work_item_activity WHERE work_item_id IN ` + ccCaseIDs,
			`DELETE FROM "case" WHERE id IN ` + ccCaseIDs,
			`DELETE FROM service_request WHERE id IN ` + ccCaseIDs,
			`DELETE FROM work_item WHERE id IN ` + ccCaseIDs,
		} {
			_, _ = scoped.Exec(ctx, stmt)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM project WHERE id = $1`, ccProjectID)
		_, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, ccAccountID)
		_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, ccStaffID)
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.80s): %v", sql, err)
		}
	}
	mustExecScoped := func(sql string, args ...any) {
		t.Helper()
		if _, err := scoped.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed scoped (%.80s): %v", sql, err)
		}
	}
	mustExec(`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id)
		VALUES ($1, $2, $2, 'test', 'test', 'Close Validation Test Account', 'CC-ACC-1', 'sf-cc-acc-1')`, ccAccountID, now)
	mustExec(`INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, account_id)
		VALUES ($1, $2, $2, 'test', 'test', 'CCTEST', 'sf-cctest', $3)`, ccProjectID, now, ccAccountID)
	mustExec(`INSERT INTO "user" (id, created_on, updated_on, user_name, first_name, last_name, email, user_type)
		VALUES ($1, $2, $2, 'cc-staff', 'Jane', 'Doe', $3, 'INTERNAL')`, ccStaffID, now, ccStaffEmail)

	for i := 1; i <= 6; i++ {
		id := ccCaseID(i)
		mustExecScoped(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, project_id)
			VALUES ($1, $2, $2, 'test', 'test', $3, $4, 'close validation integration test fixture', 'CASE'::work_item_type_enum, $5)`,
			id, now, "CC-CASE-"+id[len(id)-2:], "WSO2-CC-CASE-"+id[len(id)-2:], ccProjectID)
		mustExecScoped(`INSERT INTO "case" (id, state) VALUES ($1, 'WORK_IN_PROGRESS')`, id)
	}
	mustExecScoped(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, project_id)
		VALUES ($1, $2, $2, 'test', 'test', 'CC-SR-B1', 'WSO2-CC-SR-B1', 'close validation integration test fixture', 'SERVICE_REQUEST'::work_item_type_enum, $3)`,
		ccSRID, now, ccProjectID)
	mustExecScoped(`INSERT INTO service_request (id, state) VALUES ($1, 'WORK_IN_PROGRESS')`, ccSRID)
}

const ccSRID = "c1ce0000-0000-4000-8000-0000000000b1"

func ccCaseID(i int) string {
	return "c1ce0000-0000-4000-8000-0000000000a" + string(rune('0'+i))
}

type ccClosedRow struct {
	state          string
	resolutionCode *string
	cause          *string
	closeNotes     *string
	closedOn       *time.Time
	closedBy       *string
}

// ccReadRow reads the extension row of a plain case straight from the table.
func ccReadRow(t *testing.T, scoped *repository.Scoped, table, id string) ccClosedRow {
	t.Helper()
	var r ccClosedRow
	q := `SELECT state::TEXT, resolution_code::TEXT, cause::TEXT, close_notes, closed_on, closed_by_user_id::TEXT FROM "case" WHERE id = $1`
	if table == "service_request" {
		q = `SELECT state::TEXT, resolution_code::TEXT, cause::TEXT, close_notes, closed_on, closed_by_user_id::TEXT FROM service_request WHERE id = $1`
	}
	if err := scoped.QueryRow(repository.WithSystemIdentity(context.Background()), q, id).
		Scan(&r.state, &r.resolutionCode, &r.cause, &r.closeNotes, &r.closedOn, &r.closedBy); err != nil {
		t.Fatalf("read %s %s: %v", table, id, err)
	}
	return r
}

func ccStaffContext(t *testing.T) context.Context {
	t.Helper()
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, ccStaffEmail))
	return repository.WithCallerIdentity(ctx, repository.SearchScope{Unrestricted: true, HasInternalAccess: true, ViewerEmail: ccStaffEmail})
}

// TestCloseValidationIntegration_RequiresAllResolutionFields: with no
// write-back dispatcher configured (the plain Postgres path), closing or
// proposing a solution for a case without a resolution code, a cause or
// non-blank close notes is refused with the validation error, and the row
// is left exactly as it was.
func TestCloseValidationIntegration_RequiresAllResolutionFields(t *testing.T) {
	pool := newCustomerReplyIntegrationPool(t)
	seedCloseValidationFixture(t, pool)
	scoped := repository.NewScoped(pool)
	svc := newCustomerReplyIntegrationService(pool)

	code := domain.CaseResolutionCodeSolvedByCustomer
	cause := domain.CaseCauseUnknown
	notes := "Resolved with the customer."
	blank := "   "
	closed := domain.CaseStateClosed
	proposed := domain.CaseStateSolutionProposed

	tests := []struct {
		name  string
		id    string
		state domain.CaseState
		code  *domain.CaseResolutionCode
		cause *domain.CaseCause
		notes *string
	}{
		{"closed without resolution code", ccCaseID(1), closed, nil, &cause, &notes},
		{"closed without cause", ccCaseID(1), closed, &code, nil, &notes},
		{"closed without close notes", ccCaseID(1), closed, &code, &cause, nil},
		{"closed with blank close notes", ccCaseID(1), closed, &code, &cause, &blank},
		{"closed without any of the three", ccCaseID(1), closed, nil, nil, nil},
		{"solution proposed without resolution code", ccCaseID(2), proposed, nil, &cause, &notes},
		{"solution proposed without cause", ccCaseID(2), proposed, &code, nil, &notes},
		{"solution proposed without close notes", ccCaseID(2), proposed, &code, &cause, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := tc.state
			_, err := svc.UpdateCase(ccStaffContext(t), domain.UpdateCaseRequest{
				ID: tc.id, State: &state, ResolutionCode: tc.code, Cause: tc.cause, CloseNotes: tc.notes,
			})
			var ve *apierror.ValidationError
			if !asValidationError(err, &ve) {
				t.Fatalf("err = %T %v, want *apierror.ValidationError", err, err)
			}
			if want := "resolutionCode, cause, and closeNotes are required when state is closed or solution_proposed"; ve.Msg != want {
				t.Errorf("message = %q, want %q", ve.Msg, want)
			}
			row := ccReadRow(t, scoped, "case", tc.id)
			if row.state != "WORK_IN_PROGRESS" || row.resolutionCode != nil || row.cause != nil || row.closeNotes != nil || row.closedOn != nil || row.closedBy != nil {
				t.Errorf("row changed by a refused request: %+v", row)
			}
		})
	}
}

// TestCloseValidationIntegration_CloseWithAllFieldsPersists: the same close
// with all three fields goes through, and the row reads back closed with the
// values, the closure timestamp and the closing user.
func TestCloseValidationIntegration_CloseWithAllFieldsPersists(t *testing.T) {
	pool := newCustomerReplyIntegrationPool(t)
	seedCloseValidationFixture(t, pool)
	scoped := repository.NewScoped(pool)
	svc := newCustomerReplyIntegrationService(pool)

	code := domain.CaseResolutionCodeSolvedByCustomer
	cause := domain.CaseCauseUnknown
	notes := "Resolved with the customer."
	closed := domain.CaseStateClosed
	if _, err := svc.UpdateCase(ccStaffContext(t), domain.UpdateCaseRequest{
		ID: ccCaseID(3), State: &closed, ResolutionCode: &code, Cause: &cause, CloseNotes: &notes,
	}); err != nil {
		t.Fatalf("UpdateCase: %v", err)
	}

	row := ccReadRow(t, scoped, "case", ccCaseID(3))
	if row.state != "CLOSED" {
		t.Errorf("state = %q, want CLOSED", row.state)
	}
	if row.resolutionCode == nil || *row.resolutionCode != string(code) {
		t.Errorf("resolution_code = %v, want %q", row.resolutionCode, code)
	}
	if row.cause == nil || *row.cause != string(cause) {
		t.Errorf("cause = %v, want %q", row.cause, cause)
	}
	if row.closeNotes == nil || *row.closeNotes != notes {
		t.Errorf("close_notes = %v, want %q", row.closeNotes, notes)
	}
	if row.closedOn == nil {
		t.Error("closed_on is NULL, want the closure timestamp")
	}
	if row.closedBy == nil || *row.closedBy != ccStaffID {
		t.Errorf("closed_by_user_id = %v, want %q", row.closedBy, ccStaffID)
	}
}

// TestCloseValidationIntegration_OtherCaseLikeTypesAreExempt: the
// requirement is scoped to the plain case type; a service request closes
// without the three fields, as it did before.
func TestCloseValidationIntegration_OtherCaseLikeTypesAreExempt(t *testing.T) {
	pool := newCustomerReplyIntegrationPool(t)
	seedCloseValidationFixture(t, pool)
	scoped := repository.NewScoped(pool)
	svc := newCustomerReplyIntegrationService(pool)

	closed := domain.CaseStateClosed
	if _, err := svc.UpdateCase(ccStaffContext(t), domain.UpdateCaseRequest{ID: ccSRID, State: &closed}); err != nil {
		t.Fatalf("UpdateCase: %v", err)
	}
	if row := ccReadRow(t, scoped, "service_request", ccSRID); row.state != "CLOSED" {
		t.Errorf("state = %q, want CLOSED", row.state)
	}
}
