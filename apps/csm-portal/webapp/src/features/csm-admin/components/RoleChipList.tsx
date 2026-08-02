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

import { Chip, Stack } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { useNavigate } from "react-router";
import DirectoryEntityChip from "@features/csm-admin/components/DirectoryEntityChip";
import { INTERNAL_USER_ROLES } from "@features/csm-users/types/csmUsers";

// Roles beyond this many collapse into a single "+N more" chip that links to
// the user's profile — a table cell isn't the place to enumerate every role a
// user carries. This is the single implementation both the user list and every
// role/group/team member list use; duplicating this logic per page is exactly
// how it previously went missing from the member-list pages while the user
// list got it.
const MAX_VISIBLE_ROLES = 3;

interface RoleChipListProps {
  /** Role ids/keys this user holds. */
  roleIds: string[];
  /** Role id -> display name, e.g. from `useSearchRoles`. Falls back to the
   * raw id when a name isn't known (or the lookup hasn't loaded yet). */
  roleNameById: Map<string, string>;
  /** The user these roles belong to, so the overflow chip has somewhere to
   * send the viewer to see the rest. */
  userId: string;
  userLabel: string;
}

/**
 * A user's roles as clickable chips (each linking to that role's directory
 * page), capped at {@link MAX_VISIBLE_ROLES} with a "+N more" overflow chip
 * linking to the user's own profile. Renders "—" when the user has no roles.
 */
export default function RoleChipList({
  roleIds,
  roleNameById,
  userId,
  userLabel,
}: RoleChipListProps): JSX.Element {
  const navigate = useNavigate();

  if (roleIds.length === 0) {
    return <>—</>;
  }

  const visibleRoles = roleIds.slice(0, MAX_VISIBLE_ROLES);
  const hiddenRoleCount = Math.max(roleIds.length - MAX_VISIBLE_ROLES, 0);
  const profilePath = `/people/${encodeURIComponent(userId)}`;

  return (
    <Stack direction="row" spacing={0.5} sx={{ flexWrap: "wrap", gap: 0.5 }}>
      {visibleRoles.map((r) => (
        <DirectoryEntityChip
          key={r}
          id={r}
          name={roleNameById.get(r) ?? r}
          routeBase="/admin/roles"
          color={(INTERNAL_USER_ROLES as string[]).includes(r) ? "primary" : "default"}
        />
      ))}
      {hiddenRoleCount > 0 && (
        <Chip
          size="small"
          variant="outlined"
          label={`+${hiddenRoleCount} more`}
          clickable
          onClick={(e) => {
            e.stopPropagation();
            navigate(profilePath);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.stopPropagation();
            }
          }}
          aria-label={`View all ${roleIds.length} roles for ${userLabel}`}
        />
      )}
    </Stack>
  );
}
