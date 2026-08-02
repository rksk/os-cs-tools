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

import { Box, Chip, TablePagination, Typography } from "@wso2/oxygen-ui";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type JSX,
  type ReactNode,
} from "react";
import { useSearchParams } from "react-router";
import { useErrorBanner } from "@context/error-banner/ErrorBannerContext";
import { useCurrentUser } from "@context/current-user/CurrentUserContext";
import { useDebouncedValue } from "@hooks/useDebouncedValue";
import { useIdTokenClaims } from "@hooks/useIdTokenClaims";
import { formatBackendTimestampForDisplay } from "@utils/dateTime";
import { useBackendApi } from "@api/backend/client";
import FilteredCsvExportButton from "@components/FilteredCsvExportButton";
import CasesFilterBar, {
  type CasesFilters,
} from "@features/csm-cases/components/CasesFilterBar";
import CasesList from "@features/csm-cases/components/CasesList";
import { useGetCsmCases } from "@features/csm-cases/api/useGetCsmCases";
import {
  ASSIGNEE_FILTER_RESOLVED_EMPTY,
  buildCaseSearchFilters,
  mapCaseSearchViewToRow,
  resolveAssignedUserIds,
} from "@features/csm-cases/utils/caseSearchPayload";
import { useDirectoryUsers } from "@api/useDirectoryUsers";
import { BE_MAX_PAGE_LIMIT } from "@constants/apiConstants";
import { ALL_CASE_TYPES } from "@features/csm-cases/utils/caseType";
import {
  DEFAULT_CASES_FILTERS,
  readCasesFiltersFromUrl,
  writeCasesFiltersToUrl,
} from "@features/csm-cases/utils/casesFiltersUrl";
import {
  DEFAULT_CASES_SORT,
  type CasesSortOrder,
} from "@features/csm-cases/utils/casesSort";
import { stateLabel } from "@features/csm-dashboard/utils/abtDashboard";
import { WORK_STATE_LABEL } from "@features/csm-cases/utils/caseWorkState";
import type { BeCaseSearchPayload, BeCaseSearchResponse } from "@api/backend/types";
import type { CsmCaseRow } from "@features/csm-cases/types/csmCases";

const DEFAULT_ROWS_PER_PAGE = 20;
const ROWS_PER_PAGE_OPTIONS = [10, DEFAULT_ROWS_PER_PAGE, BE_MAX_PAGE_LIMIT];

// URL params owned by the filter state; cleared/rewritten on change while any
// other params (e.g. a `tab` selection) are preserved.
const FILTER_PARAM_KEYS = [
  "search",
  "severities",
  "states",
  "types",
  "assignees",
  "projects",
  "engagementTypes",
  "products",
] as const;

interface CsmIssuesViewProps {
  /** Optional heading shown on the left of the header row. */
  title?: string;
  /** Optional right-aligned actions (e.g. a "Create" button). */
  actions?: ReactNode;
  /** Plural noun for the count subtitle / empty states. Default "cases". */
  entityNoun?: string;
  /** Filter values forced onto every query and hidden from the user (e.g. a
   *  locked case type or project). Merged over the user's URL filters. */
  lockedFilters?: Partial<CasesFilters>;
  /** Hide the case-type filter control (use when `lockedFilters` fixes it). */
  hideTypeFilter?: boolean;
  /** Hide the project filter control (use when the view is project-scoped). */
  hideProjectFilter?: boolean;
  /** Show the engagement-type sub-filter (pass when the view is locked to engagement cases). */
  showEngagementTypeFilter?: boolean;
  /** Base path for row detail links. Defaults to "/cases". */
  detailBasePath?: string;
  /** Hide the Severity column in the list (severity is a support-case
   * concept — SRA and Engagements don't surface it, but the main case list
   * keeps it). */
  hideSeverityColumn?: boolean;
}

/**
 * Shared issues list: the cases filter bar + list + pagination, backed by
 * `POST /cases/search`. Reused for the all-cases page, the per-type lists
 * (service requests, security reports) and the project-scoped issues tab —
 * each just supplies `lockedFilters` (and hides the now-fixed control) so the
 * one component covers every "list issues of kind X" surface.
 */
export default function CsmIssuesView({
  title,
  actions,
  entityNoun = "cases",
  lockedFilters,
  hideTypeFilter,
  hideProjectFilter,
  showEngagementTypeFilter,
  detailBasePath,
  hideSeverityColumn,
}: CsmIssuesViewProps): JSX.Element {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = useMemo<CasesFilters>(
    () => readCasesFiltersFromUrl(searchParams),
    [searchParams],
  );

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(DEFAULT_ROWS_PER_PAGE);
  const [isFiltersOpen, setIsFiltersOpen] = useState(true);
  const [sortOrder, setSortOrder] = useState<CasesSortOrder>(
    DEFAULT_CASES_SORT.order,
  );

  const setFilters = useCallback(
    (next: CasesFilters) => {
      setPage(0);
      // Preserve any non-filter params (e.g. the active project-detail tab).
      const merged = new URLSearchParams(searchParams);
      FILTER_PARAM_KEYS.forEach((k) => merged.delete(k));
      writeCasesFiltersToUrl(next).forEach((v, k) => merged.set(k, v));
      setSearchParams(merged, { replace: true });
    },
    [searchParams, setSearchParams],
  );

  // Severity (S1-S4) is a support-case concept, so the severity filter is only
  // shown on the support-cases list — i.e. when the surrounding view locks the
  // record type to `case`. Every other list (service requests, engagements,
  // security reports, mixed project issues) hides it.
  const showSeverityFilter =
    lockedFilters?.caseTypes?.length === 1 &&
    lockedFilters.caseTypes[0] === "case";
  // Service Requests don't carry a severity, so the column is redundant there.
  const isServiceRequestOnly =
    lockedFilters?.caseTypes?.length === 1 &&
    lockedFilters.caseTypes[0] === "service_request";

  const debouncedSearch = useDebouncedValue(filters.search, 300);
  // User filters (debounced search) with the locked overrides applied last so
  // the fixed type/project can't be widened by a stale URL value.
  const queryFilters = useMemo<CasesFilters>(
    () => {
      const merged: CasesFilters = {
        ...filters,
        search: debouncedSearch,
        // The severity control is hidden for non-case lists, so don't let a
        // stale `severities` value from a shared URL silently filter those
        // results.
        ...(showSeverityFilter ? {} : { severities: [] }),
        ...lockedFilters,
      };
      // An unlocked, empty type selection means "no type filter applied" from
      // the FE's perspective (every issue type shown — this is the only
      // unlocked, multi-type view; every other CsmIssuesView caller locks
      // `caseTypes` to a single value and hides the control). But
      // `useGetCsmCases` omits an empty `caseTypes` from the search payload
      // entirely, and the entity-service defaults an absent/empty `types`
      // filter to support cases only (`default_case`) rather than "no
      // restriction" — so an omitted filter silently narrows the result to
      // one type instead of returning all of them. Send every known type
      // explicitly in that case so the BE default can't kick in.
      if (merged.caseTypes.length === 0) {
        merged.caseTypes = ALL_CASE_TYPES;
      }
      return merged;
    },
    [filters, debouncedSearch, showSeverityFilter, lockedFilters],
  );

  const { data, isLoading, isFetching, isError, error } = useGetCsmCases(
    queryFilters,
    page,
    rowsPerPage,
    true,
    sortOrder,
  );

  const handleSortOrderChange = (order: CasesSortOrder): void => {
    setSortOrder(order);
    setPage(0);
  };

  const { data: directoryUsers } = useDirectoryUsers();
  const { showError } = useErrorBanner();
  const hasShownErrorRef = useRef(false);

  // Export CSV: pages `/cases/search` with the exact same filters/sort as the
  // listing above (`buildCaseSearchFilters`/`resolveAssignedUserIds` are the
  // same helpers `useGetCsmCases` uses), independent of the table's own
  // page/rowsPerPage — the export always fetches the *whole* filtered result
  // set, not whatever page happens to be on screen. See
  // `useFilteredCsvExport`/`fetchAllPages` for the paging + truncation logic.
  const api = useBackendApi();
  const currentUserEmail = useIdTokenClaims()?.email;
  const currentUserId = useCurrentUser().user?.id;
  const exportSearch = queryFilters.search.trim();
  const fetchCasesExportPage = useCallback(
    async (offset: number, limit: number) => {
      // Re-resolved per page when an assignee filter is active — a small,
      // deterministic, targeted `/users/search` by email, not a directory
      // scan — rather than caching it across pages, so the export can never
      // serve a stale resolution if this closure is reused across a filter
      // change mid-export (it isn't today, but this keeps that safe by
      // construction rather than by care).
      let assignedUserIds: string[] | undefined;
      if (queryFilters.assignees.length > 0) {
        const resolved = await resolveAssignedUserIds(
          api,
          queryFilters.assignees,
          currentUserId,
        );
        if (resolved === ASSIGNEE_FILTER_RESOLVED_EMPTY) {
          return { items: [] as CsmCaseRow[], total: 0 };
        }
        assignedUserIds = resolved;
      }
      const res = await api.post<BeCaseSearchPayload, BeCaseSearchResponse>(
        "/cases/search",
        {
          pagination: { offset, limit },
          sortBy: { field: "updatedOn", order: sortOrder },
          filters: buildCaseSearchFilters(queryFilters, exportSearch, assignedUserIds),
        },
      );
      const items = (res.cases ?? []).map((c) => mapCaseSearchViewToRow(c, currentUserEmail));
      return { items, total: res.total };
    },
    [api, queryFilters, exportSearch, sortOrder, currentUserId, currentUserEmail],
  );

  // Same gate `CasesList` is given (`hideSeverityColumn={isServiceRequestOnly
  // || hideSeverityColumn}`) so the export's column set can never drift from
  // what's actually rendered.
  const showExportSeverityColumn = !(isServiceRequestOnly || hideSeverityColumn);
  const caseToCsvRow = useCallback(
    (c: CsmCaseRow): string[] => {
      const caseId =
        c.wso2CaseId && c.caseNumber
          ? `${c.wso2CaseId}/${c.caseNumber}`
          : c.wso2CaseId || c.caseNumber || "";
      const updated = formatBackendTimestampForDisplay(c.updatedAt, {
        year: "numeric",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
      const stateText =
        stateLabel(c.state) +
        (c.state === "work_in_progress" && c.workState
          ? ` (${WORK_STATE_LABEL[c.workState]})`
          : "");
      const row = [caseId, c.subject, c.projectName, c.product];
      if (showExportSeverityColumn) row.push(c.severity);
      row.push(stateText, updated ?? "");
      return row;
    },
    [showExportSeverityColumn],
  );
  const exportHeader = [
    "Case ID",
    "Subject",
    "Project",
    "Product",
    ...(showExportSeverityColumn ? ["Severity"] : []),
    "State",
    "Updated",
  ];

  useEffect(() => {
    if (isError && !hasShownErrorRef.current) {
      hasShownErrorRef.current = true;
      showError(`Could not load ${entityNoun}.`, error);
    }
    if (!isError) hasShownErrorRef.current = false;
  }, [isError, error, showError, entityNoun]);

  const cases = data?.cases ?? [];

  const availableAssigneeUsers = useMemo(() => {
    const list = (directoryUsers ?? [])
      .filter((u) => u.name)
      .map((u) => ({ name: u.name, email: u.email }));
    list.sort((a, b) => a.name.localeCompare(b.name));
    return list;
  }, [directoryUsers]);

  const availableProjects = useMemo(() => {
    const byId = new Map<string, string>();
    (data?.cases ?? []).forEach((c) => {
      if (c.projectId) byId.set(c.projectId, c.projectName || c.projectId);
    });
    return Array.from(byId, ([id, name]) => ({ id, name }));
  }, [data?.cases]);

  const total = data?.total ?? 0;
  const lastPage = total === 0 ? 0 : Math.ceil(total / rowsPerPage) - 1;
  // Clamp to the last valid page when the loaded set shrinks (filter change, rows
  // closing). React's documented pattern for adjusting state from changed inputs
  // is a guarded set during render — not an effect (the lint rule forbids
  // setState in effects); React re-renders before committing, so it's not a
  // user-visible extra paint.
  if (data !== undefined && !data.hasMore && page > lastPage) {
    setPage(lastPage);
  }
  const paginationCount = data === undefined || data.hasMore ? -1 : total;

  const handleChangeRowsPerPage = (e: ChangeEvent<HTMLInputElement>): void => {
    setRowsPerPage(parseInt(e.target.value, 10));
    setPage(0);
  };

  const breachedCount = cases.filter(
    (c) => c.minutesToBreach < 0 && c.state !== "closed",
  ).length;
  const rangeStart = total === 0 ? 0 : page * rowsPerPage + 1;
  const rangeEnd = page * rowsPerPage + cases.length;

  const subtitle =
    isLoading
      ? null
      : total === 0
        ? `No ${entityNoun}`
        : `Showing ${rangeStart}–${rangeEnd} of ${total}`;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
          flexWrap: "wrap",
        }}
      >
        <Box>
          {title && <Typography variant="h5">{title}</Typography>}
          {subtitle != null && (
            <Typography variant="body2" color="text.secondary">
              {subtitle}
            </Typography>
          )}
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          {breachedCount > 0 && (
            <Chip size="small" color="error" label={`${breachedCount} breached`} />
          )}
          <FilteredCsvExportButton<CsmCaseRow>
            entityName={entityNoun.replace(/\s+/g, "-")}
            entityNounPlural={entityNoun}
            header={exportHeader}
            toRow={caseToCsvRow}
            fetchPage={fetchCasesExportPage}
            disabled={isError || total === 0}
          />
          {actions}
        </Box>
      </Box>

      <CasesFilterBar
        filters={filters}
        onChange={setFilters}
        onReset={() => setFilters(DEFAULT_CASES_FILTERS)}
        isFiltersOpen={isFiltersOpen}
        onFiltersToggle={() => setIsFiltersOpen((v) => !v)}
        availableAssigneeUsers={availableAssigneeUsers}
        availableProjects={availableProjects}
        showSeverityFilter={showSeverityFilter}
        hideTypeFilter={hideTypeFilter}
        hideProjectFilter={hideProjectFilter}
        showEngagementTypeFilter={showEngagementTypeFilter}
      />

      <CasesList
        cases={cases}
        isLoading={isLoading || isFetching}
        skeletonCount={rowsPerPage}
        detailBasePath={detailBasePath}
        hideSeverityColumn={isServiceRequestOnly || hideSeverityColumn}
        sortOrder={sortOrder}
        onSortOrderChange={handleSortOrderChange}
      />

      <TablePagination
        component="div"
        count={paginationCount}
        page={page}
        onPageChange={(_, newPage) => setPage(newPage)}
        rowsPerPage={rowsPerPage}
        onRowsPerPageChange={handleChangeRowsPerPage}
        rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
        labelRowsPerPage={`${entityNoun[0].toUpperCase()}${entityNoun.slice(1)} per page`}
        showFirstButton
        showLastButton
      />
    </Box>
  );
}
