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

import { Box, Button, TablePagination, TextField, Typography } from "@wso2/oxygen-ui";
import { ArrowLeft } from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState, type ChangeEvent, type JSX } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router";
import type { BeWidgetResourceType } from "@api/backend/types";
import { BE_MAX_PAGE_LIMIT } from "@constants/apiConstants";
import { useCurrentUser } from "@context/current-user/CurrentUserContext";
import { useDebouncedValue } from "@hooks/useDebouncedValue";
import { useWidgetData } from "@features/csm-dashboard/api/useWidgetData";
import { WIDGET_LIST_RENDERERS } from "@features/csm-dashboard/config/widgetListConfig";
import { resourceTypeForPreviewSlug } from "@features/csm-dashboard/config/widgetResourceConfig";
import {
  parseWidgetPreviewFilters,
  resolveCurrentUserSentinels,
} from "@features/csm-dashboard/utils/widgetPreviewUrl";

const DEFAULT_ROWS_PER_PAGE = 10;
const ROWS_PER_PAGE_OPTIONS = [10, 20, BE_MAX_PAGE_LIMIT];
const SEARCH_DEBOUNCE_MS = 300;

/**
 * "View more" landing for a dashboard `shape: "list"` widget tile — the same
 * per-resourceType table the tile itself renders (see `widgetListConfig.tsx`;
 * e.g. cases render through the identical `CasesList` the Cases tab uses),
 * paginated (real `TablePagination`, not just a bigger fixed fetch) so a
 * viewer can browse the widget's whole matching set from here without
 * leaving to the resource's own tab, plus a free-text search box merged
 * into the widget's own filters (`searchQuery` — the same field every other
 * resource search in this app already uses).
 *
 * Fully URL-driven (see `buildWidgetPreviewHref` in `widgetPreviewUrl.ts`)
 * rather than router-state-based, so the page is bookmarkable/shareable and
 * survives a refresh: the resource type is `:previewSlug` in the path, the
 * widget's own id/display name are `w`/`n` query params, and each filter
 * field is its own readable query param (the signed-in user's own id, where
 * present, is masked to `@me` rather than embedded verbatim). A URL with no
 * recognizable `previewSlug` or missing required params falls back to a
 * "go to the dashboard" prompt instead of crashing.
 */
export default function DashboardWidgetPreviewPage(): JSX.Element {
  const { previewSlug } = useParams<{ previewSlug: string }>();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const { user, isLoading: isLoadingUser } = useCurrentUser();

  const resourceType = resourceTypeForPreviewSlug(previewSlug);
  const widgetId = searchParams.get("w");
  const displayName = searchParams.get("n");
  const { filters: rawFilters, needsCurrentUser } =
    parseWidgetPreviewFilters(searchParams);

  const backButton = (
    <Button
      variant="text"
      size="small"
      startIcon={<ArrowLeft size={16} />}
      onClick={() => navigate("/dashboard")}
      sx={{ alignSelf: "flex-start" }}
    >
      Back
    </Button>
  );

  if (!resourceType || !widgetId || !displayName) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        {backButton}
        <Typography variant="body2" color="text.secondary">
          Open this page from a dashboard widget&rsquo;s &ldquo;View
          more&rdquo; link.
        </Typography>
      </Box>
    );
  }

  // A widget filtered to "assigned to me" carries the `@me` sentinel until
  // the signed-in user's own id is known — hold off resolving/querying
  // until then rather than ever sending the literal placeholder upstream.
  if (needsCurrentUser && !user?.id) {
    return (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        {backButton}
        <Typography variant="h5">{displayName}</Typography>
        <Typography variant="body2" color="text.secondary">
          {isLoadingUser
            ? "Loading…"
            : "Could not resolve the signed-in user for this widget."}
        </Typography>
      </Box>
    );
  }

  const filters = resolveCurrentUserSentinels(rawFilters, user?.id);

  return (
    <DashboardWidgetPreviewContent
      widgetId={widgetId}
      displayName={displayName}
      resourceType={resourceType}
      filters={filters}
      backButton={backButton}
    />
  );
}

interface DashboardWidgetPreviewContentProps {
  widgetId: string;
  displayName: string;
  resourceType: BeWidgetResourceType;
  filters: Record<string, unknown>;
  backButton: JSX.Element;
}

function DashboardWidgetPreviewContent({
  widgetId,
  displayName,
  resourceType,
  filters,
  backButton,
}: DashboardWidgetPreviewContentProps): JSX.Element {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(DEFAULT_ROWS_PER_PAGE);
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, SEARCH_DEBOUNCE_MS);

  const queriedFilters = useMemo(() => {
    const trimmed = debouncedSearch.trim();
    return trimmed ? { ...filters, searchQuery: trimmed } : filters;
  }, [filters, debouncedSearch]);

  const { data, isLoading, isError } = useWidgetData(
    widgetId,
    resourceType,
    queriedFilters,
    "list",
    rowsPerPage,
    page * rowsPerPage,
  );
  const ListRenderer = WIDGET_LIST_RENDERERS[resourceType];

  const handleSearchChange = (e: ChangeEvent<HTMLInputElement>): void => {
    setSearch(e.target.value);
    setPage(0);
  };

  const handleRowsPerPageChange = (e: ChangeEvent<HTMLInputElement>): void => {
    setRowsPerPage(parseInt(e.target.value, 10));
    setPage(0);
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {backButton}
      <Typography variant="h5">{displayName}</Typography>
      <TextField
        size="small"
        label="Search"
        placeholder="Search…"
        value={search}
        onChange={handleSearchChange}
        slotProps={{ htmlInput: { "aria-label": "Search" } }}
        sx={{ maxWidth: 360 }}
      />
      {isError ? (
        <Typography variant="body2" color="text.secondary">
          Could not load this widget.
        </Typography>
      ) : (
        <>
          <ListRenderer items={data?.items ?? []} isLoading={isLoading} />
          <TablePagination
            component="div"
            count={data?.total ?? 0}
            page={page}
            onPageChange={(_, newPage) => setPage(newPage)}
            rowsPerPage={rowsPerPage}
            onRowsPerPageChange={handleRowsPerPageChange}
            rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
            showFirstButton
            showLastButton
          />
        </>
      )}
    </Box>
  );
}
