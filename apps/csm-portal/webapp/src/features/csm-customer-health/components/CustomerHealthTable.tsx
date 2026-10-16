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

import { type ChangeEvent, type ReactElement } from "react";
import { useNavigate } from "react-router";
import {
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography, Box,
  TablePagination, Chip, Button, CircularProgress, Skeleton, Tooltip,
} from "@wso2/oxygen-ui";
import { DownloadIcon, InfoIcon } from "@wso2/oxygen-ui-icons-react";
import { useTheme } from "@wso2/oxygen-ui";
import QueryErrorState from "@components/QueryErrorState";
import { useCustomerHealthSummary } from "../api/useCustomerHealthSummary";
import { HEALTH_STATUS_LABELS, TOOLTIP_TEXT, type AccountSummary, type RiskFilterKey } from "../api/customerHealthTypes";

type CustomerHealthTableProps = {
  accountScope: "my-accounts" | "all-accounts";
  userEmail: string | null;
  searchQuery: string;
  riskIndicators: RiskFilterKey[];
  region: string[];
  product: string | null;
  abtTeam: string | null;
  healthStatus: string | null;
  page: number;
  rowsPerPage: number;
  onPageChange: (newPage: number) => void;
  onRowsPerPageChange: (newRowsPerPage: number) => void;
  onExport: () => void;
  exporting: boolean;
};

const COLUMN_COUNT = 8;

export default function CustomerHealthTable(props: CustomerHealthTableProps): ReactElement {
  const theme = useTheme();
  const navigate = useNavigate();

  const { data, isLoading: loading, isError, error, refetch } = useCustomerHealthSummary({
    offset: props.page * props.rowsPerPage,
    limit: props.rowsPerPage,
    email: props.accountScope === "my-accounts" && props.userEmail ? props.userEmail : "",
    phrase: props.searchQuery && props.searchQuery.length >= 2 ? props.searchQuery : "",
    risks: Array.isArray(props.riskIndicators) && props.riskIndicators.length > 0 ? props.riskIndicators.join(",") : "",
    region: Array.isArray(props.region) && props.region.length > 0 ? props.region : [],
    product: props.product || "",
    abtTeam: props.abtTeam || "",
    healthStatus: props.healthStatus || "",
  });

  const accounts = data?.data || [];
  const totalCount = data?.totalCount || 0;

  const handleChangePage = (_event: unknown, newPage: number) => {
    props.onPageChange(newPage);
  };

  const handleChangeRowsPerPage = (event: ChangeEvent<HTMLInputElement>) => {
    props.onRowsPerPageChange(parseInt(event.target.value, 10));
  };

  const handleAccountTableRowClick = (rowData: AccountSummary) => {
    const localIndex = accounts.findIndex((a) => a.accountSysId === rowData.accountSysId);
    navigate(`/customer-health/account/${rowData.accountSysId}`, {
      state: {
        accountList: accounts.map((a) => ({ accountSysId: a.accountSysId, accountName: a.accountName })),
        currentIndex: localIndex,
        absoluteIndex: props.page * props.rowsPerPage + localIndex,
        totalCount,
        filterPayload: {
          email: props.accountScope === "my-accounts" && props.userEmail ? props.userEmail : "",
          phrase: props.searchQuery && props.searchQuery.length >= 2 ? props.searchQuery : "",
          risks: Array.isArray(props.riskIndicators) && props.riskIndicators.length > 0 ? props.riskIndicators.join(",") : "",
          region: Array.isArray(props.region) && props.region.length > 0 ? props.region : [],
          product: props.product || "",
          abtTeam: props.abtTeam || "",
          healthStatus: props.healthStatus || "",
        },
      },
    });
  };

  return (
    <>
      {/* Heading and count share one row (rather than stacking as two
          separate lines) specifically to cut vertical space -- reported
          live as still too much gap between the filter bar and the table
          even after trimming the surrounding margins, since the dominant
          cost was a whole second text line's own height, not the margins
          around it. */}
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", mb: 1 }}>
        <Typography variant="h6">
          Accounts Overview{" "}
          <Typography component="span" variant="body2" color="text.secondary">
            ({totalCount} accounts found
            {props.healthStatus && HEALTH_STATUS_LABELS[props.healthStatus] &&
              ` · Filtered by: ${HEALTH_STATUS_LABELS[props.healthStatus]}`})
          </Typography>
        </Typography>
        <Button
          variant="text"
          size="small"
          startIcon={props.exporting ? <CircularProgress size={14} /> : <DownloadIcon size={14} />}
          onClick={props.onExport}
          disabled={props.exporting || totalCount === 0}
        >
          {props.exporting ? "Exporting…" : "Export CSV"}
        </Button>
      </Box>

      <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, overflow: "hidden" }}>
        <TableContainer>
          <Table size="small" aria-label="customer health summary table" sx={{ tableLayout: "fixed", "& .MuiTableCell-root": { borderColor: "divider" } }}>
            <TableHead>
              <TableRow sx={{ bgcolor: "action.hover" }}>
                <TableCell rowSpan={2} sx={{ verticalAlign: "middle", width: "19%" }}>
                  Account Name
                </TableCell>
                <TableCell rowSpan={2} align="center" sx={{ verticalAlign: "middle", width: "12%" }}>
                  <HeaderTooltip label="Health Status" sub={null} text={TOOLTIP_TEXT.healthStatus} />
                </TableCell>
                <TableCell
                  colSpan={6}
                  align="center"
                  sx={{ color: "text.secondary", fontWeight: 600, fontSize: "0.72rem", letterSpacing: "1px", textTransform: "uppercase", borderBottom: `1px solid ${theme.palette.divider}` }}
                >
                  Risk Indicators
                </TableCell>
              </TableRow>
              <TableRow sx={{ bgcolor: "action.hover" }}>
                {[
                  { label: "Has Gone Live", sub: null, tip: TOOLTIP_TEXT.goLive },
                  { label: "Support Activity", sub: "(Last 6 Mo)", tip: TOOLTIP_TEXT.support },
                  { label: "Using EOL Products", sub: null, tip: TOOLTIP_TEXT.eol },
                  { label: "Abandoned Migrations", sub: null, tip: TOOLTIP_TEXT.abandoned },
                  { label: "Migration Delays", sub: null, tip: TOOLTIP_TEXT.delays },
                  { label: "Escalations", sub: "(Last 3 Mo)", tip: TOOLTIP_TEXT.escalations },
                ].map(({ label, sub, tip }) => (
                  <TableCell key={label} align="center" sx={{ width: "11.5%" }}>
                    <HeaderTooltip label={label} sub={sub} text={tip} />
                  </TableCell>
                ))}
              </TableRow>
            </TableHead>
            <TableBody>
              {loading ? (
                Array.from({ length: props.rowsPerPage }).map((_, i) => (
                  <TableRow key={i}>
                    <TableCell>
                      <Skeleton variant="rounded" width="80%" height={18} />
                    </TableCell>
                    <TableCell align="center">
                      <Skeleton variant="rounded" width={110} height={24} sx={{ mx: "auto" }} />
                    </TableCell>
                    {Array.from({ length: 6 }).map((__, c) => (
                      <TableCell key={c} align="center">
                        <Skeleton variant="rounded" width={32} height={24} sx={{ mx: "auto" }} />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : isError ? (
                <TableRow>
                  <TableCell colSpan={COLUMN_COUNT} align="center">
                    <QueryErrorState
                      message={error instanceof Error && error.message.trim() ? error.message : "Failed to load accounts."}
                      error={error}
                      onRetry={() => void refetch()}
                    />
                  </TableCell>
                </TableRow>
              ) : accounts.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={COLUMN_COUNT} align="center" sx={{ py: 4 }}>
                    <Typography variant="body2" color="text.secondary">
                      No accounts found matching the criteria.
                    </Typography>
                  </TableCell>
                </TableRow>
              ) : (
                accounts.map((acc) => (
                  <TableRow
                    key={acc.accountSysId}
                    hover
                    sx={{ cursor: "pointer" }}
                    onClick={() => handleAccountTableRowClick(acc)}
                  >
                    <TableCell component="th" scope="row" sx={{ maxWidth: 360 }}>
                      <Tooltip title={acc.accountName || ""} arrow placement="top" disableHoverListener={!acc.accountName}>
                        <Typography variant="body2" noWrap color="primary">
                          {acc.accountName || `Account ID: ${acc.accountSysId.substring(0, 8)}...`}
                        </Typography>
                      </Tooltip>
                    </TableCell>
                    <TableCell align="center">
                      <ReviewStatusBadge status={acc.healthStatus} />
                    </TableCell>
                    <TableCell align="center"><StatusIndicator isRisk={acc.hasNoGoLive.isRisk} state={acc.hasNoGoLive.state} /></TableCell>
                    <TableCell align="center"><StatusIndicator isRisk={acc.noSupportCases6mo} /></TableCell>
                    <TableCell align="center"><StatusIndicator isRisk={acc.hasEolProduct} riskShowsYes /></TableCell>
                    <TableCell align="center"><StatusIndicator isRisk={acc.hasAbandonedMigrations} riskShowsYes /></TableCell>
                    <TableCell align="center"><StatusIndicator isRisk={acc.hasMigrationDelays} riskShowsYes /></TableCell>
                    <TableCell align="center"><StatusIndicator isRisk={acc.hasRecentEscalations} riskShowsYes /></TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </TableContainer>

        <TablePagination
          rowsPerPageOptions={[5, 10, 25]}
          component="div"
          count={totalCount}
          rowsPerPage={props.rowsPerPage}
          page={props.page}
          onPageChange={handleChangePage}
          onRowsPerPageChange={handleChangeRowsPerPage}
          showFirstButton
          showLastButton
        />
      </Box>
    </>
  );
}

// Slightly smaller than a default small Chip's own padding, but NOT shrunk
// so far that "To Be Reviewed" (the longest label) gets its own text
// truncated with an ellipsis -- an earlier pass over-corrected for the
// column crowding Has Gone Live by shrinking this chip until its label no
// longer fit at all. The real fix for the crowding is the column-width
// rebalance above (Health Status widened back to 12%, the six single-letter
// risk columns narrowed from 12.5% to 11.5% to make room) -- this chip only
// needs a little horizontal padding trimmed, not a smaller font.
const reviewStatusChipSx = { fontWeight: 600, fontSize: "0.75rem", minWidth: 110, "& .MuiChip-label": { px: 1.25 } };

function ReviewStatusBadge({ status }: { status?: string }) {
  if (!status || status === "to_be_reviewed") {
    return <Chip label="To Be Reviewed" size="small" color="warning" variant="outlined" sx={reviewStatusChipSx} />;
  }
  if (status === "at_risk") {
    return <Chip label="At Risk" size="small" color="error" variant="outlined" sx={reviewStatusChipSx} />;
  }
  if (status === "healthy") {
    return <Chip label="Healthy" size="small" color="success" variant="outlined" sx={reviewStatusChipSx} />;
  }
  return null;
}

// By default the letter answers the positively-phrased column label (e.g. "Has Gone
// Live"): a risk shows "N". For columns phrased as the risk condition itself (e.g.
// "Using EOL Products"), pass riskShowsYes so a risk shows "Y" instead - the color
// (error on risk, success otherwise) never changes, only which letter reflects that state.
function StatusIndicator({ isRisk, state, riskShowsYes = false }: { isRisk: boolean; state?: string; riskShowsYes?: boolean }) {
  if (!isRisk && state === "pending") {
    return (
      <Tooltip title="Not Live Yet" arrow placement="top">
        <Chip label="P" size="small" color="warning" sx={{ fontWeight: 700, minWidth: 32 }} />
      </Tooltip>
    );
  }

  if (state === "failure" || isRisk === true) {
    return <Chip label={riskShowsYes ? "Y" : "N"} size="small" color="error" sx={{ fontWeight: 700, minWidth: 32 }} />;
  }

  return <Chip label={riskShowsYes ? "N" : "Y"} size="small" color="success" sx={{ fontWeight: 700, minWidth: 32 }} />;
}

// Every risk-indicator header renders as exactly two lines, always in the
// same shape: the label alone on line 1, and the info icon -- preceded by
// `sub` (a parenthetical qualifier like "(Last 6 Mo)") when there is one --
// on line 2. This replaced an earlier version where the icon was just the
// last inline "word" of a single wrapping block of text: a short label
// (e.g. "Has Gone Live") stayed on one line with the icon trailing right
// after it, while a longer one (e.g. "Support Activity (Last 6 Mo)") wrapped
// the browser's own way, landing the icon on its own line in some columns
// but not others -- reported live as visibly inconsistent icon placement
// across the header row. Forcing every column through the same two-line
// template (label / qualifier+icon) fixes both: the icon is always on line 2
// in the same relative position, and no column is one line while its
// neighbours are two.
function HeaderTooltip({ label, sub, text }: { label: string; sub?: string | null; text: string }) {
  return (
    <Typography component="span" variant="body2" sx={{ fontWeight: 600, lineHeight: 1.3 }}>
      <Box component="span" sx={{ display: "block", whiteSpace: "normal" }}>
        {label}
      </Box>
      <Box component="span" sx={{ display: "block", whiteSpace: "nowrap" }}>
        {sub && `${sub} `}
        <Tooltip title={text} arrow placement="top">
          <Box component="span" sx={{ display: "inline-flex", verticalAlign: "middle", color: "text.secondary", cursor: "help", "&:hover": { color: "text.primary" } }}>
            <InfoIcon size={14} />
          </Box>
        </Tooltip>
      </Box>
    </Typography>
  );
}
