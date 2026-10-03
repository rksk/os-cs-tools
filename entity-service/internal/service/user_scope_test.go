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
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// internalCallerCtx is a request context for an internal user: a validated
// identity carrying email, with the Unrestricted scope the request
// middleware would have resolved and attached.
func internalCallerCtx(t *testing.T, email string) context.Context {
	t.Helper()
	return repository.WithCallerIdentity(contextWithUserIDToken(fakeJWTWithEmail(t, email)), AccessScope{Unrestricted: true})
}

// customerCallerCtx is internalCallerCtx for a registered customer contact.
func customerCallerCtx(t *testing.T, email string) context.Context {
	t.Helper()
	return repository.WithCallerIdentity(contextWithUserIDToken(fakeJWTWithEmail(t, email)), AccessScope{ProjectIDs: []string{"proj-1"}, ViewerEmail: email})
}

func TestUserService_CreateUser_RequiresInternalCaller(t *testing.T) {
	req := domain.CreateUserRequest{FirstName: "Jane", Email: "jane.doe@example.com"}
	_, err := NewUserService(stubUserRepo{}).CreateUser(customerCallerCtx(t, "customer@example.com"), req)
	requireForbidden(t, "customer creates user", err)

	_, err = NewUserService(stubUserRepo{}).CreateUser(contextWithUserIDToken(""), req)
	var ue *apierror.UnauthorizedError
	if !errors.As(err, &ue) {
		t.Fatalf("unidentified caller: got %T (%v), want *apierror.UnauthorizedError", err, err)
	}
}

func TestUserService_GetUser_CustomerSeesOnlyInternalUsers(t *testing.T) {
	var projectAccessRead bool
	repoFor := func(userType domain.UserType) stubUserRepo {
		return stubUserRepo{
			getUserDetail: func(context.Context, string) (domain.UserDetail, error) {
				return domain.UserDetail{ID: userDetailTestID, UserType: userType, Email: "someone@example.com"}, nil
			},
			getUserRoles:  func(context.Context, string) ([]string, error) { return nil, nil },
			getUserGroups: func(context.Context, string) ([]domain.UserGroupRef, error) { return nil, nil },
			getUserProjectAccess: func(context.Context, string) ([]domain.UserContactAccess, error) {
				projectAccessRead = true
				return nil, nil
			},
		}
	}
	ctx := customerCallerCtx(t, "customer@example.com")

	_, err := NewUserService(repoFor(domain.UserTypeCustomer)).GetUser(ctx, userDetailTestID)
	var nfe *apierror.NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("customer reads customer: got %T (%v), want *apierror.NotFoundError", err, err)
	}
	if projectAccessRead {
		t.Fatal("project access must not be read for a customer caller")
	}
	if _, err := NewUserService(repoFor(domain.UserTypeInternal)).GetUser(ctx, userDetailTestID); err != nil {
		t.Fatalf("customer reads internal user: unexpected error: %v", err)
	}
	if _, err := NewUserService(repoFor(domain.UserTypeCustomer)).GetUser(contextWithUserIDToken(""), userDetailTestID); err == nil {
		t.Fatal("unidentified caller: want an error")
	}
}

func TestUserService_SearchUsers_CustomerGetsInternalUsersOnly(t *testing.T) {
	repo := stubUserRepo{searchUsers: func(context.Context, domain.SearchUsersRequest) ([]domain.User, int, error) {
		return []domain.User{
			{ID: "a", UserType: domain.UserTypeInternal},
			{ID: "b", UserType: domain.UserTypeCustomer},
			{ID: "c", UserType: domain.UserTypeInternal},
		}, 3, nil
	}}
	resp, err := NewUserService(repo).SearchUsers(customerCallerCtx(t, "customer@example.com"), domain.SearchUsersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Users) != 2 || resp.Users[0].ID != "a" || resp.Users[1].ID != "c" {
		t.Fatalf("customer page = %+v, want the two internal users", resp.Users)
	}

	resp, err = NewUserService(repo).SearchUsers(internalCallerCtx(t, "eng@example.com"), domain.SearchUsersRequest{})
	if err != nil || len(resp.Users) != 3 {
		t.Fatalf("internal caller: got %d users, err %v; want 3, nil", len(resp.Users), err)
	}
	if _, err := NewUserService(repo).SearchUsers(contextWithUserIDToken(""), domain.SearchUsersRequest{}); err == nil {
		t.Fatal("unidentified caller: want an error")
	}
}
