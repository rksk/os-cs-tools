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

import { useState, useEffect, type ReactNode } from "react";
import {
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography, Box,
  List, ListItem, ListItemIcon, ListItemText, Collapse, IconButton, Card, CardContent,
  Accordion, AccordionSummary, AccordionDetails, Chip, Skeleton,
} from "@wso2/oxygen-ui";
import { TriangleAlertIcon, ChevronDownIcon, ChevronUpIcon, CircleCheckIcon, CircleXIcon } from "@wso2/oxygen-ui-icons-react";
import { alpha, useTheme } from "@wso2/oxygen-ui";
import { useAccountHealthDetail, useAccountHealthStatus, useInitHealthTracking, isNotFoundError } from "../api/useAccountHealthDetail";
import ReviewStatusCell from "./ReviewStatusCell";
import ActionItemsSection from "./ActionItemsSection";
import type { ProjectDetail, CaseGroup, CaseLink, EolProduct } from "../api/customerHealthTypes";

// Tracks the pointer's last known screen position so forcePointerRehover
// (below) can re-dispatch a mousemove there once a risk indicator's expand/
// collapse finishes shifting the table's layout. Module-level and shared by
// every RiskCell on the page -- cheap, and there's only ever one real mouse.
let lastPointerClientX = 0;
let lastPointerClientY = 0;
if (typeof window !== "undefined") {
  window.addEventListener(
    "mousemove",
    (e) => {
      lastPointerClientX = e.clientX;
      lastPointerClientY = e.clientY;
    },
    { passive: true },
  );
}

/**
 * A browser only recomputes `:hover` (and the resulting cursor) on an actual
 * pointer-move event -- not when the DOM shifts underneath a stationary
 * mouse. Expanding or collapsing a risk indicator's detail list changes the
 * whole table row's height, which pushes every cell below it up or down, so
 * whatever element ends up under the mouse keeps showing whatever
 * hover/cursor state it had *before* the shift until the user nudges the
 * mouse themselves -- reported live as the pointer/cursor looking "wrong"
 * right after expanding or collapsing a row. Re-dispatching a synthetic
 * mousemove at the pointer's last known position, once the Collapse
 * transition has actually finished (its own onEntered/onExited, not a
 * fixed timeout racing the animation), forces the browser to recompute
 * hover state against the new layout with no visible pointer movement.
 */
function forcePointerRehover() {
  if (typeof window === "undefined") return;
  const target = document.elementFromPoint(lastPointerClientX, lastPointerClientY) ?? window;
  target.dispatchEvent(
    new MouseEvent("mousemove", {
      bubbles: true,
      cancelable: true,
      clientX: lastPointerClientX,
      clientY: lastPointerClientY,
    }),
  );
}

/**
 * `item.eolDate` arrives as a full ISO instant (`2026-03-25T00:00:00Z`,
 * always midnight — EOL is tracked to the day, never the hour) that, shown
 * verbatim inside a chip, was the single biggest contributor to the
 * expanded-row width overflow this file otherwise fixes with
 * {@link PROJECT_COLUMN_WIDTH}: the chip alone is wider than the whole
 * column. A plain calendar date carries exactly the same information in a
 * fraction of the width. Falls back to the raw string on anything that
 * doesn't parse, rather than hiding a genuinely malformed date.
 */
function formatEolDate(raw: string): string {
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
}

// A project column's fixed width — applied to both the header cell and every
// body cell in that column (RiskCell's own TableCell below), so the column
// never grows past it. Without a matching cap on the body cells, expanding a
// row with wide detail content (an EOL product's name next to its long
// "EOL: <timestamp>" chip, with nothing else bounding their combined width)
// reported live as stretching the whole column -- and so the whole table --
// well past this width, shoving every other column sideways just because one
// row happened to be open. Detail content now wraps inside this width
// instead.
const PROJECT_COLUMN_WIDTH = 250;

interface RiskCellProps {
  project: ProjectDetail;
  indicator: keyof ProjectDetail;
  detailData?: unknown[];
  renderDetailItem: (item: never, index: number) => ReactNode;
}

function RiskCell({ project, indicator, detailData, renderDetailItem }: RiskCellProps) {
  const [open, setOpen] = useState(false);
  const hasDetails = (detailData?.length || 0) > 0;

  let statusIcon: ReactNode = null;
  let statusText = "";
  let showToggle = false;

  switch (indicator) {
    case "goLiveStatus": {
      const { status, isRisk } = project.goLiveStatus;
      statusText = status;
      if (isRisk) {
        statusIcon = <StatusIndicator isRisk />;
      } else if (status.includes("Hasn't Gone Live Since")) {
        statusIcon = (
          <Box sx={{ color: "warning.main", display: "inline-flex" }}>
            <TriangleAlertIcon size={20} />
          </Box>
        );
      } else {
        statusIcon = <StatusIndicator isRisk={false} />;
      }
      showToggle = hasDetails;
      break;
    }
    case "hasRecentCases": {
      const hasCases = project.hasRecentCases;
      const count = project.totalRecentCases;
      if (hasCases) {
        statusIcon = <StatusIndicator isRisk={false} />;
        statusText = `${count} Cases`;
        showToggle = true;
      } else {
        statusIcon = <StatusIndicator isRisk />;
        statusText = "None";
        showToggle = false;
      }
      break;
    }
    case "isUsingEolProduct": {
      const isUsing = project.isUsingEolProduct;
      if (isUsing) {
        statusIcon = <StatusIndicator isRisk />;
        statusText = "In Use";
        showToggle = hasDetails;
      } else {
        statusIcon = <StatusIndicator isRisk={false} />;
        statusText = "No";
        showToggle = false;
      }
      break;
    }
    default: {
      const riskExists = !!project[indicator];
      const count = detailData?.length || 0;
      if (riskExists) {
        statusIcon = <StatusIndicator isRisk />;
        statusText = hasDetails ? `${count} Exist` : "Exist";
        showToggle = hasDetails;
      } else {
        statusIcon = <StatusIndicator isRisk={false} />;
        statusText = "None";
        showToggle = false;
      }
      break;
    }
  }

  return (
    <TableCell sx={{ minWidth: PROJECT_COLUMN_WIDTH, maxWidth: PROJECT_COLUMN_WIDTH }}>
      <Box display="flex" alignItems="center">
        {statusIcon}
        <Typography variant="body2" sx={{ ml: 1 }}>{statusText}</Typography>
        {showToggle && (
          <IconButton size="small" onClick={() => setOpen(!open)}>
            {open ? <ChevronUpIcon size={18} /> : <ChevronDownIcon size={18} />}
          </IconButton>
        )}
      </Box>
      {showToggle && (
        <Collapse in={open} timeout="auto" unmountOnExit onEntered={forcePointerRehover} onExited={forcePointerRehover}>
          <List dense disablePadding sx={{ pl: 2 }}>
            {detailData!.map((item, index) => renderDetailItem(item as never, index))}
          </List>
        </Collapse>
      )}
    </TableCell>
  );
}

interface CustomerHealthDetailTableProps {
  accountId: string;
  onAccountNameLoaded?: (name: string) => void;
}

export default function CustomerHealthDetailTable({ accountId, onAccountNameLoaded }: CustomerHealthDetailTableProps) {
  const theme = useTheme();
  const { data: accountData, isLoading: loading, isError, error } = useAccountHealthDetail(accountId);
  const { data: healthStatusData, refetch: refetchHealthStatus } = useAccountHealthStatus(accountId);
  const initHealthTracking = useInitHealthTracking();

  // Once the project list is known, ensure all projects have a health status row in the DB.
  // Idempotent — existing rows are never touched.
  useEffect(() => {
    if (accountData?.customerProjects && accountData.customerProjects.length > 0) {
      const projectSysIds = accountData.customerProjects.map((p) => p.sysId);
      initHealthTracking.mutate({ accountId, projectSysIds });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accountData]);

  const handleHealthStatusChanged = () => {
    void refetchHealthStatus();
  };

  const [addForProject, setAddForProject] = useState<string | null>(null);

  const getProjectHealthStatus = (projectSysId: string) =>
    healthStatusData?.find((s) => s.projectSysId === projectSysId) ?? null;

  useEffect(() => {
    if (accountData?.accountName && onAccountNameLoaded) onAccountNameLoaded(accountData.accountName);
  }, [accountData, onAccountNameLoaded]);

  // The project count (and so the detail table's own column count) isn't
  // known until accountData loads, so this approximates the final layout's
  // shape -- a row of project cards above a table -- rather than a fixed
  // column skeleton like a table whose columns are known up front.
  if (loading) {
    return (
      <Box sx={{ mt: 3 }}>
        <Box sx={{ display: "flex", gap: 2, mb: 3, overflowX: "hidden" }}>
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} variant="rounded" width={320} height={140} sx={{ flexShrink: 0 }} />
          ))}
        </Box>
        <Skeleton variant="rounded" height={40} sx={{ mb: 1 }} />
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} variant="rounded" height={44} sx={{ mb: 0.5 }} />
        ))}
      </Box>
    );
  }

  // FIXED (was `error.status === 500`, matching the OLD Ballerina backend's
  // behavior of 500-ing on an unknown account — the new Go backend
  // (GetAccountDetail/GetCustomerHealthDetail) deliberately returns 404
  // instead, see viewer.ErrAccountNotFound in the backend's
  // internal/viewer/viewer.go). Checking 404 via
  // isNotFoundError (BackendApiError.status === 404).
  if (isError && isNotFoundError(error)) {
    return <Typography sx={{ mt: 3, textAlign: "center" }}>Couldn't find that account.</Typography>;
  }

  if (!accountData) return <Typography sx={{ mt: 3, textAlign: "center" }}>No data available for this account.</Typography>;

  if (!accountData.customerProjects || accountData.customerProjects.length === 0) {
    return <Typography sx={{ mt: 3, textAlign: "center" }}>No projects found for this account.</Typography>;
  }

  const riskTypesCount = [
    accountData.customerProjects.some((p) => !p.hasRecentCases),
    accountData.customerProjects.some((p) => p.hasAbandonedCases),
    accountData.customerProjects.some((p) => p.isUsingEolProduct),
    accountData.customerProjects.some((p) => p.hasMigrationDelays),
    accountData.customerProjects.some((p) => p.hasEscalatedCases),
    accountData.customerProjects.some((p) => p.goLiveStatus.isRisk),
  ].filter(Boolean).length;

  return (
    <>
      <Box sx={{ mt: 3, mb: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
          <Box>
            <Typography variant="subtitle1" fontWeight={600} sx={{ color: "text.primary", lineHeight: 1.3 }}>Project Health Overview</Typography>
            <Typography variant="caption" sx={{ color: "text.secondary" }}>Review and update the health status of each project</Typography>
          </Box>
        </Box>
        <Box sx={{ display: "flex", gap: 2, overflowX: "auto", pb: 1 }}>
          {accountData.customerProjects.map((project) => {
            const phs = getProjectHealthStatus(project.sysId);
            const status = phs?.healthStatus?.status ?? "to_be_reviewed";
            const borderColor =
              status === "at_risk"
                ? theme.palette.error.main
                : status === "healthy"
                  ? theme.palette.success.main
                  : theme.palette.warning.main;
            return (
              <Card key={project.sysId} elevation={1} sx={{ minWidth: 320, maxWidth: 400, flexShrink: 0, borderLeft: `4px solid ${borderColor}`, backgroundColor: "background.paper" }}>
                <CardContent sx={{ p: 3, pb: "24px !important" }}>
                  <Typography variant="subtitle1" fontWeight="bold" sx={{ mb: 2, fontSize: "1.1rem", lineHeight: 1.5 }}>{project.name}</Typography>
                  <ReviewStatusCell
                    project={project}
                    accountSysId={accountId}
                    healthStatus={phs}
                    onStatusChanged={handleHealthStatusChanged}
                    onAddActionItem={() => {
                      setAddForProject(project.sysId);
                      setTimeout(() => document.getElementById("action-items-section")?.scrollIntoView({ behavior: "smooth" }), 150);
                    }}
                  />
                </CardContent>
              </Card>
            );
          })}
        </Box>
      </Box>
      <Accordion sx={{ mt: 2 }} disableGutters>
        <AccordionSummary
          expandIcon={
            <Box sx={{ color: "text.secondary", display: "inline-flex" }}>
              <ChevronDownIcon size={20} />
            </Box>
          }
          sx={{ minHeight: 0, backgroundColor: "action.hover", borderBottom: `1px solid ${theme.palette.divider}`, "& .MuiAccordionSummary-content": { my: 1, alignItems: "center", gap: 1.5 } }}
        >
          <Typography fontWeight={700} sx={{ fontSize: "0.95rem", color: "text.primary" }}>Risk Indicators</Typography>
          <Chip
            label={`${riskTypesCount} of 6 flagged`}
            size="small"
            sx={{
              fontSize: "0.72rem", fontWeight: 600, height: 20,
              bgcolor: alpha(riskTypesCount > 0 ? theme.palette.warning.main : theme.palette.success.main, 0.16),
              color: riskTypesCount > 0 ? theme.palette.warning.main : theme.palette.success.main,
              border: `1px solid ${alpha(riskTypesCount > 0 ? theme.palette.warning.main : theme.palette.success.main, 0.5)}`,
              "& .MuiChip-label": { px: 1 },
            }}
          />
        </AccordionSummary>
        <AccordionDetails sx={{ p: 0 }}>
          <Box sx={{ border: 1, borderColor: "divider", borderTop: 0, borderRadius: 0 }}>
            <TableContainer sx={{ overflowX: "auto" }}>
              <Table stickyHeader size="small" aria-label="customer health detail table">
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ borderBottom: "none", position: "sticky", left: 0, zIndex: 5, backgroundColor: "background.paper", minWidth: 180, py: "6px" }} />
                    <TableCell
                      colSpan={accountData.customerProjects.length}
                      align="center"
                      sx={{ py: "6px", fontSize: "0.7rem", fontWeight: 600, letterSpacing: "1.5px", color: "text.secondary", textTransform: "uppercase", borderBottom: `1px solid ${theme.palette.divider}` }}
                    >
                      Projects
                    </TableCell>
                  </TableRow>
                  <TableRow>
                    <TableCell sx={{ fontWeight: "bold", verticalAlign: "middle", borderTop: "none", position: "sticky", left: 0, zIndex: 5, backgroundColor: "background.paper", minWidth: 180 }}>
                      Risk Indicator
                    </TableCell>
                    {accountData.customerProjects.map((project) => (
                      <TableCell key={project.sysId} sx={{ fontWeight: "bold", minWidth: PROJECT_COLUMN_WIDTH, maxWidth: PROJECT_COLUMN_WIDTH }}>{project.name}</TableCell>
                    ))}
                  </TableRow>
                </TableHead>
                <TableBody>
                  <TableRow>
                    <TableCell component="th" scope="row" sx={{ position: "sticky", left: 0, backgroundColor: "background.paper", zIndex: 1 }}>Cases Within Past 6 Months</TableCell>
                    {accountData.customerProjects.map((project) => (
                      <RiskCell
                        key={project.sysId + "-cases"}
                        project={project}
                        indicator="hasRecentCases"
                        detailData={project.detailedRecentCases}
                        renderDetailItem={(item: CaseGroup, index) => (
                          <ListItem key={index} dense sx={{ pl: 4 }}>
                            <ListItemText primary={`${item.priority} - ${item.count}`} />
                          </ListItem>
                        )}
                      />
                    ))}
                  </TableRow>
                  <TableRow>
                    <TableCell component="th" scope="row" sx={{ position: "sticky", left: 0, backgroundColor: "background.paper", zIndex: 1 }}>Abandoned Product Migrations</TableCell>
                    {accountData.customerProjects.map((project) => (
                      <RiskCell
                        key={project.sysId + "-abandoned"}
                        project={project}
                        indicator="hasAbandonedCases"
                        detailData={project.detailedAbandonedCases}
                        renderDetailItem={(item: CaseLink, index) => (
                          <CaseNumberListItem key={index} label={item.number} />
                        )}
                      />
                    ))}
                  </TableRow>
                  <TableRow>
                    <TableCell component="th" scope="row" sx={{ position: "sticky", left: 0, backgroundColor: "background.paper", zIndex: 1 }}>Using EOL Products</TableCell>
                    {accountData.customerProjects.map((project) => (
                      <RiskCell
                        key={project.sysId + "-eol"}
                        project={project}
                        indicator="isUsingEolProduct"
                        detailData={project.softwareModel}
                        renderDetailItem={(item: EolProduct, index) => (
                          <Box
                            key={index}
                            sx={{
                              mx: 2, mb: 1, p: 1,
                              border: 1, borderColor: "divider", borderLeft: 3, borderLeftColor: "error.main",
                              borderRadius: 1, backgroundColor: "action.hover",
                              maxWidth: "100%", overflow: "hidden",
                            }}
                          >
                            <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
                              <Typography variant="body2" fontWeight={600} sx={{ wordBreak: "break-word" }}>
                                {item.name}
                              </Typography>
                              <Chip
                                size="small"
                                variant="outlined"
                                color="error"
                                label={`EOL: ${formatEolDate(item.eolDate)}`}
                                sx={{ alignSelf: "flex-start", maxWidth: "100%" }}
                              />
                            </Box>
                            {item.deployments && item.deployments.length > 0 && (
                              <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5, mt: 1 }}>
                                <Typography variant="caption" color="text.secondary">Deployments:</Typography>
                                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75 }}>
                                  {item.deployments.map((dep) => (
                                    <Chip key={dep.sysId} size="small" variant="outlined" label={dep.name} />
                                  ))}
                                </Box>
                              </Box>
                            )}
                          </Box>
                        )}
                      />
                    ))}
                  </TableRow>
                  <TableRow>
                    <TableCell component="th" scope="row" sx={{ position: "sticky", left: 0, backgroundColor: "background.paper", zIndex: 1 }}>Migration Delays</TableCell>
                    {accountData.customerProjects.map((project) => (
                      <RiskCell
                        key={project.sysId + "-delays"}
                        project={project}
                        indicator="hasMigrationDelays"
                        detailData={project.detailedMigrationDelays}
                        renderDetailItem={(item: CaseLink, index) => (
                          <CaseNumberListItem key={index} label={item.number} />
                        )}
                      />
                    ))}
                  </TableRow>
                  <TableRow>
                    <TableCell component="th" scope="row" sx={{ position: "sticky", left: 0, backgroundColor: "background.paper", zIndex: 1 }}>Recent Escalations (3mo)</TableCell>
                    {accountData.customerProjects.map((project) => (
                      <RiskCell
                        key={project.sysId + "-escalations"}
                        project={project}
                        indicator="hasEscalatedCases"
                        detailData={project.detailedEscalatedCases}
                        renderDetailItem={(item: CaseLink, index) => (
                          <CaseNumberListItem key={index} label={item.escalationNumber} />
                        )}
                      />
                    ))}
                  </TableRow>
                  <TableRow>
                    <TableCell component="th" scope="row" sx={{ position: "sticky", left: 0, backgroundColor: "background.paper", zIndex: 1 }}>Go Live Status</TableCell>
                    {accountData.customerProjects.map((project) => (
                      <RiskCell
                        key={project.sysId + "-golive"}
                        project={project}
                        indicator="goLiveStatus"
                        detailData={[]}
                        renderDetailItem={() => null}
                      />
                    ))}
                  </TableRow>
                </TableBody>
              </Table>
            </TableContainer>
          </Box>
        </AccordionDetails>
      </Accordion>
      <ActionItemsSection
        accountId={accountId}
        projects={accountData.customerProjects}
        healthStatusData={healthStatusData ?? []}
        onDataChanged={handleHealthStatusChanged}
        openAddForProject={addForProject}
        onAddForProjectConsumed={() => setAddForProject(null)}
      />
    </>
  );
}

function StatusIndicator({ isRisk }: { isRisk: boolean }) {
  return (
    <Box sx={{ color: isRisk ? "error.main" : "success.main", display: "inline-flex" }}>
      {isRisk ? <CircleXIcon size={22} /> : <CircleCheckIcon size={22} />}
    </Box>
  );
}

/**
 * One case/escalation number in an expanded risk indicator's detail list,
 * marked with a plain bullet dot instead of a bare, unmarked line of text --
 * reported live as hard to read once a row lists more than one case, with
 * nothing visually separating one entry from the next.
 */
function CaseNumberListItem({ label }: { label?: string }) {
  return (
    <ListItem dense disableGutters sx={{ pl: 4 }}>
      <ListItemIcon sx={{ minWidth: 20, justifyContent: "center" }}>
        <Box
          data-testid="risk-detail-bullet"
          sx={{ width: 6, height: 6, borderRadius: "50%", backgroundColor: "text.disabled" }}
        />
      </ListItemIcon>
      <ListItemText primary={label} />
    </ListItem>
  );
}
