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
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// testAbtUserSysid is the caller's ServiceNow sys_id used across the GetMe
// ABT-team-resolution tests.
var testAbtUserSysid = sysid32('9')

// abtTeamRegistryFixture is a representative registry in its configured wire
// form: a CRE team, a hyphenated flat team (no sub-team nesting), an SRE team,
// and an unclassified team with no family field. Every name is an invented
// placeholder -- real team names never appear in this repo.
const abtTeamRegistryFixture = "alpha|Alpha Team|CRE,delta|Delta-Two|CRE,beta|Beta SRE Group|SRE,gamma|Gamma Team"

// withTeamRegistry installs a parsed team registry for the duration of one
// test and clears it afterwards, so no test inherits another's registry.
func withTeamRegistry(t *testing.T, raw string) {
	t.Helper()
	teams, err := domain.ParseAbtTeamRegistry(raw)
	if err != nil {
		t.Fatalf("ParseAbtTeamRegistry(%q): %v", raw, err)
	}
	domain.SetAbtTeams(teams)
	t.Cleanup(func() { domain.SetAbtTeams(nil) })
}

// snUserMeJSON is a minimal ServiceNow GET /users/me payload for the given
// caller sys_id.
func snUserMeJSON(id string) string {
	return `{
		"id": "` + id + `",
		"email": "agent@example.com",
		"lastName": "Agent",
		"roles": ["wso2_agent"]
	}`
}

// membershipsJSON builds a group-members/search response body with the given
// groupName as the caller's single membership, or an empty memberships list
// if groupName is "".
func membershipsJSON(userID, groupName string) string {
	if groupName == "" {
		return `{"memberships": [], "totalRecords": 0}`
	}
	return `{"memberships": [{"userId": "` + userID + `", "groupId": "irrelevant-sysid", "groupName": "` + groupName + `"}], "totalRecords": 1}`
}

func TestSNUserService_GetMe_TeamMatch(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	var capturedBody []byte

	mux := http.NewServeMux()
	mux.HandleFunc("/users/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snUserMeJSON(testAbtUserSysid)))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(membershipsJSON(testAbtUserSysid, "Alpha Team")))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowUserService(client)

	got, err := svc.GetMe(contextWithUserIDToken("token"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Team == nil {
		t.Fatalf("Team = nil, want Alpha Team")
	}
	if got.Team.TeamKey != "alpha" || got.Team.TeamName != "Alpha Team" || got.Team.Family != "cre" {
		t.Fatalf("Team = %+v, want {alpha Alpha Team cre}", got.Team)
	}

	// The request body must send groupNames, never groupIds.
	var reqBody struct {
		Filters struct {
			GroupNames []string `json:"groupNames"`
			GroupIDs   []string `json:"groupIds"`
			UserID     string   `json:"userId"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(capturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal captured request body: %v", err)
	}
	if reqBody.Filters.GroupIDs != nil {
		t.Fatalf("request body carried groupIds %v, want none (name-based lookup only)", reqBody.Filters.GroupIDs)
	}
	if len(reqBody.Filters.GroupNames) == 0 {
		t.Fatalf("request body carried no groupNames, want the cached registry's display names")
	}
	found := false
	for _, n := range reqBody.Filters.GroupNames {
		if n == "Alpha Team" {
			found = true
		}
	}
	if !found {
		t.Fatalf("groupNames = %v, want it to include \"Alpha Team\"", reqBody.Filters.GroupNames)
	}
	if reqBody.Filters.UserID != testAbtUserSysid {
		t.Fatalf("userId = %q, want %q", reqBody.Filters.UserID, testAbtUserSysid)
	}
}

// TestSNUserService_GetMe_FlatTeamMatch_HyphenatedName verifies a hyphenated team name
// resolves as its own flat team -- there is no sub-team nesting, so this is
// just a normal name match like any other team.
func TestSNUserService_GetMe_FlatTeamMatch_HyphenatedName(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snUserMeJSON(testAbtUserSysid)))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(membershipsJSON(testAbtUserSysid, "Delta-Two")))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowUserService(client)

	got, err := svc.GetMe(contextWithUserIDToken("token"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Team == nil {
		t.Fatalf("Team = nil, want Delta-Two")
	}
	if got.Team.TeamKey != "delta" || got.Team.TeamName != "Delta-Two" || got.Team.Family != "cre" {
		t.Fatalf("Team = %+v, want {delta Delta-Two cre}", got.Team)
	}
}

// TestSNUserService_GetMe_UnclassifiedTeamMatch verifies a team with no
// "family" set still resolves correctly, with an empty Family rather than an
// error.
func TestSNUserService_GetMe_UnclassifiedTeamMatch(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snUserMeJSON(testAbtUserSysid)))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(membershipsJSON(testAbtUserSysid, "Gamma Team")))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowUserService(client)

	got, err := svc.GetMe(contextWithUserIDToken("token"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Team == nil {
		t.Fatalf("Team = nil, want Gamma Team")
	}
	if got.Team.TeamKey != "gamma" || got.Team.TeamName != "Gamma Team" || got.Team.Family != "" {
		t.Fatalf("Team = %+v, want {gamma \"Gamma Team\" \"\"}", got.Team)
	}
}

func TestSNUserService_GetMe_NoMatch(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snUserMeJSON(testAbtUserSysid)))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(membershipsJSON(testAbtUserSysid, "")))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowUserService(client)

	got, err := svc.GetMe(contextWithUserIDToken("token"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Team != nil {
		t.Fatalf("Team = %+v, want nil (no membership)", got.Team)
	}
}

// TestSNUserService_GetMe_GroupMembershipCallErrors_IdentityStillReturned
// verifies that a downstream failure on the group-members/search call never
// fails the overall /users/me response -- identity/roles must still come
// back, with Team simply nil.
func TestSNUserService_GetMe_GroupMembershipCallErrors_IdentityStillReturned(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snUserMeJSON(testAbtUserSysid)))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowUserService(client)

	got, err := svc.GetMe(contextWithUserIDToken("token"))
	if err != nil {
		t.Fatalf("unexpected error: %v (identity must still be returned)", err)
	}
	if got.Email != "agent@example.com" {
		t.Fatalf("Email = %q, want agent@example.com even though group lookup failed", got.Email)
	}
	if got.Team != nil {
		t.Fatalf("Team = %+v, want nil when group-membership call errors", got.Team)
	}
}

// TestSNUserService_GetMe_EmptyRegistry_IdentityStillReturned verifies that a
// deployment with no team registry configured still serves identity: roles
// come back, Team is nil, and the membership lookup is skipped entirely.
func TestSNUserService_GetMe_EmptyRegistry_IdentityStillReturned(t *testing.T) {
	withTeamRegistry(t, "")

	mux := http.NewServeMux()
	mux.HandleFunc("/users/me", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snUserMeJSON(testAbtUserSysid)))
	})
	groupMembersCalled := false
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, r *http.Request) {
		groupMembersCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(membershipsJSON(testAbtUserSysid, "Alpha Team")))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowUserService(client)

	got, err := svc.GetMe(contextWithUserIDToken("token"))
	if err != nil {
		t.Fatalf("unexpected error: %v (identity must still be returned)", err)
	}
	if got.Email != "agent@example.com" {
		t.Fatalf("Email = %q, want agent@example.com with an empty registry", got.Email)
	}
	if got.Team != nil {
		t.Fatalf("Team = %+v, want nil when no teams are configured", got.Team)
	}
	// With an empty registry, AbtGroupNames() is empty, so GetMe should
	// short-circuit and never even call group-members/search.
	if groupMembersCalled {
		t.Fatalf("group-members/search was called despite an empty registry")
	}
}

// TestGetUserMeResponse_TeamOmittedWhenNil locks in the JSON field names and
// casing a downstream CSM-backend consumer depends on: "team" (camelCase),
// omitted entirely when the caller has no resolved ABT team.
func TestGetUserMeResponse_TeamOmittedWhenNil(t *testing.T) {
	resp := domain.GetUserMeResponse{ID: "u1", Email: "agent@example.com", Roles: []string{}}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := asMap["team"]; present {
		t.Fatalf("expected \"team\" to be omitted when Team is nil, got: %s", raw)
	}
}

// TestGetUserMeResponse_TeamFieldShape locks in the exact field names/casing
// on the populated Team object: teamKey, teamName, family. This shape is
// unchanged by the sys_id -> name-based resolution switch.
func TestGetUserMeResponse_TeamFieldShape(t *testing.T) {
	resp := domain.GetUserMeResponse{
		ID: "u1", Email: "agent@example.com", Roles: []string{},
		Team: &domain.UserTeam{TeamKey: "alpha", TeamName: "Alpha Team", Family: "cre"},
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	teamRaw, present := asMap["team"]
	if !present {
		t.Fatalf("expected \"team\" to be present, got: %s", raw)
	}
	var team map[string]string
	if err := json.Unmarshal(teamRaw, &team); err != nil {
		t.Fatalf("unmarshal team: %v", err)
	}
	want := map[string]string{"teamKey": "alpha", "teamName": "Alpha Team", "family": "cre"}
	for k, v := range want {
		if team[k] != v {
			t.Fatalf("team[%q] = %q, want %q (full team object: %s)", k, team[k], v, teamRaw)
		}
	}
}

// TestSearchUsers_RejectsRoleOutsideConfiguredList proves the allow-list is
// enforced on the real request path, not just in the helper.
func TestSearchUsers_RejectsRoleOutsideConfiguredList(t *testing.T) {
	withUserRoles(t, "agent")
	withTeamRegistry(t, abtTeamRegistryFixture)

	upstreamCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/users/search", func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		_, _ = w.Write([]byte(`{"users":[],"totalRecords":0}`))
	})
	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	_, err := svc.SearchUsers(contextWithUserIDToken("token"), domain.SearchUsersRequest{
		Pagination: domain.Pagination{Limit: 20},
		Filters:    domain.SearchUsersFilters{RoleIDs: []domain.UserRole{domain.UserRoleAdmin}},
	})
	var verr *apierror.ValidationError
	if !asValidationError(err, &verr) {
		t.Fatalf("err = %v, want a ValidationError for a role outside the configured list", err)
	}
	if upstreamCalled {
		t.Fatal("upstream user search was called with an unconfigured role")
	}

	// The configured role still passes validation and reaches upstream.
	if _, err := svc.SearchUsers(contextWithUserIDToken("token"), domain.SearchUsersRequest{
		Pagination: domain.Pagination{Limit: 20},
		Filters:    domain.SearchUsersFilters{RoleIDs: []domain.UserRole{domain.UserRoleAgent}},
	}); err != nil {
		t.Fatalf("configured role \"agent\" was rejected: %v", err)
	}
	if !upstreamCalled {
		t.Fatal("configured role did not reach the upstream search")
	}
}
