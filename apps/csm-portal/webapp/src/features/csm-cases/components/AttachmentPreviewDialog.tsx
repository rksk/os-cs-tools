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
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  Typography,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";
import { useEffect, useRef, useState, type JSX } from "react";
import type { CaseAttachment } from "@features/csm-cases/types/csmCases";
import { getAttachmentPreviewKind } from "@features/csm-cases/utils/attachmentPreview";

interface AttachmentPreviewDialogProps {
  /** Attachment being previewed; the dialog is closed when this is null. */
  attachment: CaseAttachment | null;
  onClose: () => void;
  /**
   * Fetch the attachment's raw bytes. The BE content endpoint always sets
   * `Content-Disposition: attachment` and requires auth headers, so a plain
   * `<img src>` pointed at it would force a download instead of rendering —
   * the bytes are fetched here as a `Blob` and turned into an object URL for
   * the preview element instead.
   */
  fetchContent: (attachment: CaseAttachment) => Promise<Blob>;
}

/**
 * Preview for image/PDF attachments (the two families in the backend's
 * safe-content-type allowlist that make sense to preview — see
 * {@link getAttachmentPreviewKind}). Fetches the attachment's bytes via
 * `fetchContent` (the same authenticated content endpoint the download
 * action uses) and turns them into a `blob:` object URL, which is revoked on
 * close/unmount to avoid leaking memory. Images render inline; PDFs open in
 * a new browser tab instead (see the PDF-branch comment below for why).
 */
export default function AttachmentPreviewDialog({
  attachment,
  onClose,
  fetchContent,
}: AttachmentPreviewDialogProps): JSX.Element {
  const [objectUrl, setObjectUrl] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Guards the auto-open effect below so a PDF is only opened in a new tab
  // once per loaded object URL, not on every re-render.
  const pdfAutoOpenedUrlRef = useRef<string | null>(null);

  // Track which attachment the state above belongs to, and reconcile it
  // *synchronously during render* the moment `attachment` changes (React's
  // "adjusting state when a prop changes" pattern). Effects only run after
  // the render has already committed to the DOM, so resetting this state
  // from inside a `useEffect` leaves a frame where the dialog still paints
  // the *previous* attachment's stale objectUrl/error before the effect
  // fires — a visible flash of the wrong preview. Comparing here and calling
  // the setters directly in the render body avoids that: React re-renders
  // immediately with the reset state before the browser paints anything.
  const [renderedFor, setRenderedFor] = useState(attachment);
  if (attachment !== renderedFor) {
    setRenderedFor(attachment);
    setObjectUrl(null);
    setError(null);
    setLoading(!!attachment);
    pdfAutoOpenedUrlRef.current = null;
  }

  useEffect(() => {
    if (!attachment) return;

    let cancelled = false;
    let createdUrl: string | null = null;

    void fetchContent(attachment)
      .then((blob) => {
        if (cancelled) return;
        createdUrl = URL.createObjectURL(blob);
        setObjectUrl(createdUrl);
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setError(
            err instanceof Error ? err.message : "Could not load the preview.",
          );
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
      if (createdUrl) URL.revokeObjectURL(createdUrl);
    };
    // `fetchContent` is a stable useCallback from the caller's hook.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [attachment]);

  const kind = attachment ? getAttachmentPreviewKind(attachment.contentType) : null;
  const open = !!attachment;

  // PDFs used to render inline in a sandboxed iframe pointed at the blob
  // URL, but Chrome's built-in PDF viewer refuses to render at all inside a
  // sandboxed iframe (https://crbug.com/413851) and shows its own
  // "blocked by Chrome" interstitial instead of the PDF, regardless of which
  // sandbox flags are set. There is no fix that keeps the preview inline
  // without either dropping the sandbox (unsafe for attacker-controlled
  // bytes) or shipping a pdf.js renderer, so PDFs are opened in a new tab
  // instead. This fires once automatically as soon as the blob is ready;
  // the button below is the fallback for when the browser's popup blocker
  // swallows that automatic call (it runs from a user click, so it is never
  // blocked).
  useEffect(() => {
    if (kind !== "pdf" || !objectUrl) return;
    if (pdfAutoOpenedUrlRef.current === objectUrl) return;
    pdfAutoOpenedUrlRef.current = objectUrl;
    window.open(objectUrl, "_blank", "noopener,noreferrer");
  }, [kind, objectUrl]);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      aria-label={attachment ? `Preview ${attachment.filename}` : "Preview"}
    >
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1,
        }}
      >
        <Typography
          component="span"
          variant="subtitle1"
          noWrap
          sx={{ minWidth: 0 }}
        >
          {attachment?.filename}
        </Typography>
        <IconButton size="small" onClick={onClose} aria-label="Close preview">
          <X size={18} />
        </IconButton>
      </DialogTitle>
      <DialogContent
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          minHeight: 320,
          bgcolor: "action.hover",
        }}
      >
        {loading ? (
          <CircularProgress size={28} />
        ) : error ? (
          <Typography variant="body2" color="error">
            {error}
          </Typography>
        ) : objectUrl && kind === "image" ? (
          <Box
            component="img"
            src={objectUrl}
            alt={attachment?.filename}
            sx={{
              maxWidth: "100%",
              maxHeight: "70vh",
              width: "auto",
              height: "auto",
              objectFit: "contain",
            }}
          />
        ) : objectUrl && kind === "pdf" ? (
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 2,
              textAlign: "center",
              p: 3,
            }}
          >
            <Typography variant="body2" color="text.secondary">
              PDF attachments open in a new browser tab.
            </Typography>
            <Button
              variant="outlined"
              size="small"
              onClick={() =>
                window.open(objectUrl, "_blank", "noopener,noreferrer")
              }
            >
              Open {attachment?.filename}
            </Button>
          </Box>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
