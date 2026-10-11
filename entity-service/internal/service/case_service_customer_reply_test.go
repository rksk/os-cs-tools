// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// customerReplyHarness wires a caseService whose case is in caseState and
// whose comment author holds roles, recording every state UpdateCase wrote.
type customerReplyHarness struct {
	svc          *caseService
	updatedState []domain.CaseState
	fetches      int
}

func newCustomerReplyHarness(t *testing.T, caseState domain.CaseState, roles []string, userErr error, workItemType ...string) (*customerReplyHarness, context.Context) {
	t.Helper()
	h := &customerReplyHarness{}
	repo := &stubCaseRepo{
		createCaseComment: func(_ context.Context, req domain.CreateCaseCommentRequest, _ *time.Time) (domain.CaseComment, error) {
			return domain.CaseComment{ID: "comment-1", CaseID: req.CaseID, Type: req.Type, Content: req.Content}, nil
		},
		getCaseByID: func(context.Context, string, repository.SearchScope) (domain.CaseView, error) {
			h.fetches++
			st := caseState
			cv := domain.CaseView{ID: testDeploymentUUID, State: &st}
			if len(workItemType) > 0 {
				cv.Type = &workItemType[0]
			}
			return cv, nil
		},
		updateCase: func(_ context.Context, req domain.UpdateCaseRequest) (domain.Case, *domain.CaseSeverity, error) {
			h.updatedState = append(h.updatedState, *req.State)
			return domain.Case{ID: req.ID, State: req.State}, nil, nil
		},
	}
	userRepo := stubUserRepo{
		getUserByEmail: func(context.Context, string) (domain.User, error) {
			if userErr != nil {
				return domain.User{}, userErr
			}
			return domain.User{ID: "user-1", Email: "jane.doe@example.com"}, nil
		},
		getUserRoles: func(context.Context, string) ([]string, error) { return roles, nil },
	}
	h.svc = NewCaseService(repo, userRepo, nil, alwaysUnrestrictedAccess{}, nil).(*caseService)
	return h, contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))
}

func commentReq(typ domain.CommentType) domain.CreateCaseCommentRequest {
	return domain.CreateCaseCommentRequest{CaseID: testDeploymentUUID, Type: typ, Content: "reply"}
}

func TestCaseService_CustomerReply_MovesToWaitingOnWSO2(t *testing.T) {
	for _, st := range []domain.CaseState{domain.CaseStateAwaitingInfo, domain.CaseStateSolutionProposed} {
		for _, role := range []string{"customer", "customer_admin", "partner", "partner_admin", "external"} {
			t.Run(string(st)+"/"+role, func(t *testing.T) {
				h, ctx := newCustomerReplyHarness(t, st, []string{role}, nil)
				if _, err := h.svc.CreateCaseComment(ctx, commentReq(domain.CommentTypeComment)); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(h.updatedState) != 1 || h.updatedState[0] != domain.CaseStateWaitingOnWSO2 {
					t.Fatalf("expected exactly one UpdateCase to waiting_on_wso2, got %v", h.updatedState)
				}
			})
		}
	}
}

func TestCaseService_CustomerReply_NoTransition(t *testing.T) {
	cases := []struct {
		name      string
		state     domain.CaseState
		roles     []string
		userErr   error
		commentTy domain.CommentType
	}{
		{"work note", domain.CaseStateAwaitingInfo, []string{"customer"}, nil, domain.CommentTypeWorkNote},
		{"open", domain.CaseStateOpen, []string{"customer"}, nil, domain.CommentTypeComment},
		{"work in progress", domain.CaseStateWorkInProgress, []string{"customer"}, nil, domain.CommentTypeComment},
		{"closed", domain.CaseStateClosed, []string{"customer"}, nil, domain.CommentTypeComment},
		{"already waiting", domain.CaseStateWaitingOnWSO2, []string{"customer"}, nil, domain.CommentTypeComment},
		{"support engineer", domain.CaseStateAwaitingInfo, []string{"internal"}, nil, domain.CommentTypeComment},
		{"mixed identity", domain.CaseStateAwaitingInfo, []string{"customer", "internal"}, nil, domain.CommentTypeComment},
		{"no roles", domain.CaseStateAwaitingInfo, nil, nil, domain.CommentTypeComment},
		{"author unresolved", domain.CaseStateAwaitingInfo, []string{"customer"}, errors.New("no such user"), domain.CommentTypeComment},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, ctx := newCustomerReplyHarness(t, tc.state, tc.roles, tc.userErr)
			if tc.userErr != nil {
				// The comment itself needs a resolvable actor; use the M2M
				// path, whose actor is never looked up as a user.
				if _, err := h.svc.CreateCaseCommentAs(context.Background(), commentReq(tc.commentTy), "svc@example.com"); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if _, err := h.svc.CreateCaseComment(ctx, commentReq(tc.commentTy)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(h.updatedState) != 0 {
				t.Fatalf("expected no state change, got %v", h.updatedState)
			}
		})
	}
}

// A failing transition (UpdateCase error) must not fail or undo the comment.
func TestCaseService_CustomerReply_TransitionFailureDoesNotFailComment(t *testing.T) {
	h, ctx := newCustomerReplyHarness(t, domain.CaseStateAwaitingInfo, []string{"customer"}, nil)
	h.svc.repo.(*stubCaseRepo).updateCase = func(context.Context, domain.UpdateCaseRequest) (domain.Case, *domain.CaseSeverity, error) {
		return domain.Case{}, nil, errors.New("boom")
	}
	resp, err := h.svc.CreateCaseComment(ctx, commentReq(domain.CommentTypeComment))
	if err != nil {
		t.Fatalf("comment must succeed despite transition failure, got %v", err)
	}
	if resp.Comment.ID != "comment-1" {
		t.Errorf("unexpected comment id %q", resp.Comment.ID)
	}
}

// A failing case fetch must not fail the comment either.
func TestCaseService_CustomerReply_FetchFailureDoesNotFailComment(t *testing.T) {
	h, ctx := newCustomerReplyHarness(t, domain.CaseStateAwaitingInfo, []string{"customer"}, nil)
	h.svc.repo.(*stubCaseRepo).getCaseByID = func(context.Context, string, repository.SearchScope) (domain.CaseView, error) {
		return domain.CaseView{}, errors.New("boom")
	}
	if _, err := h.svc.CreateCaseComment(ctx, commentReq(domain.CommentTypeComment)); err != nil {
		t.Fatalf("comment must succeed despite fetch failure, got %v", err)
	}
	if len(h.updatedState) != 0 {
		t.Fatalf("expected no state change, got %v", h.updatedState)
	}
}

// A non-customer commenter must not pay for the case fetch at all.
func TestCaseService_CustomerReply_EngineerSkipsCaseFetch(t *testing.T) {
	h, ctx := newCustomerReplyHarness(t, domain.CaseStateAwaitingInfo, []string{"internal"}, nil)
	if _, err := h.svc.CreateCaseComment(ctx, commentReq(domain.CommentTypeComment)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.fetches != 0 {
		t.Errorf("expected no case fetch for a non-customer author, got %d", h.fetches)
	}
}

// Dual-write: the comment mirror still uses CreateBareCaseComment exactly
// once, and the resulting state change reaches the mirror exactly once via
// UpdateCase's own dispatch (not a second, separate path).
func TestCaseService_CustomerReply_DualWriteMirrorsOnceEach(t *testing.T) {
	var commentCalls, patchCalls chan struct{} = make(chan struct{}, 4), make(chan struct{}, 4)
	var patchedState *domain.CaseState
	mirror := &stubMirrorCaseService{
		createBareCaseComment: func(context.Context, string, domain.CommentType, string) (domain.CaseCommentDetail, error) {
			commentCalls <- struct{}{}
			return domain.CaseCommentDetail{}, nil
		},
		patchCaseFieldsFn: func(_ context.Context, _ string, state *domain.CaseState, _ *domain.CaseSeverity, _ *domain.CaseWorkState, _ *bool, _ *caseResolutionFields) (domain.UpdatedCase, error) {
			patchedState = state
			patchCalls <- struct{}{}
			return domain.UpdatedCase{}, nil
		},
	}
	repo := &stubCaseRepo{
		createCaseComment: func(_ context.Context, req domain.CreateCaseCommentRequest, _ *time.Time) (domain.CaseComment, error) {
			return domain.CaseComment{ID: "comment-1", CaseID: req.CaseID, Type: req.Type, Content: req.Content}, nil
		},
		getCaseByID: func(context.Context, string, repository.SearchScope) (domain.CaseView, error) {
			st := domain.CaseStateAwaitingInfo
			return domain.CaseView{ID: testDeploymentUUID, State: &st}, nil
		},
		updateCase: func(_ context.Context, req domain.UpdateCaseRequest) (domain.Case, *domain.CaseSeverity, error) {
			return domain.Case{ID: req.ID, State: req.State}, nil, nil
		},
	}
	userRepo := stubUserRepo{
		getUserByEmail: func(context.Context, string) (domain.User, error) {
			return domain.User{ID: "user-1", Email: "jane.doe@example.com"}, nil
		},
		getUserRoles: func(context.Context, string) ([]string, error) { return []string{"customer"}, nil },
	}
	svc := NewCaseServiceWithSNWriteback(repo, userRepo, nil, alwaysUnrestrictedAccess{}, nil, NewSNWritebackDispatcher(&recordingSNWritebackFailures{}), mirror, nil, "")
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))
	if _, err := svc.CreateCaseComment(ctx, commentReq(domain.CommentTypeComment)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, ch := range map[string]chan struct{}{"CreateBareCaseComment": commentCalls, "PatchCaseFields": patchCalls} {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s was never called", name)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if len(commentCalls) != 0 || len(patchCalls) != 0 {
		t.Errorf("mirror called more than once: extra comment=%d extra patch=%d", len(commentCalls), len(patchCalls))
	}
	if patchedState == nil || *patchedState != domain.CaseStateWaitingOnWSO2 {
		t.Errorf("mirrored state = %v, want waiting_on_wso2", patchedState)
	}
}

// Every case-like type with the three states transitions; announcements
// (open/closed only) are skipped cleanly, with no UpdateCase attempt.
func TestCaseService_CustomerReply_AcrossWorkItemTypes(t *testing.T) {
	cases := []struct {
		typ       string
		state     domain.CaseState
		wantMoved bool
	}{
		{"case", domain.CaseStateAwaitingInfo, true},
		{"case", domain.CaseStateSolutionProposed, true},
		{"service_request", domain.CaseStateAwaitingInfo, true},
		{"service_request", domain.CaseStateSolutionProposed, true},
		{"engagement", domain.CaseStateAwaitingInfo, true},
		{"engagement", domain.CaseStateSolutionProposed, true},
		{"security_report_analysis", domain.CaseStateAwaitingInfo, true},
		{"security_report_analysis", domain.CaseStateSolutionProposed, true},
		{"service_request", domain.CaseStateOpen, false},
		{"engagement", domain.CaseStateClosed, false},
		{"security_report_analysis", domain.CaseStateWorkInProgress, false},
		{"announcement", domain.CaseStateOpen, false},
		{"announcement", domain.CaseStateClosed, false},
		// Defensive: a (never expected) waiting state on an announcement.
		{"announcement", domain.CaseStateAwaitingInfo, false},
	}
	for _, tc := range cases {
		t.Run(tc.typ+"/"+string(tc.state), func(t *testing.T) {
			h, ctx := newCustomerReplyHarness(t, tc.state, []string{"customer"}, nil, tc.typ)
			if _, err := h.svc.CreateCaseComment(ctx, commentReq(domain.CommentTypeComment)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			moved := len(h.updatedState) == 1 && h.updatedState[0] == domain.CaseStateWaitingOnWSO2
			if moved != tc.wantMoved || (!tc.wantMoved && len(h.updatedState) != 0) {
				t.Fatalf("moved=%v (updates %v), want moved=%v", moved, h.updatedState, tc.wantMoved)
			}
		})
	}
}
