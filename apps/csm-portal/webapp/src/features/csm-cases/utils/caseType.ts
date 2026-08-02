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

import type { BeCaseType } from "@api/backend/types";

// Single source of truth for case-type ordering and labels. Lives in its own
// (backend-client-free) module so both the filter bar and the pure URL
// serializer can share it without dragging the API client into a unit test or
// tripping the react-refresh "components-only export" rule.

/** All case types, in the order they appear in the type dropdown. */
export const ALL_CASE_TYPES: BeCaseType[] = [
  "case",
  "service_request",
  "security_report_analysis",
  "announcement",
  "engagement",
];

/** Human-readable label per case type. */
export const CASE_TYPE_LABEL: Record<BeCaseType, string> = {
  case: "Case",
  service_request: "Service request",
  security_report_analysis: "Security report",
  announcement: "Announcement",
  engagement: "Engagement",
};

type ChipColor = "default" | "info" | "warning" | "success" | "error";

/** Chip color per case type — `case` (the default/majority type) stays
 * neutral, the others get a distinguishing color. */
export const CASE_TYPE_COLOR: Record<BeCaseType, ChipColor> = {
  case: "default",
  service_request: "info",
  security_report_analysis: "warning",
  announcement: "success",
  engagement: "default",
};

/**
 * Whether severity (S1-S4) is a meaningful concept for this case type. Only
 * plain support cases (`case`) actually have one set with any intent — every
 * other type still carries a `severity` in the search response (the field
 * isn't type-gated upstream), but it's not something anyone sets or acts on
 * for those, so the UI hides it there rather than show a value nobody put
 * any thought into. A missing `caseType` (legacy rows) is treated as `case`.
 */
export function caseTypeHasSeverity(caseType: BeCaseType | undefined): boolean {
  return caseType === undefined || caseType === "case";
}

/** Where a case's own detail page lives, keyed by its type — each
 * non-`case` type has its own dedicated route/detail page (different
 * fields/actions), not just a filtered view of `/cases`. */
export function caseTypeDetailBasePath(caseType: BeCaseType | undefined): string {
  switch (caseType) {
    case "service_request":
      return "/operations/service-requests";
    case "security_report_analysis":
      return "/security-center/security-reports";
    case "engagement":
      return "/engagements";
    case "announcement":
      return "/announcements";
    case "case":
    default:
      return "/cases";
  }
}
