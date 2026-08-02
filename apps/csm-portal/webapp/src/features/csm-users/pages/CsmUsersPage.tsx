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

import {
  Box,
  Checkbox,
  Chip,
  FormControl,
  InputLabel,
  ListItemText,
  MenuItem,
  OutlinedInput,
  Select,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TableSortLabel,
  TextField,
  Typography,
  type SelectChangeEvent,
} from "@wso2/oxygen-ui";
import { useMemo, useState, type ChangeEvent, type JSX, type KeyboardEvent } from "react";
import { useSearchParams } from "react-router";
import QueryErrorState from "@components/QueryErrorState";
import UserRefLink from "@components/UserRefLink";
import AsyncEntityMultiSelect from "@components/AsyncEntityMultiSelect";
import { useDebouncedValue } from "@hooks/useDebouncedValue";
import { useNavTransition } from "@hooks/useNavTransition";
import { useSearchGroups } from "@api/useSearchGroups";
import { useSearchUsers } from "@features/csm-users/api/useSearchUsers";
import { useSearchRoles } from "@features/csm-admin/api/useSearchRoles";
import { useSearchTeams } from "@features/csm-admin/api/useSearchTeams";
import DirectoryEntityChip from "@features/csm-admin/components/DirectoryEntityChip";
import {
  INTERNAL_USER_ROLES,
  type SearchUsersRequest,
  type UserSortField,
  type UserSortOrder,
} from "@features/csm-users/types/csmUsers";
import {
  readUsersFiltersFromUrl,
  writeUsersFiltersToUrl,
  type UsersFilters,
} from "@features/csm-users/utils/usersFiltersUrl";
import { BE_MAX_PAGE_LIMIT } from "@constants/apiConstants";
import type { BeGroup } from "@api/backend/types";

const DEFAULT_ROWS_PER_PAGE = 20;
// Top option is the backend's max page limit; larger requests are rejected.
const ROWS_PER_PAGE_OPTIONS = [10, 20, BE_MAX_PAGE_LIMIT];
// Roles beyond this many collapse into a single "+N more" chip that links to
// the user's profile — a table cell isn't the place to enumerate every role a
// user carries.
const MAX_VISIBLE_ROLES = 3;

/**
 * The users list, with filters reflected in the URL (`search`, `roles`,
 * `groups`, `teams`, `active`) so a filtered link is shareable and survives a
 * reload — the same `read*FiltersFromUrl` / `write*FiltersToUrl` convention
 * the cases list uses (`casesFiltersUrl.ts`). The free-text key is `search`,
 * not `q`: both this list and the cases list originally wrote it as `?q=`,
 * which collides with the app's QuickNav command palette (it treats `?q=` as
 * a one-shot deep link and pops open pre-filled with whatever's there) — keep
 * it `search` in any future change here, or that collision comes back.
 * Deliberately no project/account filter: "who is on this project" is
 * answered by the project-contacts search instead. Role, group and team
 * filters combine (AND together server-side).
 */
export default function CsmUsersPage(): JSX.Element {
  const navigate = useNavTransition();
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = useMemo(() => readUsersFiltersFromUrl(searchParams), [searchParams]);

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(DEFAULT_ROWS_PER_PAGE);
  const [sortField, setSortField] = useState<UserSortField>("name");
  const [sortOrder, setSortOrder] = useState<UserSortOrder>("asc");

  const setFilters = (next: UsersFilters): void => {
    setPage(0);
    setSearchParams(writeUsersFiltersToUrl(next), { replace: true });
  };

  const debouncedSearch = useDebouncedValue(filters.search, 300);

  // Role/team catalogues are small and curated, so one full-catalogue page is
  // enough to populate the picker (unlike groups, a live, potentially large
  // query against the backing data source — see the async group picker
  // below).
  const { data: rolesData } = useSearchRoles({ pagination: { limit: BE_MAX_PAGE_LIMIT } });
  const { data: teamsData } = useSearchTeams({ pagination: { limit: BE_MAX_PAGE_LIMIT } });
  const roles = useMemo(() => rolesData?.roles ?? [], [rolesData]);
  const teams = useMemo(() => teamsData?.teams ?? [], [teamsData]);
  const roleNameById = useMemo(
    () => new Map(roles.map((r) => [r.id, r.name])),
    [roles],
  );
  const teamNameById = useMemo(
    () => new Map(teams.map((t) => [t.id, t.name])),
    [teams],
  );

  const request = useMemo<SearchUsersRequest>(
    () => ({
      pagination: { limit: rowsPerPage, offset: page * rowsPerPage },
      filters: {
        ...(debouncedSearch.trim() && { searchQuery: debouncedSearch.trim() }),
        ...(filters.roleIds.length > 0 && { roleIds: filters.roleIds }),
        ...(filters.groupIds.length > 0 && { groupIds: filters.groupIds }),
        ...(filters.teamIds.length > 0 && { teamIds: filters.teamIds }),
        ...(filters.active !== "all" && { active: filters.active === "active" }),
      },
      sortBy: { field: sortField, order: sortOrder },
    }),
    [debouncedSearch, page, rowsPerPage, filters, sortField, sortOrder],
  );

  const { data, isLoading, isFetching, isError, error } = useSearchUsers(request);

  const handleSearchChange = (e: ChangeEvent<HTMLInputElement>) => {
    setFilters({ ...filters, search: e.target.value });
  };

  const handleChangeRowsPerPage = (e: ChangeEvent<HTMLInputElement>) => {
    setRowsPerPage(parseInt(e.target.value, 10));
    setPage(0);
  };

  const handleRoleChange = (e: SelectChangeEvent<string[]>) => {
    const value = e.target.value;
    setFilters({ ...filters, roleIds: typeof value === "string" ? value.split(",") : value });
  };

  const handleTeamChange = (e: SelectChangeEvent<string[]>) => {
    const value = e.target.value;
    setFilters({ ...filters, teamIds: typeof value === "string" ? value.split(",") : value });
  };

  const handleActiveChange = (e: SelectChangeEvent) => {
    setFilters({ ...filters, active: e.target.value as UsersFilters["active"] });
  };

  const handleSort = (field: UserSortField) => {
    if (sortField === field) {
      setSortOrder((o) => (o === "asc" ? "desc" : "asc"));
    } else {
      setSortField(field);
      setSortOrder("asc");
    }
    setPage(0);
  };

  const users = data?.users ?? [];
  const total = data?.total ?? 0;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Typography variant="body2" color="text.secondary">
        Search across username and email (case-insensitive). Filter by role, group, team and
        status.
      </Typography>

      <Stack direction={{ xs: "column", sm: "row" }} spacing={2} sx={{ flexWrap: "wrap" }}>
        <TextField
          size="small"
          label="Search users"
          placeholder="Search users by username or email"
          value={filters.search}
          onChange={handleSearchChange}
          slotProps={{ htmlInput: { "aria-label": "Search users by username or email" } }}
          sx={{ minWidth: 280, flex: 1 }}
        />

        <FormControl size="small" sx={{ minWidth: 200 }}>
          <InputLabel id="user-roles-label">Roles</InputLabel>
          <Select
            labelId="user-roles-label"
            multiple
            value={filters.roleIds}
            onChange={handleRoleChange}
            input={<OutlinedInput label="Roles" />}
            renderValue={(selected) =>
              (selected as string[]).map((id) => roleNameById.get(id) ?? id).join(", ")
            }
          >
            {roles.map((role) => (
              <MenuItem key={role.id} value={role.id}>
                <Checkbox checked={filters.roleIds.includes(role.id)} />
                <ListItemText primary={role.name} />
              </MenuItem>
            ))}
          </Select>
        </FormControl>

        <Box sx={{ minWidth: 240, flex: 1 }}>
          <AsyncEntityMultiSelect<BeGroup>
            id="user-groups-filter"
            label="Groups"
            placeholder="Search groups…"
            values={filters.groupIds}
            onChange={(next) => setFilters({ ...filters, groupIds: next })}
            useSearch={useSearchGroups}
            getId={(g) => g.id}
            getLabel={(g) => g.name}
          />
        </Box>

        <FormControl size="small" sx={{ minWidth: 200 }}>
          <InputLabel id="user-teams-label">Teams</InputLabel>
          <Select
            labelId="user-teams-label"
            multiple
            value={filters.teamIds}
            onChange={handleTeamChange}
            input={<OutlinedInput label="Teams" />}
            renderValue={(selected) =>
              (selected as string[]).map((id) => teamNameById.get(id) ?? id).join(", ")
            }
          >
            {teams.map((team) => (
              <MenuItem key={team.id} value={team.id}>
                <Checkbox checked={filters.teamIds.includes(team.id)} />
                <ListItemText primary={team.name} secondary={team.family} />
              </MenuItem>
            ))}
          </Select>
        </FormControl>

        <FormControl size="small" sx={{ minWidth: 140 }}>
          <InputLabel id="user-active-label">Status</InputLabel>
          <Select
            labelId="user-active-label"
            value={filters.active}
            onChange={handleActiveChange}
            input={<OutlinedInput label="Status" />}
          >
            <MenuItem value="all">All</MenuItem>
            <MenuItem value="active">Active</MenuItem>
            <MenuItem value="inactive">Inactive</MenuItem>
          </Select>
        </FormControl>
      </Stack>

      <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, overflow: "hidden" }}>
        <TableContainer>
          <Table size="small" aria-label="Users search results" sx={{ "& .MuiTableCell-root": { borderColor: "divider" } }}>
            <TableHead>
              <TableRow sx={{ bgcolor: "action.hover" }}>
                <TableCell>Username</TableCell>
                <TableCell sortDirection={sortField === "name" ? sortOrder : false}>
                  <TableSortLabel
                    active={sortField === "name"}
                    direction={sortField === "name" ? sortOrder : "asc"}
                    onClick={() => handleSort("name")}
                  >
                    Name
                  </TableSortLabel>
                </TableCell>
                <TableCell>Email</TableCell>
                <TableCell>Roles</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Timezone</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {isLoading || isFetching ? (
                Array.from({ length: rowsPerPage }).map((_, i) => (
                  <TableRow key={i}>
                    <TableCell><Skeleton variant="rounded" width="70%" height={18} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width="75%" height={18} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width="85%" height={18} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width={64} height={22} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width={60} height={22} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width="55%" height={18} /></TableCell>
                  </TableRow>
                ))
              ) : isError ? (
                <TableRow>
                  <TableCell colSpan={6} align="center">
                    <QueryErrorState
                      message={error instanceof Error && error.message.trim() ? error.message : "Failed to load users."}
                      error={error}
                    />
                  </TableCell>
                </TableRow>
              ) : users.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} align="center" sx={{ py: 4 }}>
                    <Typography variant="body2" color="text.secondary">
                      No users found.
                    </Typography>
                  </TableCell>
                </TableRow>
              ) : (
                users.map((u) => {
                  const profilePath = `/people/${encodeURIComponent(u.id)}`;
                  const goToProfile = (): void => navigate(profilePath);
                  const handleRowKeyDown = (e: KeyboardEvent<HTMLTableRowElement>): void => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      goToProfile();
                    }
                  };
                  const visibleRoles = u.roles?.slice(0, MAX_VISIBLE_ROLES) ?? [];
                  const hiddenRoleCount = Math.max((u.roles?.length ?? 0) - MAX_VISIBLE_ROLES, 0);

                  return (
                    <TableRow
                      key={u.id}
                      hover
                      onClick={goToProfile}
                      onKeyDown={handleRowKeyDown}
                      tabIndex={0}
                      aria-label={`View profile for ${u.name || u.userName}`}
                      sx={{ cursor: "pointer" }}
                    >
                      <TableCell>
                        <UserRefLink name={u.userName} email={u.email} userId={u.id} />
                      </TableCell>
                      <TableCell>{u.name || "—"}</TableCell>
                      <TableCell>{u.email}</TableCell>
                      <TableCell>
                        <Stack direction="row" spacing={0.5} sx={{ flexWrap: "wrap", gap: 0.5 }}>
                          {u.roles && u.roles.length > 0 ? (
                            <>
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
                                    goToProfile();
                                  }}
                                  onKeyDown={(e) => {
                                    if (e.key === "Enter" || e.key === " ") {
                                      e.stopPropagation();
                                    }
                                  }}
                                  aria-label={`View all ${u.roles.length} roles for ${u.name || u.userName}`}
                                />
                              )}
                            </>
                          ) : u.userType ? (
                            <Chip
                              size="small"
                              label={u.userType}
                              color={u.userType === "internal" ? "primary" : "default"}
                              variant="outlined"
                            />
                          ) : (
                            "—"
                          )}
                        </Stack>
                      </TableCell>
                      <TableCell>
                        {u.active === undefined ? (
                          "—"
                        ) : (
                          <Chip
                            size="small"
                            label={u.active ? "Active" : "Inactive"}
                            color={u.active ? "success" : "default"}
                            variant="outlined"
                          />
                        )}
                      </TableCell>
                      <TableCell>{u.timezone ?? "—"}</TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </TableContainer>
        <TablePagination
          component="div"
          count={total}
          page={page}
          onPageChange={(_, newPage) => setPage(newPage)}
          rowsPerPage={rowsPerPage}
          onRowsPerPageChange={handleChangeRowsPerPage}
          rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
          showFirstButton
          showLastButton
        />
      </Box>
    </Box>
  );
}
