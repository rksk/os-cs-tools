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
	"io"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
)

// entityReferenceClient abstracts the entity service reference-data operations: the role
// catalogue and the team registry, both of which back the user-directory filters.
type entityReferenceClient interface {
	SearchRoles(ctx context.Context, body []byte) ([]byte, error)
	SearchTeams(ctx context.Context, body []byte) ([]byte, error)
}

// ReferenceHandler handles HTTP requests for the role catalogue and team registry.
type ReferenceHandler struct {
	entity entityReferenceClient
}

// NewReferenceHandler creates a ReferenceHandler backed by the given entity client.
func NewReferenceHandler(entity entityReferenceClient) *ReferenceHandler {
	return &ReferenceHandler{entity: entity}
}

// SearchRoles handles POST /roles/search.
func (h *ReferenceHandler) SearchRoles(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "SearchRoles", "Failed to search roles.", h.entity.SearchRoles)
}

// SearchTeams handles POST /teams/search.
func (h *ReferenceHandler) SearchTeams(w http.ResponseWriter, r *http.Request) {
	h.forward(w, r, "SearchTeams", "Failed to search teams.", h.entity.SearchTeams)
}

// forward carries the shared read-body / validate / passthrough sequence for both search
// endpoints. They differ only in which client call they make and what they are called in
// logs, so the sequence lives in one place rather than being duplicated per endpoint.
func (h *ReferenceHandler) forward(
	w http.ResponseWriter,
	r *http.Request,
	op string,
	failureMsg string,
	call func(ctx context.Context, body []byte) ([]byte, error),
) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	// Both endpoints accept an absent body, meaning "no filters, default page". Only a
	// non-empty body has to be valid JSON.
	if len(body) > 0 && !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := call(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity "+op+" failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, failureMsg)
		return
	}

	writeJSON(w, http.StatusOK, result)
}
