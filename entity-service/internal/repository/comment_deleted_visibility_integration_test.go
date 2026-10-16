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
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// A soft-deleted comment (deleted_at IS NOT NULL) must never reach a
// customer-portal caller through any read path, while an internal caller's
// results are unchanged. Needs CASE_STATS_TEST_DSN (skipped otherwise).

const (
	dcProjectID = "75000000-0000-0000-0000-000000000001"
	dcAccountID = "75111111-0000-0000-0000-000000000001"
	dcContactID = "75222222-0000-0000-0000-000000000001"
	dcCaseID    = "75333333-0000-0000-0000-000000000001"
	dcConvID    = "75333333-0000-0000-0000-000000000002"

	dcLiveCase    = "75444444-0000-0000-0000-000000000001"
	dcDeletedCase = "75444444-0000-0000-0000-000000000002"
	dcLiveConv    = "75444444-0000-0000-0000-000000000003"
	dcDeletedConv = "75444444-0000-0000-0000-000000000004"

	dcMember = "dc-member@test.local"
	dcStaff  = "dc-staff@test.local"
)

func dcContexts() (internal, staffViaPortal, member context.Context) {
	base := context.Background()
	internal = repository.WithSystemIdentity(base)
	staffViaPortal = repository.WithCallerIdentity(base, repository.SearchScope{Unrestricted: true, ViewerEmail: dcStaff, HasInternalAccess: true, ViaCustomerPortal: true})
	member = repository.WithCallerIdentity(base, repository.SearchScope{ProjectIDs: []string{dcProjectID}, ViewerEmail: dcMember})
	return
}

func seedDeletedCommentFixture(t *testing.T, pool *pgxpool.Pool) *repository.Scoped {
	t.Helper()
	internal, _, _ := dcContexts()
	scoped := repository.NewScoped(pool)

	cleanup := func() {
		_, _ = scoped.Exec(internal, `DELETE FROM comment WHERE work_item_id IN ($1, $2)`, dcCaseID, dcConvID)
		_, _ = scoped.Exec(internal, `DELETE FROM conversation WHERE id = $1`, dcConvID)
		_, _ = scoped.Exec(internal, `DELETE FROM work_item WHERE project_id = $1`, dcProjectID)
		_, _ = pool.Exec(internal, `DELETE FROM project_contact WHERE project_id = $1`, dcProjectID)
		_, _ = pool.Exec(internal, `DELETE FROM project WHERE id = $1`, dcProjectID)
		_, _ = pool.Exec(internal, `DELETE FROM account_contact WHERE id = $1`, dcContactID)
		_, _ = pool.Exec(internal, `DELETE FROM account WHERE id = $1`, dcAccountID)
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC()
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

	mustPool(`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id)
		VALUES ($1, $2, $2, 'test', 'test', 'DC Test Account', 'DC-ACC-1', 'sf-dc-acc-1')`, dcAccountID, now)
	mustPool(`INSERT INTO account_contact (id, created_on, updated_on, created_by, updated_by, user_name, account_id)
		VALUES ($1, $2, $2, 'test', 'test', 'DC Test Contact', $3)`, dcContactID, now, dcAccountID)
	mustPool(`INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, account_id)
		VALUES ($1, $2, $2, 'test', 'test', 'DCTEST', 'sf-dctest', $3)`, dcProjectID, now, dcAccountID)
	mustPool(`INSERT INTO project_contact (id, created_on, updated_on, created_by, updated_by, email, account_contact_id, project_id, state)
		VALUES (gen_random_uuid(), $1, $1, 'test', 'test', $2, $3, $4, 'REGISTERED')`, now, dcMember, dcContactID, dcProjectID)

	mustScoped(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, project_id)
		VALUES ($1, $2, $2, 'test', 'test', 'DC-CASE-1', 'DC-WSO2-1', 'dc case', 'CASE', $3)`, dcCaseID, now, dcProjectID)
	mustScoped(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, project_id)
		VALUES ($1, $2, $2, 'test', 'test', 'DC-CONV-1', 'DC-WSO2-2', 'dc conversation', 'CONVERSATION', $3)`, dcConvID, now, dcProjectID)
	mustScoped(`INSERT INTO conversation (id, state) VALUES ($1, 'ACTIVE')`, dcConvID)

	for _, c := range []struct {
		id, workItem, body string
		deleted            bool
		at                 time.Time
	}{
		{dcLiveCase, dcCaseID, "live case comment", false, now.Add(-1 * time.Hour)},
		{dcDeletedCase, dcCaseID, "deleted case comment", true, now.Add(-2 * time.Hour)},
		{dcLiveConv, dcConvID, "live chat message", false, now.Add(-1 * time.Hour)},
		{dcDeletedConv, dcConvID, "deleted chat message", true, now.Add(-2 * time.Hour)},
	} {
		mustScoped(`INSERT INTO comment (id, created_on, created_by, type, work_item_id, content, deleted_at, deleted_by)
			VALUES ($1, $2, 'test', 'COMMENT', $3, $4, CASE WHEN $5 THEN NOW() END, CASE WHEN $5 THEN 'test' END)`,
			c.id, c.at, c.workItem, c.body, c.deleted)
	}
	return scoped
}

func idsOf[T any](rows []T, id func(T) string) map[string]bool {
	out := map[string]bool{}
	for _, r := range rows {
		out[id(r)] = true
	}
	return out
}

func TestDeletedCommentVisibility_CaseComments(t *testing.T) {
	pool := caseStatsPool(t)
	scoped := seedDeletedCommentFixture(t, pool)
	repo := repository.NewCaseRepository(scoped)
	internal, staffViaPortal, member := dcContexts()
	req := domain.SearchCaseCommentsRequest{CaseID: dcCaseID, Pagination: domain.Pagination{Limit: 50}}

	rows, total, err := repo.SearchCaseComments(internal, req)
	if err != nil {
		t.Fatalf("internal: %v", err)
	}
	if got := idsOf(rows, func(c domain.CaseComment) string { return c.ID }); total != 2 || !got[dcLiveCase] || !got[dcDeletedCase] {
		t.Fatalf("internal caller must see live and deleted unchanged, total=%d got=%v", total, got)
	}

	for name, ctx := range map[string]context.Context{"customer member": member, "staff via customer portal": staffViaPortal} {
		rows, total, err := repo.SearchCaseComments(ctx, req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := idsOf(rows, func(c domain.CaseComment) string { return c.ID }); total != 1 || len(rows) != 1 || !got[dcLiveCase] {
			t.Fatalf("%s must see only the live comment (total and rows), total=%d got=%v", name, total, got)
		}
	}
}

func TestDeletedCommentVisibility_CaseActivities(t *testing.T) {
	pool := caseStatsPool(t)
	scoped := seedDeletedCommentFixture(t, pool)
	repo := repository.NewCaseRepository(scoped)
	internal, staffViaPortal, member := dcContexts()
	req := domain.SearchCaseActivitiesRequest{CaseID: dcCaseID, Pagination: domain.Pagination{Limit: 50}}

	rows, total, err := repo.SearchCaseActivities(internal, req)
	if err != nil {
		t.Fatalf("internal: %v", err)
	}
	if got := idsOf(rows, func(a domain.CaseActivity) string { return a.ID }); total != 2 || !got[dcLiveCase] || !got[dcDeletedCase] {
		t.Fatalf("internal caller must see live and deleted unchanged, total=%d got=%v", total, got)
	}

	for name, ctx := range map[string]context.Context{"customer member": member, "staff via customer portal": staffViaPortal} {
		rows, total, err := repo.SearchCaseActivities(ctx, req)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := idsOf(rows, func(a domain.CaseActivity) string { return a.ID }); total != 1 || len(rows) != 1 || !got[dcLiveCase] {
			t.Fatalf("%s must see only the live comment (total and rows), total=%d got=%v", name, total, got)
		}
	}
}

func TestDeletedCommentVisibility_GenericSearch(t *testing.T) {
	pool := caseStatsPool(t)
	scoped := seedDeletedCommentFixture(t, pool)
	repo := repository.NewCommentRepository(scoped)
	internal, staffViaPortal, member := dcContexts()
	page := domain.Pagination{Limit: 50}

	// excludeDeleted=false mimics the service's token check failing to
	// classify the caller: the resolved identity alone must still hide the row.
	rows, total, err := repo.SearchComments(internal, dcCaseID, domain.ReferenceTypeCase, nil, false, page)
	if err != nil {
		t.Fatalf("internal: %v", err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("internal caller must see live and deleted unchanged, total=%d rows=%d", total, len(rows))
	}

	for name, ctx := range map[string]context.Context{"customer member": member, "staff via customer portal": staffViaPortal} {
		rows, total, err := repo.SearchComments(ctx, dcCaseID, domain.ReferenceTypeCase, nil, false, page)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := idsOf(rows, func(c repository.CommentRow) string { return c.ID }); total != 1 || !got[dcLiveCase] {
			t.Fatalf("%s must see only the live comment, total=%d got=%v", name, total, got)
		}
	}
}

func TestDeletedCommentVisibility_ConversationStats(t *testing.T) {
	pool := caseStatsPool(t)
	scoped := seedDeletedCommentFixture(t, pool)
	repo := repository.NewConversationRepository(scoped)
	internal, staffViaPortal, member := dcContexts()

	d, err := repo.GetConversation(internal, dcConvID)
	if err != nil {
		t.Fatalf("internal: %v", err)
	}
	if d.MessageCount != 2 {
		t.Fatalf("internal caller must count live and deleted unchanged, got %d", d.MessageCount)
	}

	for name, ctx := range map[string]context.Context{"customer member": member, "staff via customer portal": staffViaPortal} {
		d, err := repo.GetConversation(ctx, dcConvID)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d.MessageCount != 1 || (d.InitialMessage != nil && *d.InitialMessage == "deleted chat message") {
			t.Fatalf("%s must not count or surface the deleted message, count=%d initial=%v", name, d.MessageCount, d.InitialMessage)
		}
	}
}
