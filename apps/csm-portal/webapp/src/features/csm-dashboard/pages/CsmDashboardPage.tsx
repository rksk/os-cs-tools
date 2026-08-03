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

import { Box, Skeleton, Typography } from "@wso2/oxygen-ui";
import { useCallback, useMemo, type JSX } from "react";
import { useLocation, useNavigate } from "react-router";
import AbtDashboardHeader from "@features/csm-dashboard/components/AbtDashboardHeader";
import AgentsLandingPagePilot from "@features/csm-dashboard/components/AgentsLandingPagePilot";
import { useDashboardList } from "@features/csm-dashboard/api/useDashboardList";
import { useTeams } from "@features/csm-dashboard/api/useTeams";
import { useCurrentUser } from "@context/current-user/CurrentUserContext";
import type { DashboardKey } from "@features/csm-dashboard/types/abtDashboard";
import {
  buildDashboardHash,
  parseDashboardHash,
} from "@features/csm-dashboard/utils/dashboardUrlHash";

/**
 * Top-level CSM dashboard. The dashboard list is BE-driven (`GET
 * /dashboards`), and the initial selection depends on the signed-in user's
 * own ABT team membership (`GET /users/me`'s `team`, via `useCurrentUser`):
 * a user WITH a resolved team defaults to the first dashboard with BOTH
 * `isDefault` and `isTeamBased` set (with that team auto-selected — see
 * `selectedTeamId` below); a user with no team defaults to the first
 * dashboard with `isDefault` set and `isTeamBased` NOT set. If no dashboard
 * matches that preferred predicate, this falls back to the BE's own (any)
 * `isDefault` entry, then to the first dashboard in the list — never to an
 * empty selection. The URL always wins over all of that when it names a
 * (valid) dashboard — see `parseDashboardHash`.
 *
 * Dashboards are selected purely by dropdown — there is no other
 * per-dashboard scoping control. Every dashboard in the registry has at
 * least one real (config-driven) widget, so this always renders the real
 * widget grid.
 */
export default function CsmDashboardPage(): JSX.Element {
  const location = useLocation();
  const navigate = useNavigate();

  const dashboardList = useDashboardList();
  const list = dashboardList.data;
  const currentUser = useCurrentUser();

  const { dashboardId: hashDashboardId, teamId: hashTeamIdRaw } = useMemo(
    () => parseDashboardHash(location.hash),
    [location.hash],
  );

  const urlEntry = list?.find((d) => d.id === hashDashboardId);

  const userHasTeam = Boolean(currentUser.user?.team);
  // The preferred predicate per the user's own team membership: BOTH
  // isDefault and isTeamBased must match (not isTeamBased alone, and not
  // isDefault alone) — see the module doc comment above.
  const preferredEntry = userHasTeam
    ? list?.find((d) => d.isDefault && d.isTeamBased)
    : list?.find((d) => d.isDefault && !d.isTeamBased);
  // Fallback 1: the BE's own (any) isDefault entry, regardless of
  // isTeamBased — covers a registry that has no isDefault+isTeamBased (or
  // isDefault+!isTeamBased) combination configured at all.
  const anyDefaultEntry = list?.find((d) => d.isDefault);
  // Fallback 2: the first dashboard in the list — never render nothing just
  // because the registry has no isDefault entry configured.
  const firstEntry = list && list.length > 0 ? list[0] : undefined;
  // True only while we genuinely don't know yet whether this user has a
  // team — a failed profile fetch (isError) must not hang this forever, so
  // it falls straight through to the defaults below instead.
  const userProfilePending = currentUser.isLoading && !currentUser.isError;

  // The URL always wins when it names a dashboard actually in the loaded
  // list (stale/hand-edited hash falls through to the defaults below,
  // never crashes). Only when it doesn't do we need to pick a default —
  // and picking that default depends on the user's own team membership, so
  // hold off (skeleton) until that's resolved, unless it errored.
  let currentEntry = urlEntry;
  if (!currentEntry && list) {
    if (userProfilePending) {
      currentEntry = undefined;
    } else {
      currentEntry = preferredEntry ?? anyDefaultEntry ?? firstEntry;
    }
  }

  const dashboardKey = currentEntry?.id as DashboardKey | undefined;
  const isTeamBased = currentEntry?.isTeamBased ?? false;

  // Only apply a hash team id when the CURRENT dashboard is team-based — a
  // stale suffix left over from a previously selected team-based dashboard
  // (or a hand-edited URL) must not leak into a non-team-based one.
  const hashTeamId = isTeamBased ? hashTeamIdRaw : undefined;
  // Default to the signed-in user's own team once their profile has
  // resolved, but only ever as a default: the moment the URL itself names a
  // team (including one written by the user's own pick — see
  // `handleTeamChange`), that value always wins over this one, so a manual
  // switch is never fought on re-render.
  const defaultTeamId =
    isTeamBased && !hashTeamId && userHasTeam ? currentUser.user?.team?.teamKey : undefined;
  const selectedTeamId = hashTeamId ?? defaultTeamId;

  // Every team, for resolving the selected team's `groupId` (the
  // `__current_team__` filter placeholder's real value) — same query
  // AbtDashboardHeader's own team selector uses; react-query dedupes the
  // fetch since both share the same query key.
  const teams = useTeams(isTeamBased);
  const selectedTeamGroupId = teams.data?.find((t) => t.id === selectedTeamId)?.groupId;

  const writeHash = useCallback(
    (nextDashboardId: string, nextTeamId: string | undefined) => {
      navigate(
        { pathname: location.pathname, search: location.search, hash: buildDashboardHash(nextDashboardId, nextTeamId) },
        { replace: true },
      );
    },
    [navigate, location.pathname, location.search],
  );

  const handleDashboardChange = useCallback(
    (key: DashboardKey) => {
      const nextEntry = list?.find((d) => d.id === key);
      // Switching to a dashboard that isn't team-based: clear any stale
      // team selection rather than leaving an inapplicable one in the URL.
      // Switching between two team-based dashboards keeps the current
      // selection instead of resetting it.
      const nextTeamId = nextEntry?.isTeamBased ? selectedTeamId : undefined;
      writeHash(key, nextTeamId);
    },
    [list, selectedTeamId, writeHash],
  );

  const handleTeamChange = useCallback(
    (teamId: string | undefined) => {
      if (!dashboardKey) return;
      writeHash(dashboardKey, teamId);
    },
    [dashboardKey, writeHash],
  );

  const dashboardListData = useMemo(() => dashboardList.data ?? [], [dashboardList.data]);

  if (dashboardList.isError) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
        <Typography variant="h5">Dashboard</Typography>
        <Typography variant="body2" color="text.secondary">
          Could not load the dashboard list.
        </Typography>
      </Box>
    );
  }

  if (dashboardKey === undefined) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
        <Skeleton variant="rounded" height={32} width={240} />
        <Skeleton variant="rounded" height={200} />
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <AbtDashboardHeader
        dashboardKey={dashboardKey}
        onDashboardChange={handleDashboardChange}
        dashboardList={dashboardListData}
        selectedTeamId={selectedTeamId}
        onTeamChange={handleTeamChange}
      />
      <AgentsLandingPagePilot
        dashboardId={dashboardKey}
        selectedTeamGroupId={selectedTeamGroupId}
      />
    </Box>
  );
}
