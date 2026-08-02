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

package service

import (
	"context"
	"sort"
	"strings"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// teamService serves the team registry.
//
// The registry itself is deployment configuration installed at startup -- team names are
// organisation vocabulary and are deliberately not hardcoded in this repo. A team's id is
// its registry key rather than the backing group's id, because those ids differ between
// environments while the key does not.
type teamService struct{}

// NewTeamService constructs a TeamService.
func NewTeamService() TeamService {
	return &teamService{}
}

func (s *teamService) SearchTeams(
	_ context.Context, req domain.SearchTeamsRequest,
) (domain.SearchTeamsResponse, error) {
	registry := domain.AbtTeams()

	teams := make([]domain.Team, 0, len(registry))
	for _, t := range registry {
		teams = append(teams, domain.Team{
			ID:     t.TeamKey,
			Name:   t.DisplayName,
			Family: string(t.Family),
		})
	}
	sort.Slice(teams, func(i, j int) bool { return teams[i].Name < teams[j].Name })

	if q := strings.TrimSpace(req.Filters.SearchQuery); q != "" {
		needle := strings.ToLower(q)
		filtered := make([]domain.Team, 0, len(teams))
		for _, t := range teams {
			if strings.Contains(strings.ToLower(t.Name), needle) ||
				strings.Contains(strings.ToLower(t.ID), needle) {
				filtered = append(filtered, t)
			}
		}
		teams = filtered
	}

	total := len(teams)
	offset, limit, length := clampCatalogPagination(req.Pagination, total)

	return domain.SearchTeamsResponse{
		Teams:  teams[offset : offset+length],
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}
