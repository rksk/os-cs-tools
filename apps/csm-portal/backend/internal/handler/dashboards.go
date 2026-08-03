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
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/dashboard"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
)

// dashboardPieSliceView is one wedge of a Shape "pie" widget — see
// dashboard.PieSlice. Query is this slice's own criteria only (already
// __current_user__-resolved), meant to be merged under the parent widget's
// own (also resolved) Query by the caller.
type dashboardPieSliceView struct {
	Label string         `json:"label"`
	Color string         `json:"color,omitempty"`
	Query map[string]any `json:"query"`
}

// dashboardWidgetView is a single widget's filter criteria and display
// metadata, returned as part of GET /dashboards/{dashboardId}. The caller
// resolves each widget's own data by issuing its own POST /{resourceType}s/search
// request (see ResourceType), passing Query as that request's filters.
type dashboardWidgetView struct {
	WidgetID     string                  `json:"widgetId"`
	DisplayName  string                  `json:"displayName"`
	Description  string                  `json:"description,omitempty"`
	ResourceType dashboard.ResourceType  `json:"resourceType"`
	Shape        dashboard.Shape         `json:"shape"`
	GridWidth    int                     `json:"gridWidth"`
	Query        map[string]any          `json:"query"`
	GroupBy      string                  `json:"groupBy,omitempty"`
	ListLimit    int                     `json:"listLimit,omitempty"`
	Slices       []dashboardPieSliceView `json:"slices,omitempty"`
	Section      string                  `json:"section,omitempty"`
}

// dashboardListItemView is a dashboard's list-level metadata, returned by
// GET /dashboards. IsTeamBased is included here (not just on the detail
// view) so the frontend can decide whether to show a team selector for the
// currently-selected dashboard without waiting on a second fetch.
type dashboardListItemView struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	IsDefault   bool   `json:"isDefault"`
	IsTeamBased bool   `json:"isTeamBased"`
}

// dashboardDetailView is a dashboard's full metadata plus its resolved
// widgets, returned by GET /dashboards/{dashboardId}.
type dashboardDetailView struct {
	ID          string                `json:"id"`
	DisplayName string                `json:"displayName"`
	IsDefault   bool                  `json:"isDefault"`
	TargetTeam  string                `json:"targetTeam"`
	IsTeamBased bool                  `json:"isTeamBased"`
	Widgets     []dashboardWidgetView `json:"widgets"`
}

// DashboardHandler handles HTTP requests for the config-driven dashboard
// widget pilot.
type DashboardHandler struct {
	entity entityUserClient
}

// NewDashboardHandler creates a DashboardHandler backed by the given entity
// client, used to resolve the caller's own platform user id (see
// resolveCurrentUserID) for widgets whose filters need it.
func NewDashboardHandler(entity entityUserClient) *DashboardHandler {
	return &DashboardHandler{entity: entity}
}

// resolveCurrentUserID returns the caller's platform user id — the same id
// GET /users/me resolves via the entity service — for substituting
// dashboard.CurrentUserPlaceholder into widget filters.
//
// This is deliberately NOT user.UserID from the JWT: that claim is whatever
// identity value the gateway/IdP embeds (e.g. the Asgardeo subject), which is
// a different id than the platform's own SN/Postgres-backed user record.
// Using the JWT claim directly here was the actual bug behind an
// "identity-mapping gap" this task had, until now, treated as an accepted
// ServiceNow DEV environment limitation: /cases/search correctly rejected
// that id with "no active user found for sys_id ..." because it was never a
// valid sys_id to begin with. Falls back to the JWT claim (rather than an
// empty string) only if the entity lookup itself fails, so a transient
// entity-service error degrades to the previous (broken but non-crashing)
// behavior instead of a hard failure.
func (h *DashboardHandler) resolveCurrentUserID(r *http.Request, user *middleware.UserInfo) string {
	raw, err := h.entity.GetUserMe(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetUserMe failed while resolving dashboard current-user id", "userID", user.UserID, "err", err)
		return user.UserID
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &me); err != nil {
		slog.ErrorContext(r.Context(), "entity GetUserMe: parse response failed while resolving dashboard current-user id", "userID", user.UserID, "err", err)
		return user.UserID
	}
	if me.ID == "" {
		slog.ErrorContext(r.Context(), "entity GetUserMe returned an empty id while resolving dashboard current-user id", "userID", user.UserID)
		return user.UserID
	}
	return me.ID
}

// GetDashboards handles GET /dashboards.
func (h *DashboardHandler) GetDashboards(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	views := make([]dashboardListItemView, 0, len(dashboard.Dashboards))
	for _, d := range dashboard.Dashboards {
		views = append(views, dashboardListItemView{
			ID:          d.ID,
			DisplayName: d.DisplayName,
			IsDefault:   d.IsDefault,
			IsTeamBased: d.IsTeamBased,
		})
	}

	writeJSONValue(w, http.StatusOK, views)
}

// GetDashboardDetail handles GET /dashboards/{dashboardId}.
func (h *DashboardHandler) GetDashboardDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	dashboardID := r.PathValue("dashboardId")
	d, ok := dashboard.DashboardByID(dashboardID)
	if !ok {
		writeError(w, http.StatusNotFound, ErrMsgNotFound)
		return
	}

	currentUserID := h.resolveCurrentUserID(r, user)

	widgets := make([]dashboardWidgetView, 0, len(d.Widgets))
	for _, tpl := range d.Widgets {
		var slices []dashboardPieSliceView
		if len(tpl.Slices) > 0 {
			slices = make([]dashboardPieSliceView, 0, len(tpl.Slices))
			for _, slice := range tpl.Slices {
				slices = append(slices, dashboardPieSliceView{
					Label: slice.Label,
					Color: slice.Color,
					Query: dashboard.ResolveSliceFilters(slice, currentUserID),
				})
			}
		}
		widgets = append(widgets, dashboardWidgetView{
			WidgetID:     tpl.ID,
			DisplayName:  tpl.DisplayName,
			Description:  tpl.Description,
			ResourceType: tpl.ResourceType,
			Shape:        tpl.Shape,
			GridWidth:    tpl.GridWidth,
			Query:        dashboard.ResolveFilters(tpl, currentUserID),
			GroupBy:      tpl.GroupBy,
			ListLimit:    tpl.ListLimit,
			Slices:       slices,
			Section:      tpl.Section,
		})
	}

	writeJSONValue(w, http.StatusOK, dashboardDetailView{
		ID:          d.ID,
		DisplayName: d.DisplayName,
		IsDefault:   d.IsDefault,
		TargetTeam:  d.TargetTeam,
		IsTeamBased: d.IsTeamBased,
		Widgets:     widgets,
	})
}
