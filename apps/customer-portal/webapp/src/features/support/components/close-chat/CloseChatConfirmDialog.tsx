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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
} from "@wso2/oxygen-ui";
import type { JSX } from "react";

export interface CloseChatConfirmDialogProps {
  /** Whether the dialog is visible. */
  open: boolean;
  /** Whether the close request is in flight (disables the actions). */
  isClosing: boolean;
  /** Called when the user dismisses the dialog. */
  onCancel: () => void;
  /** Called when the user confirms closing the chat. */
  onConfirm: () => void;
}

/**
 * Confirmation dialog shown before a Novera chat is closed. Shared by the
 * conversations list and the support dashboard so the copy and behavior stay
 * in one place.
 *
 * @param {CloseChatConfirmDialogProps} props - Open state and handlers.
 * @returns {JSX.Element} The confirmation dialog.
 */
export default function CloseChatConfirmDialog({
  open,
  isClosing,
  onCancel,
  onConfirm,
}: CloseChatConfirmDialogProps): JSX.Element {
  return (
    <Dialog
      open={open}
      onClose={isClosing ? undefined : onCancel}
      maxWidth="xs"
      fullWidth
    >
      <DialogTitle>Close this chat?</DialogTitle>
      <DialogContent>
        <DialogContentText>
          This chat is about to be permanently closed. Once closed, it cannot be
          resumed.
        </DialogContentText>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} disabled={isClosing}>
          Cancel
        </Button>
        <Button
          variant="contained"
          color="error"
          onClick={onConfirm}
          disabled={isClosing}
        >
          {isClosing ? "Closing…" : "Close chat"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
