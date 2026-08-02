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

import { keepPreviousData, useQuery, type UseQueryResult } from "@tanstack/react-query";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useBackendApi } from "@api/backend/client";
import type { BeGroupSearchPayload, BeGroupSearchResponse } from "@api/backend/types";

/**
 * Paginated group search (`POST /groups/search`) for the Groups directory
 * list page. Distinct from `useSearchGroups` (the type-ahead picker used by
 * the change-request "Assignment group" field): that one always asks for a
 * fixed small page and drops the total count, which isn't enough for a
 * page-through admin list. Groups are a live query against the backing data
 * source, unlike the curated role/team catalogues, so this list can be much
 * larger than either of those.
 */
export function useSearchGroupsPage(
  request: BeGroupSearchPayload,
  enabled = true,
): UseQueryResult<BeGroupSearchResponse, Error> {
  const api = useBackendApi();

  return useQuery<BeGroupSearchResponse, Error>({
    queryKey: [ApiQueryKeys.CSM_ADMIN_GROUPS, request],
    queryFn: () =>
      api.post<BeGroupSearchPayload, BeGroupSearchResponse>("/groups/search", request),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
  });
}
