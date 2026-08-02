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

import { Box, Button, Card, Skeleton, Tooltip, Typography, alpha, useTheme } from "@wso2/oxygen-ui";
import { ArrowRight, Info } from "@wso2/oxygen-ui-icons-react";
import type { JSX, ReactNode } from "react";
import { Link as RouterLink } from "react-router";
import type { BeWidgetResourceType, BeWidgetShape } from "@api/backend/types";
import { useCurrentUser } from "@context/current-user/CurrentUserContext";
import { useWidgetData } from "@features/csm-dashboard/api/useWidgetData";
import { WIDGET_RESOURCE_CONFIG } from "@features/csm-dashboard/config/widgetResourceConfig";
import { WIDGET_LIST_RENDERERS } from "@features/csm-dashboard/config/widgetListConfig";
import { buildWidgetPreviewHref } from "@features/csm-dashboard/utils/widgetPreviewUrl";

interface DashboardWidgetTileProps {
  widgetId: string;
  displayName: string;
  resourceType: BeWidgetResourceType;
  shape: BeWidgetShape;
  filters: Record<string, unknown>;
  /** Only meaningful for shape "list"; how many rows to render. Defaults to
   * 4 (see useWidgetData's DEFAULT_LIST_LIMIT) — set explicitly per-widget
   * via the backend's DASHBOARDS_CONFIG, not overridden here. */
  listLimit?: number;
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
  resourceType,
  shape,
  filters,
  listLimit,
}: DashboardWidgetTileProps): JSX.Element {
  const theme = useTheme();
  const { user } = useCurrentUser();
  const { data, isLoading, isError } = useWidgetData(
    widgetId,
    resourceType,
    filters,
    shape,
    listLimit,
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

  const href = config.buildHref(filters);
  const Icon = config.icon;
  const isListShape = shape === "list";

  // Count tiles only — a list-shape tile's real table already has its own
  // header row and border right where this would otherwise sit, so it just
  // overlapped rather than adding anything.
  //
  // Tooltip copy is intentionally empty until the per-widget messages are
  // finalized — the icon renders now so the layout/interaction is in place
  // ahead of that content.
  const infoIcon = shape === "count" && (
    <Tooltip title="">
      <Box
        component="span"
        sx={{
          position: "absolute",
          top: 12,
          right: 12,
          zIndex: 1,
          display: "inline-flex",
          color: "text.secondary",
        }}
      >
        <Info size={14} />
      </Box>
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

  let body: ReactNode;
  if (isLoading) {
    body = <Skeleton variant="rounded" height={48} />;
  } else if (isError) {
    body = (
      <Typography variant="body2" color="text.secondary">
        Could not load this widget.
      </Typography>
    );
  } else if (shape === "count") {
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
          <Typography variant="caption" color="text.secondary">
            {displayName}
          </Typography>
          <Typography variant="h5" sx={{ mt: 0.5 }}>
            {data?.total ?? 0}
          </Typography>
        </Box>
      </Box>
    );
  } else {
    // pie/bar: no aggregate endpoint exists anywhere in the stack today, so
    // there is nothing to resolve or render yet — see `BeWidgetShape`.
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
          <Typography variant="caption" color="text.secondary">
            {displayName}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Not yet supported.
          </Typography>
        </Box>
      </Box>
    );
  }

  return (
    <Card
      variant="outlined"
      component={RouterLink}
      to={href}
      sx={{
        position: "relative",
        p: 1.75,
        display: "block",
        height: "100%",
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
    >
      {infoIcon}
      {body}
    </Card>
  );
}
