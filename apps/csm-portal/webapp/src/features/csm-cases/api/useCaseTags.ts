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
  useMutation,
  useQueryClient,
  type QueryKey,
  type UseMutationResult,
} from "@tanstack/react-query";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useBackendApi } from "@api/backend/client";
import type { BeAddCaseTagPayload, BeTag } from "@api/backend/types";

/**
 * The query the tag mutations below invalidate once they land, so the detail
 * that shows the tags refetches: the case detail by default. The routes take
 * a work item id, so a change request is tagged through the very same hooks
 * (`CsmChangeRequestDetailPage` passes its own detail key,
 * `[CHANGE_REQUEST_DETAILS, id]`) rather than a copy of them.
 */
function detailKeyFor(workItemId: string | undefined, detailQueryKey?: QueryKey): QueryKey {
  return detailQueryKey ?? [ApiQueryKeys.CSM_CASE_DETAIL, workItemId ?? ""];
}

/**
 * Add a free-text tag to a case via `POST /cases/{id}/tags` (ServiceNow data
 * source only; the caller surfaces a rejection on another source). Tags are
 * genuinely free-text on the backing data source (SN's generic label
 * mechanism) — there is no closed enum to validate against client-side. On
 * success, invalidates the case detail so the new tag shows — or
 * `detailQueryKey` instead, for a work item whose detail lives under another
 * key (a change request, see {@link detailKeyFor}).
 */
export function useAddCaseTag(
  caseId: string | undefined,
  detailQueryKey?: QueryKey,
): UseMutationResult<BeTag, Error, string> {
  const api = useBackendApi();
  const queryClient = useQueryClient();

  return useMutation<BeTag, Error, string>({
    mutationFn: async (label): Promise<BeTag> => {
      if (!caseId) {
        throw new Error("Cannot add a tag without a case id.");
      }
      return api.post<BeAddCaseTagPayload, BeTag>(
        `/cases/${encodeURIComponent(caseId)}/tags`,
        { label },
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: detailKeyFor(caseId, detailQueryKey),
      });
    },
  });
}

/**
 * Add a free-text tag to a case given at call time, rather than bound to one
 * case per hook instance like {@link useAddCaseTag} above — for a flow that
 * creates several cases and tags each one afterward (e.g. the announcement
 * create page's security-label flow), where the case ids aren't known until
 * each create call resolves.
 */
export function useAddTagToCase(): UseMutationResult<
  BeTag,
  Error,
  { caseId: string; label: string }
> {
  const api = useBackendApi();
  const queryClient = useQueryClient();

  return useMutation<BeTag, Error, { caseId: string; label: string }>({
    mutationFn: async ({ caseId, label }): Promise<BeTag> =>
      api.post<BeAddCaseTagPayload, BeTag>(`/cases/${encodeURIComponent(caseId)}/tags`, {
        label,
      }),
    onSuccess: (_tag, variables) => {
      queryClient.invalidateQueries({
        queryKey: [ApiQueryKeys.CSM_CASE_DETAIL, variables.caseId],
      });
    },
  });
}

/**
 * Remove a tag from a case via `DELETE /cases/{id}/tags/{tagId}` (ServiceNow
 * data source only). On success, invalidates the case detail so the chip
 * drops without a manual refetch — or `detailQueryKey` instead, as for
 * {@link useAddCaseTag}.
 */
export function useRemoveCaseTag(
  caseId: string | undefined,
  detailQueryKey?: QueryKey,
): UseMutationResult<void, Error, string> {
  const api = useBackendApi();
  const queryClient = useQueryClient();

  return useMutation<void, Error, string>({
    mutationFn: async (tagId): Promise<void> => {
      if (!caseId) {
        throw new Error("Cannot remove a tag without a case id.");
      }
      await api.del<void>(
        `/cases/${encodeURIComponent(caseId)}/tags/${encodeURIComponent(tagId)}`,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: detailKeyFor(caseId, detailQueryKey),
      });
    },
  });
}
