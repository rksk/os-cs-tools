/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";
import { useBackendApi } from "@api/backend/client";
import type {
  CaseAlertChannel,
  CaseAlertChatRoom,
  CaseAlertGroupRoute,
  CaseAlertLadder,
  CaseAlertLadderValue,
  CaseAlertRoomLadder,
  CaseAlertSettings,
} from "../types";

const SETTINGS_KEY = ["team-schedule", "case-alerts", "settings"] as const;
const BASE = "/team-schedule/case-alerts/settings";

/** Somebody who may see nothing: no section. */
export const NO_CASE_ALERT_SETTINGS: CaseAlertSettings = {
  canManage: { view: false, roomLadders: [], routeLadders: [] },
  environment: "",
  webhooksEnabled: false,
  revision: { saved: 0 },
  rooms: [],
  routes: [],
  values: [],
};

/** Whether the reader may see the Case Alerts settings at all. */
export function canViewCaseAlertSettings(s: CaseAlertSettings | undefined): boolean {
  return Boolean(s?.canManage.view);
}

/**
 * The Case Alerts chat rooms, group routes and ladder values, what the alert
 * service last reported running, and what the reader may change.
 * The page asks it to decide whether to show the section (and, for somebody who
 * leads nothing, the tab): the server answers view false, not a refusal, for
 * somebody who may see nothing.
 */
export function useCaseAlertSettings(): UseQueryResult<CaseAlertSettings, Error> {
  const api = useBackendApi();
  return useQuery<CaseAlertSettings, Error>({
    queryKey: SETTINGS_KEY,
    queryFn: async () => (await api.get<CaseAlertSettings>(BASE)) ?? NO_CASE_ALERT_SETTINGS,
    staleTime: 60 * 1000,
  });
}

export interface SaveRoomInput {
  ladder: CaseAlertRoomLadder;
  teamKey?: string;
  label: string;
  enabled?: boolean;
  teamless?: boolean;
  /** A team room's own channel; "" goes back to the ladder's. */
  channel?: CaseAlertChannel | "";
}

export interface SaveRouteInput {
  ladder: CaseAlertLadder;
  groupName: string;
  teamKey?: string;
  rotaCode?: string;
  enabled?: boolean;
}

/** Every change reads the settings again, so saved and running revisions
 *  and the room's hint stay true to the server. */
function useSettingsMutation<TVars, TResult>(
  fn: (vars: TVars) => Promise<TResult>,
): UseMutationResult<TResult, Error, TVars> {
  const qc = useQueryClient();
  return useMutation<TResult, Error, TVars>({
    mutationFn: fn,
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: SETTINGS_KEY });
    },
  });
}

export function useSaveCaseAlertRoom(): UseMutationResult<CaseAlertChatRoom, Error, SaveRoomInput> {
  const api = useBackendApi();
  return useSettingsMutation((room: SaveRoomInput) => api.put<SaveRoomInput, CaseAlertChatRoom>(`${BASE}/rooms`, room));
}

/** Saves a room's webhook URL. Write-only: it never comes back, only a hint. */
export function useSetCaseAlertWebhook(): UseMutationResult<CaseAlertChatRoom, Error, { roomId: string; url: string }> {
  const api = useBackendApi();
  return useSettingsMutation(({ roomId, url }: { roomId: string; url: string }) =>
    api.put<{ url: string }, CaseAlertChatRoom>(`${BASE}/rooms/${encodeURIComponent(roomId)}/webhook`, { url }),
  );
}

export function useDeleteCaseAlertRoom(): UseMutationResult<unknown, Error, string> {
  const api = useBackendApi();
  return useSettingsMutation((roomId: string) => api.del(`${BASE}/rooms/${encodeURIComponent(roomId)}`));
}

export interface TestCardResult {
  lastTestStatus: "delivered" | "failed";
  lastTestAt: string;
}

export function useTestCaseAlertRoom(): UseMutationResult<TestCardResult, Error, string> {
  const api = useBackendApi();
  return useSettingsMutation((roomId: string) =>
    api.postEmpty<TestCardResult>(`${BASE}/rooms/${encodeURIComponent(roomId)}/test`),
  );
}

export function useSaveCaseAlertRoute(): UseMutationResult<CaseAlertGroupRoute, Error, SaveRouteInput> {
  const api = useBackendApi();
  return useSettingsMutation((route: SaveRouteInput) =>
    api.put<SaveRouteInput, CaseAlertGroupRoute>(`${BASE}/routes`, route),
  );
}

export function useDeleteCaseAlertRoute(): UseMutationResult<unknown, Error, string> {
  const api = useBackendApi();
  return useSettingsMutation((routeId: string) => api.del(`${BASE}/routes/${encodeURIComponent(routeId)}`));
}

export interface SaveValuesInput {
  ladder: CaseAlertRoomLadder;
  /** Key to value, in the file's shape for that key. All are saved together,
   *  or none. */
  values: Record<string, unknown>;
}

/** Saves one or more ladder values; answers with every saved value of that
 *  ladder. */
export function useSaveCaseAlertValues(): UseMutationResult<
  { values: CaseAlertLadderValue[] },
  Error,
  SaveValuesInput
> {
  const api = useBackendApi();
  return useSettingsMutation((input: SaveValuesInput) =>
    api.put<SaveValuesInput, { values: CaseAlertLadderValue[] }>(`${BASE}/values`, input),
  );
}

/** Removes a saved ladder value: the ladder uses the file's again. */
export function useDeleteCaseAlertValue(): UseMutationResult<unknown, Error, string> {
  const api = useBackendApi();
  return useSettingsMutation((valueId: string) => api.del(`${BASE}/values/${encodeURIComponent(valueId)}`));
}
