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

import { Alert, Box, Button, IconButton, Stack, Tooltip } from "@wso2/oxygen-ui";
import { Check, Copy, X } from "@wso2/oxygen-ui-icons-react";
import {
  BANNER_HEADER_GAP_PX,
  BANNER_RIGHT_GAP_PX,
  HEADER_HEIGHT_PX,
} from "@constants/common";
import { useState, type JSX } from "react";

interface ErrorBannerProps {
  /** User-facing error message; supplied by the caller. */
  message: string;
  /** The request's correlation ID, when the triggering error carried one.
   *  Rendered as a separate copy affordance rather than folded into
   *  `message` — the banner auto-dismisses on a timer, so a raw id embedded
   *  in the text is otherwise only copyable by hand before it vanishes. */
  referenceId?: string;
  onClose: () => void;
}

/**
 * ErrorBanner component displayed above the footer at the right corner.
 * Uses Oxygen UI Alert component. Displays the message passed from the caller,
 * plus a one-click copy button for the correlation ID when one is available.
 *
 * @param {ErrorBannerProps} props - Component props.
 * @returns {JSX.Element} The ErrorBanner JSX.
 */
export default function ErrorBanner({
  message,
  referenceId,
  onClose,
}: ErrorBannerProps): JSX.Element {
  const [copied, setCopied] = useState(false);

  const handleCopy = (): void => {
    if (!referenceId || !navigator.clipboard) return;
    navigator.clipboard.writeText(referenceId).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }, /* swallow — clipboard denied/unavailable */ () => {});
  };

  return (
    <Box
      sx={{
        position: "fixed",
        top: HEADER_HEIGHT_PX + BANNER_HEADER_GAP_PX,
        right: BANNER_RIGHT_GAP_PX,
        width: 400,
        zIndex: 1500,
      }}
    >
      <Alert
        severity="error"
        elevation={6}
        sx={{ width: "100%" }}
        action={
          // `action` replaces Alert's own auto-rendered close button (MUI only
          // renders that button from `onClose` when no `action` is supplied),
          // so once a copy button is added here the close control must be
          // included in `action` too, or it silently disappears.
          <Stack direction="row" spacing={0.5} alignItems="center">
            {referenceId && (
              <>
                <Tooltip title={copied ? "Copied!" : "Copy reference ID"} placement="top">
                  <Button
                    size="small"
                    variant="text"
                    color="inherit"
                    onClick={handleCopy}
                    sx={{ minWidth: 0, p: 0.5 }}
                    aria-label="Copy reference ID"
                  >
                    {copied ? <Check size={14} /> : <Copy size={14} />}
                  </Button>
                </Tooltip>
                {/* The icon/tooltip swap on copy isn't announced to screen readers on
                    its own; this visually-hidden live region is the actual completion
                    signal for assistive tech. */}
                <Box
                  aria-live="polite"
                  sx={{
                    position: "absolute",
                    width: 1,
                    height: 1,
                    overflow: "hidden",
                    clip: "rect(0 0 0 0)",
                  }}
                >
                  {copied ? "Reference ID copied." : ""}
                </Box>
              </>
            )}
            <IconButton size="small" color="inherit" onClick={onClose} aria-label="Close">
              <X size={16} />
            </IconButton>
          </Stack>
        }
      >
        {message}
      </Alert>
    </Box>
  );
}
