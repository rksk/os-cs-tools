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

import { Box, Button, Card, IconButton, Skeleton, Tooltip, Typography, alpha, useTheme } from "@wso2/oxygen-ui";
import { ArrowRight, RefreshCw } from "@wso2/oxygen-ui-icons-react";
import type { JSX, KeyboardEvent, MouseEvent, ReactNode } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { BeDashboardPieSlice, BeWidgetResourceType, BeWidgetShape } from "@api/backend/types";
import { useCurrentUser } from "@context/current-user/CurrentUserContext";
import { useWidgetData } from "@features/csm-dashboard/api/useWidgetData";
import { useWidgetPieData, type PieSliceResult } from "@features/csm-dashboard/api/useWidgetPieData";
import { WIDGET_RESOURCE_CONFIG } from "@features/csm-dashboard/config/widgetResourceConfig";
import { WIDGET_LIST_RENDERERS } from "@features/csm-dashboard/config/widgetListConfig";
import { buildWidgetPreviewHref } from "@features/csm-dashboard/utils/widgetPreviewUrl";
import { mergeWidgetFilters } from "@features/csm-dashboard/utils/widgetFilterMerge";
import { resolveTeamPlaceholder } from "@features/csm-dashboard/utils/teamFilterPlaceholder";
import DashboardPieChart from "@features/csm-dashboard/components/DashboardPieChart";
import DashboardBarChart from "@features/csm-dashboard/components/DashboardBarChart";

interface DashboardWidgetTileProps {
  widgetId: string;
  displayName: string;
  /** Explanatory subtitle shown under `displayName` — only rendered for
   * shapes "pie"/"bar" today, but not shape-specific by design. */
  description?: string;
  resourceType: BeWidgetResourceType;
  shape: BeWidgetShape;
  filters: Record<string, unknown>;
  /** Only meaningful for shape "list"; how many rows to render. Defaults to
   * 4 (see useWidgetData's DEFAULT_LIST_LIMIT) — set explicitly per-widget
   * via the backend's DASHBOARDS_CONFIG, not overridden here. */
  listLimit?: number;
  /** Only meaningful for shape "pie"/"bar"; one search per slice (see
   * `useWidgetPieData`). Empty/absent renders an empty chart rather than
   * crashing. */
  slices?: BeDashboardPieSlice[];
  /** The currently selected team's own `groupId` (see `BeTeam.groupId`),
   * threaded down from `CsmDashboardPage` for resolving this widget's own
   * `__current_team__` filter placeholder (see `teamFilterPlaceholder.ts`)
   * — never the team registry key. `undefined` for a non-team-based
   * dashboard, or while the team isn't resolved yet (any `integrationCsTeam`
   * filter entry carrying the placeholder is then dropped, not sent
   * literally). */
  selectedTeamGroupId?: string;
}

/**
 * Single dashboard widget tile: fetches and renders its own data
 * independently of any sibling tile, so one widget's loading/error state
 * never affects another's. Renders a big number for `shape: "count"`; for
 * `shape: "list"` renders that resource type's own real table (see
 * `widgetListConfig.tsx` — e.g. cases render through the same `CasesList`
 * the Cases tab itself uses), capped at `listLimit`, with a "View more" link
 * to that widget's own preview page (`DashboardWidgetPreviewPage`, more rows,
 * same table) — not directly to the resource's own tab (that's "View all",
 * one hop further, via `widgetResourceConfig.ts`'s `buildHref`).
 *
 * `shape: "count"` tiles are themselves one big link straight to the
 * resource's own tab; `shape: "list"` tiles can't be (their rows and "View
 * more" need their own nested links), so only they get a plain, non-link
 * `Card`.
 */
export default function DashboardWidgetTile({
  widgetId,
  displayName,
  description,
  resourceType,
  shape,
  filters,
  listLimit,
  slices,
  selectedTeamGroupId,
}: DashboardWidgetTileProps): JSX.Element {
  const theme = useTheme();
  const navigate = useNavigate();
  const { user } = useCurrentUser();
  // Shape "pie" resolves via useWidgetPieData instead (one search per
  // slice) — skip this one's own network call rather than wasting it, but
  // still call the hook unconditionally (rules of hooks; a widget's shape
  // never changes across this component's lifetime).
  const { data, isLoading, isError, refetch } = useWidgetData(
    widgetId,
    resourceType,
    filters,
    shape,
    listLimit,
    0,
    shape !== "pie" && shape !== "bar",
    selectedTeamGroupId,
  );
  const pieData = useWidgetPieData(
    resourceType,
    filters,
    shape === "pie" || shape === "bar" ? (slices ?? []) : [],
    selectedTeamGroupId,
  );
  const config = WIDGET_RESOURCE_CONFIG[resourceType];

  if (!config) {
    // resourceType came from a runtime-configurable backend registry (not a
    // compile-time-checked Go literal) — an unrecognized value must not
    // crash this tile's render (config.buildHref below would throw).
    return (
      <Card variant="outlined" sx={{ p: 1.75 }}>
        <Typography variant="caption" color="text.secondary">
          {displayName}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          Unsupported widget type.
        </Typography>
      </Card>
    );
  }

  // The count-shape tile's own click-through href — resolved so a "View
  // all" link never carries the literal `__current_team__` placeholder
  // into the destination resource's own filters (see
  // `teamFilterPlaceholder.ts`).
  const href = config.buildHref(resolveTeamPlaceholder(filters, selectedTeamGroupId));
  const Icon = config.icon;
  const isListShape = shape === "list";

  // Refetches just this tile's own data — the count/list query (disabled for
  // pie/bar, see the useWidgetData call above) and the pie/bar per-slice
  // queries (empty for count/list, see useWidgetPieData above) are both
  // called unconditionally rather than branching on `shape`: whichever one
  // this tile doesn't actually use is already disabled/empty, so refetching
  // it is a no-op, not a wasted request.
  const handleRefresh = (e: MouseEvent): void => {
    // Count tiles render their whole Card as a router Link (see the `href`
    // return below) — without stopping propagation, clicking refresh would
    // also navigate to the tile's click-through destination.
    e.preventDefault();
    e.stopPropagation();
    void refetch();
    pieData.refetch();
  };

  // The skeleton (loading) state already occupies this same top-right corner
  // in every shape's markup below, so showing the refresh button at the same
  // time visually collides with it — only render it once this tile has
  // settled into either real data or an error.
  const isTileLoading = shape === "pie" || shape === "bar" ? pieData.isLoading : isLoading;
  const refreshButton = !isTileLoading && (
    <Tooltip title="Refresh this widget">
      <IconButton
        size="small"
        onClick={handleRefresh}
        aria-label={`Refresh ${displayName}`}
        sx={{
          position: "absolute",
          top: 8,
          right: 8,
          zIndex: 1,
          color: "text.secondary",
        }}
      >
        <RefreshCw size={13} />
      </IconButton>
    </Tooltip>
  );

  const header = (
    <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.25 }}>
      <Box
        sx={{
          p: 0.75,
          mt: 0.25,
          borderRadius: "50%",
          bgcolor: alpha(theme.palette[config.iconColor].light, 0.1),
          color: theme.palette[config.iconColor].light,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
        }}
      >
        <Icon size={16} />
      </Box>
      <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
        {displayName}
      </Typography>
    </Box>
  );

  if (isListShape) {
    const ListRenderer = WIDGET_LIST_RENDERERS[resourceType];
    return (
      <Card variant="outlined" sx={{ position: "relative", p: 1.75, height: "100%" }}>
        {refreshButton}
        {header}
        {isLoading ? (
          <Skeleton variant="rounded" height={28 * (listLimit ?? 4) + 40} sx={{ mt: 1 }} />
        ) : isError ? (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            Could not load this widget.
          </Typography>
        ) : (
          <>
            {/* CasesList/TimeCardsTable (case, time_card) carry no margin of
                their own — this is the one place spacing between the header
                and the table is enforced for every resource type, so the
                header's icon never sits flush against the table's border. */}
            <Box sx={{ mt: 1.5 }}>
              <ListRenderer items={data?.items ?? []} isLoading={false} />
            </Box>
            {(data?.total ?? 0) > (listLimit ?? 4) && (
              <Box sx={{ display: "flex", justifyContent: "flex-end", mt: 1 }}>
                <Button
                  component={RouterLink}
                  to={buildWidgetPreviewHref({
                    previewSlug: config.previewSlug,
                    widgetId,
                    displayName,
                    filters,
                    currentUserId: user?.id,
                  })}
                  size="small"
                  variant="text"
                  endIcon={<ArrowRight size={14} />}
                >
                  View more
                </Button>
              </Box>
            )}
          </>
        )}
      </Card>
    );
  }

  if (shape === "pie" || shape === "bar") {
    // Each slice/legend row (or bar) navigates independently to that
    // slice's OWN filtered list, so — same rationale as shape "list" — this
    // can't be one big link the way shape "count" is. It's still a
    // click-through target in its own right, though: clicking the tile
    // anywhere OTHER than a slice/legend row (the header, the padding, the
    // empty state) goes to the widget's own base filters, the same
    // destination a "count" tile with those filters would produce.
    //
    // That tile-level target keeps its role="button"/Enter+Space keyboard
    // handling (a real <a> only activates on Enter, not Space, and this
    // control has always supported both) -- but it is now a SIBLING of the
    // refresh button and the chart, not their ancestor: an element with an
    // interactive role makes its descendants' own roles presentational to
    // assistive tech, so nesting the refresh IconButton and the chart's own
    // role="button" legend rows inside it would hide them from screen
    // readers even though they're still clickable. The target is absolutely
    // positioned to fill the card and sits behind (`zIndex: 0`) a
    // `pointerEvents: "none"` content layer, so it only ever receives clicks
    // that land on genuine background -- pointer events are switched back on
    // (`pointerEvents: "auto"`) for the refresh button and the chart
    // specifically, letting their own click and keyboard handling work
    // exactly as before.
    const ChartComponent = shape === "pie" ? DashboardPieChart : DashboardBarChart;
    const tileHref = config.buildHref(resolveTeamPlaceholder(filters, selectedTeamGroupId));
    const handleTileClick = (): void => {
      void navigate(tileHref);
    };
    const handleTileKeyDown = (e: KeyboardEvent): void => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        handleTileClick();
      }
    };
    return (
      <Card
        variant="outlined"
        sx={{
          position: "relative",
          p: 1.75,
          height: "100%",
        }}
      >
        <Box
          role="button"
          tabIndex={0}
          aria-label={`View all cases for ${displayName}`}
          onClick={handleTileClick}
          onKeyDown={handleTileKeyDown}
          sx={{
            position: "absolute",
            inset: 0,
            zIndex: 0,
            borderRadius: "inherit",
            cursor: "pointer",
            "&:hover": { bgcolor: "action.hover" },
            "&:focus-visible": {
              outline: "2px solid",
              outlineColor: "primary.main",
              outlineOffset: -2,
            },
          }}
        />
        <Box sx={{ position: "relative", zIndex: 1, height: "100%", pointerEvents: "none" }}>
          <Box sx={{ pointerEvents: "auto" }}>{refreshButton}</Box>
          {/* The header's own bottom padding — not just a top margin on the
              chart below it — so the chart's top edge (and, at the size this
              chart renders at, its tooltip) never sits flush against/behind
              the title row above it. */}
          <Box sx={{ pb: description ? 1 : 2.5 }}>{header}</Box>
          {description && (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              {description}
            </Typography>
          )}
          <Box sx={{ pointerEvents: "auto" }}>
            <ChartComponent
              slices={pieData.slices}
              total={pieData.total}
              isLoading={pieData.isLoading}
              isError={pieData.isError}
              onSliceClick={(slice: PieSliceResult) =>
                navigate(
                  config.buildHref(
                    resolveTeamPlaceholder(
                      mergeWidgetFilters(filters, slice.query),
                      selectedTeamGroupId,
                    ),
                  ),
                )
              }
            />
          </Box>
        </Box>
      </Card>
    );
  }

  let body: ReactNode;
  if (isLoading) {
    body = <Skeleton variant="rounded" height={48} />;
  } else if (isError) {
    body = (
      <Typography variant="body2" color="text.secondary">
        Could not load this widget.
      </Typography>
    );
  } else {
    body = (
      <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.25 }}>
        <Box
          sx={{
            p: 0.75,
            mt: 0.25,
            borderRadius: "50%",
            bgcolor: alpha(theme.palette[config.iconColor].light, 0.1),
            color: theme.palette[config.iconColor].light,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
          }}
        >
          <Icon size={16} />
        </Box>
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography variant="caption" color="text.secondary" noWrap>
            {displayName}
          </Typography>
          <Typography
            noWrap
            sx={{
              mt: 0.5,
              lineHeight: 1.1,
              fontWeight: 700,
              fontSize: "3.25rem",
              overflow: "hidden",
              textOverflow: "ellipsis",
            }}
          >
            {data?.total ?? 0}
          </Typography>
        </Box>
      </Box>
    );
  }

  return (
    // Same sibling-target restructuring as the pie/bar branch above: the
    // whole-card click-through used to BE the refresh IconButton's own
    // ancestor (`Card component={RouterLink}`), which hides a nested
    // interactive control from assistive tech exactly like a nested
    // role="button" does. The anchor here is a background sibling instead;
    // see the pie/bar branch's comment for the pointer-events layering.
    <Card
      variant="outlined"
      sx={{
        position: "relative",
        p: 1.75,
        height: "100%",
      }}
    >
      <Box
        component={RouterLink}
        to={href}
        // The visible count + label sit in the pointer-events-none content
        // layer above this anchor, not inside it as descendant text anymore
        // (that's the whole point -- see the comment above), so it needs its
        // own accessible name instead of inheriting one from its content.
        aria-label={`${displayName}: ${data?.total ?? 0}`}
        sx={{
          position: "absolute",
          inset: 0,
          zIndex: 0,
          display: "block",
          borderRadius: "inherit",
          cursor: "pointer",
          color: "inherit",
          textDecoration: "none",
          transition: "box-shadow 0.2s ease, transform 0.15s ease",
          "&:hover": {
            boxShadow: `0 0 0 1px ${theme.palette.primary.main}, 0 4px 16px rgba(0,0,0,0.12)`,
            transform: "translateY(-2px)",
          },
          "&:focus-visible": { outline: "2px solid", outlineColor: "primary.main", outlineOffset: -2 },
        }}
      />
      <Box sx={{ position: "relative", zIndex: 1, height: "100%", pointerEvents: "none" }}>
        <Box sx={{ pointerEvents: "auto" }}>{refreshButton}</Box>
        {body}
      </Box>
    </Card>
  );
}
