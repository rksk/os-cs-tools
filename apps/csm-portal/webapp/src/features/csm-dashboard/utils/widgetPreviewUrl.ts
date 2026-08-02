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

const RESERVED_PARAMS = new Set(["w", "n"]);

/** Placeholder swapped in for the signed-in user's own id wherever a
 * widget's (opaque, backend-resolved) filters carry it — e.g. "My Cases"
 * resolves to `assignedUserIds: ["<real uuid>"]` — so a bookmarked/shared
 * preview URL never carries a bare internal user id. */
const CURRENT_USER_SENTINEL = "@me";

function isStringArray(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((v) => typeof v === "string");
}

/**
 * Builds the URL a dashboard widget tile's "View more" link points at — a
 * real, bookmarkable/shareable/refresh-safe URL (no router state): the
 * resource type is the path segment (`previewSlug`, from
 * `WIDGET_RESOURCE_CONFIG`), the widget's own id/display name are `w`/`n`
 * query params, and each filter field is its own readable query param
 * (e.g. `severities=critical`) rather than one opaque JSON blob — and the
 * signed-in user's own id, wherever it appears, is masked to `@me` (see
 * `CURRENT_USER_SENTINEL`). Read back by `parseWidgetPreviewFilters` /
 * `resolveCurrentUserSentinels` in `DashboardWidgetPreviewPage`.
 */
export function buildWidgetPreviewHref(params: {
  previewSlug: string;
  widgetId: string;
  displayName: string;
  filters: Record<string, unknown>;
  /** The signed-in user's own id, so it can be masked rather than embedded
   * verbatim in the URL. Omit if not yet known — the filter value(s) are
   * then left as-is rather than masked. */
  currentUserId?: string;
}): string {
  const q = new URLSearchParams();
  q.set("w", params.widgetId);
  q.set("n", params.displayName);
  for (const [key, value] of Object.entries(params.filters)) {
    if (RESERVED_PARAMS.has(key)) continue;
    if (isStringArray(value)) {
      if (value.length === 0) continue;
      const masked = value.map((v) =>
        v === params.currentUserId ? CURRENT_USER_SENTINEL : v,
      );
      q.set(key, masked.join(","));
    } else if (typeof value === "string") {
      q.set(key, value === params.currentUserId ? CURRENT_USER_SENTINEL : value);
    }
  }
  return `/dashboard/${params.previewSlug}?${q.toString()}`;
}

export interface ParsedWidgetPreviewFilters {
  filters: Record<string, unknown>;
  /** True if a filter value still carries the `@me` sentinel and needs
   * `resolveCurrentUserSentinels` before it's safe to query with. */
  needsCurrentUser: boolean;
}

/** Parses every non-reserved (`w`/`n`) query param back into the widget's
 * filters object — the inverse of `buildWidgetPreviewHref`. Every value is
 * decoded as a comma-split string array (matching how every current dashboard
 * widget filter field is shaped — see `widgetResourceConfig.ts`'s
 * translators), so this never throws. */
export function parseWidgetPreviewFilters(
  searchParams: URLSearchParams,
): ParsedWidgetPreviewFilters {
  const filters: Record<string, unknown> = {};
  let needsCurrentUser = false;

  for (const [key, raw] of searchParams.entries()) {
    if (RESERVED_PARAMS.has(key)) continue;

    const values = raw.split(",");
    if (values.includes(CURRENT_USER_SENTINEL)) needsCurrentUser = true;
    filters[key] = values;
  }

  return { filters, needsCurrentUser };
}

/** Substitutes the `@me` sentinel back to the signed-in user's own id —
 * see `buildWidgetPreviewHref`'s masking of that same id. Returns `filters`
 * unchanged if `currentUserId` isn't known yet (caller should hold off
 * querying in that case — see `needsCurrentUser`). */
export function resolveCurrentUserSentinels(
  filters: Record<string, unknown>,
  currentUserId: string | undefined,
): Record<string, unknown> {
  if (!currentUserId) return filters;
  const resolved: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(filters)) {
    resolved[key] = Array.isArray(value)
      ? value.map((v) => (v === CURRENT_USER_SENTINEL ? currentUserId : v))
      : value === CURRENT_USER_SENTINEL
        ? currentUserId
        : value;
  }
  return resolved;
}
