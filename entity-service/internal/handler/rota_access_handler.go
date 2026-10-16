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
	"net/http"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// RotaAccessHandler serves the Team Schedule's Rota access tab: who holds a
// rota admin role or is a team's rota marker, and granting or removing either.
// The service decides who may; grants are PUT and removals DELETE, both
// idempotent, so a retried click lands where the first one meant to.
type RotaAccessHandler struct {
	svc service.RotaAccessService
}

// NewRotaAccessHandler constructs a RotaAccessHandler.
func NewRotaAccessHandler(svc service.RotaAccessService) *RotaAccessHandler {
	return &RotaAccessHandler{svc: svc}
}

// GetRotaAccess handles GET /team-schedule/rota-access.
func (h *RotaAccessHandler) GetRotaAccess(w http.ResponseWriter, r *http.Request) {
	resp, err := h.svc.Get(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeScheduleJSON(w, http.StatusOK, resp)
}

// GrantRotaAdmin handles PUT /team-schedule/rota-access/admins/{family}/{userId}.
func (h *RotaAccessHandler) GrantRotaAdmin(w http.ResponseWriter, r *http.Request) {
	h.done(w, r, h.svc.GrantRotaAdmin(r.Context(), r.PathValue("family"), r.PathValue("userId")))
}

// RevokeRotaAdmin handles DELETE /team-schedule/rota-access/admins/{family}/{userId}.
func (h *RotaAccessHandler) RevokeRotaAdmin(w http.ResponseWriter, r *http.Request) {
	h.done(w, r, h.svc.RevokeRotaAdmin(r.Context(), r.PathValue("family"), r.PathValue("userId")))
}

// GrantRotaMarker handles PUT /team-schedule/rota-access/markers/{teamKey}/{userId}.
func (h *RotaAccessHandler) GrantRotaMarker(w http.ResponseWriter, r *http.Request) {
	h.done(w, r, h.svc.GrantRotaMarker(r.Context(), r.PathValue("teamKey"), r.PathValue("userId")))
}

// RevokeRotaMarker handles DELETE /team-schedule/rota-access/markers/{teamKey}/{userId}.
func (h *RotaAccessHandler) RevokeRotaMarker(w http.ResponseWriter, r *http.Request) {
	h.done(w, r, h.svc.RevokeRotaMarker(r.Context(), r.PathValue("teamKey"), r.PathValue("userId")))
}

// done answers a grant or removal: 204, or the service's refusal.
func (h *RotaAccessHandler) done(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
