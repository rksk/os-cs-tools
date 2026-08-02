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
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// withUserRoles installs a parsed role allow-list for one test and restores the
// defaults afterwards, so no test inherits another's list.
func withUserRoles(t *testing.T, raw string) {
	t.Helper()
	roles, err := ParseUserRoles(raw)
	if err != nil {
		t.Fatalf("ParseUserRoles(%q): %v", raw, err)
	}
	SetUserRoles(roles)
	t.Cleanup(func() { SetUserRoles(defaultUserRoles) })
}

// TestParseUserRoles_UnsetFallsBackToDefaults: a zero-config deployment must
// behave exactly as the previously hardcoded list did.
func TestParseUserRoles_UnsetFallsBackToDefaults(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		roles, err := ParseUserRoles(raw)
		if err != nil {
			t.Fatalf("ParseUserRoles(%q) returned error: %v", raw, err)
		}
		if len(roles) != 10 {
			t.Fatalf("ParseUserRoles(%q) returned %d roles, want the 10 defaults: %v", raw, len(roles), roles)
		}
		want := map[domain.UserRole]bool{
			domain.UserRoleAgent: true, domain.UserRoleAdmin: true, domain.UserRoleCommenter: true,
			domain.UserRoleCustomer: true, domain.UserRoleCustomerAdmin: true, domain.UserRolePartner: true,
			domain.UserRolePartnerAdmin: true, domain.UserRoleInternal: true, domain.UserRoleExternal: true,
			domain.UserRoleTimecardApprover: true,
		}
		for _, r := range roles {
			if !want[r] {
				t.Fatalf("unexpected default role %q", r)
			}
			delete(want, r)
		}
		if len(want) != 0 {
			t.Fatalf("defaults are missing %v", want)
		}
	}
}

func TestParseUserRoles_OverridesAndTrims(t *testing.T) {
	roles, err := ParseUserRoles("  agent , timecard_approver ,")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 || roles[0] != domain.UserRoleAgent || roles[1] != domain.UserRoleTimecardApprover {
		t.Fatalf("roles = %v, want [agent timecard_approver]", roles)
	}
}

func TestParseUserRoles_RejectsDuplicates(t *testing.T) {
	_, err := ParseUserRoles("agent,admin,agent")
	if err == nil {
		t.Fatal("expected an error for a duplicated role name")
	}
	if !strings.Contains(err.Error(), "agent") {
		t.Fatalf("error %q does not name the duplicated value", err)
	}
}

// TestSetUserRoles_DrivesBothFilterAndCatalogue is the property the whole
// change exists to preserve: one configured list feeds the roleIds validation
// and POST /roles/search, so the dropdown and the filter cannot disagree.
func TestSetUserRoles_DrivesBothFilterAndCatalogue(t *testing.T) {
	withUserRoles(t, "agent,timecard_approver")

	// Catalogue side.
	resp, err := NewRoleService().SearchRoles(context.Background(), domain.SearchRolesRequest{})
	if err != nil {
		t.Fatalf("SearchRoles: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("SearchRoles returned %d roles, want 2: %+v", resp.Total, resp.Roles)
	}
	if resp.Roles[0].ID != "agent" || resp.Roles[0].Name != "Agent" {
		t.Fatalf("roles[0] = %+v, want {agent Agent}", resp.Roles[0])
	}
	if resp.Roles[1].ID != "timecard_approver" || resp.Roles[1].Name != "Timecard Approver" {
		t.Fatalf("roles[1] = %+v, want {timecard_approver Timecard Approver}", resp.Roles[1])
	}

	// Filter side: a configured role is accepted...
	if !isValidUserRole(domain.UserRoleAgent) {
		t.Fatal("configured role \"agent\" was rejected by the roleIds validation")
	}
	// ...and a role outside the configured list is rejected, even though it is
	// one of the committed defaults.
	if isValidUserRole(domain.UserRoleAdmin) {
		t.Fatal("role \"admin\" is not in the configured list but passed validation")
	}
}

// TestSearchTeams_ReflectsConfiguredRegistry: the team catalogue is exactly
// what was configured, nothing hardcoded.
func TestSearchTeams_ReflectsConfiguredRegistry(t *testing.T) {
	withTeamRegistry(t, "alpha|Alpha Team|CRE,beta|Beta Team|SRE,gamma|Gamma Team")

	resp, err := NewTeamService().SearchTeams(context.Background(), domain.SearchTeamsRequest{})
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("SearchTeams returned %d teams, want 3: %+v", resp.Total, resp.Teams)
	}
	want := []domain.Team{
		{ID: "alpha", Name: "Alpha Team", Family: "cre"},
		{ID: "beta", Name: "Beta Team", Family: "sre"},
		{ID: "gamma", Name: "Gamma Team", Family: ""},
	}
	for i, w := range want {
		if resp.Teams[i] != w {
			t.Fatalf("teams[%d] = %+v, want %+v", i, resp.Teams[i], w)
		}
	}
}

// TestSearchTeams_EmptyRegistry: no configuration means an empty catalogue,
// not a panic and not a stale hardcoded list.
func TestSearchTeams_EmptyRegistry(t *testing.T) {
	withTeamRegistry(t, "")

	resp, err := NewTeamService().SearchTeams(context.Background(), domain.SearchTeamsRequest{})
	if err != nil {
		t.Fatalf("SearchTeams: %v", err)
	}
	if resp.Total != 0 || len(resp.Teams) != 0 {
		t.Fatalf("SearchTeams = %+v, want an empty catalogue", resp)
	}
}
