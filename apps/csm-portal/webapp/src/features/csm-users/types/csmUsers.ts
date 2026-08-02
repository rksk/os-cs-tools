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

/**
 * Whether the user is staff or a customer/partner contact. The value set
 * differs by backing data source: one emits `internal`, `customer` or
 * `system`, the other `internal` or `external`. Callers branching on
 * staff-vs-not must treat everything other than `internal` alike, not just
 * `customer` or just `external` — see `isInternalUser` in `UserProfilePage`.
 */
export type UserType = "internal" | "external" | "customer" | "system";

/**
 * Roles as modelled by the ServiceNow data source. `internal`/`agent`/`admin`
 * are WSO2-internal staff; the rest are external (customer/partner) contacts.
 */
export type SnUserRole =
  | "internal"
  | "agent"
  | "admin"
  | "commenter"
  | "external"
  | "customer"
  | "customer_admin"
  | "partner"
  | "partner_admin";

/** Internal-facing roles: the translation of the old `userType === "internal"`. */
export const INTERNAL_USER_ROLES: SnUserRole[] = ["internal", "agent", "admin"];

export type UserSortField = "name" | "createdOn" | "updatedOn";
export type UserSortOrder = "asc" | "desc";

/** User shape returned by the postgres data source. */
export interface User {
  id: string;
  userName: string;
  firstName: string;
  lastName: string;
  email: string;
  phone?: string | null;
  timezone?: string | null;
  userType: UserType;
  createdAt: string;
  updatedAt: string;
}

/**
 * User shape returned by the ServiceNow data source. No `firstName`/`lastName`
 * (use `name`), no `hasMore` on the envelope. `userType` and `mobilePhone` are
 * present on the full-profile response (`GET /users/{id}`); `userType` may be
 * absent from the search-list response on an older backend, so callers must
 * still fall back to `roles` (see `isInternalUser`).
 */
export interface SnUser {
  id: string;
  userName: string;
  name: string;
  email: string;
  timeZone?: string | null;
  mobilePhone?: string | null;
  userType?: UserType;
  active: boolean;
  createdOn: string;
  updatedOn: string;
  roles: string[];
}

/** One group the user belongs to. */
export interface UserGroupRef {
  id: string;
  name: string;
}

/** One CRE/SRE team the user belongs to (a subset of their groups). */
export interface UserTeamRef {
  id: string;
  name: string;
  family?: string;
}

/**
 * One project-contact row for an external user, reported as stored rather
 * than as filtered. A row with no linked contact record makes the project,
 * and every case on it, silently invisible to that user —
 * {@link grantsCaseAccess} is the verdict, mirroring
 * {@link contactRecordPresent} directly (deliberately not the stricter
 * email-match rule the live access check enforces, since that only diverges
 * for integration/system accounts).
 */
export interface UserProjectAccess {
  projectId: string;
  projectName: string;
  projectKey: string;
  contactEmail: string;
  contactRecordPresent: boolean;
  contactRecordEmail?: string;
  registrationState?: string;
  notificationsEnabled?: boolean;
  roles?: string[];
  grantsCaseAccess: boolean;
}

/**
 * `GET /users/{id}` response: the user row plus every group they belong to,
 * the subset of those that are teams, and — for external contacts — their
 * per-project access. Populated only for users sourced from the data source
 * that carries group and project-contact records. The membership and
 * project-access blocks are best-effort: empty rather than absent when their
 * upstream lookup fails.
 */
export interface SnUserDetail extends SnUser {
  groups?: UserGroupRef[];
  teams?: UserTeamRef[];
  /** Present for external contacts only. */
  projectAccess?: UserProjectAccess[];
}

export interface UserSearchFilters {
  searchQuery?: string;
  /**
   * Filter by one or more role keys, as returned by `POST /roles/search`.
   * Backing data source only. Named `roleIds` (not `roles`) to match the
   * backend contract — a role's "id" is its key (e.g. "agent"), not a
   * separate numeric identifier.
   */
  roleIds?: string[];
  /** Restrict to specific users by id. Intersects with the other filters and
   * lifts the active-only default. Backing data source only. */
  userIds?: string[];
  /** Restrict to members of these groups (group ids). Backing data source
   * only. */
  groupIds?: string[];
  /** Restrict to members of these teams, by team registry key (not a UUID).
   * Backing data source only. */
  teamIds?: string[];
  userNames?: string[];
  emails?: string[];
  /** Backing data source only. */
  active?: boolean | null;
}

export interface UserSortBy {
  /** ServiceNow data source only. */
  field: UserSortField;
  order?: UserSortOrder;
}

export interface SearchUsersRequest {
  pagination?: {
    limit?: number;
    offset?: number;
  };
  filters?: UserSearchFilters;
  sortBy?: UserSortBy;
}

/** Postgres-data-source response envelope. */
export interface UserSearchResponse {
  users: User[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

/** ServiceNow-data-source response envelope (no `hasMore`). */
export interface SnUserSearchResponse {
  users: SnUser[];
  total: number;
  limit: number;
  offset: number;
}

/** Either data source's raw envelope; `/users/search` returns a `oneOf`. */
export type SearchUsersResponse = UserSearchResponse | SnUserSearchResponse;

/**
 * Source-agnostic row the UI renders. Both `User` and `SnUser` normalize into
 * this so screens don't branch on the live data source.
 */
export interface NormalizedUser {
  id: string;
  userName: string;
  name: string;
  email: string;
  timezone: string | null;
  userType?: UserType;
  /** Present only from the ServiceNow source. */
  active?: boolean;
  /** Present only from the ServiceNow source. */
  roles?: string[];
  /** Present from either source, when the caller requested the full profile. */
  phone?: string | null;
  createdOn?: string;
  updatedOn?: string;
}

export interface NormalizedUserSearchResult {
  users: NormalizedUser[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}

function isSnUser(u: User | SnUser): u is SnUser {
  return "name" in u || "active" in u || "roles" in u;
}

/** Maps either source's user shape into {@link NormalizedUser}. */
export function normalizeUser(u: User | SnUser): NormalizedUser {
  if (isSnUser(u)) {
    return {
      id: u.id,
      userName: u.userName,
      name: u.name?.trim() || "",
      email: u.email,
      timezone: u.timeZone ?? null,
      userType: u.userType,
      active: u.active,
      roles: u.roles,
      phone: u.mobilePhone ?? null,
      createdOn: u.createdOn,
      updatedOn: u.updatedOn,
    };
  }
  return {
    id: u.id,
    userName: u.userName,
    name: `${u.firstName ?? ""} ${u.lastName ?? ""}`.trim(),
    email: u.email,
    timezone: u.timezone ?? null,
    userType: u.userType,
    phone: u.phone ?? null,
    createdOn: u.createdAt,
    updatedOn: u.updatedAt,
  };
}

/**
 * Full-profile counterpart of {@link NormalizedUser}: adds the group/team
 * memberships and (for external contacts) per-project access that only
 * `GET /users/{id}` returns.
 */
export interface NormalizedUserDetail extends NormalizedUser {
  groups?: UserGroupRef[];
  teams?: UserTeamRef[];
  projectAccess?: UserProjectAccess[];
}

/** Maps `GET /users/{id}`'s response into {@link NormalizedUserDetail}. */
export function normalizeUserDetail(u: SnUserDetail): NormalizedUserDetail {
  return {
    ...normalizeUser(u),
    groups: u.groups,
    teams: u.teams,
    projectAccess: u.projectAccess,
  };
}

/** Normalizes either response envelope; derives `hasMore` when absent. */
export function normalizeUserSearchResponse(
  res: SearchUsersResponse,
): NormalizedUserSearchResult {
  const users = (res.users ?? []).map((u) => normalizeUser(u));
  const hasMore =
    "hasMore" in res
      ? res.hasMore
      : res.offset + users.length < res.total;
  return { users, total: res.total, limit: res.limit, offset: res.offset, hasMore };
}
