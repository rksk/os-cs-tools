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

package csmclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DedupTag returns the exact, stable tag internal/handler.MapToIncident
// embeds in every CreateIncidentRequest.Subject it builds, keyed off the
// buffered alert row's own primary-key id (internal/idgen, assigned before
// the row is persisted — see internal/store.Store.Enqueue's doc comment).
//
// Format: "[alert:<row-id>]", e.g. "[alert:1b9d6bcd-bbfd-4b2d-9b5d-ab8dfbbd4bed]".
// This exact string is what internal/worker's pre-retry dedup check
// (SearchIncidentByTag) later searches for via SearchIncidentsFilters.SearchQuery
// to find an incident a previous, lost-response attempt may already have
// created. Changing this format is a breaking change for any row already
// buffered with the old tag baked into its persisted payload — don't change
// it without a migration plan for in-flight rows.
func DedupTag(alertID string) string {
	return "[alert:" + alertID + "]"
}

// SearchIncidentsFilters is the filter subset of entity-service's own
// SearchIncidentsFilters (internal/domain/entity.go) this service actually
// sends. The real upstream type carries more optional fields (Priorities,
// ParentIDs, Number, a generic Filters array) — this service only ever
// needs free-text SearchQuery for the dedup check, so the rest are left
// zero-valued/omitted rather than modeled here.
type SearchIncidentsFilters struct {
	SearchQuery string `json:"searchQuery"`
}

// IncidentSort mirrors entity-service's IncidentSort. Left zero-valued by
// SearchIncidentByTag — sort order doesn't matter when Pagination.Limit is 1
// and any match at all is treated the same way.
type IncidentSort struct {
	Field string `json:"field,omitempty"`
	Order string `json:"order,omitempty"`
}

// Pagination mirrors entity-service's Pagination.
type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// SearchIncidentsRequest is the request body for csm-integration-service's
// POST /incidents/search — a thin proxy of entity-service's own
// SearchIncidentsRequest, proxied as-is (field names/JSON tags copied
// verbatim), matching the same convention CreateIncidentRequest already
// follows in this package.
type SearchIncidentsRequest struct {
	Filters    SearchIncidentsFilters `json:"filters"`
	SortBy     IncidentSort           `json:"sortBy"`
	Pagination Pagination             `json:"pagination"`
}

// searchIncidentView is the subset of entity-service's SearchIncidentView
// this service actually reads out of a search hit — enough to record
// against the buffered alert row exactly like a fresh CreateIncident result
// would be (see CreateIncidentResult).
type searchIncidentView struct {
	ID     *string `json:"id"`
	Number *string `json:"number"`
}

// searchIncidentsResponse is the response body for POST /incidents/search,
// decoded tolerantly (unknown fields ignored), matching
// createIncidentResponse's convention in incidents.go.
type searchIncidentsResponse struct {
	Incidents []searchIncidentView `json:"incidents"`
	Total     int                  `json:"total"`
	Offset    int                  `json:"offset"`
	Limit     int                  `json:"limit"`
}

// SearchIncidentByTag calls POST /incidents/search on csm-integration-service
// with searchQuery=tag and Pagination{Limit: 1}, and reports whether a
// matching incident already exists.
//
// This is the pre-retry dedup check internal/worker runs before ever
// retrying a delivery that already failed once: a failed POST /incidents
// call does not prove the incident wasn't actually created on the far side
// (the response could have been lost to a timeout or connection reset), so
// blindly retrying risks creating a duplicate. See internal/worker.attempt's
// doc comment for the full call site and the fail-open behavior on a search
// error.
//
// Known limitation: like CreateIncident, this endpoint is ServiceNow-backed
// and requires a forwarded end-user identity token this stack cannot
// currently supply, so it also 401s on every call today (see this package's
// doc comment and this service's README/CLAUDE.md). That means the dedup
// check this method exists to support is not yet actually effective in
// production — every retry today will get a search error here and proceed
// to attempt creation anyway (fail-open, by design — see internal/worker).
// This method is structurally correct and ready for when that
// infrastructure gap is closed; it does not itself work around it.
func (c *Client) SearchIncidentByTag(ctx context.Context, tag string) (*CreateIncidentResult, bool, error) {
	req := SearchIncidentsRequest{
		Filters:    SearchIncidentsFilters{SearchQuery: tag},
		Pagination: Pagination{Limit: 1, Offset: 0},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, false, fmt.Errorf("csmclient: marshal SearchIncidentsRequest: %w", err)
	}

	respBody, err := c.do(ctx, http.MethodPost, "/incidents/search", body)
	if err != nil {
		return nil, false, err
	}

	var resp searchIncidentsResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, false, fmt.Errorf("csmclient: decode SearchIncidentByTag response: %w", err)
	}

	if len(resp.Incidents) == 0 {
		return nil, false, nil
	}

	hit := resp.Incidents[0]
	result := &CreateIncidentResult{}
	if hit.ID != nil {
		result.IncidentID = *hit.ID
	}
	if hit.Number != nil {
		result.IncidentNumber = *hit.Number
	}
	return result, true, nil
}
