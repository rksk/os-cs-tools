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

import { useState } from "react";
import { useErrorBanner } from "@context/error-banner/ErrorBannerContext";
import { useUpdateConversationState } from "@features/support/api/useUpdateConversationState";

export interface CloseConversationFlow {
  /** Whether the confirmation dialog should be shown. */
  isConfirmOpen: boolean;
  /** Whether the close request is in flight. */
  isClosing: boolean;
  /** Open the confirmation dialog for the given conversation. */
  requestClose: (conversationId: string) => void;
  /** Confirm and send the close request for the pending conversation. */
  confirmClose: () => void;
  /** Dismiss the confirmation dialog without closing. */
  cancelClose: () => void;
}

/**
 * Owns the "close a chat" interaction — confirmation-dialog state plus the
 * PATCH mutation — so the conversations list and the support dashboard share a
 * single implementation instead of duplicating the dialog and mutation.
 *
 * @param {string} projectId - Project ID for cache invalidation.
 * @returns {CloseConversationFlow} Dialog state and handlers.
 */
export function useCloseConversationFlow(
  projectId: string,
): CloseConversationFlow {
  const { showError } = useErrorBanner();
  const closeConversation = useUpdateConversationState(projectId, "closed");
  const [chatIdToClose, setChatIdToClose] = useState<string | null>(null);

  const requestClose = (conversationId: string): void => {
    if (conversationId) {
      setChatIdToClose(conversationId);
    }
  };

  const cancelClose = (): void => setChatIdToClose(null);

  const confirmClose = (): void => {
    if (!chatIdToClose) return;
    closeConversation.mutate(chatIdToClose, {
      onSuccess: () => setChatIdToClose(null),
      onError: (error: Error) => {
        setChatIdToClose(null);
        showError(
          error.message || "Failed to close the chat. Please try again.",
        );
      },
    });
  };

  return {
    isConfirmOpen: chatIdToClose !== null,
    isClosing: closeConversation.isPending,
    requestClose,
    confirmClose,
    cancelClose,
  };
}
