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

import { useEffect, useRef, type JSX } from "react";
import { Alert, alpha, Box, Button, colors, Paper, Skeleton, Stack, Typography } from "@wso2/oxygen-ui";
import {
  Circle,
  CircleCheckBig,
  CircleDashed,
  CircleHelp,
  CircleMinus,
} from "@wso2/oxygen-ui-icons-react";
import type {
  ChangeRequestWorkflowStage,
  ChangeRequestWorkflowStageStatus,
} from "@features/operations/types/changeRequests";

export type ChangeRequestWorkflowProps = {
  /** The steps, as {@link buildChangeRequestWorkflowStages} builds them. */
  stages: ChangeRequestWorkflowStage[];
  /** The index of the step the change request is in (-1 when unknown): the connector below each earlier step is highlighted. */
  currentStateIndex: number;
  /** True while the evidence (the approvals) is still being read: the steps are shown as placeholders, never as "no record". */
  isLoading?: boolean;
  /**
   * True when the approvals could not be read: the panel says so above the
   * steps (the steps that needed them read "could not be loaded", see
   * `APPROVALS_UNAVAILABLE`), never "No approval recorded". It stays true while
   * they are read again: the last read failed until one succeeds.
   */
  approvalsUnavailable?: boolean;
  /** Reads the approvals again; a Retry button is offered with the notice when given. */
  onRetry?: () => void;
  /**
   * True while they are read again: the notice stays, and its button reads
   * "Retrying…" and does nothing (aria-disabled, not disabled, so the focus a
   * keyboard user gave it stays there).
   */
  isRetrying?: boolean;
};

/** What the step's marker says to assistive technology, by status. */
const STATUS_LABELS: Record<ChangeRequestWorkflowStageStatus, string> = {
  completed: "completed",
  current: "current step",
  "not-required": "not required",
  "no-record": "no record of this step",
  unavailable: "record could not be loaded",
  pending: "not reached",
  disabled: "not applicable",
};

/** Whether the step's caption is one of the neutral ones (italic, secondary). */
const isNeutral = (status: ChangeRequestWorkflowStageStatus): boolean =>
  status === "no-record" || status === "not-required" || status === "unavailable";

/** How many steps the placeholder shows while the evidence loads. */
const PLACEHOLDER_STEPS = 11;

function markerColors(status: ChangeRequestWorkflowStageStatus): {
  bgcolor: string;
  borderColor: string;
  borderStyle: "solid" | "dashed" | "dotted";
} {
  switch (status) {
    case "completed":
      return { bgcolor: alpha(colors.green[500], 0.1), borderColor: colors.green[500], borderStyle: "solid" };
    case "current":
      return { bgcolor: alpha(colors.blue[500], 0.1), borderColor: colors.blue[500], borderStyle: "solid" };
    case "no-record":
      return { bgcolor: alpha(colors.grey[500], 0.08), borderColor: colors.grey[400], borderStyle: "dashed" };
    case "not-required":
      return { bgcolor: "transparent", borderColor: colors.grey[300], borderStyle: "dashed" };
    case "unavailable":
      return { bgcolor: alpha(colors.orange[500], 0.08), borderColor: colors.orange[300], borderStyle: "dotted" };
    case "disabled":
      return { bgcolor: alpha(colors.grey[500], 0.05), borderColor: colors.grey[200], borderStyle: "solid" };
    default:
      return { bgcolor: alpha(colors.grey[500], 0.1), borderColor: colors.grey[300], borderStyle: "solid" };
  }
}

function StageMarkerIcon({ status }: { status: ChangeRequestWorkflowStageStatus }): JSX.Element {
  switch (status) {
    case "completed":
      return <CircleCheckBig size={20} color={colors.green[600]} />;
    case "current":
      return <Circle size={20} color={colors.blue[600]} fill={colors.blue[600]} />;
    case "no-record":
      // A dashed ring: the step was passed, but nothing on record says it happened.
      return <CircleDashed size={20} color={colors.grey[500]} />;
    case "not-required":
      // A struck ring: the step does not apply to this change request.
      return <CircleMinus size={20} color={colors.grey[500]} />;
    case "unavailable":
      // A question mark: the record that would say could not be read.
      return <CircleHelp size={20} color={colors.orange[800]} />;
    default:
      return <Circle size={20} color={colors.grey[400]} fill="none" />;
  }
}

/**
 * The "Change Request Workflow" panel: every step of the flow with what the
 * record says of it. A tick means the record shows the step happened; a step
 * the change request passed with nothing on record is a dashed, unticked ring
 * captioned "No approval recorded", and one that does not apply to the change
 * (its requirement box is off) a struck ring captioned "Not required". When the
 * approvals could not be read, a step only they could vouch for is a dotted ring
 * with a question mark captioned "Approval record could not be loaded", and the
 * panel says so above the steps (with a Retry): a failed read is not a negative
 * result.
 */
export default function ChangeRequestWorkflow({
  stages,
  currentStateIndex,
  isLoading = false,
  approvalsUnavailable = false,
  onRetry,
  isRetrying = false,
}: ChangeRequestWorkflowProps): JSX.Element {
  const headingRef = useRef<HTMLHeadingElement | null>(null);
  // Set when Retry is used. A retry that succeeds takes the notice away, and
  // with it the button that had focus, which would drop to the document body:
  // the panel's heading takes it then, but only when focus has nowhere else to
  // be -- never from a control the customer has moved to since.
  const retryUsedRef = useRef(false);
  const showsSteps = !isLoading && !approvalsUnavailable;
  useEffect(() => {
    if (!showsSteps || !retryUsedRef.current) return;
    retryUsedRef.current = false;
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    headingRef.current?.focus();
  }, [showsSteps]);

  return (
    <Paper variant="outlined" data-testid="change-request-workflow">
      <Box sx={{ px: 3, pt: 3 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
          <Typography variant="h6" ref={headingRef} tabIndex={-1} sx={{ outline: "none" }}>
            Change Request Workflow
          </Typography>
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Track the progress of this change request through each stage
        </Typography>
      </Box>

      <Box sx={{ px: 3, pb: 3 }}>
        {/* Mounted for as long as the last read failed, retries included: a
            status region that stays is where a retry that fails again is
            heard (its button reads "Retry" again). */}
        {!isLoading && approvalsUnavailable && (
          <Alert
            severity="warning"
            role="status"
            data-testid="change-request-workflow-approvals-unavailable"
            sx={{ mb: 2 }}
            action={
              onRetry ? (
                <Button
                  color="inherit"
                  size="small"
                  aria-disabled={isRetrying || undefined}
                  sx={isRetrying ? { cursor: "progress" } : undefined}
                  onClick={() => {
                    if (isRetrying) return;
                    retryUsedRef.current = true;
                    onRetry();
                  }}
                >
                  {isRetrying ? "Retrying…" : "Retry"}
                </Button>
              ) : undefined
            }
          >
            The approval records could not be loaded. The steps that depend on
            them are shown as unknown.
          </Alert>
        )}
        {isLoading ? (
          <Stack spacing={2} data-testid="change-request-workflow-loading" aria-busy="true">
            {Array.from({ length: PLACEHOLDER_STEPS }, (_, index) => (
              <Box key={index} sx={{ display: "flex", gap: 2, alignItems: "center" }}>
                <Skeleton variant="circular" width={40} height={40} />
                <Box sx={{ flex: 1 }}>
                  <Skeleton variant="text" width="40%" />
                  <Skeleton variant="text" width="70%" />
                </Box>
              </Box>
            ))}
          </Stack>
        ) : (
          <Stack spacing={0}>
            {stages.map((stage, index) => {
              const marker = markerColors(stage.status);
              const muted = stage.status === "disabled" || stage.status === "not-required";
              const captionColor =
                stage.status === "completed" || stage.status === "current"
                  ? "text.primary"
                  : isNeutral(stage.status)
                    ? "text.secondary"
                    : "text.disabled";
              return (
                <Box
                  key={stage.name}
                  sx={{ position: "relative" }}
                  data-testid="change-request-workflow-stage"
                  data-stage-name={stage.name}
                  data-stage-status={stage.status}
                >
                  <Box sx={{ display: "flex", gap: 2 }}>
                    <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
                      <Box
                        role="img"
                        aria-label={`${stage.name}: ${STATUS_LABELS[stage.status]}`}
                        sx={{
                          width: 40,
                          height: 40,
                          borderRadius: "50%",
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                          border: "2px solid",
                          borderStyle: marker.borderStyle,
                          borderColor: marker.borderColor,
                          bgcolor: marker.bgcolor,
                          opacity: stage.status === "disabled" ? 0.5 : 1,
                        }}
                      >
                        <StageMarkerIcon status={stage.status} />
                      </Box>
                      {index < stages.length - 1 && (
                        <Box
                          sx={{
                            width: 2,
                            height: 64,
                            mt: 0.5,
                            bgcolor: currentStateIndex > index ? colors.green[300] : colors.grey[200],
                            opacity: 1,
                          }}
                        />
                      )}
                    </Box>
                    <Box sx={{ flex: 1, pb: index < stages.length - 1 ? 2 : 0 }}>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                        <Box>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                            <Typography
                              variant="body2"
                              fontWeight={stage.current ? 600 : 500}
                              color={
                                stage.disabled
                                  ? "text.disabled"
                                  : stage.current
                                    ? colors.blue[900]
                                    : stage.status === "not-required"
                                      ? "text.secondary"
                                      : "text.primary"
                              }
                              sx={{ opacity: muted ? 0.7 : 1 }}
                            >
                              {stage.name}
                            </Typography>
                            {stage.current && (
                              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                                <Box
                                  sx={{
                                    width: 6,
                                    height: 6,
                                    bgcolor: colors.blue[600],
                                    borderRadius: "50%",
                                    flexShrink: 0,
                                  }}
                                />
                                <Typography
                                  variant="caption"
                                  sx={{ color: colors.blue[800], fontSize: "0.7rem" }}
                                >
                                  Current
                                </Typography>
                              </Box>
                            )}
                          </Box>
                          <Typography
                            variant="caption"
                            color={captionColor}
                            sx={{
                              opacity: stage.disabled ? 0.5 : 1,
                              fontStyle: isNeutral(stage.status) ? "italic" : "normal",
                            }}
                          >
                            {stage.description}
                          </Typography>
                        </Box>
                      </Box>
                    </Box>
                  </Box>
                </Box>
              );
            })}
          </Stack>
        )}
      </Box>
    </Paper>
  );
}
