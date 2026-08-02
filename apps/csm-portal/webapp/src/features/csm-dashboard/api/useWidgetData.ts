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

import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useBackendApi } from "@api/backend/client";
import type { BeWidgetResourceType, BeWidgetShape } from "@api/backend/types";
import { WIDGET_RESOURCE_CONFIG } from "@features/csm-dashboard/config/widgetResourceConfig";

/** Default number of rows fetched for a `shape: "list"` widget when the
 * template doesn't set its own `listLimit`. */
const DEFAULT_LIST_LIMIT = 4;

export interface WidgetData {
  /** Total matching records — what a `shape: "count"` tile renders. */
  total: number;
  /** The resolved page of records — what a `shape: "list"` tile renders. */
  items: Record<string, unknown>[];
}

/**
 * Resolves one dashboard widget's own data, independently of any sibling
 * widget on the same dashboard: a single `POST` to that `resourceType`'s own
 * search endpoint (see `WIDGET_RESOURCE_CONFIG`) with the widget's own
 * filters. Always reads both `total` and the item page off the response —
 * a `count`-shape widget only needs `total`, a `list`-shape widget only
 * needs `items`, but fetching both from the one call keeps this a single
 * code path instead of two near-identical ones.
 */
export function useWidgetData(
  widgetId: string,
  resourceType: BeWidgetResourceType,
  filters: Record<string, unknown>,
  shape: BeWidgetShape,
  listLimit?: number,
  /** Row offset for a `shape: "list"` widget — only meaningful there, and
   * only actually used by `DashboardWidgetPreviewPage`'s pagination; the
   * compact tile always fetches from the start. */
  offset = 0,
  /** Set to `false` to defer the query — used by `DashboardWidgetPreviewPage`
   * while it's still waiting to resolve a `@me`-style filter sentinel (see
   * `widgetPreviewUrl.ts`) into the signed-in user's real id, so a request
   * never goes out with the literal placeholder still in it. */
  enabled = true,
): UseQueryResult<WidgetData, Error> {
  const api = useBackendApi();
  const config = WIDGET_RESOURCE_CONFIG[resourceType];
  const limit = shape === "list" ? (listLimit ?? DEFAULT_LIST_LIMIT) : 1;
  const effectiveOffset = shape === "list" ? offset : 0;

  return useQuery<WidgetData, Error>({
    queryKey: [
      ApiQueryKeys.CSM_DASHBOARD_WIDGET_DATA,
      widgetId,
      resourceType,
      filters,
      limit,
      effectiveOffset,
    ],
    enabled,
    queryFn: async (): Promise<WidgetData> => {
      if (!config) {
        // A widget's resourceType came back from the backend (now a
        // runtime-configurable registry, not a compile-time-checked Go
        // literal) with no matching entry here — fail this widget's query
        // rather than crash on the property accesses below.
        throw new Error(`Unsupported widget resourceType: ${resourceType}`);
      }
      const res = await api.post<
        { filters: Record<string, unknown>; pagination: { offset: number; limit: number } },
        Record<string, unknown>
      >(config.searchEndpoint, {
        filters,
        pagination: { offset: effectiveOffset, limit },
      });
      const total = typeof res.total === "number" ? res.total : 0;
      const rawItems = res[config.itemsKey];
      const items = Array.isArray(rawItems)
        ? (rawItems as Record<string, unknown>[])
        : [];
      return { total, items };
    },
    staleTime: 60_000,
  });
}
