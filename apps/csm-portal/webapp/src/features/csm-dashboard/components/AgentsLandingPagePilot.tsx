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

import { Box, Card, Skeleton, Typography } from "@wso2/oxygen-ui";
import { useQueryClient } from "@tanstack/react-query";
import { useState, type JSX } from "react";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useDashboard } from "@features/csm-dashboard/api/useDashboard";
import DashboardWidgetTile from "@features/csm-dashboard/components/DashboardWidgetTile";
import SectionCard from "@features/csm-dashboard/components/SectionCard";
import RefreshButton from "@features/csm-dashboard/components/RefreshButton";

/** Placeholder tile count while the dashboard detail is in flight. */
const PILOT_TILE_COUNT = 3;

interface AgentsLandingPagePilotProps {
  /** Id of the dashboard to render (e.g. "agents_pilot"). */
  dashboardId: string;
}

/**
 * Pilot section for the config-driven dashboard widget system: renders
 * whichever dashboard's real `single_score` widgets are passed in via
 * `dashboardId`. The dashboard's metadata plus its widget templates —
 * display metadata and each widget's filter criteria — are fetched once via
 * {@link useDashboard}; each rendered tile then resolves its own data
 * independently. Today only the "agents_pilot" dashboard has real widgets
 * (see CsmDashboardPage.tsx), but this component is generic over any
 * dashboard id with widgets.
 */
export default function AgentsLandingPagePilot({
  dashboardId,
}: AgentsLandingPagePilotProps): JSX.Element {
  const queryClient = useQueryClient();
  const { data, isLoading, isError, isFetching, refetch } =
    useDashboard(dashboardId);
  // Separate from `isFetching` (which only covers the dashboard's own
  // metadata refetch): each tile resolves its own count/list data via its
  // own `useWidgetData` query, so a "refresh" click has to also invalidate
  // those — tracked here so the skeleton grid stays up across the whole
  // round trip, not just the metadata half of it.
  const [isRefreshing, setIsRefreshing] = useState(false);

  const handleRefresh = async (): Promise<void> => {
    setIsRefreshing(true);
    try {
      await Promise.all([
        refetch(),
        queryClient.invalidateQueries({
          queryKey: [ApiQueryKeys.CSM_DASHBOARD_WIDGET_DATA],
        }),
      ]);
    } finally {
      setIsRefreshing(false);
    }
  };

  return (
    <SectionCard
      action={
        <RefreshButton
          onRefresh={() => void handleRefresh()}
          isFetching={isFetching || isRefreshing}
          label="Refresh widget pilot"
        />
      }
    >
      {isError ? (
        <Typography variant="body2" color="text.secondary">
          Could not load the widget pilot.
        </Typography>
      ) : (
        <Box
          sx={{
            display: "grid",
            gap: 1.5,
            // 12-column grid, matching each widget's own `gridWidth`; on very
            // small screens there's only room for 4 columns, so a wide widget
            // there wraps to (at most) one extra row rather than overflowing.
            gridTemplateColumns: {
              xs: "repeat(4, minmax(0, 1fr))",
              sm: "repeat(12, minmax(0, 1fr))",
            },
          }}
        >
          {isLoading || isRefreshing
            ? Array.from({ length: PILOT_TILE_COUNT }, (_, i) => (
                <Card key={i} variant="outlined" sx={{ p: 1.75, gridColumn: "span 4" }}>
                  <Skeleton variant="rounded" height={48} />
                </Card>
              ))
            : (data?.widgets ?? []).map((widget) => (
                <Box
                  key={widget.widgetId}
                  sx={
                    // A list-shape widget renders a real table (4 rows,
                    // several columns) — its configured `gridWidth` was
                    // sized for the old compact text list, so it always
                    // spans the full row here regardless of that value.
                    widget.shape === "list"
                      ? { gridColumn: "1 / -1" }
                      : {
                          gridColumn: {
                            xs: `span ${Math.min(widget.gridWidth, 4)}`,
                            sm: `span ${widget.gridWidth}`,
                          },
                        }
                  }
                >
                  <DashboardWidgetTile
                    widgetId={widget.widgetId}
                    displayName={widget.displayName}
                    resourceType={widget.resourceType}
                    shape={widget.shape}
                    filters={widget.filters}
                    listLimit={widget.listLimit}
                  />
                </Box>
              ))}
        </Box>
      )}
    </SectionCard>
  );
}
