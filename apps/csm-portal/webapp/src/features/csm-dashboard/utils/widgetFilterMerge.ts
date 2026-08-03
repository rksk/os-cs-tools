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

import { isCaseFieldFilterArray, type WidgetCaseFieldFilterLike } from "./widgetPreviewUrl";

/**
 * Merges a pie/bar widget's per-slice `query` under its own base `query`
 * (see `PieSlice`'s doc comment on the backend: "slice keys win on
 * conflict"), the way `useWidgetPieData`/`DashboardWidgetTile`'s click-through
 * both need. Both arguments are criteria objects — the widget-config key
 * that carries them was renamed `filters` -> `query`, but the criteria
 * object's OWN inner `filters` array (the case-search DSL below) keeps its
 * name, and so does the search request body's `filters` property.
 *
 * For every resourceType except `case`, criteria are a flat
 * `{ [namedField]: values }` record, so a plain object spread already gives
 * "slice keys win on conflict" for free — the slice's own keys simply
 * overwrite the base's same-named keys, and every other base key survives.
 *
 * `case` widgets carry the generic field/op/values DSL nested under a single
 * `filters` array property (`{ filters: BeCaseFieldFilter[] }`). A plain
 * object spread there is wrong: both objects have exactly one key
 * (`"filters"`), so the slice's array would silently replace the base's
 * array wholesale instead of overriding just the fields it actually
 * specifies — e.g. a "Critical" severity slice would lose the widget's own
 * base state filter entirely, and start counting cases in every state, not
 * just the open/in-progress ones the base widget itself is scoped to. This
 * function detects that shape and merges the two arrays by `field`, keeping
 * every base entry whose field the slice doesn't itself specify.
 */
/** Same shape check as `isCaseFieldFilterArray`, but also accepts a
 * genuinely empty array — a slice or base widget legitimately carrying zero
 * extra filter conditions is not the same as "not this shape at all", and
 * must still trigger the array-merge path below rather than the naive
 * fallback spread (which would otherwise wipe out the other side's
 * non-empty array whenever one side happens to be empty). */
function isCaseFieldFilterArrayOrEmpty(value: unknown): value is WidgetCaseFieldFilterLike[] {
  return (Array.isArray(value) && value.length === 0) || isCaseFieldFilterArray(value);
}

export function mergeWidgetFilters(
  base: Record<string, unknown>,
  slice: Record<string, unknown>,
): Record<string, unknown> {
  const merged = { ...base, ...slice };
  const baseArr = base.filters;
  const sliceArr = slice.filters;
  if (isCaseFieldFilterArrayOrEmpty(baseArr) && isCaseFieldFilterArrayOrEmpty(sliceArr)) {
    const sliceFields = new Set(sliceArr.map((f) => f.field));
    const combined: WidgetCaseFieldFilterLike[] = [
      ...baseArr.filter((f) => !sliceFields.has(f.field)),
      ...sliceArr,
    ];
    merged.filters = combined;
  }
  return merged;
}
