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
	"fmt"
	"strings"
	"sync"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// defaultUserRoles is the assignable-role allow-list used when none is
// configured.
//
// Unlike the team registry, roles DO get a committed default: these names are
// generic platform vocabulary (agent, admin, customer, ...) that is already
// public in this repo, not organisation-specific vocabulary, so committing
// them leaks nothing and a zero-config deployment behaves exactly as it did
// when the list was hardcoded. Adding a role is a configuration change, not a
// code change -- deliberately not a closed compile-time set.
var defaultUserRoles = []domain.UserRole{
	domain.UserRoleAgent,
	domain.UserRoleAdmin,
	domain.UserRoleCommenter,
	domain.UserRoleCustomer,
	domain.UserRoleCustomerAdmin,
	domain.UserRolePartner,
	domain.UserRolePartnerAdmin,
	domain.UserRoleInternal,
	domain.UserRoleExternal,
	domain.UserRoleTimecardApprover,
}

// The configured allow-list does double duty: it validates the roleIds user
// filter and it is the catalogue POST /roles/search serves. One list drives
// both, so the dropdown and the filter can never disagree. userRoles keeps
// the configured order for the catalogue response; validUserRoles is the
// same set as a map, kept in sync in SetUserRoles, so isValidUserRole is an
// O(1) lookup rather than a linear scan on every filtered search request.
var (
	userRolesMu    sync.RWMutex
	userRoles      = append([]domain.UserRole(nil), defaultUserRoles...)
	validUserRoles = toUserRoleSet(defaultUserRoles)
)

// toUserRoleSet builds a membership set from an ordered role list.
func toUserRoleSet(roles []domain.UserRole) map[domain.UserRole]bool {
	set := make(map[domain.UserRole]bool, len(roles))
	for _, r := range roles {
		set[r] = true
	}
	return set
}

// SetUserRoles installs the assignable-role allow-list. Called once during
// startup with the parsed configuration, and by tests.
func SetUserRoles(roles []domain.UserRole) {
	userRolesMu.Lock()
	defer userRolesMu.Unlock()
	userRoles = append([]domain.UserRole(nil), roles...)
	validUserRoles = toUserRoleSet(roles)
}

// UserRoleCatalogue returns the configured allow-list, in configured order.
func UserRoleCatalogue() []domain.UserRole {
	userRolesMu.RLock()
	defer userRolesMu.RUnlock()
	return append([]domain.UserRole(nil), userRoles...)
}

// isValidUserRole reports whether role is in the configured allow-list.
func isValidUserRole(role domain.UserRole) bool {
	userRolesMu.RLock()
	defer userRolesMu.RUnlock()
	return validUserRoles[role]
}

// ParseUserRoles parses the assignable-role allow-list from its configuration
// form: a comma-separated list of role names, whitespace around each trimmed.
// An empty string yields defaultUserRoles.
//
// A duplicate is an error rather than a silent de-duplication: it would double
// an entry in the role catalogue, and it is always a typo. Errors name the
// offending value so a bad deploy fails at startup, not at the first request.
func ParseUserRoles(raw string) ([]domain.UserRole, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]domain.UserRole(nil), defaultUserRoles...), nil
	}

	parts := strings.Split(raw, ",")
	roles := make([]domain.UserRole, 0, len(parts))
	seen := make(map[domain.UserRole]bool, len(parts))

	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			// Tolerate a trailing or doubled comma.
			continue
		}
		role := domain.UserRole(name)
		if seen[role] {
			return nil, fmt.Errorf("user role list: %q appears more than once", name)
		}
		seen[role] = true
		roles = append(roles, role)
	}

	if len(roles) == 0 {
		return nil, fmt.Errorf("user role list: no role names found in %q", raw)
	}
	return roles, nil
}
