// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import { useAsgardeo } from "@asgardeo/react";
import { useAuthApiClient } from "@/hooks/useAuthApiClient";
import { useLogger } from "@hooks/useLogger";
import { ApiQueryKeys } from "@constants/apiConstants";
import { parseApiResponseMessage } from "@utils/ApiError";

/** Terminal status a conversation can be moved to via PATCH /conversations/:id. */
export type ConversationStatusAction = "closed" | "abandoned" | "converted";

/**
 * Moves a Novera conversation to a terminal state via
 * PATCH /conversations/:id, so it can no longer be resumed. "closed" is a
 * user-initiated close from the chat list; "abandoned" is set automatically
 * when a case is created before the assistant responds. On success, refreshes
 * the conversation list and stats for the project.
 *
 * @param {string} projectId - Project ID for cache invalidation.
 * @param {ConversationStatusAction} status - Target terminal status.
 * @returns {UseMutationResult<void, Error, string>} Mutation keyed by conversationId.
 */
export function useUpdateConversationState(
  projectId: string,
  status: ConversationStatusAction,
): UseMutationResult<void, Error, string> {
  const logger = useLogger();
  const queryClient = useQueryClient();
  const { isSignedIn, isLoading: isAuthLoading } = useAsgardeo();
  const authFetch = useAuthApiClient();

  return useMutation<void, Error, string>({
    mutationFn: async (conversationId: string): Promise<void> => {
      if (!conversationId) {
        throw new Error("Conversation ID is required to update a chat");
      }
      if (!isSignedIn || isAuthLoading) {
        throw new Error("User must be signed in to update a chat");
      }

      const baseUrl = window.config?.CUSTOMER_PORTAL_BACKEND_BASE_URL;
      if (!baseUrl) {
        throw new Error("CUSTOMER_PORTAL_BACKEND_BASE_URL is not configured");
      }

      const response = await authFetch(
        `${baseUrl}/conversations/${conversationId}`,
        {
          method: "PATCH",
          body: JSON.stringify({ status }),
        },
      );
      if (!response.ok) {
        const text = await response.text();
        throw new Error(
          parseApiResponseMessage(text, response.status, response.statusText),
        );
      }
      logger.debug("[useUpdateConversationState] Conversation updated:", {
        conversationId,
        status,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [ApiQueryKeys.CONVERSATIONS_SEARCH, projectId],
      });
      queryClient.invalidateQueries({
        queryKey: [ApiQueryKeys.CONVERSATION_STATS, projectId],
      });
      void queryClient.refetchQueries({
        queryKey: [ApiQueryKeys.CONVERSATIONS_SEARCH, projectId],
        type: "active",
      });
    },
  });
}
