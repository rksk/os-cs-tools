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
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/viewer"
)

// entityAccountClient abstracts the entity service account operations used by AccountHandler.
type entityAccountClient interface {
	GetAccount(ctx context.Context, id string) ([]byte, error)
	SearchAccounts(ctx context.Context, body []byte) ([]byte, error)
	SearchAccountContacts(ctx context.Context, accountID string, body []byte) ([]byte, error)
	UpdateAccountTeams(ctx context.Context, id string, body []byte) ([]byte, error)
}

// AccountHandler handles HTTP requests for account operations, delegating to the
// entity service for data access.
type AccountHandler struct {
	entity entityAccountClient
}

// NewAccountHandler creates an AccountHandler backed by the given entity client.
func NewAccountHandler(entity entityAccountClient) *AccountHandler {
	return &AccountHandler{entity: entity}
}

// GetAccount handles GET /accounts/{id}.
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	result, err := h.entity.GetAccount(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetAccount failed", "userID", user.UserID, "accountID", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve account.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SearchAccounts handles POST /accounts/search.
func (h *AccountHandler) SearchAccounts(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.SearchAccounts(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchAccounts failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to search accounts.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SearchAccountContacts handles POST /accounts/{id}/contacts/search.
// The endpoint is path-scoped, so the request body is capped and forwarded to the
// entity service as-is (no fields are injected) and the response is returned verbatim.
func (h *AccountHandler) SearchAccountContacts(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if len(body) > 0 && !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.SearchAccountContacts(r.Context(), id, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchAccountContacts failed", "userID", user.UserID, "accountID", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to search account contacts.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// UpdateAccountTeams handles PATCH /accounts/{id}: updates an account's CRE
// team and/or SRE team assignment. The endpoint is path-scoped, so the
// request body is capped and forwarded to the entity service as-is (no
// fields are injected) and the response is returned verbatim. Restricted to
// callers holding the "admin" role — enforced by the PermAdmin permission
// this route is registered with (see cmd/server/main.go), not by this
// handler; every route's access is decided at registration, not inline.
func (h *AccountHandler) UpdateAccountTeams(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.UpdateAccountTeams(r.Context(), id, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity UpdateAccountTeams failed", "userID", user.UserID, "accountID", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to update account teams.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// viewerAccountClient abstracts the account-scoped escalation read used by
// ViewerAccountHandler. GetAccounts/GetAccountByID/GetProjectsByAccount used
// to live here too -- removed in favor of calling CS Portal's own
// GET /accounts/{id}, POST /accounts/search, and POST /projects/search
// (filtered by accountId) directly, since the viewer audience reads the
// same entity-service data those routes already serve. The account-scoped
// escalation list has no shared route (CS Portal's own
// /cases/{id}/escalations is per-case), so it stays here, backed by
// entity-service (see accounts_postgres.go).
type viewerAccountClient interface {
	GetEscalationsByAccount(ctx context.Context, accountNumber string, offset, limit int) ([]viewer.EscalationDetail, error)
}

// ViewerAccountHandler handles HTTP requests for the viewer audience's
// account-escalation list. Reading and listing accounts/projects goes
// through CS Portal's own /accounts and /projects routes directly.
type ViewerAccountHandler struct {
	escalations viewerAccountClient
	accessGuard *AccessGuard
}

// NewViewerAccountHandler creates a ViewerAccountHandler.
func NewViewerAccountHandler(escalations viewerAccountClient, accessGuard *AccessGuard) *ViewerAccountHandler {
	return &ViewerAccountHandler{escalations: escalations, accessGuard: accessGuard}
}

// maxPaginationLimit bounds "limit" on every paginated viewer route: the
// value is forwarded as the upstream page size, so an unbounded value lets
// a caller force this backend to buffer an arbitrarily large response in
// memory.
const maxPaginationLimit = 100

// parsePaginationParams parses required, non-negative "offset" and
// positive, maxPaginationLimit-bounded "limit" query params, matching the
// Ballerina resource functions' non-nilable int offset/'limit params
// (framework-rejected on missing/invalid there; validated explicitly here
// for the same effect).
func parsePaginationParams(w http.ResponseWriter, r *http.Request) (offset, limit int, ok bool) {
	q := r.URL.Query()
	offset, err := strconv.Atoi(q.Get("offset"))
	if err != nil || offset < 0 {
		writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
		return 0, 0, false
	}
	limit, err = strconv.Atoi(q.Get("limit"))
	if err != nil || limit < 1 || limit > maxPaginationLimit {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("limit must be an integer between 1 and %d", maxPaginationLimit))
		return 0, 0, false
	}
	return offset, limit, true
}

func optionalQueryParam(r *http.Request, key string) *string {
	if !r.URL.Query().Has(key) {
		return nil
	}
	v := r.URL.Query().Get(key)
	if v == "" {
		return nil
	}
	return &v
}

// GetAccountEscalations handles GET /accounts/{accountId}/escalations.
func (h *ViewerAccountHandler) GetAccountEscalations(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}

	accountID := r.PathValue("accountId")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	offset, limit, ok := parsePaginationParams(w, r)
	if !ok {
		return
	}

	result, err := h.escalations.GetEscalationsByAccount(r.Context(), accountID, offset, limit)
	if err != nil {
		if errors.Is(err, viewer.ErrAccountNotFound) {
			writeError(w, http.StatusNotFound, ErrMsgNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "GetEscalationsByAccount failed", "userID", user.UserID, "accountID", accountID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve account escalations.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}
