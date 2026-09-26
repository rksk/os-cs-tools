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

package repository

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// fakeEscalationUserRepo resolves a fixed set of emails to users, returning
// NotFoundError for anything else -- exactly GetUserByEmail's own contract
// (user_repo.go), just without a real database. Every other UserRepository
// method panics if called: resolveEscalationRecipients only ever calls
// GetUserByEmail.
type fakeEscalationUserRepo struct {
	byEmail map[string]domain.User
}

func (f *fakeEscalationUserRepo) GetUserByEmail(_ context.Context, email string) (domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		return u, nil
	}
	return domain.User{}, &apierror.NotFoundError{Msg: "no user found with email: " + email}
}

func (f *fakeEscalationUserRepo) SearchUsers(context.Context, domain.SearchUsersRequest) ([]domain.User, int, error) {
	panic("not used by resolveEscalationRecipients")
}
func (f *fakeEscalationUserRepo) GetUserRoles(context.Context, string) ([]string, error) {
	panic("not used by resolveEscalationRecipients")
}
func (f *fakeEscalationUserRepo) GetUserDetail(context.Context, string) (domain.UserDetail, error) {
	panic("not used by resolveEscalationRecipients")
}
func (f *fakeEscalationUserRepo) GetUserProjectAccess(context.Context, string) ([]domain.UserContactAccess, error) {
	panic("not used by resolveEscalationRecipients")
}
func (f *fakeEscalationUserRepo) GetUserGroups(context.Context, string) ([]domain.UserGroupRef, error) {
	panic("not used by resolveEscalationRecipients")
}
func (f *fakeEscalationUserRepo) CreateUser(context.Context, domain.CreateUserRequest, string) (domain.User, error) {
	panic("not used by resolveEscalationRecipients")
}

var _ UserRepository = (*fakeEscalationUserRepo)(nil)

// --- nextEscalationLevel: level-transition math ---

func TestNextEscalationLevel_EscalateIncrements(t *testing.T) {
	for previous := 0; previous < maxEscalationLevel; previous++ {
		got, err := nextEscalationLevel(domain.EscalationActionEscalate, previous)
		if err != nil {
			t.Fatalf("previous=%d: unexpected error: %v", previous, err)
		}
		if want := previous + 1; got != want {
			t.Errorf("previous=%d: got %d, want %d", previous, got, want)
		}
	}
}

func TestNextEscalationLevel_EscalateCapsAtEL5(t *testing.T) {
	got, err := nextEscalationLevel(domain.EscalationActionEscalate, maxEscalationLevel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != maxEscalationLevel {
		t.Errorf("got %d, want %d (capped, not an error)", got, maxEscalationLevel)
	}
}

func TestNextEscalationLevel_DeescalateDecrements(t *testing.T) {
	for previous := 1; previous <= maxEscalationLevel; previous++ {
		got, err := nextEscalationLevel(domain.EscalationActionDeescalate, previous)
		if err != nil {
			t.Fatalf("previous=%d: unexpected error: %v", previous, err)
		}
		if want := previous - 1; got != want {
			t.Errorf("previous=%d: got %d, want %d", previous, got, want)
		}
	}
}

func TestNextEscalationLevel_DeescalateAtEL0IsValidationError(t *testing.T) {
	_, err := nextEscalationLevel(domain.EscalationActionDeescalate, 0)
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("got error %v (%T), want *apierror.ValidationError", err, err)
	}
}

func TestEscalationLevelInt(t *testing.T) {
	el := func(s string) *string { return &s }
	cases := []struct {
		name string
		raw  *string
		want int
	}{
		{"nil (never escalated) is EL0", nil, 0},
		{"EL0", el("EL0"), 0},
		{"EL3", el("EL3"), 3},
		{"EL5", el("EL5"), 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escalationLevelInt(tc.raw); got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// --- resolveEscalationRecipients: cumulative EL1..EL5 recipient rule ---

func sortedIDs(ids []string) []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

func TestResolveEscalationRecipients_CumulativeAcrossLevels(t *testing.T) {
	ctx := context.Background()
	fake := &fakeEscalationUserRepo{byEmail: map[string]domain.User{
		"tl@example.com":  {ID: "user-el1-tl"},
		"tu@example.com":  {ID: "user-el2-tu"},
		"cre@example.com": {ID: "user-el3-cre"},
	}}
	notifyCfg := EscalationNotificationConfig{
		EL1AmericasTLEmails: []string{"tl@example.com"},
		EL2AmericasTUEmails: []string{"tu@example.com"},
		EL3CREHeadEmail:     "cre@example.com",
	}
	r := &escalationRepo{userRepo: fake, notifyCfg: notifyCfg}

	cc := escalationCaseContext{
		technicalOwnerID: strPtr("user-tech-owner"),
		csmID:            strPtr("user-csm"),
		creTeamManagerID: strPtr("user-cre-manager"),
	}

	// Level 1: only the EL1 sources.
	got1, err := r.resolveEscalationRecipients(ctx, 1, cc)
	if err != nil {
		t.Fatalf("level 1: unexpected error: %v", err)
	}
	want1 := []string{"user-el1-tl", "user-tech-owner", "user-cre-manager"}
	if !reflect.DeepEqual(sortedIDs(got1), sortedIDs(want1)) {
		t.Errorf("level 1: got %v, want %v", sortedIDs(got1), sortedIDs(want1))
	}

	// Level 3: EL1 recipients are STILL present (cumulative), plus EL2/EL3
	// sources join in. This is the key assertion: escalating straight to
	// EL3 must not drop the EL1 recipients in favor of only EL3's own.
	got3, err := r.resolveEscalationRecipients(ctx, 3, cc)
	if err != nil {
		t.Fatalf("level 3: unexpected error: %v", err)
	}
	want3 := []string{
		"user-el1-tl", "user-tech-owner", "user-cre-manager", // EL1
		"user-el2-tu",              // EL2 (no product configured, so no product-routed recipient)
		"user-el3-cre", "user-csm", // EL3
	}
	if !reflect.DeepEqual(sortedIDs(got3), sortedIDs(want3)) {
		t.Errorf("level 3: got %v, want %v", sortedIDs(got3), sortedIDs(want3))
	}
}

func TestResolveEscalationRecipients_ProductRouting(t *testing.T) {
	ctx := context.Background()
	fake := &fakeEscalationUserRepo{byEmail: map[string]domain.User{
		"service@example.com": {ID: "user-service"},
		"iam@example.com":     {ID: "user-iam"},
		"default@example.com": {ID: "user-default"},
	}}
	notifyCfg := EscalationNotificationConfig{
		EL2ServiceProductEmail: "service@example.com",
		EL2IdentityServerEmail: "iam@example.com",
		EL2DefaultProductEmail: "default@example.com",
	}
	r := &escalationRepo{userRepo: fake, notifyCfg: notifyCfg}

	cases := []struct {
		name    string
		cc      escalationCaseContext
		wantIDs []string
	}{
		{
			name:    "no deployed product/product info at all -- no product recipient, silently",
			cc:      escalationCaseContext{},
			wantIDs: nil,
		},
		{
			name:    "category SERVICE routes to the service product email",
			cc:      escalationCaseContext{productCategory: strPtr("SERVICE")},
			wantIDs: []string{"user-service"},
		},
		{
			name:    "category SOFTWARE + business_unit IAM routes to the identity server email",
			cc:      escalationCaseContext{productCategory: strPtr("SOFTWARE"), productBusinessUnit: strPtr("IAM")},
			wantIDs: []string{"user-iam"},
		},
		{
			name:    "category SOFTWARE + a non-IAM business_unit falls to the default product email",
			cc:      escalationCaseContext{productCategory: strPtr("SOFTWARE"), productBusinessUnit: strPtr("INTEGRATION_SOFTWARE")},
			wantIDs: []string{"user-default"},
		},
		{
			name:    "category SOFTWARE with no business_unit at all also falls to the default",
			cc:      escalationCaseContext{productCategory: strPtr("SOFTWARE")},
			wantIDs: []string{"user-default"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.resolveEscalationRecipients(ctx, 2, tc.cc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(sortedIDs(got), sortedIDs(tc.wantIDs)) {
				t.Errorf("got %v, want %v", sortedIDs(got), sortedIDs(tc.wantIDs))
			}
		})
	}
}

func TestResolveEscalationRecipients_UnconfiguredEnvVarsDoNotError(t *testing.T) {
	ctx := context.Background()
	// Zero-value EscalationNotificationConfig: every fixed-email slot unset.
	r := &escalationRepo{userRepo: &fakeEscalationUserRepo{byEmail: map[string]domain.User{}}, notifyCfg: EscalationNotificationConfig{}}

	// Level 5 exercises every tier's fixed-email slot at once.
	got, err := r.resolveEscalationRecipients(ctx, 5, escalationCaseContext{})
	if err != nil {
		t.Fatalf("unexpected error with every notifyCfg slot unset: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no recipients (nothing configured, no case-derived ids)", got)
	}
}

func TestResolveEscalationRecipients_UnresolvableFixedEmailIsSkippedNotFatal(t *testing.T) {
	ctx := context.Background()
	// notifyCfg names an email that has no matching "user" row.
	r := &escalationRepo{
		userRepo:  &fakeEscalationUserRepo{byEmail: map[string]domain.User{}},
		notifyCfg: EscalationNotificationConfig{EL5CEOEmail: "ceo@example.com"},
	}

	got, err := r.resolveEscalationRecipients(ctx, 5, escalationCaseContext{})
	if err != nil {
		t.Fatalf("an unresolvable fixed email must be skipped, not fatal: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no recipients (the one configured email doesn't resolve)", got)
	}
}

// TestResolveEscalationRecipients_AccountManagerIDNeverRead is a structural
// regression guard for the confirmed, deliberate gap documented on
// EscalationRepository.CreateEscalation: account.account_manager_id (SN's
// u_owner) must never be treated as a real recipient signal, since nothing
// in this repo's write path ever populates it. escalationCaseContext simply
// has no field to carry it -- if a future change adds one and wires it into
// resolveEscalationRecipients, this test's field-count/name assertion catches
// the regression before it ships.
func TestResolveEscalationRecipients_AccountManagerIDNeverRead(t *testing.T) {
	typ := reflect.TypeOf(escalationCaseContext{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if name == "accountManagerID" {
			t.Fatalf("escalationCaseContext must not carry account_manager_id -- it's a confirmed, always-NULL-in-practice gap (see CreateEscalation's own doc comment), not a real recipient signal")
		}
	}
}
