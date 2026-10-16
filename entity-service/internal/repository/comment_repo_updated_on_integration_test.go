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

package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// Adding a comment to a case is activity on it: CreateComment and
// CreateCaseComment stamp the case's updated_on / updated_by in the same
// statement, so the lists' Updated column, their updated-date filters and the
// dashboard reminder widgets see a case that was just nudged as touched. Only
// case-like work items are stamped: a change request's updated_by is the record
// of who proposed a time, a conversation's updated_on feeds its "resolved in the
// last 30 days" count, and an incident's updated_by is what its events report.
//
// This is real SQL against the real schema as a restricted login (row-level
// security in force), so it also proves the extra UPDATE is allowed for a
// project member and cannot touch a work item the caller cannot see.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run CommentUpdatedOn

const (
	cuProjectID   = "75000000-0000-0000-0000-000000000001"
	cuAccountID   = "75111111-0000-0000-0000-000000000001"
	cuContactID   = "75222222-0000-0000-0000-000000000001"
	cuCaseID      = "75333333-0000-0000-0000-000000000001"
	cuIncidentID  = "75333333-0000-0000-0000-000000000002"
	cuSiblingID   = "75333333-0000-0000-0000-000000000003"
	cuChangeReqID = "75333333-0000-0000-0000-000000000004"
	cuConvID      = "75333333-0000-0000-0000-000000000005"

	cuMember   = "cu-member@test.local"
	cuStranger = "cu-stranger@test.local"
	cuStaff    = "cu-staff@test.local"

	// cuSeedWriter is the updated_by every seeded work item starts with.
	cuSeedWriter = "seed"
)

var cuAllItems = []string{cuCaseID, cuIncidentID, cuSiblingID, cuChangeReqID, cuConvID}

// cuStale is the updated_on every seeded work item starts with: far enough in
// the past that any bump is unmistakable.
var cuStale = time.Now().UTC().Add(-10 * 24 * time.Hour).Truncate(time.Second)

func seedCommentUpdatedOnFixture(t *testing.T) (context.Context, *repository.Scoped, repository.CommentRepository, repository.CaseRepository) {
	t.Helper()
	pool := caseStatsPool(t)
	internal := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)

	cleanup := func() {
		_, _ = scoped.Exec(internal, `DELETE FROM comment WHERE work_item_id = ANY($1::uuid[])`, cuAllItems)
		_, _ = scoped.Exec(internal, `DELETE FROM work_item WHERE project_id = $1`, cuProjectID)
		_, _ = pool.Exec(internal, `DELETE FROM project_contact WHERE project_id = $1`, cuProjectID)
		_, _ = pool.Exec(internal, `DELETE FROM project WHERE id = $1`, cuProjectID)
		_, _ = pool.Exec(internal, `DELETE FROM account_contact WHERE id = $1`, cuContactID)
		_, _ = pool.Exec(internal, `DELETE FROM account WHERE id = $1`, cuAccountID)
	}
	cleanup()
	t.Cleanup(cleanup)

	mustPool := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(internal, sql, args...); err != nil {
			t.Fatalf("seed (%.80s): %v", sql, err)
		}
	}
	mustScoped := func(sql string, args ...any) {
		t.Helper()
		if _, err := scoped.Exec(internal, sql, args...); err != nil {
			t.Fatalf("seed scoped (%.80s): %v", sql, err)
		}
	}

	now := time.Now().UTC()
	mustPool(`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id)
		VALUES ($1, $2, $2, 'test', 'test', 'CU Test Account', 'CU-ACC-1', 'sf-cu-acc-1')`, cuAccountID, now)
	mustPool(`INSERT INTO account_contact (id, created_on, updated_on, created_by, updated_by, user_name, account_id)
		VALUES ($1, $2, $2, 'test', 'test', 'CU Test Contact', $3)`, cuContactID, now, cuAccountID)
	mustPool(`INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, account_id)
		VALUES ($1, $2, $2, 'test', 'test', 'CUTEST', 'sf-cutest', $3)`, cuProjectID, now, cuAccountID)
	mustPool(`INSERT INTO project_contact (id, created_on, updated_on, created_by, updated_by, email, account_contact_id, project_id, state)
		VALUES (gen_random_uuid(), $1, $1, 'test', 'test', $2, $3, $4, 'REGISTERED')`, now, cuMember, cuContactID, cuProjectID)

	// Two cases (the one commented on and a sibling that must stay untouched),
	// and one work item of each type a comment must NOT stamp.
	for _, w := range []struct{ id, number, wso2, typ string }{
		{cuCaseID, "CU-CASE-1", "CU-WSO2-1", "CASE"},
		{cuSiblingID, "CU-CASE-2", "CU-WSO2-2", "CASE"},
	} {
		mustScoped(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, project_id)
			VALUES ($1, $2, $2, $3, $3, $4, $5, 'cu case', $6::work_item_type_enum, $7)`, w.id, cuStale, cuSeedWriter, w.number, w.wso2, w.typ, cuProjectID)
	}
	for _, w := range []struct{ id, number, typ string }{
		{cuIncidentID, "CU-INC-1", "INCIDENT"},
		{cuChangeReqID, "CU-CHG-1", "CHANGE_REQUEST"},
		{cuConvID, "CU-CONV-1", "CONVERSATION"},
	} {
		mustScoped(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type, project_id)
			VALUES ($1, $2, $2, $3, $3, $4, 'cu item', $5::work_item_type_enum, $6)`, w.id, cuStale, cuSeedWriter, w.number, w.typ, cuProjectID)
	}

	return internal, scoped, repository.NewCommentRepository(scoped), repository.NewCaseRepository(scoped)
}

func memberCtx() context.Context {
	return repository.WithCallerIdentity(context.Background(), repository.SearchScope{ProjectIDs: []string{cuProjectID}, ViewerEmail: cuMember})
}

// dbNow is the DATABASE's clock, not the test host's: the stamp is the
// database's NOW(), so comparing it with the host clock would be flaky against
// a remote database.
func dbNow(t *testing.T, ctx context.Context, scoped *repository.Scoped) time.Time {
	t.Helper()
	var now time.Time
	if err := scoped.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatalf("read database clock: %v", err)
	}
	return now.UTC()
}

func workItemStamp(t *testing.T, ctx context.Context, scoped *repository.Scoped, id string) (time.Time, string) {
	t.Helper()
	var updatedOn time.Time
	var updatedBy string
	if err := scoped.QueryRow(ctx, `SELECT updated_on, updated_by FROM work_item WHERE id = $1`, id).Scan(&updatedOn, &updatedBy); err != nil {
		t.Fatalf("read work_item stamp: %v", err)
	}
	return updatedOn.UTC(), updatedBy
}

func requireUntouched(t *testing.T, ctx context.Context, scoped *repository.Scoped, id, what string) {
	t.Helper()
	if gotOn, gotBy := workItemStamp(t, ctx, scoped, id); !gotOn.Equal(cuStale) || gotBy != cuSeedWriter {
		t.Errorf("%s: work_item stamp = %s / %q, want it untouched (%s / %s)", what, gotOn, gotBy, cuStale, cuSeedWriter)
	}
}

func commentCount(t *testing.T, ctx context.Context, scoped *repository.Scoped, id string) int {
	t.Helper()
	var n int
	if err := scoped.QueryRow(ctx, `SELECT count(*) FROM comment WHERE work_item_id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	return n
}

func TestCommentUpdatedOnIntegration_MemberCommentBumpsTheCase(t *testing.T) {
	internal, scoped, repo, _ := seedCommentUpdatedOnFixture(t)

	before := dbNow(t, internal, scoped)
	row, err := repo.CreateComment(memberCtx(), cuCaseID, domain.ReferenceTypeCase, "COMMENT", "<p>Please let us know.</p>", cuMember)
	if err != nil {
		t.Fatalf("CreateComment as a project member: %v", err)
	}

	gotOn, gotBy := workItemStamp(t, internal, scoped, cuCaseID)
	if gotOn.Before(before) {
		t.Fatalf("work_item.updated_on = %s, want it advanced to the comment's time (>= %s); it was left at the stale %s", gotOn, before, cuStale)
	}
	if !gotOn.Equal(row.CreatedOn.UTC()) {
		t.Errorf("work_item.updated_on = %s, comment.created_on = %s, want the same instant (one statement, one NOW())", gotOn, row.CreatedOn.UTC())
	}
	if gotBy != cuMember {
		t.Errorf("work_item.updated_by = %q, want the commenter %q", gotBy, cuMember)
	}
	if n := commentCount(t, internal, scoped, cuCaseID); n != 1 {
		t.Errorf("comment rows for the case = %d, want exactly the one just written", n)
	}
	// Only the commented case moved.
	requireUntouched(t, internal, scoped, cuSiblingID, "a sibling case")
}

func TestCommentUpdatedOnIntegration_InternalWorkNoteBumpsTheCase(t *testing.T) {
	internal, scoped, repo, _ := seedCommentUpdatedOnFixture(t)

	before := dbNow(t, internal, scoped)
	if _, err := repo.CreateComment(internal, cuCaseID, domain.ReferenceTypeCase, "WORK_NOTE", "internal note", cuStaff); err != nil {
		t.Fatalf("CreateComment as internal: %v", err)
	}
	gotOn, gotBy := workItemStamp(t, internal, scoped, cuCaseID)
	if gotOn.Before(before) {
		t.Fatalf("work_item.updated_on = %s after an internal work note, want >= %s", gotOn, before)
	}
	if gotBy != cuStaff {
		t.Errorf("work_item.updated_by = %q, want %q", gotBy, cuStaff)
	}
}

// POST /cases/{id}/comments is the route "Request update" and every case reply
// take, and it is a different repository method from the reference-generic
// comment create above, so it is held to the same rule on its own.
func TestCommentUpdatedOnIntegration_CaseCommentBumpsTheCase(t *testing.T) {
	internal, scoped, _, caseRepo := seedCommentUpdatedOnFixture(t)

	before := dbNow(t, internal, scoped)
	c, err := caseRepo.CreateCaseComment(memberCtx(), domain.CreateCaseCommentRequest{
		CaseID: cuCaseID, CreatedBy: cuMember, Type: domain.CommentTypeComment, Content: "<p>Please let us know.</p>",
	}, nil)
	if err != nil {
		t.Fatalf("CreateCaseComment as a project member: %v", err)
	}

	gotOn, gotBy := workItemStamp(t, internal, scoped, cuCaseID)
	if gotOn.Before(before) {
		t.Fatalf("work_item.updated_on = %s, want it advanced to the comment's time (>= %s); it was left at the stale %s", gotOn, before, cuStale)
	}
	if !gotOn.Equal(c.CreatedOn.UTC()) {
		t.Errorf("work_item.updated_on = %s, comment.created_on = %s, want the same instant", gotOn, c.CreatedOn.UTC())
	}
	if gotBy != cuMember {
		t.Errorf("work_item.updated_by = %q, want the commenter %q", gotBy, cuMember)
	}
	requireUntouched(t, internal, scoped, cuSiblingID, "a sibling case")
}

// A change request's updated_by names who proposed a time (the last writer), so
// no comment may replace it; a conversation's updated_on feeds its resolved
// count; an incident's updated_by is what its events report. Each keeps exactly
// the behaviour it had: the comment is written and the work item is left alone.
func TestCommentUpdatedOnIntegration_OnlyCaseLikeWorkItemsAreStamped(t *testing.T) {
	internal, scoped, repo, caseRepo := seedCommentUpdatedOnFixture(t)

	generic := []struct {
		name string
		ref  domain.ReferenceType
		id   string
	}{
		{"a change request through POST /comments", domain.ReferenceTypeChangeRequest, cuChangeReqID},
		{"an incident through POST /comments", domain.ReferenceTypeIncident, cuIncidentID},
		{"a conversation through POST /comments", domain.ReferenceTypeConversation, cuConvID},
	}
	for _, tc := range generic {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := repo.CreateComment(internal, tc.id, tc.ref, "COMMENT", "a note", cuStaff); err != nil {
				t.Fatalf("CreateComment: %v", err)
			}
			if n := commentCount(t, internal, scoped, tc.id); n != 1 {
				t.Errorf("comment rows = %d, want the one just written", n)
			}
			requireUntouched(t, internal, scoped, tc.id, tc.name)
		})
	}

	// The case route takes the id of ANY work item (a customer's BFF forwards
	// whatever id it is given), a change request's included.
	t.Run("a change request through POST /cases/{id}/comments", func(t *testing.T) {
		if _, err := caseRepo.CreateCaseComment(internal, domain.CreateCaseCommentRequest{
			CaseID: cuChangeReqID, CreatedBy: cuStaff, Type: domain.CommentTypeComment, Content: "another note",
		}, nil); err != nil {
			t.Fatalf("CreateCaseComment: %v", err)
		}
		requireUntouched(t, internal, scoped, cuChangeReqID, "a change request through the case route")
	})
}

func TestCommentUpdatedOnIntegration_MirroredHistoryDoesNotBumpTheCase(t *testing.T) {
	internal, scoped, _, caseRepo := seedCommentUpdatedOnFixture(t)

	// ServiceNow's initial comments are copied in with their own past
	// timestamp when a case is created under dual write: that is history, not
	// something happening to the case now.
	past := cuStale.Add(-48 * time.Hour)
	c, err := caseRepo.CreateCaseCommentAsSystem(internal, domain.CreateCaseCommentRequest{
		CaseID: cuCaseID, CreatedBy: "customer@test.local", Type: domain.CommentTypeComment, Content: "opening message",
	}, &past)
	if err != nil {
		t.Fatalf("CreateCaseCommentAsSystem with a past timestamp: %v", err)
	}
	if !c.CreatedOn.UTC().Equal(past) {
		t.Fatalf("comment.created_on = %s, want the mirrored %s", c.CreatedOn.UTC(), past)
	}
	requireUntouched(t, internal, scoped, cuCaseID, "a mirrored history comment")
}

// The stamp is the transaction's NOW(); a comment that was queued behind a write
// stamped later than that must not move updated_on (or its writer) backwards.
func TestCommentUpdatedOnIntegration_NeverMovesUpdatedOnBackwards(t *testing.T) {
	internal, scoped, repo, caseRepo := seedCommentUpdatedOnFixture(t)

	later := dbNow(t, internal, scoped).Add(time.Hour)
	if _, err := scoped.Exec(internal, `UPDATE work_item SET updated_on = $2, updated_by = 'later-writer' WHERE id = $1`, cuCaseID, later); err != nil {
		t.Fatalf("seed a later stamp: %v", err)
	}
	check := func(name string) {
		t.Helper()
		gotOn, gotBy := workItemStamp(t, internal, scoped, cuCaseID)
		if !gotOn.Equal(later.Truncate(time.Microsecond)) || gotBy != "later-writer" {
			t.Errorf("%s: stamp = %s / %q, want the later one kept (%s / later-writer)", name, gotOn, gotBy, later)
		}
	}

	if _, err := repo.CreateComment(internal, cuCaseID, domain.ReferenceTypeCase, "COMMENT", "x", cuStaff); err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	check("generic route")
	if _, err := caseRepo.CreateCaseComment(internal, domain.CreateCaseCommentRequest{
		CaseID: cuCaseID, CreatedBy: cuStaff, Type: domain.CommentTypeComment, Content: "y",
	}, nil); err != nil {
		t.Fatalf("CreateCaseComment: %v", err)
	}
	check("case route")
	if n := commentCount(t, internal, scoped, cuCaseID); n != 2 {
		t.Errorf("comment rows = %d, want both written", n)
	}
}

// The comment and the stamp are one statement: when the insert is refused
// (a member may not write a work note) nothing is stamped either.
func TestCommentUpdatedOnIntegration_RefusedInsertLeavesTheStampAlone(t *testing.T) {
	internal, scoped, repo, caseRepo := seedCommentUpdatedOnFixture(t)

	_, err := repo.CreateComment(memberCtx(), cuCaseID, domain.ReferenceTypeCase, "WORK_NOTE", "a member's work note", cuMember)
	if !isRLSViolation(err) {
		t.Fatalf("CreateComment of a work note as a member: error = %v, want a row-level-security violation", err)
	}
	_, err = caseRepo.CreateCaseComment(memberCtx(), domain.CreateCaseCommentRequest{
		CaseID: cuCaseID, CreatedBy: cuMember, Type: domain.CommentTypeWorkNote, Content: "a member's work note",
	}, nil)
	if !isRLSViolation(err) {
		t.Fatalf("CreateCaseComment of a work note as a member: error = %v, want a row-level-security violation", err)
	}

	requireUntouched(t, internal, scoped, cuCaseID, "a refused work note")
	if n := commentCount(t, internal, scoped, cuCaseID); n != 0 {
		t.Errorf("comment rows after refused writes = %d, want 0", n)
	}
}

func TestCommentUpdatedOnIntegration_RefusedCommentLeavesTheWorkItemAlone(t *testing.T) {
	internal, scoped, repo, caseRepo := seedCommentUpdatedOnFixture(t)
	stranger := repository.WithCallerIdentity(context.Background(), repository.SearchScope{ViewerEmail: cuStranger})

	notFound := func(t *testing.T, err error) {
		t.Helper()
		var nf *apierror.NotFoundError
		if !errors.As(err, &nf) {
			t.Fatalf("error = %v, want a NotFoundError", err)
		}
	}

	t.Run("a case reference naming an incident", func(t *testing.T) {
		_, err := repo.CreateComment(memberCtx(), cuIncidentID, domain.ReferenceTypeCase, "COMMENT", "should not land", cuMember)
		notFound(t, err)
		requireUntouched(t, internal, scoped, cuIncidentID, "the incident")
	})
	t.Run("a caller who is not on the project, through POST /comments", func(t *testing.T) {
		_, err := repo.CreateComment(stranger, cuCaseID, domain.ReferenceTypeCase, "COMMENT", "should not land", cuStranger)
		notFound(t, err)
		requireUntouched(t, internal, scoped, cuCaseID, "the case")
	})
	t.Run("a caller who is not on the project, through POST /cases/{id}/comments", func(t *testing.T) {
		_, err := caseRepo.CreateCaseComment(stranger, domain.CreateCaseCommentRequest{
			CaseID: cuCaseID, CreatedBy: cuStranger, Type: domain.CommentTypeComment, Content: "should not land",
		}, nil)
		notFound(t, err)
		requireUntouched(t, internal, scoped, cuCaseID, "the case")
	})

	if n := commentCount(t, internal, scoped, cuCaseID) + commentCount(t, internal, scoped, cuIncidentID); n != 0 {
		t.Errorf("comment rows after refused writes = %d, want 0", n)
	}
}

// Concurrent comments on one case all land, nothing deadlocks, and the stamp is
// the latest of them (the guard keeps it monotonic): updated_on ends equal to
// the newest comment's created_on.
func TestCommentUpdatedOnIntegration_ConcurrentCommentsAllLand(t *testing.T) {
	internal, scoped, repo, caseRepo := seedCommentUpdatedOnFixture(t)

	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var err error
			if i%2 == 0 {
				_, err = repo.CreateComment(internal, cuCaseID, domain.ReferenceTypeCase, "COMMENT", "concurrent", cuStaff)
			} else {
				_, err = caseRepo.CreateCaseComment(internal, domain.CreateCaseCommentRequest{
					CaseID: cuCaseID, CreatedBy: cuStaff, Type: domain.CommentTypeComment, Content: "concurrent",
				}, nil)
			}
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a concurrent comment failed: %v", err)
		}
	}

	if n := commentCount(t, internal, scoped, cuCaseID); n != workers {
		t.Fatalf("comment rows = %d, want %d", n, workers)
	}
	var newest time.Time
	if err := scoped.QueryRow(internal, `SELECT max(created_on) FROM comment WHERE work_item_id = $1`, cuCaseID).Scan(&newest); err != nil {
		t.Fatalf("read newest comment: %v", err)
	}
	if gotOn, _ := workItemStamp(t, internal, scoped, cuCaseID); !gotOn.Equal(newest.UTC()) {
		t.Errorf("work_item.updated_on = %s, want the newest comment's time %s", gotOn, newest.UTC())
	}
}
