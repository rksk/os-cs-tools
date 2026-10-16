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

package service

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// TestIsValidCaseStateTransition_ExternalTable pins the legacy ServiceNow
// scoped-app's own ALLOWED_STATE_TRANSITIONS table, state by state.
func TestIsValidCaseStateTransition_ExternalTable(t *testing.T) {
	cases := []struct {
		from domain.CaseState
		to   domain.CaseState
		want bool
	}{
		{domain.CaseStateOpen, domain.CaseStateClosed, true},
		{domain.CaseStateOpen, domain.CaseStateWorkInProgress, false},
		{domain.CaseStateWorkInProgress, domain.CaseStateClosed, true},
		{domain.CaseStateWorkInProgress, domain.CaseStateAwaitingInfo, false},
		{domain.CaseStateWaitingOnWSO2, domain.CaseStateClosed, true},
		{domain.CaseStateSolutionProposed, domain.CaseStateClosed, true},
		{domain.CaseStateSolutionProposed, domain.CaseStateReopened, true},
		{domain.CaseStateSolutionProposed, domain.CaseStateWaitingOnWSO2, true},
		{domain.CaseStateSolutionProposed, domain.CaseStateAwaitingInfo, false},
		{domain.CaseStateAwaitingInfo, domain.CaseStateClosed, true},
		{domain.CaseStateAwaitingInfo, domain.CaseStateWaitingOnWSO2, true},
		{domain.CaseStateAwaitingInfo, domain.CaseStateSolutionProposed, false},
		{domain.CaseStateReopened, domain.CaseStateClosed, true},
		{domain.CaseStateReopened, domain.CaseStateWorkInProgress, false},
		{domain.CaseStateClosed, domain.CaseStateOpen, false},
	}
	for _, tc := range cases {
		if got := isValidCaseStateTransition(false, tc.from, tc.to); got != tc.want {
			t.Errorf("isValidCaseStateTransition(external, %q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

// TestIsValidCaseStateTransition_InternalTable pins the legacy ServiceNow
// scoped-app's own SUPERUSER_STATE_TRANSITIONS table, state by state, plus
// this codebase's own confirmed-intentional addition (reopened ->
// work_in_progress, absent from the legacy table).
func TestIsValidCaseStateTransition_InternalTable(t *testing.T) {
	cases := []struct {
		from domain.CaseState
		to   domain.CaseState
		want bool
	}{
		{domain.CaseStateOpen, domain.CaseStateWorkInProgress, true},
		{domain.CaseStateOpen, domain.CaseStateClosed, false},
		{domain.CaseStateWorkInProgress, domain.CaseStateWaitingOnWSO2, true},
		{domain.CaseStateWorkInProgress, domain.CaseStateAwaitingInfo, true},
		{domain.CaseStateWorkInProgress, domain.CaseStateSolutionProposed, true},
		{domain.CaseStateWorkInProgress, domain.CaseStateClosed, true},
		{domain.CaseStateWorkInProgress, domain.CaseStateOpen, false},
		{domain.CaseStateAwaitingInfo, domain.CaseStateWaitingOnWSO2, true},
		{domain.CaseStateAwaitingInfo, domain.CaseStateClosed, false},
		{domain.CaseStateWaitingOnWSO2, domain.CaseStateWorkInProgress, true},
		{domain.CaseStateWaitingOnWSO2, domain.CaseStateClosed, false},
		{domain.CaseStateSolutionProposed, domain.CaseStateClosed, true},
		{domain.CaseStateSolutionProposed, domain.CaseStateWaitingOnWSO2, true},
		{domain.CaseStateSolutionProposed, domain.CaseStateReopened, false},
		{domain.CaseStateReopened, domain.CaseStateWorkInProgress, true},
		{domain.CaseStateClosed, domain.CaseStateWorkInProgress, false},
	}
	for _, tc := range cases {
		if got := isValidCaseStateTransition(true, tc.from, tc.to); got != tc.want {
			t.Errorf("isValidCaseStateTransition(internal, %q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

// TestCaseService_UpdateCase_RejectsIllegalTransitionForExternalCaller is the
// end-to-end regression test: this backend used to validate only that a
// requested state was a real enum value, never that moving to it from the
// case's current state was legal for the caller's own tier -- so a
// customer-facing BFF forwarding a caller's own stateKey unchecked could
// move any case to any state. An external caller moving an open case
// straight to work_in_progress (legal for an agent, never for a customer)
// must now be rejected here, at the one place both BFFs' writes converge.
func TestCaseService_UpdateCase_RejectsIllegalTransitionForExternalCaller(t *testing.T) {
	workInProgress := domain.CaseStateWorkInProgress
	repo := &stubCaseRepo{
		getCaseByID: func(_ context.Context, id string, _ repository.SearchScope) (domain.CaseView, error) {
			open := domain.CaseStateOpen
			return domain.CaseView{ID: id, State: &open}, nil
		},
	}
	svc := NewCaseService(repo, stubUserRepo{}, nil, stubAccess{scope: AccessScope{ProjectIDs: []string{"p1"}}}, nil)

	_, err := svc.UpdateCase(context.Background(), domain.UpdateCaseRequest{ID: testDeploymentUUID, State: &workInProgress})
	var ve *apierror.ValidationError
	if !asValidationError(err, &ve) {
		t.Fatalf("expected a ValidationError, got %T: %v", err, err)
	}
	if ve.Code != "case_invalid_state_transition" {
		t.Errorf("error code = %q, want %q", ve.Code, "case_invalid_state_transition")
	}
}

// TestCaseService_UpdateCase_AllowsLegalTransitionForExternalCaller is the
// positive counterpart: the same open->closed move an external caller is
// explicitly allowed (ALLOWED_STATE_TRANSITIONS) must still succeed.
func TestCaseService_UpdateCase_AllowsLegalTransitionForExternalCaller(t *testing.T) {
	closed := domain.CaseStateClosed
	repo := &stubCaseRepo{
		getCaseByID: func(_ context.Context, id string, _ repository.SearchScope) (domain.CaseView, error) {
			open := domain.CaseStateOpen
			return domain.CaseView{ID: id, State: &open}, nil
		},
		updateCase: func(_ context.Context, req domain.UpdateCaseRequest) (domain.Case, *domain.CaseSeverity, error) {
			return domain.Case{ID: req.ID, State: req.State}, nil, nil
		},
	}
	svc := NewCaseService(repo, stubUserRepo{}, nil, stubAccess{scope: AccessScope{ProjectIDs: []string{"p1"}}}, nil)

	if _, err := svc.UpdateCase(context.Background(), domain.UpdateCaseRequest{ID: testDeploymentUUID, State: &closed}); err != nil {
		t.Fatalf("unexpected error for a legal external transition: %v", err)
	}
}

// TestCaseService_UpdateCase_NoOpStateRePatchIsNeverAnIllegalTransition
// proves a caller re-PATCHing the case's current state is never rejected as
// an illegal transition (it isn't a transition at all) -- even though
// neither table lists a state as its own legal next state.
func TestCaseService_UpdateCase_NoOpStateRePatchIsNeverAnIllegalTransition(t *testing.T) {
	open := domain.CaseStateOpen
	repo := &stubCaseRepo{
		getCaseByID: func(_ context.Context, id string, _ repository.SearchScope) (domain.CaseView, error) {
			st := domain.CaseStateOpen
			return domain.CaseView{ID: id, State: &st}, nil
		},
		updateCase: func(_ context.Context, req domain.UpdateCaseRequest) (domain.Case, *domain.CaseSeverity, error) {
			return domain.Case{ID: req.ID, State: req.State}, nil, nil
		},
	}
	// External tier: open has no entry mapping to itself in either table.
	svc := NewCaseService(repo, stubUserRepo{}, nil, stubAccess{scope: AccessScope{ProjectIDs: []string{"p1"}}}, nil)

	if _, err := svc.UpdateCase(context.Background(), domain.UpdateCaseRequest{ID: testDeploymentUUID, State: &open}); err != nil {
		t.Fatalf("unexpected error for a no-op state re-PATCH: %v", err)
	}
}
