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
	"net/http"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// CaseAlertSettingsHandler serves the settings section of the Team Schedule's
// Alert Settings tab -- chat rooms, ServiceNow group routes and ladder values --
// and the two routes csm-notification-service uses at start-up. The service
// decides who may; this only moves JSON.
type CaseAlertSettingsHandler struct {
	svc service.CaseAlertSettingsService
}

// NewCaseAlertSettingsHandler constructs a CaseAlertSettingsHandler.
func NewCaseAlertSettingsHandler(svc service.CaseAlertSettingsService) *CaseAlertSettingsHandler {
	return &CaseAlertSettingsHandler{svc: svc}
}

func writeCaseAlertJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// GetSettings handles GET /team-schedule/case-alerts/settings.
func (h *CaseAlertSettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Get(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCaseAlertJSON(w, http.StatusOK, resp)
}

// SaveRoom handles PUT /team-schedule/case-alerts/settings/rooms.
func (h *CaseAlertSettingsHandler) SaveRoom(w http.ResponseWriter, r *http.Request) {
	var req domain.SaveCaseAlertRoomRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	room, err := h.svc.SaveRoom(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCaseAlertJSON(w, http.StatusOK, room)
}

// SetWebhook handles PUT /team-schedule/case-alerts/settings/rooms/{roomId}/webhook.
func (h *CaseAlertSettingsHandler) SetWebhook(w http.ResponseWriter, r *http.Request) {
	var req domain.SetCaseAlertWebhookRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	room, err := h.svc.SetWebhook(r.Context(), r.PathValue("roomId"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCaseAlertJSON(w, http.StatusOK, room)
}

// DeleteRoom handles DELETE /team-schedule/case-alerts/settings/rooms/{roomId}.
func (h *CaseAlertSettingsHandler) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRoom(r.Context(), r.PathValue("roomId")); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SendTestCard handles POST /team-schedule/case-alerts/settings/rooms/{roomId}/test.
func (h *CaseAlertSettingsHandler) SendTestCard(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.SendTestCard(r.Context(), r.PathValue("roomId"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCaseAlertJSON(w, http.StatusOK, resp)
}

// SaveRoute handles PUT /team-schedule/case-alerts/settings/routes.
func (h *CaseAlertSettingsHandler) SaveRoute(w http.ResponseWriter, r *http.Request) {
	var req domain.SaveCaseAlertRouteRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	route, err := h.svc.SaveRoute(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCaseAlertJSON(w, http.StatusOK, route)
}

// DeleteRoute handles DELETE /team-schedule/case-alerts/settings/routes/{routeId}.
func (h *CaseAlertSettingsHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteRoute(r.Context(), r.PathValue("routeId")); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SaveValues handles PUT /team-schedule/case-alerts/settings/values.
func (h *CaseAlertSettingsHandler) SaveValues(w http.ResponseWriter, r *http.Request) {
	var req domain.SaveCaseAlertValuesRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SaveValues(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCaseAlertJSON(w, http.StatusOK, resp)
}

// DeleteValue handles DELETE /team-schedule/case-alerts/settings/values/{valueId}.
func (h *CaseAlertSettingsHandler) DeleteValue(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteValue(r.Context(), r.PathValue("valueId")); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetRuntime handles GET /team-schedule/case-alerts/settings/runtime: for
// csm-notification-service only, with the webhook URLs opened.
func (h *CaseAlertSettingsHandler) GetRuntime(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Runtime(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeCaseAlertJSON(w, http.StatusOK, resp)
}

// RecordApplied handles PUT /team-schedule/case-alerts/settings/applied.
func (h *CaseAlertSettingsHandler) RecordApplied(w http.ResponseWriter, r *http.Request) {
	var req domain.CaseAlertAppliedReport
	if !decodeRequest(w, r, &req) {
		return
	}
	if err := h.svc.RecordApplied(r.Context(), req); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
