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

package recipientlinks

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/entity"
)

type fakeEntityClient struct {
	users []entity.UserRoleInfo
	err   error
}

func (f *fakeEntityClient) SearchUsersByEmail(ctx context.Context, emails []string) ([]entity.UserRoleInfo, error) {
	return f.users, f.err
}

func testConfig() Config {
	return Config{
		CustomerRoles:   []string{"customer_admin", "customer_viewer"},
		CSMRoles:        []string{"csm_agent", "team_lead"},
		CustomerBaseURL: "https://customer.example.com",
		CSMBaseURL:      "https://csm.example.com",
	}
}

func TestResolveLinks_CustomerRole(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "customer@acme.com", Roles: []string{"customer_admin"}, UserType: "customer"},
	}}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"customer@acme.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	want := "https://customer.example.com/projects/PROJ-1/support/cases/CASE-1"
	if len(links) != 1 || links[0].CaseLink != want {
		t.Errorf("links = %+v, want a single link %q", links, want)
	}
}

func TestResolveLinks_CSMRole(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "agent@wso2.com", Roles: []string{"csm_agent"}, UserType: "internal"},
	}}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"agent@wso2.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	want := "https://csm.example.com/cases/CASE-1"
	if len(links) != 1 || links[0].CaseLink != want {
		t.Errorf("links = %+v, want a single link %q", links, want)
	}
}

func TestResolveLinks_MixedRolesOneEvent(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "customer@acme.com", Roles: []string{"customer_admin"}, UserType: "customer"},
		{Email: "agent@wso2.com", Roles: []string{"csm_agent"}, UserType: "internal"},
	}}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"customer@acme.com", "agent@wso2.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	byEmail := map[string]string{links[0].Email: links[0].CaseLink, links[1].Email: links[1].CaseLink}
	if byEmail["customer@acme.com"] != "https://customer.example.com/projects/PROJ-1/support/cases/CASE-1" {
		t.Errorf("customer link = %q", byEmail["customer@acme.com"])
	}
	if byEmail["agent@wso2.com"] != "https://csm.example.com/cases/CASE-1" {
		t.Errorf("csm link = %q", byEmail["agent@wso2.com"])
	}
}

// TestResolveLinks_UnmatchedRole_Wso2Domain_ResolvesToCSM verifies the
// email-domain fallback: a recipient whose roles match neither list, but
// whose email is on WSO2's own domain, is routed to the CSM portal.
func TestResolveLinks_UnmatchedRole_Wso2Domain_ResolvesToCSM(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "mystery@wso2.com", Roles: []string{"some_new_role"}},
	}}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"mystery@wso2.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	want := "https://csm.example.com/cases/CASE-1"
	if len(links) != 1 || links[0].CaseLink != want {
		t.Errorf("links = %+v, want domain fallback to CSM link %q", links, want)
	}
}

// TestResolveLinks_UnmatchedRole_ExternalDomain_ResolvesToCustomer mirrors
// the case above for a non-WSO2 domain.
func TestResolveLinks_UnmatchedRole_ExternalDomain_ResolvesToCustomer(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "mystery@acme.com", Roles: []string{"some_new_role"}},
	}}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"mystery@acme.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	want := "https://customer.example.com/projects/PROJ-1/support/cases/CASE-1"
	if len(links) != 1 || links[0].CaseLink != want {
		t.Errorf("links = %+v, want domain fallback to customer link %q", links, want)
	}
}

// TestResolveLinks_RecipientNotFound_Wso2Domain_ResolvesToCSM verifies the
// domain fallback also applies when entity-service has no record for the
// recipient at all, not just when roles are merely unmatched.
func TestResolveLinks_RecipientNotFound_Wso2Domain_ResolvesToCSM(t *testing.T) {
	client := &fakeEntityClient{users: nil}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"ghost@wso2.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	want := "https://csm.example.com/cases/CASE-1"
	if len(links) != 1 || links[0].CaseLink != want {
		t.Errorf("links = %+v, want domain fallback to CSM link %q", links, want)
	}
}

// TestResolveLinks_RecipientNotFound_ExternalDomain_ResolvesToCustomer
// mirrors the case above for a not-found recipient on a non-WSO2 domain.
func TestResolveLinks_RecipientNotFound_ExternalDomain_ResolvesToCustomer(t *testing.T) {
	client := &fakeEntityClient{users: nil}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"ghost@example.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	want := "https://customer.example.com/projects/PROJ-1/support/cases/CASE-1"
	if len(links) != 1 || links[0].CaseLink != want {
		t.Errorf("links = %+v, want domain fallback to customer link %q", links, want)
	}
}

// TestResolveLinks_MatchedRole_TakesPriorityOverDomain verifies role
// classification still wins over the domain fallback when both are
// available — domain is only consulted once role classification fails.
func TestResolveLinks_MatchedRole_TakesPriorityOverDomain(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "agent@acme.com", Roles: []string{"csm_agent"}, UserType: "internal"},
	}}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), []string{"agent@acme.com"}, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	want := "https://csm.example.com/cases/CASE-1"
	if len(links) != 1 || links[0].CaseLink != want {
		t.Errorf("links = %+v, want role match (CSM) even though email domain isn't internal, got %q", links, want)
	}
}

func TestResolveLinks_EmptyEmails_ReturnsNilWithoutCallingEntity(t *testing.T) {
	client := &fakeEntityClient{}
	r := New(client, testConfig())

	links, err := r.ResolveLinks(t.Context(), nil, "PROJ-1", "CASE-1")
	if err != nil {
		t.Fatalf("ResolveLinks() error = %v", err)
	}
	if links != nil {
		t.Errorf("links = %+v, want nil", links)
	}
}

func TestResolveLinks_SearchFails_ReturnsError(t *testing.T) {
	client := &fakeEntityClient{err: context.DeadlineExceeded}
	r := New(client, testConfig())

	if _, err := r.ResolveLinks(t.Context(), []string{"a@b.com"}, "PROJ-1", "CASE-1"); err == nil {
		t.Fatal("expected an error when the entity-service search fails")
	}
}

func TestIsCustomer_CustomerRole(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "customer@acme.com", Roles: []string{"customer_admin"}},
	}}
	r := New(client, testConfig())

	got, err := r.IsCustomer(t.Context(), "customer@acme.com")
	if err != nil {
		t.Fatalf("IsCustomer() error = %v", err)
	}
	if !got {
		t.Error("got false, want true for a CustomerRoles match")
	}
}

func TestIsCustomer_CSMRole(t *testing.T) {
	client := &fakeEntityClient{users: []entity.UserRoleInfo{
		{Email: "agent@wso2.com", Roles: []string{"csm_agent"}},
	}}
	r := New(client, testConfig())

	got, err := r.IsCustomer(t.Context(), "agent@wso2.com")
	if err != nil {
		t.Fatalf("IsCustomer() error = %v", err)
	}
	if got {
		t.Error("got true, want false for a CSMRoles match")
	}
}

func TestIsCustomer_NoRoleMatch_FallsBackToEmailDomain(t *testing.T) {
	testCases := []struct {
		name  string
		email string
		want  bool
	}{
		{"wso2.com falls back to internal", "unroled@wso2.com", false},
		{"non-wso2.com falls back to external", "unroled@acme.com", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeEntityClient{} // no matching user found
			r := New(client, testConfig())

			got, err := r.IsCustomer(t.Context(), tc.email)
			if err != nil {
				t.Fatalf("IsCustomer() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("IsCustomer(%q) = %v, want %v", tc.email, got, tc.want)
			}
		})
	}
}

func TestIsCustomer_SearchFails_ReturnsError(t *testing.T) {
	client := &fakeEntityClient{err: context.DeadlineExceeded}
	r := New(client, testConfig())

	if _, err := r.IsCustomer(t.Context(), "a@b.com"); err == nil {
		t.Fatal("expected an error when the entity-service search fails")
	}
}

// TestServiceRequestLink: an SR opens on the portal's service-request page,
// not /cases/<id>.
func TestServiceRequestLink(t *testing.T) {
	r := New(nil, testConfig())
	want := "https://csm.example.com/operations/service-requests/sr-1"
	if got := r.ServiceRequestLink("sr-1"); got != want {
		t.Errorf("ServiceRequestLink = %q, want %q", got, want)
	}
}

// captureHandler records every slog record at any level so a test can assert
// on level and attributes.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func recordAttrs(r slog.Record) map[string]slog.Value {
	m := map[string]slog.Value{}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value
		return true
	})
	return m
}

// TestClassification drives ResolveLinks and IsCustomer from the same table,
// so the two entry points are proven to agree. Precedence: user type, then
// roles, then email domain. Only the domain fallback logs, at warn, with the
// raw userType and never the email address.
func TestClassification(t *testing.T) {
	const (
		csmLink      = "https://csm.example.com/cases/CASE-1"
		customerLink = "https://customer.example.com/projects/PROJ-1/support/cases/CASE-1"
		email        = "mystery@wso2.com"
	)
	tests := []struct {
		name         string
		users        []entity.UserRoleInfo
		wantCustomer bool
		wantWarn     bool
		wantUserType string
		wantRoles    int64
	}{
		{name: "internal userType, no roles", users: []entity.UserRoleInfo{{Email: email, UserType: "internal"}}},
		{name: "customer userType", users: []entity.UserRoleInfo{{Email: email, UserType: "customer"}}, wantCustomer: true},
		{name: "external userType", users: []entity.UserRoleInfo{{Email: email, UserType: "external"}}, wantCustomer: true},
		{name: "mixed-case INTERNAL", users: []entity.UserRoleInfo{{Email: email, UserType: "INTERNAL"}}},
		{
			// userType wins over roles: an internal user holding a customer
			// role entry is still CSM.
			name:  "internal userType beats customer role",
			users: []entity.UserRoleInfo{{Email: email, UserType: "internal", Roles: []string{"customer_admin"}}},
		},
		{name: "empty userType, CSM role", users: []entity.UserRoleInfo{{Email: email, Roles: []string{"csm_agent"}}}},
		{
			name:         "empty userType, customer role",
			users:        []entity.UserRoleInfo{{Email: email, Roles: []string{"customer_admin"}}},
			wantCustomer: true,
		},
		{
			name:      "empty userType, unmatched roles, wso2 email: fallback",
			users:     []entity.UserRoleInfo{{Email: email, Roles: []string{"a", "b"}}},
			wantWarn:  true,
			wantRoles: 2,
		},
		{name: "not found: fallback", users: nil, wantWarn: true},
		{
			name:         "system userType, unmatched roles: fallback",
			users:        []entity.UserRoleInfo{{Email: email, UserType: "system", Roles: []string{"a"}}},
			wantWarn:     true,
			wantUserType: "system",
			wantRoles:    1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &captureHandler{}
			prev := slog.Default()
			slog.SetDefault(slog.New(h))
			t.Cleanup(func() { slog.SetDefault(prev) })

			r := New(&fakeEntityClient{users: tt.users}, testConfig())
			links, err := r.ResolveLinks(t.Context(), []string{email}, "PROJ-1", "CASE-1")
			if err != nil {
				t.Fatalf("ResolveLinks() error = %v", err)
			}
			want := csmLink
			if tt.wantCustomer {
				want = customerLink
			}
			if len(links) != 1 || links[0].CaseLink != want {
				t.Errorf("links = %+v, want a single link %q", links, want)
			}
			// userType-resolved and role-resolved cases are all wso2.com
			// addresses, so a customer result here proves the domain did not decide.

			if !tt.wantWarn {
				if len(h.records) != 0 {
					t.Fatalf("got %d log records, want none", len(h.records))
				}
			} else {
				if len(h.records) != 1 {
					t.Fatalf("got %d log records, want 1", len(h.records))
				}
				rec := h.records[0]
				if rec.Level != slog.LevelWarn {
					t.Errorf("level = %v, want warn", rec.Level)
				}
				attrs := recordAttrs(rec)
				if got := attrs["found"]; got.Kind() != slog.KindBool || got.Bool() != (len(tt.users) > 0) {
					t.Errorf("found attr = %v", got)
				}
				if got, ok := attrs["roleCount"]; !ok || got.Int64() != tt.wantRoles {
					t.Errorf("roleCount attr = %v (present=%v), want %d", got, ok, tt.wantRoles)
				}
				if got, ok := attrs["userType"]; !ok || got.String() != tt.wantUserType {
					t.Errorf("userType attr = %v (present=%v), want %q", got, ok, tt.wantUserType)
				}
				if got := attrs["caseID"].String(); got != "CASE-1" {
					t.Errorf("caseID attr = %q, want CASE-1", got)
				}
				if got, ok := attrs["isCustomer"]; !ok || got.Bool() != tt.wantCustomer {
					t.Errorf("isCustomer attr = %v (present=%v), want %v", got, ok, tt.wantCustomer)
				}
				for k, v := range attrs {
					if strings.Contains(v.String(), "mystery@") {
						t.Errorf("attr %q leaks the email address", k)
					}
				}
				if strings.Contains(rec.Message, "mystery@") {
					t.Error("message leaks the email address")
				}
			}

			// IsCustomer must agree with the link, and never log the email.
			h.records = nil
			got, err := r.IsCustomer(t.Context(), email)
			if err != nil {
				t.Fatalf("IsCustomer() error = %v", err)
			}
			if got != tt.wantCustomer {
				t.Errorf("IsCustomer = %v, want %v", got, tt.wantCustomer)
			}
			for _, rec := range h.records {
				if strings.Contains(rec.Message, "mystery@") {
					t.Error("IsCustomer log leaks the email address")
				}
			}
		})
	}
}

// TestClassification_DomainFallbackCustomer: a non-wso2 address the user type
// and roles cannot resolve falls back to the customer portal.
func TestClassification_DomainFallbackCustomer(t *testing.T) {
	r := New(&fakeEntityClient{users: []entity.UserRoleInfo{{Email: "x@acme.com", UserType: "system"}}}, testConfig())
	got, err := r.IsCustomer(t.Context(), "x@acme.com")
	if err != nil || !got {
		t.Errorf("IsCustomer = %v, %v; want true, nil", got, err)
	}
}
