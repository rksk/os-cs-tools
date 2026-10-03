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

// Comment visibility on the Postgres data source: internal notes stay with
// staff (the comment policies, migration 0179) and soft-deleted comments leave
// the case thread and activity feed for everyone. Needs an RLS-enforcing role,
// like the other RLS integration tests; skipped without CASE_STATS_TEST_DSN.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run CommentVisibility

package repository_test

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const (
	cvCommentID   = "7a000000-0000-4000-8000-0000000000c1"
	cvWorkNoteID  = "7a000000-0000-4000-8000-0000000000c2"
	cvDeletedID   = "7a000000-0000-4000-8000-0000000000c3"
	cvCustomerTag = "cv customer comment"
)

func TestCommentVisibility_WorkNotesAndDeletedComments(t *testing.T) {
	pool := caseStatsPool(t)
	seedRLSScopedRemaining(t, pool)
	rlsPrecondition(t, pool)

	caseID := rsWorkItems[0] // an open case on project A
	sys := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)
	cleanup := func() {
		_, _ = scoped.Exec(sys, `DELETE FROM comment WHERE work_item_id = $1`, caseID)
	}
	cleanup()
	t.Cleanup(cleanup)
	if _, err := scoped.Exec(sys, `INSERT INTO comment (id, created_on, created_by, type, work_item_id, content, deleted_at) VALUES
		($1, now() - interval '3 minutes', 'jane.doe@example.com', 'COMMENT',   $4, 'visible comment', NULL),
		($2, now() - interval '2 minutes', 'jane.doe@example.com', 'WORK_NOTE', $4, 'internal note',   NULL),
		($3, now() - interval '1 minute',  'jane.doe@example.com', 'COMMENT',   $4, 'retracted',       now())`,
		cvCommentID, cvWorkNoteID, cvDeletedID, caseID); err != nil {
		t.Fatalf("seed comments: %v", err)
	}

	repo := repository.NewCaseRepository(scoped)
	customer, _ := rsCustomer(rsMemberA, rsProjectA)
	page := domain.Pagination{Limit: 50}

	commentIDs := func(t *testing.T, ctx context.Context, filter *domain.CommentType) (map[string]bool, int) {
		t.Helper()
		req := domain.SearchCaseCommentsRequest{CaseID: caseID, Pagination: page}
		if filter != nil {
			req.Filters = &domain.CommentFilters{Type: filter}
		}
		got, total, err := repo.SearchCaseComments(ctx, req)
		if err != nil {
			t.Fatalf("SearchCaseComments: %v", err)
		}
		ids := map[string]bool{}
		for _, c := range got {
			ids[c.ID] = true
		}
		return ids, total
	}

	t.Run("internal sees comments and work notes, not deleted comments", func(t *testing.T) {
		ids, total := commentIDs(t, rsInternal(), nil)
		if !ids[cvCommentID] || !ids[cvWorkNoteID] || ids[cvDeletedID] || len(ids) != 2 || total != 2 {
			t.Errorf("internal comments = %v (total %d), want the comment and the work note only", ids, total)
		}
	})

	t.Run("customer sees only the comment, even when asking for work notes", func(t *testing.T) {
		ids, total := commentIDs(t, customer, nil)
		if !ids[cvCommentID] || len(ids) != 1 || total != 1 {
			t.Errorf("customer comments = %v (total %d), want only the comment", ids, total)
		}
		workNote := domain.CommentTypeWorkNote
		ids, total = commentIDs(t, customer, &workNote)
		if len(ids) != 0 || total != 0 {
			t.Errorf("customer work-note search = %v (total %d), want none", ids, total)
		}
	})

	t.Run("activity feed follows the same rules", func(t *testing.T) {
		count := func(ctx context.Context) (map[string]bool, int) {
			acts, total, err := repo.SearchCaseActivities(ctx, domain.SearchCaseActivitiesRequest{CaseID: caseID, Pagination: page})
			if err != nil {
				t.Fatalf("SearchCaseActivities: %v", err)
			}
			ids := map[string]bool{}
			for _, a := range acts {
				ids[a.ID] = true
			}
			return ids, total
		}
		if ids, total := count(rsInternal()); !ids[cvCommentID] || !ids[cvWorkNoteID] || ids[cvDeletedID] || total != 2 {
			t.Errorf("internal activity = %v (total %d), want the comment and the work note only", ids, total)
		}
		if ids, total := count(customer); !ids[cvCommentID] || ids[cvWorkNoteID] || ids[cvDeletedID] || total != 1 {
			t.Errorf("customer activity = %v (total %d), want only the comment", ids, total)
		}
	})

	t.Run("no customer read of the table returns a work note", func(t *testing.T) {
		var n int
		if err := scoped.QueryRow(customer, `SELECT COUNT(*) FROM comment WHERE work_item_id = $1 AND type = 'WORK_NOTE'`, caseID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("customer sees %d work notes, want 0", n)
		}
	})

	t.Run("customer cannot write a work note but can write a comment", func(t *testing.T) {
		_, err := repo.CreateCaseComment(customer, domain.CreateCaseCommentRequest{
			CaseID: caseID, CreatedBy: rsMemberA, Type: domain.CommentTypeWorkNote, Content: "customer note",
		}, nil)
		if !repository.IsRLSPolicyViolation(err) {
			t.Errorf("customer work note: err = %v, want a row-level security violation", err)
		}
		if _, err := repo.CreateCaseComment(customer, domain.CreateCaseCommentRequest{
			CaseID: caseID, CreatedBy: rsMemberA, Type: domain.CommentTypeComment, Content: cvCustomerTag,
		}, nil); err != nil {
			t.Errorf("customer comment: %v", err)
		}
		if _, err := repo.CreateCaseComment(rsInternal(), domain.CreateCaseCommentRequest{
			CaseID: caseID, CreatedBy: "jane.doe@example.com", Type: domain.CommentTypeWorkNote, Content: "staff note",
		}, nil); err != nil {
			t.Errorf("internal work note: %v", err)
		}
	})

	t.Run("customer cannot turn a comment into a work note", func(t *testing.T) {
		_, err := scoped.Exec(customer, `UPDATE comment SET type = 'WORK_NOTE' WHERE id = $1`, cvCommentID)
		if !repository.IsRLSPolicyViolation(err) {
			t.Errorf("customer type change: err = %v, want a row-level security violation", err)
		}
	})
}
