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

import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { useAsgardeo } from "@asgardeo/react";
import { useAuthApiClient } from "@/hooks/useAuthApiClient";
import { ApiError } from "@utils/ApiError";
import { CORRELATION_ID_HEADER, newCorrelationId } from "@utils/correlationId";
import { useLogger } from "@hooks/useLogger";
import { ApiQueryKeys } from "@constants/apiConstants";
import type { ChangeRequestApprovalsResponse } from "@features/operations/types/changeRequests";

/**
 * Fetches a change request's approval stages (`GET /change-requests/{id}/approvals`).
 *
 * The workflow panel reads them as its evidence: a step before the current one
 * is shown done only when the record says it happened. The customer's own
 * stages come whole; WSO2's internal stages come as a label and a status only.
 *
 * @param {string} changeRequestId - The change request id.
 * @returns {UseQueryResult<ChangeRequestApprovalsResponse, Error>} Query result with the approval stages.
 */
export default function useGetChangeRequestApprovals(
  changeRequestId: string,
): UseQueryResult<ChangeRequestApprovalsResponse, Error> {
  const logger = useLogger();
  const { isSignedIn, isLoading: isAuthLoading } = useAsgardeo();
  const authFetch = useAuthApiClient();

  return useQuery<ChangeRequestApprovalsResponse, Error>({
    queryKey: [ApiQueryKeys.CHANGE_REQUEST_APPROVALS, changeRequestId],
    queryFn: async (): Promise<ChangeRequestApprovalsResponse> => {
      logger.debug(
        `Fetching change request approvals: changeRequestId=${changeRequestId}`,
      );

      try {
        const baseUrl = window.config?.CUSTOMER_PORTAL_BACKEND_BASE_URL;
        if (!baseUrl) {
          throw new Error("CUSTOMER_PORTAL_BACKEND_BASE_URL is not configured");
        }

        const requestUrl = `${baseUrl}/change-requests/${encodeURIComponent(changeRequestId)}/approvals`;
        const correlationId = newCorrelationId();
        const response = await authFetch(requestUrl, {
          method: "GET",
          headers: { [CORRELATION_ID_HEADER]: correlationId },
        });

        if (!response.ok) {
          let apiMessage: string | undefined;
          try {
            const errBody = await response.json();
            if (typeof errBody?.message === "string") {
              apiMessage = errBody.message;
            }
          } catch {
            // ignore – body may not be JSON
          }
          throw new ApiError(
            response.status,
            response.statusText,
            apiMessage ?? `Error fetching change request approvals: ${response.statusText}`,
            response.headers.get(CORRELATION_ID_HEADER) ?? correlationId,
          );
        }

        const data: ChangeRequestApprovalsResponse = await response.json();
        logger.debug("[useGetChangeRequestApprovals] Data received", {
          stages: Array.isArray(data?.approvals) ? data.approvals.length : 0,
        });
        return data;
      } catch (error) {
        logger.error(
          "[useGetChangeRequestApprovals] Error:",
          error instanceof Error ? error.message : String(error),
        );
        throw error;
      }
    },
    enabled: !!changeRequestId && isSignedIn && !isAuthLoading,
    staleTime: 0,
  });
}
