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
	"fmt"
	"sort"
	"strings"
)

// entityProductsClient is the subset of internal/entity.CustomerEntityClient
// needed to list products and ABT teams from Postgres.
type entityProductsClient interface {
	SearchProducts(ctx context.Context, body []byte) ([]byte, error)
	SearchTeams(ctx context.Context, body []byte) ([]byte, error)
}

type entitySearchProductsRequest struct {
	Pagination entityPagination `json:"pagination"`
}

type entityProduct struct {
	Name string `json:"name"`
}

type entitySearchProductsResponse struct {
	Products []entityProduct `json:"products"`
	Total    int             `json:"total"`
}

// postgresLookupsClient implements lookupsClient entirely against
// entity-service (Postgres) — no ServiceNow dependency at all. GetProductList
// reads entity-service's own productService. GetABTTeamList used to delegate
// to ServiceNow's sys_user_group table (children of a "CS_INT_CRT_LK" group),
// but that hierarchy was never mirrored into entity-service's "group" table
// (checked directly: every row's parent_id is NULL). The equivalent data
// does exist, just under a different shape: entity-service's own `team`
// table (the hand-curated CSM_TEAM_REGISTRY, already exposed via
// POST /teams/search) tags each ABT team's own row with a `type` ending in
// "-abt" (e.g. "Apollo sre-abt", "Atlas cre-abt") rather than a separate
// name/hierarchy -- see GetABTTeamList below for the exact filter.
type postgresLookupsClient struct {
	entity entityProductsClient
}

// NewPostgresLookupsClient builds a postgresLookupsClient. entity is
// the same *entity.CustomerEntityClient every other CS Portal handler uses.
func NewPostgresLookupsClient(entity entityProductsClient) *postgresLookupsClient {
	return &postgresLookupsClient{entity: entity}
}

// entityProductsPageLimit is entity-service's own hard cap (confirmed
// against a real running instance: a limit above 50 is rejected outright
// with "limit cannot exceed 50", the same global cap normalizePagination
// enforces everywhere else in that service) — unlike the ServiceNow-backed
// implementation, which fetches up to 500 products in one page, this must
// page through in batches of 50. productListPageCap bounds the number of
// pages fetched (25 * 50 = 1250 products) purely as a runaway-loop safety
// net, not an expected real limit.
const (
	entityProductsPageLimit = 50
	productListPageCap      = 25
)

// GetProductList implements lookupsClient.
func (c *postgresLookupsClient) GetProductList(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	products := make([]string, 0, entityProductsPageLimit)

	for page := 0; page < productListPageCap; page++ {
		body, err := json.Marshal(entitySearchProductsRequest{
			Pagination: entityPagination{Limit: entityProductsPageLimit, Offset: page * entityProductsPageLimit},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal entity-service products request: %w", err)
		}
		raw, err := c.entity.SearchProducts(ctx, body)
		if err != nil {
			return nil, err
		}
		var resp entitySearchProductsResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal entity-service products response: %w", err)
		}

		// Trim/dedupe/sort keeps the response shape the webapp has always
		// consumed: distinct, non-empty, sorted product names.
		for _, p := range resp.Products {
			name := strings.TrimSpace(p.Name)
			if name != "" && !seen[name] {
				seen[name] = true
				products = append(products, name)
			}
		}

		if len(resp.Products) < entityProductsPageLimit || (page+1)*entityProductsPageLimit >= resp.Total {
			break
		}
	}

	sort.Strings(products)
	return products, nil
}

type entitySearchTeamsRequest struct {
	Pagination entityPagination `json:"pagination"`
}

type entityTeam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type entitySearchTeamsResponse struct {
	Teams []entityTeam `json:"teams"`
	Total int          `json:"total"`
}

// entityTeamsPageLimit mirrors entityProductsPageLimit -- entity-service
// caps every search's limit at 50, and the team registry is small (17 rows
// in the environment this was checked against), so one page covers it in
// practice; the loop below still pages through properly rather than
// assuming that stays true.
const (
	entityTeamsPageLimit = 50
	teamListPageCap      = 25
)

// GetABTTeamList implements lookupsClient. "ABT team" has no name or
// hierarchy of its own in entity-service's `team` table -- it's a `type`
// value ending in "-abt" (checked directly: "Apollo sre-abt", "Atlas
// cre-abt", "Castor cre-abt", "Draco cre-abt", "Phoenix cre-abt", "Rigel
// cre-abt", "Sirius cre-abt", "Vega cre-abt" were the ABT rows in the
// environment this was checked against, alongside plain "cre"/
// "cre-leadership" rows that are not). Returns each team's bare name (e.g.
// "Apollo"), stripping the trailing " <type>" suffix the `name` column
// itself does not carry on its own -- confirmed: `name` is already just
// "Apollo", `type` is the separate "sre-abt"/"cre-abt" column.
func (c *postgresLookupsClient) GetABTTeamList(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	teams := make([]string, 0, entityTeamsPageLimit)

	for page := 0; page < teamListPageCap; page++ {
		body, err := json.Marshal(entitySearchTeamsRequest{
			Pagination: entityPagination{Limit: entityTeamsPageLimit, Offset: page * entityTeamsPageLimit},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal entity-service teams request: %w", err)
		}
		raw, err := c.entity.SearchTeams(ctx, body)
		if err != nil {
			return nil, err
		}
		var resp entitySearchTeamsResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal entity-service teams response: %w", err)
		}

		for _, t := range resp.Teams {
			if !strings.HasSuffix(strings.ToLower(t.Type), "-abt") {
				continue
			}
			name := strings.TrimSpace(t.Name)
			if name != "" && !seen[name] {
				seen[name] = true
				teams = append(teams, name)
			}
		}

		if len(resp.Teams) < entityTeamsPageLimit || (page+1)*entityTeamsPageLimit >= resp.Total {
			break
		}
	}

	sort.Strings(teams)
	return teams, nil
}
