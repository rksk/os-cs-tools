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

import type { DeescalateCaseModalProps } from "@features/support/types/supportComponents";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  TextField,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { TriangleAlert, X } from "@wso2/oxygen-ui-icons-react";
import { useCallback, useState, type ChangeEvent, type JSX } from "react";
import { usePostCaseEscalation } from "@features/support/api/usePostCaseEscalation";

const INLINE_ERROR_STATUSES = new Set([400, 403, 404, 409]);

/**
 * Modal for de-escalating a case that is currently escalated.
 * Reason is optional. HTTP 4xx errors are shown inline; 500+ errors are forwarded via onError.
 *
 * @param {DeescalateCaseModalProps} props - Modal control props.
 * @returns {JSX.Element} The de-escalate case modal.
 */
export default function DeescalateCaseModal({
  open,
  caseId,
  onClose,
  onSuccess,
  onError,
}: DeescalateCaseModalProps): JSX.Element {
  const theme = useTheme();
  const [reason, setReason] = useState("");
  const [inlineError, setInlineError] = useState<string | null>(null);

  const { mutate, isPending } = usePostCaseEscalation(caseId);

  const resetAndClose = useCallback(() => {
    setReason("");
    setInlineError(null);
    onClose();
  }, [onClose]);

  const handleClose = useCallback(() => {
    if (isPending) return;
    resetAndClose();
  }, [isPending, resetAndClose]);

  const handleReasonChange = useCallback(
    (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      setReason(e.target.value);
      if (inlineError) setInlineError(null);
    },
    [inlineError],
  );

  const handleConfirm = useCallback(() => {
    if (isPending) return;
    setInlineError(null);
    const trimmedReason = reason.trim();
    mutate(
      {
        action: "DEESCALATE",
        ...(trimmedReason ? { reason: trimmedReason } : {}),
      },
      {
        onSuccess: () => {
          resetAndClose();
          onSuccess?.();
        },
        onError: (err) => {
          if (INLINE_ERROR_STATUSES.has(err.status)) {
            setInlineError(err.message);
          } else {
            const msg = err.message ?? "Failed to de-escalate case. Please try again.";
            onError?.(msg);
            resetAndClose();
          }
        },
      },
    );
  }, [reason, isPending, mutate, resetAndClose, onSuccess, onError]);

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
      aria-labelledby="deescalate-case-modal-title"
    >
      <IconButton
        aria-label="Close"
        size="small"
        onClick={handleClose}
        disabled={isPending}
        sx={{ position: "absolute", right: 12, top: 12, zIndex: 1 }}
      >
        <X size={18} />
      </IconButton>

      <DialogTitle
        id="deescalate-case-modal-title"
        sx={{ pr: 6, pb: 0.5 }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <TriangleAlert size={22} color={theme.palette.warning.main} />
          <Typography variant="h6" component="span" fontWeight={700}>
            De-escalate Case
          </Typography>
        </Box>
        <Typography
          variant="body2"
          color="text.secondary"
          sx={{ mt: 0.5, fontWeight: "normal" }}
        >
          This will remove the escalation from this case.
        </Typography>
      </DialogTitle>

      <DialogContent sx={{ pt: 1.5 }}>
        {inlineError && (
          <Alert severity="error" onClose={() => setInlineError(null)} sx={{ mb: 2 }}>
            {inlineError}
          </Alert>
        )}

        <Typography variant="body2" fontWeight={600} sx={{ mb: 1 }}>
          Reason (optional)
        </Typography>
        <TextField
          id="deescalation-reason"
          placeholder="Optionally describe why this case no longer needs to be escalated..."
          value={reason}
          onChange={handleReasonChange}
          fullWidth
          multiline
          rows={4}
          disabled={isPending}
          inputProps={{ "aria-label": "Reason for de-escalation" }}
        />
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 2.5, gap: 1 }}>
        <Button variant="outlined" onClick={handleClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleConfirm}
          disabled={isPending}
          startIcon={
            isPending ? (
              <CircularProgress size={16} color="inherit" />
            ) : (
              <TriangleAlert size={16} />
            )
          }
        >
          {isPending ? "De-escalating..." : "Confirm De-escalation"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
