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
import type { BeRoleSearchPayload, BeRoleSearchResponse } from "@api/backend/types";

/**
 * Searches the platform's curated role catalogue (`POST /roles/search`) —
 * the same values `UserSearchFilters.roleIds` accepts. Unlike
 * `/groups/search`, this isn't a live query against the backing data source:
 * it's a fixed, small vocabulary, so it's cached longer and reused for both
 * the Roles directory page (paginated) and the user-list role picker (one
 * full-catalogue page).
 */
export function useSearchRoles(
  request: BeRoleSearchPayload,
  enabled = true,
): UseQueryResult<BeRoleSearchResponse, Error> {
  const api = useBackendApi();

  return useQuery<BeRoleSearchResponse, Error>({
    queryKey: [ApiQueryKeys.CSM_ADMIN_ROLES, request],
    queryFn: () =>
      api.post<BeRoleSearchPayload, BeRoleSearchResponse>("/roles/search", request),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60_000,
  });
}
