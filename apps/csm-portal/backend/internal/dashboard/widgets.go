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

// Package dashboard holds the pilot's config-driven dashboard widget
// templates. Each widget resolves to a search against that ResourceType's own
// /search endpoint (every resource's search payload shape is
// {filters: {...}, pagination: {...}}) — there is no generic filter DSL and
// no database backing this; the registry itself is loaded at process startup
// from a directory of per-dashboard JSON files (DASHBOARDS_DIR — see LoadDir
// and Registry in registry.go), or from the deprecated DASHBOARDS_CONFIG
// environment variable when no directory is set (see ParseDashboardsConfig).
// Both are wired up in cmd/server/main.go.
package dashboard

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// CurrentUserPlaceholder marks a filter value that must be resolved to the
// requesting user's id before the filters are sent upstream. It never
// reaches the entity service: ResolveFilters always substitutes it.
const CurrentUserPlaceholder = "__current_user__"

// ResourceType identifies which resource a widget's filters search against.
type ResourceType string

const (
	ResourceCase                 ResourceType = "case"
	ResourceIncident             ResourceType = "incident"
	ResourceChangeRequest        ResourceType = "change_request"
	ResourceAccount              ResourceType = "account"
	ResourceProject              ResourceType = "project"
	ResourceUser                 ResourceType = "user"
	ResourceTimeCard             ResourceType = "time_card"
	ResourceProblem              ResourceType = "problem"
	ResourceProductVulnerability ResourceType = "product_vulnerability"
)

// Shape is how a widget's resolved data should be rendered.
type Shape string

const (
	ShapeCount Shape = "count" // single resolved number
	ShapeList  Shape = "list"  // top-N matching records
	ShapePie   Shape = "pie"   // one search per Slices entry, each resolved via its own total — see PieSlice
	ShapeBar   Shape = "bar"   // same resolution as ShapePie (one search per Slices entry); differs only in how the frontend renders the resolved data
)

// PieSlice is one wedge of a Shape "pie" widget. The caller resolves its
// value by issuing that widget's own ResourceType's /search with Query
// merged under the widget's own base Query (this slice's keys win on
// conflict) and pagination.limit=1, reading total off the response — the
// exact same mechanism Shape "count" uses, just once per slice.
type PieSlice struct {
	Label string `json:"label"`
	// Color is a palette key ("primary", "secondary", "success", "error",
	// "info", "warning") the frontend already uses elsewhere in this system
	// (see WidgetTemplate's own icon color convention on the frontend) — not
	// validated here, forwarded verbatim. Falls back to a fixed rotation over
	// the same palette on the frontend if omitted.
	Color string `json:"color,omitempty"`
	// Query is this slice's own search criteria (see WidgetTemplate.Query).
	Query map[string]any `json:"query"`

	// legacyFilters holds a pre-rename config's "filters" key, moved into
	// Query by migrateLegacyWidgetKeys. See WidgetTemplate.legacyFilters.
	legacyFilters map[string]any
}

// UnmarshalJSON decodes a PieSlice, accepting both the current "query" key
// and the deprecated "filters" key it replaced (see
// WidgetTemplate.UnmarshalJSON for why).
func (s *PieSlice) UnmarshalJSON(data []byte) error {
	type alias PieSlice
	var raw struct {
		alias
		LegacyFilters map[string]any `json:"filters"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = PieSlice(raw.alias)
	s.legacyFilters = raw.LegacyFilters
	return nil
}

// WidgetTemplate is resource-agnostic: Query is opaque JSON, forwarded
// verbatim (after __current_user__ substitution) as the filters object of
// that ResourceType's own /search payload (every resource's search payload
// shape is {filters: {...}, pagination: {...}}). The BE never interprets
// filter contents beyond substituting the current-user placeholder and
// migrating deprecated key names (see migrateLegacyWidgetKeys).
type WidgetTemplate struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	// Description is an explanatory subtitle shown under DisplayName —
	// config-owned text, not hardcoded per ResourceType/Shape on the
	// frontend.
	Description  string         `json:"description,omitempty"`
	ResourceType ResourceType   `json:"resourceType"`
	Shape        Shape          `json:"shape"`
	GridWidth    int            `json:"gridWidth"` // 1-12, CSS grid columns out of 12
	Query        map[string]any `json:"query"`
	GroupBy      string         `json:"groupBy,omitempty"`   // unused
	ListLimit    int            `json:"listLimit,omitempty"` // only meaningful for Shape list; how many records to show
	Slices       []PieSlice     `json:"slices,omitempty"`    // only meaningful for Shape pie/bar; one search per slice
	// Section groups widgets sharing the same (non-empty) value under a
	// titled sub-section within the dashboard, in the order that value
	// first appears among the dashboard's widgets — e.g. a handful of
	// "count" widgets all set to Section: "SLA Violation" render together
	// under that heading, separately from the dashboard's other widgets.
	// Widgets with no Section (the common case) render in one untitled
	// group, exactly as before this field existed.
	Section string `json:"section,omitempty"`

	// legacyFilters holds a pre-rename config's "filters" key so
	// migrateLegacyWidgetKeys can move it into Query. Unexported so it can
	// never be re-emitted on the wire: the deprecated shape is accepted on
	// input only.
	legacyFilters map[string]any
}

// UnmarshalJSON decodes a WidgetTemplate, accepting both the current "query"
// key and the deprecated "filters" key it replaced. Deployed environments
// carry DASHBOARDS_CONFIG in an env var, so a rename here is not atomic with
// a config rollout — and encoding/json silently leaves an unknown key's field
// at its zero value, which would give every widget a nil Query and render 0
// everywhere with no error at all. See migrateLegacyWidgetKeys, which does
// the actual move plus the deprecation warning (it has the dashboard/widget
// ids this method does not).
func (w *WidgetTemplate) UnmarshalJSON(data []byte) error {
	type alias WidgetTemplate
	var raw struct {
		alias
		LegacyFilters map[string]any `json:"filters"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*w = WidgetTemplate(raw.alias)
	w.legacyFilters = raw.LegacyFilters
	return nil
}

// Dashboard is a single dashboard's metadata plus its widget templates.
type Dashboard struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	// Type classifies the dashboard's audience (see Type). It drives the
	// frontend's automatic dashboard selection from the caller's team
	// family. It deliberately does not replace IsDefault or IsTeamBased --
	// all three coexist -- so the three can be set to states that
	// contradict each other; validate rejects those at load time.
	Type      Type `json:"type,omitempty"`
	IsDefault bool `json:"isDefault"`
	// TargetTeam is purely descriptive metadata (e.g. for a future FE team
	// picker); it is not enforced anywhere. GET /dashboards still returns
	// every dashboard to every caller regardless of team membership.
	TargetTeam string `json:"targetTeam"`
	// IsTeamBased marks a dashboard whose FE view should offer a team
	// selector (populated from POST /teams/search) alongside the dashboard
	// switcher. This is currently UI skeleton only: selecting a team does
	// not yet scope any widget's data. Wiring a selected team into widget
	// filters (e.g. resolving its member user IDs into a case widget's
	// assignedUserIds) is deliberately deferred to a later increment.
	IsTeamBased bool             `json:"isTeamBased"`
	Widgets     []WidgetTemplate `json:"widgets"`
}

// ParseDashboardsConfig decodes DASHBOARDS_CONFIG, a JSON array of Dashboard
// objects (see the Dashboard and WidgetTemplate json tags for the expected
// shape).
//
// DASHBOARDS_CONFIG is DEPRECATED in favour of DASHBOARDS_DIR (see LoadDir),
// and is honoured only when no directory is configured. Cramming every
// dashboard into one environment variable makes a definition unreviewable in
// a diff and gives an error nothing to name; a directory of per-dashboard
// files gives both back.
//
// An empty value yields no dashboards and no error — a deployment with
// neither setting must still start. Anything else that fails is an error the
// caller is expected to make fatal: this used to log and return nil, which
// meant one stray character silently emptied every dashboard in the product
// with a single log line to show for it.
//
// Cross-field validation is the same as the directory loader's, except that
// "type" is not required here: values already deployed in this variable
// predate the field. A definition without one gets a warning and is simply
// invisible to automatic dashboard selection.
func ParseDashboardsConfig(raw string) ([]Dashboard, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	slog.Warn("DASHBOARDS_CONFIG is deprecated; move each dashboard into its own JSON file and set DASHBOARDS_DIR instead")

	var dashboards []Dashboard
	if err := json.Unmarshal([]byte(raw), &dashboards); err != nil {
		return nil, fmt.Errorf("DASHBOARDS_CONFIG: parse: %w", err)
	}

	loaded := make([]sourced, 0, len(dashboards))
	for i, d := range dashboards {
		loaded = append(loaded, sourced{dashboard: d, source: fmt.Sprintf("DASHBOARDS_CONFIG[%d]", i)})
	}
	return finalize(loaded, false)
}

// migrateLegacyWidgetKeys upgrades a pre-rename DASHBOARDS_CONFIG in place,
// logging one deprecation warning per widget (or slice) it had to touch so a
// deployment still on the old shape is visible in the logs rather than
// silently working forever:
//
//   - widget/slice "filters"       -> "query"
//   - criteria "orGroups": [[..]]  -> "anyOf": [{"filters": [..]}]
//
// The new key always wins when both are present. Nothing here interprets
// filter contents beyond these two renames.
func migrateLegacyWidgetKeys(dashboards []Dashboard) {
	for di := range dashboards {
		d := &dashboards[di]
		for wi := range d.Widgets {
			w := &d.Widgets[wi]
			if w.Query == nil && w.legacyFilters != nil {
				slog.Warn(`DASHBOARDS_CONFIG: widget key "filters" is deprecated, rename it to "query"`,
					"dashboardId", d.ID, "widgetId", w.ID)
				w.Query = w.legacyFilters
			}
			w.legacyFilters = nil
			migrateLegacyCriteriaKeys(w.Query, d.ID, w.ID, "")
			for si := range w.Slices {
				s := &w.Slices[si]
				if s.Query == nil && s.legacyFilters != nil {
					slog.Warn(`DASHBOARDS_CONFIG: slice key "filters" is deprecated, rename it to "query"`,
						"dashboardId", d.ID, "widgetId", w.ID, "slice", s.Label)
					s.Query = s.legacyFilters
				}
				s.legacyFilters = nil
				migrateLegacyCriteriaKeys(s.Query, d.ID, w.ID, s.Label)
			}
		}
	}
}

// migrateLegacyCriteriaKeys rewrites the deprecated case-search "orGroups"
// key inside one criteria object into the current "anyOf" shape, in place.
// Each legacy branch was a bare array of filter predicates with implicit AND
// semantics; each current branch is an object carrying its own "filters"
// array. A branch that is already an object is passed through untouched, so
// a half-migrated config is not corrupted.
func migrateLegacyCriteriaKeys(query map[string]any, dashboardID, widgetID, slice string) {
	if query == nil {
		return
	}
	legacy, ok := query["orGroups"]
	if !ok {
		return
	}
	delete(query, "orGroups")
	if _, exists := query["anyOf"]; exists {
		return
	}
	branches, ok := legacy.([]any)
	if !ok {
		return
	}
	attrs := []any{"dashboardId", dashboardID, "widgetId", widgetID}
	if slice != "" {
		attrs = append(attrs, "slice", slice)
	}
	slog.Warn(`DASHBOARDS_CONFIG: criteria key "orGroups" is deprecated, rename it to "anyOf" and wrap each branch as {"filters": [...]}`, attrs...)
	migrated := make([]any, 0, len(branches))
	for _, branch := range branches {
		if arr, isArray := branch.([]any); isArray {
			migrated = append(migrated, map[string]any{"filters": arr})
			continue
		}
		migrated = append(migrated, branch)
	}
	query["anyOf"] = migrated
}

// ResolveFilters returns tpl's Query with CurrentUserPlaceholder substituted
// by currentUserID wherever it appears as a string inside a []any (the only
// place a per-user value belongs in a criteria object — e.g. assignedUserIds,
// userIds). It walks the object generically by key, so no criteria key name
// is hardcoded here. It does not mutate tpl.Query.
func ResolveFilters(tpl WidgetTemplate, currentUserID string) map[string]any {
	return substituteCurrentUser(tpl.Query, currentUserID).(map[string]any)
}

// ResolveSliceFilters is ResolveFilters' counterpart for one Shape "pie"
// slice: substitutes CurrentUserPlaceholder in slice.Query only. It
// deliberately does NOT merge in the widget's own base Query — the caller
// (frontend) merges this slice's query under the widget's own resolved
// Query itself (this slice's keys win on conflict), the same way it
// already merges any other per-slice override. Sending the two separately,
// rather than one pre-merged object per slice, avoids repeating the base
// criteria' JSON in every slice over the wire. Does not mutate slice.Query.
func ResolveSliceFilters(slice PieSlice, currentUserID string) map[string]any {
	return substituteCurrentUser(slice.Query, currentUserID).(map[string]any)
}

func substituteCurrentUser(v any, currentUserID string) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, sub := range val {
			out[k] = substituteCurrentUser(sub, currentUserID)
		}
		return out
	case []string:
		out := make([]string, len(val))
		for i, s := range val {
			if s == CurrentUserPlaceholder {
				s = currentUserID
			}
			out[i] = s
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, sub := range val {
			out[i] = substituteCurrentUser(sub, currentUserID)
		}
		return out
	case string:
		if val == CurrentUserPlaceholder {
			return currentUserID
		}
		return val
	default:
		return val
	}
}
