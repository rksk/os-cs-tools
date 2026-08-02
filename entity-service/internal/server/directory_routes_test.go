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

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/config"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// newDirectoryRouter builds the real router against a stubbed upstream, so the
// three directory searches are exercised over HTTP exactly as a caller hits
// them. teamRegistry and userRoles are the raw configuration values.
func newDirectoryRouter(t *testing.T, teamRegistry, userRoles string) http.Handler {
	t.Helper()

	upstream := http.NewServeMux()
	upstream.HandleFunc("/oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 3600})
	})
	// Groups are a live query against the backing data source, unchanged by the
	// move of the curated vocabularies into configuration.
	upstream.HandleFunc("/groups/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"groups":[{"id":"11111111111111111111111111111111","name":"Some Group","active":true,"parent":null}],` +
			`"totalRecords":1,"offset":0,"limit":20}`))
	})
	srv := httptest.NewServer(upstream)
	t.Cleanup(srv.Close)

	teams, err := domain.ParseAbtTeamRegistry(teamRegistry)
	if err != nil {
		t.Fatalf("ParseAbtTeamRegistry(%q): %v", teamRegistry, err)
	}
	domain.SetAbtTeams(teams)
	t.Cleanup(func() { domain.SetAbtTeams(nil) })

	roles, err := service.ParseUserRoles(userRoles)
	if err != nil {
		t.Fatalf("ParseUserRoles(%q): %v", userRoles, err)
	}
	service.SetUserRoles(roles)
	t.Cleanup(func() {
		defaults, _ := service.ParseUserRoles("")
		service.SetUserRoles(defaults)
	})

	return NewRouter(nil, &config.Config{
		DataSource:                               config.DataSourceServiceNow,
		ServiceNowIntegrationServiceBaseURL:      srv.URL,
		ServiceNowIntegrationServiceTokenURL:     srv.URL + "/oauth2/token",
		ServiceNowIntegrationServiceClientID:     "test-client",
		ServiceNowIntegrationServiceClientSecret: "test-secret",
	})
}

func postJSON(t *testing.T, router http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST %s = %d, want 200: %s", path, rec.Code, rec.Body.String())
	}
	return rec
}

// TestDirectorySearches_AllThreeStillWork covers the whole surface this change
// touches: teams and roles now come from configuration, groups are still a
// live query against the backing data source.
func TestDirectorySearches_AllThreeStillWork(t *testing.T) {
	router := newDirectoryRouter(t, "alpha|Alpha Team|CRE,beta|Beta Team|SRE", "agent,timecard_approver")

	t.Run("teams reflect the configured registry", func(t *testing.T) {
		rec := postJSON(t, router, "/teams/search", `{"pagination":{"limit":20}}`)
		var got struct {
			Teams []struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Family string `json:"family"`
			} `json:"teams"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
		}
		if got.Total != 2 {
			t.Fatalf("total = %d, want 2: %s", got.Total, rec.Body.String())
		}
		if got.Teams[0].ID != "alpha" || got.Teams[0].Name != "Alpha Team" || got.Teams[0].Family != "cre" {
			t.Fatalf("teams[0] = %+v, want the configured alpha row", got.Teams[0])
		}
		if got.Teams[1].ID != "beta" || got.Teams[1].Family != "sre" {
			t.Fatalf("teams[1] = %+v, want the configured beta row", got.Teams[1])
		}
	})

	t.Run("roles reflect the configured allow-list", func(t *testing.T) {
		rec := postJSON(t, router, "/roles/search", `{"pagination":{"limit":20}}`)
		var got struct {
			Roles []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"roles"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
		}
		if got.Total != 2 {
			t.Fatalf("total = %d, want the 2 configured roles: %s", got.Total, rec.Body.String())
		}
		if got.Roles[0].ID != "agent" || got.Roles[1].ID != "timecard_approver" {
			t.Fatalf("roles = %+v, want [agent timecard_approver]", got.Roles)
		}
	})

	t.Run("groups are still a live upstream query", func(t *testing.T) {
		rec := postJSON(t, router, "/groups/search", `{"pagination":{"limit":20}}`)
		if !strings.Contains(rec.Body.String(), "Some Group") {
			t.Fatalf("groups response = %s, want the upstream row", rec.Body.String())
		}
	})
}

// TestRolesSearch_UnconfiguredUsesDefaults: a deployment that sets nothing
// still serves the committed default catalogue.
func TestRolesSearch_UnconfiguredUsesDefaults(t *testing.T) {
	router := newDirectoryRouter(t, "", "")

	rec := postJSON(t, router, "/roles/search", `{"pagination":{"limit":20}}`)
	var got struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	if got.Total != 10 {
		t.Fatalf("total = %d, want the 10 default roles: %s", got.Total, rec.Body.String())
	}

	// An unconfigured team registry is an empty catalogue, not an error.
	rec = postJSON(t, router, "/teams/search", `{"pagination":{"limit":20}}`)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	if got.Total != 0 {
		t.Fatalf("total = %d, want an empty team catalogue: %s", got.Total, rec.Body.String())
	}
}
