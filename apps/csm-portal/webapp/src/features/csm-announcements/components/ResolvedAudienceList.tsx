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

import { memo, type JSX } from "react";
import { Box, Button, CircularProgress, LinearProgress, Typography } from "@wso2/oxygen-ui";
import { Download } from "@wso2/oxygen-ui-icons-react";
import type { ResolvedAudienceProject } from "@features/csm-announcements/api/useResolveAnnouncementAudience";
import { downloadAudienceCsv } from "@features/csm-announcements/utils/audienceCsv";

interface ResolvedAudienceListProps {
  projects: ResolvedAudienceProject[];
  total: number;
  /**
   * The audience is not fully resolved yet. With no rows to show this reads as
   * a blank "Resolving audience…" spinner; combine with `isLoadingMore` to show
   * the rows that have already arrived instead.
   */
  isLoading: boolean;
  isError: boolean;
  /**
   * Rows are already available but more are still on the way: show them, with
   * a progress line and a trailing "loading more" row, instead of hiding
   * everything behind the spinner until the last page lands. Omit (or leave
   * false) for a source that only has its rows once it has all of them.
   */
  isLoadingMore?: boolean;
  /** Server-reported match count, used only to word the progress line. */
  expectedTotal?: number | null;
}

/**
 * One row is its own memoized component so that appending a page re-renders
 * only the new rows, not every row already on screen — the list grows to a
 * couple of thousand entries one page at a time while it loads.
 */
const AudienceRow = memo(function AudienceRow({
  project,
}: {
  project: ResolvedAudienceProject;
}): JSX.Element {
  return (
    <Box
      sx={{
        display: "flex",
        justifyContent: "space-between",
        gap: 1,
        px: 1.5,
        py: 0.75,
        borderBottom: 1,
        borderColor: "divider",
        "&:last-of-type": { borderBottom: 0 },
      }}
    >
      <Typography variant="body2" noWrap sx={{ minWidth: 0 }}>
        {project.name}
        {project.accountName ? ` — ${project.accountName}` : ""}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ flex: "none" }}>
        {project.key}
      </Typography>
    </Box>
  );
});

/**
 * Live preview of the "all customer projects" scope's resolved audience —
 * count, a scrollable project list, and a CSV export, per the Phase 1
 * brief's "resolved recipient list before send." This is the review step
 * that stands in for the Account Life Cycle filter the platform doesn't
 * have yet (see AudienceScopeControls' own doc comment): the engineer scans
 * this list, not just a count, before submitting.
 *
 * The full audience is dozens of pages, so while it loads the list shows what
 * has arrived so far (`isLoadingMore`) rather than a blank spinner. The count
 * is only worded as a definitive match once loading finishes, and the CSV
 * export stays disabled until then — a partial file would read as the whole
 * audience.
 */
export default function ResolvedAudienceList({
  projects,
  total,
  isLoading,
  isError,
  isLoadingMore = false,
  expectedTotal = null,
}: ResolvedAudienceListProps): JSX.Element {
  // Nothing to show yet (first page still in flight, or a refresh of an
  // already-resolved list): the blank spinner state.
  const resolvingBlank = isLoading && !isLoadingMore;
  const showRows = !isError && (!isLoading || isLoadingMore);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
        <Typography variant="body2">
          {resolvingBlank ? (
            "Resolving audience…"
          ) : isError ? (
            "Couldn't resolve the audience. Try again."
          ) : isLoadingMore ? (
            <>
              Loaded <strong>{total}</strong>
              {expectedTotal !== null ? ` of ${expectedTotal}` : ""} project{total === 1 ? "" : "s"} so
              far…
            </>
          ) : (
            <>
              <strong>{total}</strong> project{total === 1 ? "" : "s"} match{total === 1 ? "es" : ""}{" "}
              this scope
            </>
          )}
        </Typography>
        <Button
          size="small"
          variant="outlined"
          startIcon={<Download size={14} />}
          disabled={isLoading || isError || total === 0}
          onClick={() =>
            downloadAudienceCsv(projects, `announcement-audience-${new Date().toISOString().slice(0, 10)}.csv`)
          }
        >
          Export CSV
        </Button>
      </Box>

      {/* Visible without scrolling the list: the "loading remaining" row below
          sits at the bottom of a short scroll box. Determinate once the
          server has told us how many to expect. */}
      {showRows && isLoadingMore && (
        <LinearProgress
          aria-hidden
          variant={expectedTotal ? "determinate" : "indeterminate"}
          value={expectedTotal ? Math.min(100, (total / expectedTotal) * 100) : undefined}
          sx={{ height: 3, borderRadius: 1 }}
        />
      )}

      <Box
        aria-busy={isLoading}
        sx={{
          border: 1,
          borderColor: "divider",
          borderRadius: 1,
          maxHeight: 220,
          overflowY: "auto",
        }}
      >
        {resolvingBlank && (
          <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
            <CircularProgress size={20} />
          </Box>
        )}
        {!isLoading && !isError && total === 0 && (
          <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>
            No projects match these filters.
          </Typography>
        )}
        {showRows && projects.map((p) => <AudienceRow key={p.id} project={p} />)}
        {showRows && isLoadingMore && (
          <Box
            role="status"
            sx={{ display: "flex", alignItems: "center", gap: 1, px: 1.5, py: 1 }}
          >
            <CircularProgress size={14} />
            <Typography variant="body2" color="text.secondary">
              Loading remaining projects…
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  );
}
