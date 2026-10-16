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

import type { BeChoiceListItem, BeVulnerabilityPriority } from "@api/backend/types";

type ChipColor = "default" | "info" | "warning" | "success" | "error";

/**
 * Human-readable label for a `VulnerabilityPriority` enum value (used in the
 * filter select). The `priority` field on `ProductVulnerabilityView` is already
 * a label string from the upstream (e.g. "High"), so use it directly for display.
 */
const PRIORITY_LABEL: Record<BeVulnerabilityPriority, string> = {
  info: "Info",
  low: "Low",
  medium: "Medium",
  high: "High",
  critical: "Critical",
  unknown: "Unknown",
};

/**
 * MUI/Oxygen chip colour keyed on the *lowercase* priority string. Works with
 * both the enum values (filter select) and the label strings from
 * `ProductVulnerabilityView.priority` (e.g. "High" → lower-cased to "high").
 */
const PRIORITY_COLOR: Record<string, ChipColor> = {
  critical: "error",
  high: "error",
  medium: "warning",
  low: "info",
  info: "default",
  unknown: "default",
};

/** All vulnerability priorities in display order (highest to lowest). */
export const VULNERABILITY_PRIORITIES = [
  "critical",
  "high",
  "medium",
  "low",
  "info",
  "unknown",
] as BeVulnerabilityPriority[];

/**
 * `GetVulnerabilityMeta`'s `severities` entries key `id` by the upstream's own
 * severity id scheme (a string-encoded integer), not the lowercase
 * `BeVulnerabilityPriority` enum this app's own search filter already uses —
 * confirmed against entity-service's `vulnerabilityPriorityToSeverityID`
 * (`sn_product_vulnerability_service.go`), which both the ServiceNow and
 * Postgres implementations of `GetVulnerabilityMeta` funnel through, so this
 * correspondence is stable across data sources.
 */
const SEVERITY_ID_TO_PRIORITY: Record<string, BeVulnerabilityPriority> = {
  "0": "info",
  "1": "low",
  "2": "medium",
  "3": "high",
  "4": "critical",
  "5": "unknown",
};

/**
 * Maps `GetVulnerabilityMeta`'s `severities` choice list onto the ordered
 * `BeVulnerabilityPriority` list this app's filter already renders, with each
 * entry's own real display `label` instead of the static `PRIORITY_LABEL` map
 * — so the filter reflects whatever this environment's
 * `product_vulnerability_severity_enum` (or ServiceNow choice list) actually
 * defines, rather than the maintained-by-hand fallback. An id this app
 * doesn't recognize is skipped (never shown with no label); the result keeps
 * `VULNERABILITY_PRIORITIES`'s own highest-to-lowest order rather than
 * whatever order the backend returned.
 */
export function vulnerabilityMetaToPriorityOptions(
  severities: BeChoiceListItem[] | undefined,
): { priority: BeVulnerabilityPriority; label: string }[] {
  if (!severities?.length) {
    return VULNERABILITY_PRIORITIES.map((priority) => ({
      priority,
      label: vulnerabilityPriorityLabel(priority),
    }));
  }
  const labelByPriority = new Map<BeVulnerabilityPriority, string>();
  for (const { id, label } of severities) {
    const priority = SEVERITY_ID_TO_PRIORITY[id];
    if (priority) labelByPriority.set(priority, label);
  }
  return VULNERABILITY_PRIORITIES.filter((p) => labelByPriority.has(p)).map((priority) => ({
    priority,
    label: labelByPriority.get(priority) as string,
  }));
}

export function vulnerabilityPriorityLabel(priority: BeVulnerabilityPriority): string {
  return PRIORITY_LABEL[priority] ?? priority;
}

/**
 * Returns the MUI chip colour for a priority value. Accepts both enum values
 * ("high") and label strings from the view ("High") by normalising to lowercase.
 */
export function vulnerabilityPriorityColor(priority?: string | null): ChipColor {
  if (!priority) return "default";
  return PRIORITY_COLOR[priority.toLowerCase()] ?? "default";
}
