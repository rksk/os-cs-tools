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
	"log/slog"
	"net/http"
)

// lookupsClient abstracts the dropdown lookups used by LookupsHandler.
type lookupsClient interface {
	GetProductList(ctx context.Context) ([]string, error)
	GetABTTeamList(ctx context.Context) ([]string, error)
}

// LookupsHandler handles HTTP requests for the viewer audience's small
// standalone dropdown lookups, delegating to entity-service.
type LookupsHandler struct {
	lookups     lookupsClient
	accessGuard *AccessGuard
}

// NewLookupsHandler creates a LookupsHandler. accessGuard enforces
// PermViewerAccess, the viewer audience's blanket gate.
func NewLookupsHandler(lookups lookupsClient, accessGuard *AccessGuard) *LookupsHandler {
	return &LookupsHandler{lookups: lookups, accessGuard: accessGuard}
}

// GetProducts handles GET /products.
func (h *LookupsHandler) GetProducts(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}

	products, err := h.lookups.GetProductList(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "GetProductList failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve products.")
		return
	}

	writeJSONValue(w, http.StatusOK, products)
}

// GetABTTeams handles GET /abt-teams.
func (h *LookupsHandler) GetABTTeams(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}

	teams, err := h.lookups.GetABTTeamList(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "GetABTTeamList failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve ABT teams.")
		return
	}

	writeJSONValue(w, http.StatusOK, teams)
}
