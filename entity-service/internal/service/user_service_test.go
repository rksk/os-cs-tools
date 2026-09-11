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

package service

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// TestUserService_GetMe_ResolvesCallerFromToken proves GetMe decodes the
// caller's email from the x-user-id-token JWT and returns the matching
// Postgres user row, with empty (not fabricated) roles/groups since the
// Postgres data source has no such tables.
func TestUserService_GetMe_ResolvesCallerFromToken(t *testing.T) {
	timezone := "Asia/Colombo"
	repo := stubUserRepo{
		getUserByEmail: func(_ context.Context, email string) (domain.User, error) {
			if email != "jane.doe@example.com" {
				t.Fatalf("GetUserByEmail called with unexpected email: %q", email)
			}
			return domain.User{
				ID:        "11111111-1111-1111-1111-111111111111",
				FirstName: "Jane",
				LastName:  "Doe",
				Email:     email,
				Timezone:  &timezone,
			}, nil
		},
	}
	svc := NewUserService(repo)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))

	resp, err := svc.GetMe(ctx)
	if err != nil {
		t.Fatalf("GetMe returned error: %v", err)
	}
	if resp.ID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("ID = %q, want the user's row id", resp.ID)
	}
	if resp.Email != "jane.doe@example.com" {
		t.Errorf("Email = %q, want jane.doe@example.com", resp.Email)
	}
	if resp.FirstName == nil || *resp.FirstName != "Jane" {
		t.Errorf("FirstName = %v, want Jane", resp.FirstName)
	}
	if resp.LastName != "Doe" {
		t.Errorf("LastName = %q, want Doe", resp.LastName)
	}
	if resp.TimeZone == nil || *resp.TimeZone != timezone {
		t.Errorf("TimeZone = %v, want %q", resp.TimeZone, timezone)
	}
	if len(resp.Roles) != 0 {
		t.Errorf("Roles = %v, want empty (Postgres has no roles table)", resp.Roles)
	}
	if len(resp.Groups) != 0 {
		t.Errorf("Groups = %v, want empty (Postgres has no group tables)", resp.Groups)
	}
}

// TestUserService_GetMe_RequiresToken proves a missing x-user-id-token header
// fails as Unauthorized rather than falling through to some other identity.
func TestUserService_GetMe_RequiresToken(t *testing.T) {
	svc := NewUserService(stubUserRepo{})
	ctx := contextWithUserIDToken("")

	_, err := svc.GetMe(ctx)
	if _, ok := err.(*apierror.UnauthorizedError); !ok {
		t.Fatalf("GetMe error = %v (%T), want *apierror.UnauthorizedError", err, err)
	}
}

// TestUserService_GetMe_RejectsMalformedToken proves an undecodable
// x-user-id-token surfaces as a ValidationError, not an opaque failure.
func TestUserService_GetMe_RejectsMalformedToken(t *testing.T) {
	svc := NewUserService(stubUserRepo{})
	ctx := contextWithUserIDToken("not-a-jwt")

	_, err := svc.GetMe(ctx)
	if _, ok := err.(*apierror.ValidationError); !ok {
		t.Fatalf("GetMe error = %v (%T), want *apierror.ValidationError", err, err)
	}
}

// TestUserService_GetMe_PropagatesRepoNotFound proves a token whose email has
// no matching Postgres user surfaces the repository's NotFoundError verbatim.
func TestUserService_GetMe_PropagatesRepoNotFound(t *testing.T) {
	repo := stubUserRepo{
		getUserByEmail: func(context.Context, string) (domain.User, error) {
			return domain.User{}, &apierror.NotFoundError{Msg: "no user found with email: ghost@example.com"}
		},
	}
	svc := NewUserService(repo)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, "ghost@example.com"))

	_, err := svc.GetMe(ctx)
	if _, ok := err.(*apierror.NotFoundError); !ok {
		t.Fatalf("GetMe error = %v (%T), want *apierror.NotFoundError", err, err)
	}
}
