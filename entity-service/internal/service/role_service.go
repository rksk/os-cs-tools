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

// roleService serves the platform's assignable-role catalogue.
//
// The catalogue is this layer's own curated list, not a query against the backing data
// source. That source carries dozens of roles that ship with the product and mean nothing
// to a CS engineer, and their ids differ per environment. The keys here are stable, are
// exactly what the user search accepts in roleIds, and come from the same configured
// allow-list that validates that filter -- so the dropdown and the filter can never
// disagree.
type roleService struct{}

// NewRoleService constructs a RoleService.
func NewRoleService() RoleService {
	return &roleService{}
}

func (s *roleService) SearchRoles(
	_ context.Context, req domain.SearchRolesRequest,
) (domain.SearchRolesResponse, error) {
	catalogue := UserRoleCatalogue()
	roles := make([]domain.Role, 0, len(catalogue))
	for _, key := range catalogue {
		roles = append(roles, domain.Role{ID: string(key), Name: roleDisplayName(string(key))})
	}
	// Configuration order is whatever the deployer typed; sort so paging is stable.
	sort.Slice(roles, func(i, j int) bool { return roles[i].ID < roles[j].ID })

	if q := strings.TrimSpace(req.Filters.SearchQuery); q != "" {
		needle := strings.ToLower(q)
		filtered := make([]domain.Role, 0, len(roles))
		for _, r := range roles {
			if strings.Contains(strings.ToLower(r.ID), needle) ||
				strings.Contains(strings.ToLower(r.Name), needle) {
				filtered = append(filtered, r)
			}
		}
		roles = filtered
	}

	total := len(roles)
	offset, limit, length := clampCatalogPagination(req.Pagination, total)

	return domain.SearchRolesResponse{
		Roles:  roles[offset : offset+length],
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}, nil
}

// roleDisplayName turns a role key into something readable, so callers do not have to
// hand-maintain their own label map.
func roleDisplayName(key string) string {
	words := strings.Split(key, "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// clampCatalogPagination bounds an offset/limit pair against a known total, for the
// in-memory catalogues. It returns the offset, the effective page size to echo back to the
// caller, and a length that is always safe to slice with. The two differ on the last page:
// echoing the length as the limit would tell a caller advancing by limit that the page size
// shrank, which every other search in this package does not do.
func clampCatalogPagination(p domain.Pagination, total int) (offset, limit, length int) {
	limit = p.Limit
	if limit <= 0 {
		limit = catalogDefaultLimit
	}
	if limit > catalogMaxLimit {
		limit = catalogMaxLimit
	}

	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}

	length = limit
	if offset+length > total {
		length = total - offset
	}
	return offset, limit, length
}

const (
	catalogDefaultLimit = 50
	catalogMaxLimit     = 200
)
