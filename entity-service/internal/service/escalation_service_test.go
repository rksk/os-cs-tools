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
	"errors"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

const escalationTestCaseID = "11111111-1111-1111-1111-111111111111"

// fakeEscalationRepoForService is a minimal repository.EscalationRepository
// fake for escalationService's request-validation tests -- it never touches
// a real database. It records the last CreateEscalation call so a test can
// assert what escalationService normalized/forwarded.
type fakeEscalationRepoForService struct {
	lastAction     domain.EscalationAction
	lastReason     *string
	lastActorEmail string
	createErr      error
	createResp     domain.CreatedEscalation
}

func (f *fakeEscalationRepoForService) SearchEscalations(context.Context, []string, []int, string, string, int, int) ([]domain.Escalation, int, error) {
	panic("not used by these tests")
}

func (f *fakeEscalationRepoForService) CreateEscalation(_ context.Context, _ string, action domain.EscalationAction, reason *string, actorEmail string) (domain.CreatedEscalation, error) {
	f.lastAction = action
	f.lastReason = reason
	f.lastActorEmail = actorEmail
	if f.createErr != nil {
		return domain.CreatedEscalation{}, f.createErr
	}
	return f.createResp, nil
}

// fakeUserRepoForEscalationService resolves exactly one known email.
type fakeUserRepoForEscalationService struct {
	knownEmail string
	user       domain.User
}

func (f *fakeUserRepoForEscalationService) GetUserByEmail(_ context.Context, email string) (domain.User, error) {
	if email == f.knownEmail {
		return f.user, nil
	}
	return domain.User{}, &apierror.NotFoundError{Msg: "no user found with email: " + email}
}
func (f *fakeUserRepoForEscalationService) SearchUsers(context.Context, domain.SearchUsersRequest) ([]domain.User, int, error) {
	panic("not used")
}
func (f *fakeUserRepoForEscalationService) GetUserRoles(context.Context, string) ([]string, error) {
	panic("not used")
}
func (f *fakeUserRepoForEscalationService) GetUserDetail(context.Context, string) (domain.UserDetail, error) {
	panic("not used")
}
func (f *fakeUserRepoForEscalationService) GetUserProjectAccess(context.Context, string) ([]domain.UserContactAccess, error) {
	panic("not used")
}
func (f *fakeUserRepoForEscalationService) GetUserGroups(context.Context, string) ([]domain.UserGroupRef, error) {
	panic("not used")
}
func (f *fakeUserRepoForEscalationService) CreateUser(context.Context, domain.CreateUserRequest, string) (domain.User, error) {
	panic("not used")
}

func newTestEscalationService(repo *fakeEscalationRepoForService) (EscalationService, *fakeUserRepoForEscalationService) {
	userRepo := &fakeUserRepoForEscalationService{
		knownEmail: "engineer@example.com",
		user:       domain.User{ID: "user-engineer", Email: "engineer@example.com"},
	}
	return NewEscalationService(repo, userRepo), userRepo
}

func TestEscalationService_CreateEscalation_InvalidCaseID(t *testing.T) {
	svc, _ := newTestEscalationService(&fakeEscalationRepoForService{})
	_, err := svc.CreateEscalation(contextWithUserIDToken(fakeJWTWithEmail(t, "engineer@example.com")), domain.CreateEscalationRequest{CaseID: "not-a-uuid"})
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("got %v (%T), want *apierror.ValidationError", err, err)
	}
}

func TestEscalationService_CreateEscalation_InvalidAction(t *testing.T) {
	svc, _ := newTestEscalationService(&fakeEscalationRepoForService{})
	badAction := domain.EscalationAction("SIDEWAYS")
	_, err := svc.CreateEscalation(contextWithUserIDToken(fakeJWTWithEmail(t, "engineer@example.com")), domain.CreateEscalationRequest{
		CaseID: escalationTestCaseID,
		Action: &badAction,
	})
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("got %v (%T), want *apierror.ValidationError", err, err)
	}
}

func TestEscalationService_CreateEscalation_EscalateRequiresReason(t *testing.T) {
	svc, _ := newTestEscalationService(&fakeEscalationRepoForService{})
	_, err := svc.CreateEscalation(contextWithUserIDToken(fakeJWTWithEmail(t, "engineer@example.com")), domain.CreateEscalationRequest{CaseID: escalationTestCaseID})
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("got %v (%T), want *apierror.ValidationError (reason required for the defaulted ESCALATE action)", err, err)
	}

	blank := "   "
	_, err = svc.CreateEscalation(contextWithUserIDToken(fakeJWTWithEmail(t, "engineer@example.com")), domain.CreateEscalationRequest{CaseID: escalationTestCaseID, Reason: &blank})
	if !errors.As(err, &valErr) {
		t.Fatalf("blank reason: got %v (%T), want *apierror.ValidationError", err, err)
	}
}

func TestEscalationService_CreateEscalation_DeescalateDoesNotRequireReason(t *testing.T) {
	repo := &fakeEscalationRepoForService{createResp: domain.CreatedEscalation{
		PreviousLevel: domain.ChoiceListItem{Label: "2"},
		CurrentLevel:  domain.ChoiceListItem{Label: "1"},
	}}
	svc, _ := newTestEscalationService(repo)
	deescalate := domain.EscalationActionDeescalate
	_, err := svc.CreateEscalation(contextWithUserIDToken(fakeJWTWithEmail(t, "engineer@example.com")), domain.CreateEscalationRequest{
		CaseID: escalationTestCaseID,
		Action: &deescalate,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastAction != domain.EscalationActionDeescalate {
		t.Errorf("forwarded action = %q, want DEESCALATE", repo.lastAction)
	}
}

func TestEscalationService_CreateEscalation_NoUserIDTokenIsUnauthorized(t *testing.T) {
	svc, _ := newTestEscalationService(&fakeEscalationRepoForService{})
	reason := "customer escalation"
	_, err := svc.CreateEscalation(contextWithUserIDToken(""), domain.CreateEscalationRequest{CaseID: escalationTestCaseID, Reason: &reason})
	var unauthErr *apierror.UnauthorizedError
	if !errors.As(err, &unauthErr) {
		t.Fatalf("got %v (%T), want *apierror.UnauthorizedError", err, err)
	}
}

func TestEscalationService_CreateEscalation_NormalizesLowercaseActionAndForwardsActorEmail(t *testing.T) {
	repo := &fakeEscalationRepoForService{createResp: domain.CreatedEscalation{
		PreviousLevel: domain.ChoiceListItem{Label: "0"},
		CurrentLevel:  domain.ChoiceListItem{Label: "1"},
	}}
	svc, _ := newTestEscalationService(repo)
	lowerEscalate := domain.EscalationAction("escalate")
	reason := "customer requested management involvement"
	resp, err := svc.CreateEscalation(contextWithUserIDToken(fakeJWTWithEmail(t, "engineer@example.com")), domain.CreateEscalationRequest{
		CaseID: escalationTestCaseID,
		Action: &lowerEscalate,
		Reason: &reason,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastAction != domain.EscalationActionEscalate {
		t.Errorf("forwarded action = %q, want the upper-cased ESCALATE", repo.lastAction)
	}
	if repo.lastActorEmail != "engineer@example.com" {
		t.Errorf("forwarded actor email = %q, want engineer@example.com", repo.lastActorEmail)
	}
	if resp.Message == "" {
		t.Error("expected a non-empty message")
	}
}
