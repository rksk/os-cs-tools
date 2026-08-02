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

import { Box, Button, Typography } from "@wso2/oxygen-ui";
import { ArrowLeft } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";
import { Link as RouterLink, useLocation, useParams } from "react-router";
import DirectoryMembersList, {
  type DirectoryMemberFilterKey,
} from "@features/csm-admin/components/DirectoryMembersList";

interface DirectoryMemberPageProps {
  filterKey: DirectoryMemberFilterKey;
  /** Singular noun, e.g. "role". */
  entityNoun: string;
  /** Where the "Back" link and the breadcrumb send the user, e.g. "/admin/roles". */
  listPath: string;
  /** Plural label for the breadcrumb, e.g. "Roles". */
  listLabel: string;
}

/**
 * Shared shell for a role/group/team's member page: the entity's name
 * (carried as router state by the directory row's link — see
 * `DirectoryEntityTable` — falling back to the raw id for a direct/shared
 * link that arrived without it) plus the member list itself. One shell, three
 * thin per-entity pages (`RoleMembersPage`, `GroupMembersPage`,
 * `TeamMembersPage`) so each still has its own route component to test the
 * filter key against.
 */
export default function DirectoryMemberPage({
  filterKey,
  entityNoun,
  listPath,
  listLabel,
}: DirectoryMemberPageProps): JSX.Element {
  const { id } = useParams<{ id: string }>();
  const location = useLocation();
  const state = location.state as { name?: string } | null;
  const name = state?.name ?? id ?? "";

  if (!id) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        <Typography variant="h5">Not found</Typography>
        <Typography variant="body2" color="text.secondary">
          No {entityNoun} id was given.
        </Typography>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
      <Button
        component={RouterLink}
        to={listPath}
        variant="text"
        size="small"
        startIcon={<ArrowLeft size={16} />}
        sx={{ alignSelf: "flex-start" }}
      >
        Back to {listLabel}
      </Button>

      <Box>
        <Typography variant="h5">{name}</Typography>
        <Typography variant="body2" color="text.secondary">
          Members of this {entityNoun}
        </Typography>
      </Box>

      <DirectoryMembersList filterKey={filterKey} entityId={id} entityNoun={entityNoun} />
    </Box>
  );
}
