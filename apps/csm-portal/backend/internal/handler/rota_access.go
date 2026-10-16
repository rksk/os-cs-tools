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
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
)

// entityRotaAccessClient is the entity-service calls the Team Schedule's Rota
// access tab needs. Its own interface rather than more methods on
// entityScheduleClient, so nothing that fakes the schedule client changes.
type entityRotaAccessClient interface {
	GetRotaAccess(ctx context.Context) ([]byte, error)
	PutRotaAdmin(ctx context.Context, family, userID string) ([]byte, error)
	DeleteRotaAdmin(ctx context.Context, family, userID string) ([]byte, error)
	PutRotaMarker(ctx context.Context, teamKey, userID string) ([]byte, error)
	DeleteRotaMarker(ctx context.Context, teamKey, userID string) ([]byte, error)
}

// RotaAccessHandler proxies the Rota access tab: who holds a rota admin role
// (one family each) or is a team's rota marker, and granting or removing
// either. Who may is entity-service's decision -- a CSM admin for rota admin,
// a CSM admin or the family's rota admin for a team's marker -- so this checks
// only the shape of the request and forwards the caller's identity.
type RotaAccessHandler struct {
	entity entityRotaAccessClient
}

// NewRotaAccessHandler creates a RotaAccessHandler.
func NewRotaAccessHandler(entity entityRotaAccessClient) *RotaAccessHandler {
	return &RotaAccessHandler{entity: entity}
}

var (
	rotaFamilyRe  = regexp.MustCompile(`(?i)^(cre|sre|sme)$`)
	rotaTeamKeyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
)

// GetRotaAccess handles GET /team-schedule/rota-access. Somebody who may
// change nothing gets an empty answer, which is how the page hides the tab.
func (h *RotaAccessHandler) GetRotaAccess(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}
	result, err := h.entity.GetRotaAccess(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetRotaAccess failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to load rota access.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// PutRotaAdmin handles PUT /team-schedule/rota-access/admins/{family}/{userId}.
func (h *RotaAccessHandler) PutRotaAdmin(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "family", rotaFamilyRe, "PutRotaAdmin", "Failed to grant rota admin.", h.entity.PutRotaAdmin)
}

// DeleteRotaAdmin handles DELETE /team-schedule/rota-access/admins/{family}/{userId}.
func (h *RotaAccessHandler) DeleteRotaAdmin(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "family", rotaFamilyRe, "DeleteRotaAdmin", "Failed to remove rota admin.", h.entity.DeleteRotaAdmin)
}

// PutRotaMarker handles PUT /team-schedule/rota-access/markers/{teamKey}/{userId}.
func (h *RotaAccessHandler) PutRotaMarker(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "teamKey", rotaTeamKeyRe, "PutRotaMarker", "Failed to add the rota marker.", h.entity.PutRotaMarker)
}

// DeleteRotaMarker handles DELETE /team-schedule/rota-access/markers/{teamKey}/{userId}.
func (h *RotaAccessHandler) DeleteRotaMarker(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, "teamKey", rotaTeamKeyRe, "DeleteRotaMarker", "Failed to remove the rota marker.", h.entity.DeleteRotaMarker)
}

// change checks one grant or removal's path, forwards it, and answers 204.
//
// A refusal keeps entity-service's own reason -- "only a CSM admin can grant
// or remove rota admin", "that person is not on lyra" -- so the tab can say
// why rather than only that nothing changed. Anything else stays generic.
func (h *RotaAccessHandler) change(w http.ResponseWriter, r *http.Request, scopeName string, scopeRe *regexp.Regexp,
	op, fallbackMsg string, call func(ctx context.Context, scope, userID string) ([]byte, error)) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}
	scope, target := r.PathValue(scopeName), r.PathValue("userId")
	if !scopeRe.MatchString(scope) || !uuidRe.MatchString(target) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	if _, err := call(r.Context(), scope, target); err != nil {
		slog.ErrorContext(r.Context(), "entity "+op+" failed", "userID", user.UserID, "err", err)
		var apiErr *apierror.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
				writeError(w, apiErr.StatusCode, upstreamErrorMessageStrict(apiErr.Body, fallbackMsg))
				return
			}
		}
		mapUpstreamErrorGeneric(w, err, fallbackMsg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
