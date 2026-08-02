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
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// snUserRowJSON is one user row as the upstream search returns it.
func snUserRowJSON(sysid, email, userType string) string {
	return `{"id":"` + sysid + `","userName":"` + email + `","name":"Test User","email":"` + email +
		`","timeZone":"Asia/Colombo","mobilePhone":"+94700000000","userType":"` + userType +
		`","active":true,"createdOn":"2020-01-01 00:00:00","updatedOn":"2020-01-02 00:00:00",` +
		`"roles":["snc_internal"]}`
}

func usersSearchJSON(rows ...string) string {
	body := `{"users":[`
	for i, r := range rows {
		if i > 0 {
			body += ","
		}
		body += r
	}
	return body + `],"totalRecords":` + itoa(len(rows)) + `,"offset":0,"limit":20}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// A team filter that resolves to no members must return an empty page. Falling through
// would send no id filter upstream and return every user, which reads as "everyone
// matched" — the opposite of the truth.
func TestSNUserService_SearchUsers_TeamFilterNoMembers(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	searchCalled := false

	mux := http.NewServeMux()
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"memberships": [], "totalRecords": 0}`))
	})
	mux.HandleFunc("/users/search", func(w http.ResponseWriter, _ *http.Request) {
		searchCalled = true
		_, _ = w.Write([]byte(usersSearchJSON(snUserRowJSON(sysid32('1'), "someone@wso2.com", "internal"))))
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	got, err := svc.SearchUsers(contextWithUserIDToken("token"), domain.SearchUsersRequest{
		Pagination: domain.Pagination{Limit: 20},
		Filters:    domain.SearchUsersFilters{TeamIDs: []string{"alpha"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Users) != 0 || got.Total != 0 {
		t.Fatalf("got %d users (total %d), want an empty page", len(got.Users), got.Total)
	}
	if searchCalled {
		t.Fatal("upstream user search was called despite zero resolved members")
	}
}

// A team filter that does resolve must send the members' ids upstream.
func TestSNUserService_SearchUsers_TeamFilterSendsUserIDs(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	memberSysid := sysid32('a')
	var captured []byte

	mux := http.NewServeMux()
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(membershipsJSON(memberSysid, "Alpha Team")))
	})
	mux.HandleFunc("/users/search", func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(usersSearchJSON(snUserRowJSON(memberSysid, "member@wso2.com", "internal"))))
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	got, err := svc.SearchUsers(contextWithUserIDToken("token"), domain.SearchUsersRequest{
		Pagination: domain.Pagination{Limit: 20},
		Filters:    domain.SearchUsersFilters{TeamIDs: []string{"alpha"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Users) != 1 {
		t.Fatalf("got %d users, want 1", len(got.Users))
	}
	if got.Users[0].UserType != domain.UserTypeInternal {
		t.Fatalf("userType = %q, want internal", got.Users[0].UserType)
	}
	if got.Users[0].MobilePhone == nil || *got.Users[0].MobilePhone != "+94700000000" {
		t.Fatalf("mobilePhone = %v, want +94700000000", got.Users[0].MobilePhone)
	}

	var req struct {
		Filters struct {
			UserIDs []string `json:"userIds"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(captured, &req); err != nil {
		t.Fatalf("upstream request body not JSON: %v", err)
	}
	if len(req.Filters.UserIDs) != 1 || req.Filters.UserIDs[0] != memberSysid {
		t.Fatalf("userIds = %v, want [%s]", req.Filters.UserIDs, memberSysid)
	}
}

// An unknown team key is a client error, not a silent empty result.
func TestSNUserService_SearchUsers_UnknownTeamKey(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	_, err := svc.SearchUsers(contextWithUserIDToken("token"), domain.SearchUsersRequest{
		Pagination: domain.Pagination{Limit: 20},
		Filters:    domain.SearchUsersFilters{TeamIDs: []string{"no-such-team"}},
	})
	var verr *apierror.ValidationError
	if !asValidationError(err, &verr) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
}

// GetUser enriches the row with every group the user is in, and marks registry teams.
func TestSNUserService_GetUser_GroupsAndTeams(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	userSysid := sysid32('b')

	mux := http.NewServeMux()
	mux.HandleFunc("/users/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(usersSearchJSON(snUserRowJSON(userSysid, "staff@wso2.com", "internal"))))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"memberships":[` +
			`{"userId":"` + userSysid + `","groupId":"` + sysid32('c') + `","groupName":"Alpha Team"},` +
			`{"userId":"` + userSysid + `","groupId":"` + sysid32('d') + `","groupName":"Some Other Group"}` +
			`],"totalRecords":2}`))
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	got, err := svc.GetUser(contextWithUserIDToken("token"), sysidToUUID(userSysid))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(got.Groups))
	}
	if len(got.Teams) != 1 || got.Teams[0].ID != "alpha" {
		t.Fatalf("teams = %+v, want just alpha", got.Teams)
	}
	// Staff get no project-access block.
	if got.ProjectAccess != nil {
		t.Fatalf("projectAccess = %+v, want nil for an internal user", got.ProjectAccess)
	}
}

// An external contact gets project access, including rows that grant nothing.
func TestSNUserService_GetUser_ExternalProjectAccess(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	userSysid := sysid32('e')
	projectSysid := sysid32('f')

	mux := http.NewServeMux()
	mux.HandleFunc("/users/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(usersSearchJSON(snUserRowJSON(userSysid, "jane@example.com", "external"))))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"memberships": [], "totalRecords": 0}`))
	})
	mux.HandleFunc("/project-contacts/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"contacts":[{"projectId":"` + projectSysid + `","projectName":"Proj A",` +
			`"projectKey":"PROJA","contactEmail":"jane@example.com","customerContactPresent":false,` +
			`"customerContactEmail":null,"registrationState":"INVITED","notificationsEnabled":false,` +
			`"roles":[],"grantsCaseAccess":false}],"totalRecords":1}`))
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	got, err := svc.GetUser(contextWithUserIDToken("token"), sysidToUUID(userSysid))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.ProjectAccess) != 1 {
		t.Fatalf("got %d project-access rows, want 1", len(got.ProjectAccess))
	}
	row := got.ProjectAccess[0]
	if row.GrantsCaseAccess {
		t.Fatal("grantsCaseAccess = true, want false for a row with no contact record")
	}
	if row.ContactRecordPresent {
		t.Fatal("contactRecordPresent = true, want false")
	}
	if row.ProjectID != sysidToUUID(projectSysid) {
		t.Fatalf("projectId = %q, want the converted id", row.ProjectID)
	}
	if row.ProjectKey != "PROJA" {
		t.Fatalf("projectKey = %q, want %q", row.ProjectKey, "PROJA")
	}
}

// A failing enrichment must not fail the whole profile.
func TestSNUserService_GetUser_EnrichmentDegrades(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	userSysid := sysid32('1')

	mux := http.NewServeMux()
	mux.HandleFunc("/users/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(usersSearchJSON(snUserRowJSON(userSysid, "staff@wso2.com", "internal"))))
	})
	mux.HandleFunc("/group-members/search", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	got, err := svc.GetUser(contextWithUserIDToken("token"), sysidToUUID(userSysid))
	if err != nil {
		t.Fatalf("group lookup failure should not fail the profile, got %v", err)
	}
	if got.Email != "staff@wso2.com" {
		t.Fatalf("email = %q, want the user row to still be returned", got.Email)
	}
	if len(got.Groups) != 0 || len(got.Teams) != 0 {
		t.Fatalf("groups/teams = %+v/%+v, want both empty", got.Groups, got.Teams)
	}
}

// An id with no match is a 404, not an empty struct.
func TestSNUserService_GetUser_NotFound(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"users":[],"totalRecords":0,"offset":0,"limit":1}`))
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	_, err := svc.GetUser(contextWithUserIDToken("token"), sysidToUUID(sysid32('2')))
	var nf *apierror.NotFoundError
	if !asNotFoundError(err, &nf) {
		t.Fatalf("err = %v, want a NotFoundError", err)
	}
}

func asValidationError(err error, target **apierror.ValidationError) bool {
	v, ok := err.(*apierror.ValidationError)
	if ok {
		*target = v
	}
	return ok
}

func asNotFoundError(err error, target **apierror.NotFoundError) bool {
	v, ok := err.(*apierror.NotFoundError)
	if ok {
		*target = v
	}
	return ok
}

// Malformed ids are rejected at this boundary. uuidToSysid passes a non-canonical value
// through unchanged, so without this check a bogus id reaches upstream, which answers with
// an opaque error or an empty page that reads like a legitimate "no such user".
func TestSNUserService_SearchUsers_RejectsMalformedFilterIDs(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/search", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream search called; a malformed filter id should be rejected first")
	})
	mux.HandleFunc("/group-members/search", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("membership search called; a malformed filter id should be rejected first")
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	tests := []struct {
		name    string
		filters domain.SearchUsersFilters
	}{
		{"malformed userIds", domain.SearchUsersFilters{UserIDs: []string{"not-a-uuid"}}},
		{"malformed groupIds", domain.SearchUsersFilters{GroupIDs: []string{"not-a-uuid"}}},
		{"too many userIds", domain.SearchUsersFilters{UserIDs: repeatUUID(snUserIDFilterLimit + 1)}},
		{"too many groupIds", domain.SearchUsersFilters{GroupIDs: repeatUUID(snGroupIDFilterLimit + 1)}},
		{"too many teamIds", domain.SearchUsersFilters{TeamIDs: repeatKey(snTeamIDFilterLimit + 1)}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.SearchUsers(contextWithUserIDToken("token"), domain.SearchUsersRequest{
				Pagination: domain.Pagination{Limit: 20},
				Filters:    tc.filters,
			})
			var verr *apierror.ValidationError
			if !asValidationError(err, &verr) {
				t.Fatalf("err = %v, want a ValidationError", err)
			}
		})
	}
}

// GetUser must not report a malformed id as a missing one: "id is required" sends the caller
// looking for a parameter they did supply.
func TestSNUserService_GetUser_DistinguishesEmptyFromMalformedID(t *testing.T) {
	withTeamRegistry(t, abtTeamRegistryFixture)

	mux := http.NewServeMux()
	mux.HandleFunc("/users/search", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("upstream search called; an invalid id should be rejected first")
	})

	svc := NewServiceNowUserService(newTestSNClient(t, mux))

	emptyErr := getUserErr(t, svc, "")
	if emptyErr.Msg != "id is required" {
		t.Errorf("empty id message = %q, want \"id is required\"", emptyErr.Msg)
	}

	malformedErr := getUserErr(t, svc, "not-a-uuid")
	if malformedErr.Msg == "id is required" {
		t.Errorf("malformed id reported as %q; it must not read as a missing parameter", malformedErr.Msg)
	}
	if !strings.Contains(malformedErr.Msg, "invalid UUID") {
		t.Errorf("malformed id message = %q, want it to name the invalid UUID", malformedErr.Msg)
	}
}

func getUserErr(t *testing.T, svc SNUserService, id string) *apierror.ValidationError {
	t.Helper()
	_, err := svc.GetUser(contextWithUserIDToken("token"), id)
	var verr *apierror.ValidationError
	if !asValidationError(err, &verr) {
		t.Fatalf("GetUser(%q) err = %v, want a ValidationError", id, err)
	}
	return verr
}

// repeatUUID builds n distinct valid UUIDs, for exercising the filter count caps.
func repeatUUID(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("00000000-0000-0000-0000-%012d", i)
	}
	return out
}

func repeatKey(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("team-%d", i)
	}
	return out
}
