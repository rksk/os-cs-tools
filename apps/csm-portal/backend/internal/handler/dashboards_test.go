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

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/dashboard"
)

// testDashboardsConfigJSON is the pilot's 6-dashboard registry, identical to
// the DASHBOARDS_CONFIG example documented in .env.example. dashboard.Dashboards
// is populated from DASHBOARDS_CONFIG only in cmd/server/main.go, which tests
// never run, so TestMain below seeds it directly via the same parse function
// production uses — every assertion in this file exercises the real
// production parsing/lookup/resolution path, just with the config supplied
// in-process instead of via the environment.
const testDashboardsConfigJSON = `[
  {"id":"agents_pilot","displayName":"Engineer overview","isDefault":true,"targetTeam":"cs_engineers","widgets":[
    {"id":"my_patches","displayName":"My Patches","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"tag","op":"in","values":["patch"]},{"field":"state","op":"in","values":["open","work_in_progress","waiting_on_wso2","reopened","awaiting_info"]}]}},
    {"id":"my_reminders","displayName":"My Reminders","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"state","op":"in","values":["awaiting_info","solution_proposed"]}]}},
    {"id":"open_incident_team","displayName":"Open Incident (Team)","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"tag","op":"in","values":["s_dip"]},{"field":"state","op":"in","values":["work_in_progress","open","waiting_on_wso2","reopened"]}]}},
    {"id":"my_critical_open","displayName":"My Critical & High Cases","resourceType":"case","shape":"list","gridWidth":3,"listLimit":5,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"severity","op":"in","values":["catastrophic","critical"]},{"field":"state","op":"in","values":["open","work_in_progress"]}]}}
  ]},
  {"id":"operations","displayName":"Operations","targetTeam":"cs_operations","widgets":[
    {"id":"p0_p1_open","displayName":"P0/P1 Open","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"severity","op":"in","values":["catastrophic","critical"]},{"field":"state","op":"in","values":["open","work_in_progress"]}]}},
    {"id":"open_critical_incidents","displayName":"Open Critical Incidents","resourceType":"incident","shape":"count","gridWidth":4,"filters":{"priorities":["CRITICAL","HIGH"]}},
    {"id":"crs_awaiting_approval","displayName":"CRs Awaiting Approval","resourceType":"change_request","shape":"count","gridWidth":4,"filters":{"states":["customer_approval"]}}
  ]},
  {"id":"iam","displayName":"IAM CS","targetTeam":"iam_cs","widgets":[
    {"id":"iam_open_cases","displayName":"IAM Open Cases","resourceType":"case","shape":"count","gridWidth":6,"filters":{"filters":[{"field":"tag","op":"in","values":["iam"]},{"field":"state","op":"in","values":["open","work_in_progress","awaiting_info"]}]}},
    {"id":"asgardeo_open_cases","displayName":"Asgardeo Open Cases","resourceType":"case","shape":"count","gridWidth":6,"filters":{"filters":[{"field":"tag","op":"in","values":["asgardeo"]},{"field":"state","op":"in","values":["open","work_in_progress","awaiting_info"]}]}}
  ]},
  {"id":"security","displayName":"Security center","targetTeam":"security","widgets":[
    {"id":"critical_vulns","displayName":"Critical Vulnerabilities","resourceType":"product_vulnerability","shape":"count","gridWidth":4,"filters":{"priority":"critical"}},
    {"id":"high_vulns","displayName":"High Vulnerabilities","resourceType":"product_vulnerability","shape":"count","gridWidth":4,"filters":{"priority":"high"}},
    {"id":"sra_cases_open","displayName":"Open SRAs","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"type","op":"in","values":["security_report_analysis"]},{"field":"state","op":"in","values":["open","work_in_progress","awaiting_info"]}]}}
  ]},
  {"id":"team_performance","displayName":"Team performance","targetTeam":"cs_team_leads","isTeamBased":true,"widgets":[
    {"id":"time_cards_pending_approval","displayName":"Time Cards Pending Approval","resourceType":"time_card","shape":"count","gridWidth":6,"filters":{"states":["pending"]}},
    {"id":"team_open_cases","displayName":"Team Open P0/P1","resourceType":"case","shape":"count","gridWidth":6,"filters":{"filters":[{"field":"severity","op":"in","values":["catastrophic","critical"]},{"field":"state","op":"in","values":["open","work_in_progress"]}]}},
    {"id":"cases_by_severity","displayName":"Cases by severity","description":"Share of active cases at each severity level.","resourceType":"case","shape":"pie","gridWidth":6,"filters":{"filters":[{"field":"state","op":"in","values":["open","work_in_progress"]}]},"slices":[
      {"label":"Critical","color":"error","filters":{"filters":[{"field":"severity","op":"in","values":["critical"]}]}},
      {"label":"Mine","filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]}]}}
    ]},
    {"id":"incident_wow","displayName":"Incident WOW","section":"SLA Violation","resourceType":"incident","shape":"count","gridWidth":6,"filters":{}}
  ]},
  {"id":"abt","displayName":"ABT Dashboard","targetTeam":"cs_engineers","isTeamBased":true,"widgets":[
    {"id":"abt_my_open_incident","displayName":"Open Incident","section":"My Work","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"tag","op":"in","values":["s_dip"]},{"field":"state","op":"in","values":["open","work_in_progress","waiting_on_wso2","reopened"]}]}},
    {"id":"abt_my_open_query","displayName":"Open Query","section":"My Work","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"state","op":"in","values":["waiting_on_wso2"]},{"field":"tag","op":"notIn","values":["s_dip"]}]}},
    {"id":"abt_my_patches","displayName":"My Patches","section":"My Work","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"tag","op":"in","values":["patch"]},{"field":"state","op":"in","values":["open","work_in_progress","waiting_on_wso2","reopened","awaiting_info"]}]}},
    {"id":"abt_my_pending_closure","displayName":"Pending Closure","section":"My Work","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"resolutionNotes","op":"isEmpty"},{"field":"state","op":"in","values":["solution_proposed"]}]}},
    {"id":"abt_my_reminders","displayName":"My Reminders","section":"My Work","resourceType":"case","shape":"count","gridWidth":3,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"state","op":"in","values":["awaiting_info","solution_proposed"]}]}},
    {"id":"abt_my_discussions","displayName":"My Discussions","section":"My Work","resourceType":"case","shape":"list","gridWidth":3,"listLimit":5,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"tag","op":"in","values":["s_dip"]},{"field":"state","op":"in","values":["open","work_in_progress","waiting_on_wso2","reopened","awaiting_info"]}]}},
    {"id":"abt_on_going_cases","displayName":"On Going Cases","section":"My Work","resourceType":"case","shape":"list","gridWidth":6,"listLimit":5,"filters":{"filters":[{"field":"assignedUserId","op":"in","values":["__current_user__"]},{"field":"state","op":"in","values":["open","work_in_progress","waiting_on_wso2","reopened","awaiting_info"]},{"field":"tag","op":"notIn","values":["s_dip"]},{"field":"projectOnboardingStatus","op":"in","values":["Completed"]}]}},
    {"id":"abt_overall_open_incident","displayName":"Open Incident","section":"Overall","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"tag","op":"in","values":["s_dip"]},{"field":"state","op":"in","values":["work_in_progress","open","waiting_on_wso2","reopened"]}]}},
    {"id":"abt_overall_open_query","displayName":"Open Query","section":"Overall","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"tag","op":"notIn","values":["s_dip"]},{"field":"state","op":"in","values":["open","waiting_on_wso2","reopened"]}]}},
    {"id":"abt_overall_unassigned","displayName":"Unassigned Cases","section":"Overall","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"assignedUserId","op":"isEmpty"},{"field":"state","op":"in","values":["open","work_in_progress","awaiting_info","waiting_on_wso2","reopened"]}]}},
    {"id":"abt_team_wow_p1","displayName":"WOW P1","section":"Overall","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"severity","op":"in","values":["catastrophic"]},{"field":"state","op":"in","values":["waiting_on_wso2"]},{"field":"tag","op":"notIn","values":["s_dip"]}]}},
    {"id":"abt_team_discussions_ongoing","displayName":"Discussions on Going","section":"Overall","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"state","op":"in","values":["waiting_on_wso2","open","reopened"]},{"field":"tag","op":"in","values":["s_dip"]}]}},
    {"id":"abt_iam_open_cases","displayName":"IAM Open Cases","section":"Overall","resourceType":"case","shape":"count","gridWidth":4,"filters":{"filters":[{"field":"tag","op":"in","values":["iam"]},{"field":"state","op":"in","values":["open","work_in_progress","awaiting_info"]}]}}
  ]}
]`

// filterValuesByField finds the "values" array of the first entry in a case
// widget's resolved filters (the {"filters":[{"field","op","values"}, ...]}
// shape, see .env.example's DASHBOARDS_CONFIG) whose "field" matches, and
// reports whether one was found.
func filterValuesByField(filters map[string]any, field string) ([]any, bool) {
	arr, ok := filters["filters"].([]any)
	if !ok {
		return nil, false
	}
	for _, entry := range arr {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if m["field"] != field {
			continue
		}
		values, ok := m["values"].([]any)
		return values, ok
	}
	return nil, false
}

// TestMain seeds dashboard.Dashboards before any test in this package runs.
// In production it is populated once at process startup by cmd/server/main.go
// from DASHBOARDS_CONFIG; tests never invoke main(), so they must seed it
// themselves via the same ParseDashboardsConfig function.
func TestMain(m *testing.M) {
	dashboard.Dashboards = dashboard.ParseDashboardsConfig(testDashboardsConfigJSON)
	if len(dashboard.Dashboards) != 6 {
		panic(fmt.Sprintf("TestMain: seeding dashboard.Dashboards failed, got %d dashboards, want 6", len(dashboard.Dashboards)))
	}
	os.Exit(m.Run())
}

// dashboardWidgetJSONKeys are the top-level JSON keys openapi.yaml's
// DashboardWidget schema declares. Kept in sync with that schema by hand;
// the tests below fail if the handler's actual response keys ever diverge
// from this set, catching an unannounced field rename/add/remove that a
// struct-only decode (which silently ignores unknown keys and zero-values
// missing ones) would miss.
//
// groupBy and listLimit are omitempty on the wire and are not included here;
// widgets that set them are checked individually where relevant.
var dashboardWidgetJSONKeys = []string{"widgetId", "displayName", "resourceType", "shape", "gridWidth", "filters"}

// dashboardListItemJSONKeys are the top-level JSON keys openapi.yaml's
// DashboardListItem schema declares.
var dashboardListItemJSONKeys = []string{"id", "displayName", "isDefault", "isTeamBased"}

// dashboardDetailJSONKeys are the top-level JSON keys openapi.yaml's
// Dashboard schema declares.
var dashboardDetailJSONKeys = []string{"id", "displayName", "isDefault", "targetTeam", "isTeamBased", "widgets"}

func assertJSONKeys(t *testing.T, obj map[string]json.RawMessage, want []string, context string) {
	t.Helper()
	wantKeys := append([]string(nil), want...)
	sort.Strings(wantKeys)
	gotKeys := make([]string, 0, len(obj))
	for k := range obj {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("%s JSON keys = %v, want %v", context, gotKeys, wantKeys)
	}
}

// assertJSONKeysSuperset is like assertJSONKeys but only requires every key in
// want to be present; used for widgets that additionally carry an omitempty
// field (groupBy/listLimit) beyond the base set.
func assertJSONKeysSuperset(t *testing.T, obj map[string]json.RawMessage, want []string, context string) {
	t.Helper()
	for _, k := range want {
		if _, ok := obj[k]; !ok {
			t.Errorf("%s missing expected key %q; got keys %v", context, k, keysOf(obj))
		}
	}
}

func keysOf(obj map[string]json.RawMessage) []string {
	out := make([]string, 0, len(obj))
	for k := range obj {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func withDashboardID(r *http.Request, dashboardID string) *http.Request {
	r.SetPathValue("dashboardId", dashboardID)
	return r
}

// resolvedCurrentUserID is mockEntityUserClient's default GET /users/me id
// (helpers_test.go). It is deliberately NOT testUser.UserID (the JWT claim):
// the handler resolves __current_user__ via the entity service's /users/me,
// the same id GET /users/me itself returns — see
// DashboardHandler.resolveCurrentUserID's doc comment for why.
const resolvedCurrentUserID = "11111111-1111-1111-1111-111111111111"

func TestGetDashboards(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := httptest.NewRequest(http.MethodGet, "/dashboards", nil)
		w := httptest.NewRecorder()
		h.GetDashboards(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("returns all dashboards in registry order with correct isDefault", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withUser(httptest.NewRequest(http.MethodGet, "/dashboards", nil))
		w := httptest.NewRecorder()
		h.GetDashboards(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		body := w.Body.Bytes()

		var results []dashboardListItemView
		if err := json.Unmarshal(body, &results); err != nil {
			t.Fatalf("decode response body: %v; raw: %s", err, body)
		}
		if len(results) != len(dashboard.Dashboards) {
			t.Fatalf("len(results) = %d, want %d", len(results), len(dashboard.Dashboards))
		}

		var raw []map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("decode response body as raw keys: %v; raw: %s", err, body)
		}
		for i, obj := range raw {
			assertJSONKeys(t, obj, dashboardListItemJSONKeys, fmt.Sprintf("result[%d]", i))
		}

		for i, want := range dashboard.Dashboards {
			got := results[i]
			if got.ID != want.ID {
				t.Errorf("result[%d].ID = %q, want %q (registry order must be preserved)", i, got.ID, want.ID)
			}
			if got.DisplayName != want.DisplayName {
				t.Errorf("result[%d].DisplayName = %q, want %q", i, got.DisplayName, want.DisplayName)
			}
			if got.IsDefault != want.IsDefault {
				t.Errorf("result[%d].IsDefault = %v, want %v", i, got.IsDefault, want.IsDefault)
			}
			if got.IsTeamBased != want.IsTeamBased {
				t.Errorf("result[%d].IsTeamBased = %v, want %v", i, got.IsTeamBased, want.IsTeamBased)
			}
		}

		wantTeamBased := map[string]bool{"team_performance": true, "abt": true}
		teamBasedCount := 0
		for _, res := range results {
			if res.IsTeamBased {
				teamBasedCount++
				if !wantTeamBased[res.ID] {
					t.Errorf("unexpected team-based dashboard %q, want one of team_performance/abt", res.ID)
				}
			}
		}
		if teamBasedCount != len(wantTeamBased) {
			t.Errorf("teamBasedCount = %d, want exactly %d (team_performance, abt)", teamBasedCount, len(wantTeamBased))
		}

		defaultCount := 0
		for _, res := range results {
			if res.IsDefault {
				defaultCount++
				if res.ID != "agents_pilot" {
					t.Errorf("unexpected default dashboard %q, want agents_pilot", res.ID)
				}
			}
		}
		if defaultCount != 1 {
			t.Errorf("default dashboard count = %d, want 1", defaultCount)
		}
	})
}

// TestAllDashboardsHaveWidgets is the "no more mock/empty placeholders"
// guarantee: every dashboard in the registry now has real widgets.
func TestAllDashboardsHaveWidgets(t *testing.T) {
	if len(dashboard.Dashboards) != 6 {
		t.Fatalf("len(dashboard.Dashboards) = %d, want 6", len(dashboard.Dashboards))
	}
	for _, d := range dashboard.Dashboards {
		if len(d.Widgets) == 0 {
			t.Errorf("dashboard %q has no widgets, want at least 1", d.ID)
		}
	}
}

func TestGetDashboardDetail(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/agents_pilot", nil), "agents_pilot")
		w := httptest.NewRecorder()
		h.GetDashboardDetail(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("unknown dashboard id returns 404", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/bogus", nil), "bogus"))
		w := httptest.NewRecorder()
		h.GetDashboardDetail(w, r)
		assertStatus(t, w, http.StatusNotFound)
		assertErrorMessage(t, w, ErrMsgNotFound)
	})

	t.Run("agents_pilot returns metadata and its four widgets", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/agents_pilot", nil), "agents_pilot"))
		w := httptest.NewRecorder()
		h.GetDashboardDetail(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		body := w.Body.Bytes()
		t.Logf("GET /dashboards/agents_pilot response: %s", body)

		// Decode into the real production type (dashboardDetailView, defined
		// in dashboards.go), not a duplicate ad hoc struct — a JSON tag
		// rename on the real type breaks this decode/assertions directly,
		// instead of silently zero-valuing a field in a copy that has
		// already drifted from what's actually returned.
		var result dashboardDetailView
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode response body: %v; raw: %s", err, body)
		}

		if result.ID != "agents_pilot" {
			t.Errorf("ID = %q, want %q", result.ID, "agents_pilot")
		}
		if result.DisplayName != "Engineer overview" {
			t.Errorf("DisplayName = %q, want %q", result.DisplayName, "Engineer overview")
		}
		if !result.IsDefault {
			t.Errorf("IsDefault = %v, want true", result.IsDefault)
		}
		if result.TargetTeam != "cs_engineers" {
			t.Errorf("TargetTeam = %q, want %q", result.TargetTeam, "cs_engineers")
		}
		if len(result.Widgets) != 4 {
			t.Fatalf("len(result.Widgets) = %d, want 4", len(result.Widgets))
		}

		// Confirm the actual top-level JSON keys match openapi.yaml's
		// declared Dashboard schema exactly.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("decode response body as raw keys: %v; raw: %s", err, body)
		}
		assertJSONKeys(t, raw, dashboardDetailJSONKeys, "response")

		// Confirm each widget's JSON keys match openapi.yaml's declared
		// DashboardWidget schema exactly (allowing the omitempty
		// groupBy/listLimit extras) — catches an added/removed field that
		// the struct decode above wouldn't (json.Unmarshal ignores unknown
		// keys and zero-values missing ones).
		var rawWidgets []map[string]json.RawMessage
		if err := json.Unmarshal(raw["widgets"], &rawWidgets); err != nil {
			t.Fatalf("decode widgets as raw keys: %v; raw: %s", err, raw["widgets"])
		}
		for i, obj := range rawWidgets {
			assertJSONKeysSuperset(t, obj, dashboardWidgetJSONKeys, fmt.Sprintf("widgets[%d]", i))
		}

		byID := make(map[string]int)
		for i, res := range result.Widgets {
			byID[res.WidgetID] = i
			if res.DisplayName == "" {
				t.Errorf("widget %s has empty displayName", res.WidgetID)
			}
		}

		wantResourceShape := map[string]struct {
			resourceType dashboard.ResourceType
			shape        dashboard.Shape
			gridWidth    int
		}{
			"my_patches":         {dashboard.ResourceCase, dashboard.ShapeCount, 3},
			"my_reminders":       {dashboard.ResourceCase, dashboard.ShapeCount, 3},
			"open_incident_team": {dashboard.ResourceCase, dashboard.ShapeCount, 3},
			"my_critical_open":   {dashboard.ResourceCase, dashboard.ShapeList, 3},
		}
		for id, want := range wantResourceShape {
			idx, ok := byID[id]
			if !ok {
				t.Fatalf("missing widget %q in response", id)
			}
			got := result.Widgets[idx]
			if got.ResourceType != want.resourceType {
				t.Errorf("widget %s resourceType = %q, want %q", id, got.ResourceType, want.resourceType)
			}
			if got.Shape != want.shape {
				t.Errorf("widget %s shape = %q, want %q", id, got.Shape, want.shape)
			}
			if got.GridWidth != want.gridWidth {
				t.Errorf("widget %s gridWidth = %d, want %d", id, got.GridWidth, want.gridWidth)
			}
		}

		if idx := byID["my_critical_open"]; result.Widgets[idx].ListLimit != 5 {
			t.Errorf("widget my_critical_open listLimit = %d, want 5", result.Widgets[idx].ListLimit)
		}

		for _, id := range []string{"my_patches", "my_reminders"} {
			idx, ok := byID[id]
			if !ok {
				t.Fatalf("missing widget %q in response", id)
			}
			filters := result.Widgets[idx].Filters
			assigned, present := filterValuesByField(filters, "assignedUserId")
			if !present {
				t.Fatalf("widget %s filters has no assignedUserId field entry", id)
			}
			if len(assigned) != 1 || assigned[0] != resolvedCurrentUserID {
				t.Errorf("widget %s assignedUserId values = %v, want [%q]", id, assigned, resolvedCurrentUserID)
			}
			for _, uid := range assigned {
				if uid == "__current_user__" {
					t.Errorf("widget %s assignedUserId values leaked the unresolved placeholder", id)
				}
			}
		}

		// open_incident_team has no assignedUserId filter entry in its
		// template and must not gain one during substitution:
		// substituteCurrentUser only rewrites values already present, it
		// never adds entries.
		teamIdx, ok := byID["open_incident_team"]
		if !ok {
			t.Fatalf("missing widget %q in response", "open_incident_team")
		}
		teamFilters := result.Widgets[teamIdx].Filters
		if v, present := filterValuesByField(teamFilters, "assignedUserId"); present {
			t.Errorf("widget open_incident_team filters unexpectedly has an assignedUserId field entry: %v", v)
		}

		// my_critical_open DOES carry an assignedUserId filter (the current
		// user's critical/high cases) — verify it resolved cleanly.
		criticalIdx, ok := byID["my_critical_open"]
		if !ok {
			t.Fatalf("missing widget %q in response", "my_critical_open")
		}
		assigned, present := filterValuesByField(result.Widgets[criticalIdx].Filters, "assignedUserId")
		if !present {
			t.Fatalf("widget my_critical_open filters has no assignedUserId field entry")
		}
		if len(assigned) != 1 || assigned[0] != resolvedCurrentUserID {
			t.Errorf("widget my_critical_open assignedUserId values = %v, want [%q]", assigned, resolvedCurrentUserID)
		}
	})

	t.Run("operations dashboard has three resource-type-diverse widgets", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/operations", nil), "operations"))
		w := httptest.NewRecorder()
		h.GetDashboardDetail(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		body := w.Body.Bytes()

		var result dashboardDetailView
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode response body: %v; raw: %s", err, body)
		}
		if result.ID != "operations" {
			t.Errorf("ID = %q, want %q", result.ID, "operations")
		}
		if result.TargetTeam != "cs_operations" {
			t.Errorf("TargetTeam = %q, want %q", result.TargetTeam, "cs_operations")
		}
		if len(result.Widgets) != 3 {
			t.Fatalf("len(result.Widgets) = %d, want 3", len(result.Widgets))
		}

		byID := make(map[string]dashboardWidgetView)
		for _, wd := range result.Widgets {
			byID[wd.WidgetID] = wd
		}

		wantTypes := map[string]dashboard.ResourceType{
			"p0_p1_open":              dashboard.ResourceCase,
			"open_critical_incidents": dashboard.ResourceIncident,
			"crs_awaiting_approval":   dashboard.ResourceChangeRequest,
		}
		for id, wantType := range wantTypes {
			got, ok := byID[id]
			if !ok {
				t.Fatalf("missing widget %q in response", id)
			}
			if got.ResourceType != wantType {
				t.Errorf("widget %s resourceType = %q, want %q", id, got.ResourceType, wantType)
			}
		}

		incident, ok := byID["open_critical_incidents"]
		if !ok {
			t.Fatalf("missing widget %q in response", "open_critical_incidents")
		}
		prioritiesRaw, present := incident.Filters["priorities"]
		if !present {
			t.Fatalf("open_critical_incidents filters has no priorities key: %v", incident.Filters)
		}
		priorities, ok := prioritiesRaw.([]any)
		if !ok || len(priorities) != 2 || priorities[0] != "CRITICAL" || priorities[1] != "HIGH" {
			t.Errorf("open_critical_incidents filters.priorities = %v, want [CRITICAL HIGH] unmodified", prioritiesRaw)
		}
	})

	t.Run("security dashboard's product_vulnerability widget has a scalar string filter", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/security", nil), "security"))
		w := httptest.NewRecorder()
		h.GetDashboardDetail(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		body := w.Body.Bytes()
		t.Logf("GET /dashboards/security response: %s", body)

		var result dashboardDetailView
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode response body: %v; raw: %s", err, body)
		}
		if len(result.Widgets) != 3 {
			t.Fatalf("len(result.Widgets) = %d, want 3", len(result.Widgets))
		}

		byID := make(map[string]dashboardWidgetView)
		for _, wd := range result.Widgets {
			byID[wd.WidgetID] = wd
		}

		critical, ok := byID["critical_vulns"]
		if !ok {
			t.Fatalf("missing widget %q in response", "critical_vulns")
		}
		if critical.ResourceType != dashboard.ResourceProductVulnerability {
			t.Errorf("critical_vulns resourceType = %q, want %q", critical.ResourceType, dashboard.ResourceProductVulnerability)
		}
		priority, present := critical.Filters["priority"]
		if !present {
			t.Fatalf("critical_vulns filters has no priority key: %v", critical.Filters)
		}
		if s, ok := priority.(string); !ok || s != "critical" {
			t.Errorf("critical_vulns filters.priority = %v (%T), want string %q", priority, priority, "critical")
		}
	})

	t.Run("team_performance's pie widget resolves description, slices, and per-slice current-user placeholders", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/team_performance", nil), "team_performance"))
		w := httptest.NewRecorder()
		h.GetDashboardDetail(w, r)

		assertStatus(t, w, http.StatusOK)
		body := w.Body.Bytes()
		t.Logf("GET /dashboards/team_performance response: %s", body)

		var result dashboardDetailView
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode response body: %v; raw: %s", err, body)
		}

		byID := make(map[string]dashboardWidgetView)
		for _, wd := range result.Widgets {
			byID[wd.WidgetID] = wd
		}
		pie, ok := byID["cases_by_severity"]
		if !ok {
			t.Fatalf("missing widget %q in response", "cases_by_severity")
		}
		if pie.Description != "Share of active cases at each severity level." {
			t.Errorf("cases_by_severity Description = %q, want the configured subtitle", pie.Description)
		}
		if len(pie.Slices) != 2 {
			t.Fatalf("len(cases_by_severity.Slices) = %d, want 2", len(pie.Slices))
		}

		var critical, mine *dashboardPieSliceView
		for i := range pie.Slices {
			switch pie.Slices[i].Label {
			case "Critical":
				critical = &pie.Slices[i]
			case "Mine":
				mine = &pie.Slices[i]
			}
		}
		if critical == nil {
			t.Fatalf("missing the %q slice in cases_by_severity.Slices", "Critical")
		}
		if critical.Color != "error" {
			t.Errorf("Critical slice Color = %q, want %q", critical.Color, "error")
		}
		if _, present := filterValuesByField(critical.Filters, "state"); present {
			t.Errorf("Critical slice Filters must not carry the widget's own base filters, got %v", critical.Filters)
		}

		if mine == nil {
			t.Fatalf("missing the %q slice in cases_by_severity.Slices", "Mine")
		}
		assigned, ok := filterValuesByField(mine.Filters, "assignedUserId")
		if !ok || len(assigned) != 1 || assigned[0] != resolvedCurrentUserID {
			t.Errorf("Mine slice assignedUserId = %v, want [%q] (resolved, not the raw placeholder)", assigned, resolvedCurrentUserID)
		}

		// Confirm the wire keys match the updated openapi.yaml schema.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("decode response body as raw keys: %v; raw: %s", err, body)
		}
		var rawWidgets []map[string]json.RawMessage
		if err := json.Unmarshal(raw["widgets"], &rawWidgets); err != nil {
			t.Fatalf("decode widgets as raw keys: %v; raw: %s", err, raw["widgets"])
		}
		var rawPie map[string]json.RawMessage
		for _, obj := range rawWidgets {
			var id string
			if err := json.Unmarshal(obj["widgetId"], &id); err == nil && id == "cases_by_severity" {
				rawPie = obj
			}
		}
		if rawPie == nil {
			t.Fatalf("cases_by_severity not found among raw widgets: %v", rawWidgets)
		}
		assertJSONKeysSuperset(t, rawPie, append(append([]string(nil), dashboardWidgetJSONKeys...), "description", "slices"), "cases_by_severity")
	})

	t.Run("team_performance's incident_wow widget carries its configured section, unset for widgets with no section", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/team_performance", nil), "team_performance"))
		w := httptest.NewRecorder()
		h.GetDashboardDetail(w, r)

		assertStatus(t, w, http.StatusOK)
		var result dashboardDetailView
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode response body: %v; raw: %s", err, w.Body.Bytes())
		}
		byID := make(map[string]dashboardWidgetView)
		for _, wd := range result.Widgets {
			byID[wd.WidgetID] = wd
		}

		wow, ok := byID["incident_wow"]
		if !ok {
			t.Fatalf("missing widget %q in response", "incident_wow")
		}
		if wow.Section != "SLA Violation" {
			t.Errorf("incident_wow.Section = %q, want %q", wow.Section, "SLA Violation")
		}

		teamOpenCases, ok := byID["team_open_cases"]
		if !ok {
			t.Fatalf("missing widget %q in response", "team_open_cases")
		}
		if teamOpenCases.Section != "" {
			t.Errorf("team_open_cases.Section = %q, want empty (no section configured)", teamOpenCases.Section)
		}
	})

	t.Run("every dashboard in the registry now has at least one widget", func(t *testing.T) {
		h := NewDashboardHandler(&mockEntityUserClient{})
		for _, d := range dashboard.Dashboards {
			r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/"+d.ID, nil), d.ID))
			w := httptest.NewRecorder()
			h.GetDashboardDetail(w, r)
			assertStatus(t, w, http.StatusOK)

			var result dashboardDetailView
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatalf("dashboard %s: decode response body: %v; raw: %s", d.ID, err, w.Body.Bytes())
			}
			if len(result.Widgets) == 0 {
				t.Errorf("dashboard %s has 0 widgets in the response, want > 0", d.ID)
			}
		}
	})
}

// TestResolveCurrentUserID_UsesEntityUsersMeNotJWTClaim guards against the
// regression this task shipped once already: substituting the raw JWT
// userid claim into a widget's assignedUserIds instead of the platform user
// id GET /users/me resolves via the entity service. The two ids are
// deliberately different here (testUser.UserID vs the mock's GetUserMe id)
// so a reversion back to user.UserID would fail this test immediately
// rather than silently reintroducing the bug.
func TestResolveCurrentUserID_UsesEntityUsersMeNotJWTClaim(t *testing.T) {
	const entityResolvedID = "22222222-2222-2222-2222-222222222222"
	if entityResolvedID == testUser.UserID {
		t.Fatal("test setup bug: entityResolvedID must differ from testUser.UserID")
	}

	mock := &mockEntityUserClient{
		getUserMeFn: func(ctx context.Context) ([]byte, error) {
			return []byte(`{"id":"` + entityResolvedID + `"}`), nil
		},
	}
	h := NewDashboardHandler(mock)
	r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/agents_pilot", nil), "agents_pilot"))
	w := httptest.NewRecorder()
	h.GetDashboardDetail(w, r)
	assertStatus(t, w, http.StatusOK)

	var result dashboardDetailView
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response body: %v; raw: %s", err, w.Body.Bytes())
	}
	byID := make(map[string]dashboardWidgetView)
	for _, wd := range result.Widgets {
		byID[wd.WidgetID] = wd
	}

	assigned, ok := filterValuesByField(byID["my_patches"].Filters, "assignedUserId")
	if !ok || len(assigned) != 1 {
		t.Fatalf("my_patches assignedUserId values = %v, want a 1-element array", assigned)
	}
	if assigned[0] != entityResolvedID {
		t.Errorf("my_patches assignedUserId values[0] = %v, want the entity-resolved id %q (not the JWT claim %q)",
			assigned[0], entityResolvedID, testUser.UserID)
	}
}

// TestResolveCurrentUserID_FallsBackToJWTClaimOnEntityError confirms a
// transient entity-service failure degrades to the previous (broken but
// non-crashing) behavior — the endpoint still returns 200, not a 500 — since
// dashboards are best-effort convenience data, not core functionality.
func TestResolveCurrentUserID_FallsBackToJWTClaimOnEntityError(t *testing.T) {
	mock := &mockEntityUserClient{
		getUserMeFn: func(ctx context.Context) ([]byte, error) {
			return nil, errors.New("entity unavailable")
		},
	}
	h := NewDashboardHandler(mock)
	r := withUser(withDashboardID(httptest.NewRequest(http.MethodGet, "/dashboards/agents_pilot", nil), "agents_pilot"))
	w := httptest.NewRecorder()
	h.GetDashboardDetail(w, r)
	assertStatus(t, w, http.StatusOK)

	var result dashboardDetailView
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response body: %v; raw: %s", err, w.Body.Bytes())
	}
	byID := make(map[string]dashboardWidgetView)
	for _, wd := range result.Widgets {
		byID[wd.WidgetID] = wd
	}
	assigned, ok := filterValuesByField(byID["my_patches"].Filters, "assignedUserId")
	if !ok || len(assigned) != 1 || assigned[0] != testUser.UserID {
		t.Errorf("my_patches assignedUserId values = %v, want the JWT-claim fallback [%q]", assigned, testUser.UserID)
	}
}
