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

import type { ChipProps } from "@wso2/oxygen-ui";
import type { IncidentPriority, IncidentSearchPayloadDto, IncidentState } from "@src/types";

// Direct port of the webapp's utils/incidents.ts label/color maps
// (apps/csm-portal/webapp/src/features/csm-operations/utils/incidents.ts).

export const INCIDENT_STATE_LABELS: Record<IncidentState, string> = {
  NEW: "New",
  IN_PROGRESS: "In Progress",
  ON_HOLD: "On Hold",
  RESOLVED: "Resolved",
  CLOSED: "Closed",
  CANCELLED: "Cancelled",
};

// New is unstarted (default), in-progress/on-hold are in-flight (info/warning), resolved/closed
// are terminal-good, cancelled is a terminal-problem state.
export const INCIDENT_STATE_COLORS: Record<IncidentState, NonNullable<ChipProps["color"]>> = {
  NEW: "default",
  IN_PROGRESS: "info",
  ON_HOLD: "warning",
  RESOLVED: "success",
  CLOSED: "success",
  CANCELLED: "error",
};

export const INCIDENT_PRIORITY_LABELS: Record<IncidentPriority, string> = {
  CRITICAL: "Critical",
  HIGH: "High",
  MODERATE: "Moderate",
  LOW: "Low",
  PLANNING: "Planning",
};

export const INCIDENT_PRIORITY_COLORS: Record<IncidentPriority, NonNullable<ChipProps["color"]>> = {
  CRITICAL: "error",
  HIGH: "error",
  MODERATE: "warning",
  LOW: "default",
  PLANNING: "default",
};

function humanize(value: string): string {
  return value.replace(/_/g, " ");
}

export function incidentStateLabel(state?: string | null): string {
  if (!state) return "—";
  return INCIDENT_STATE_LABELS[state as IncidentState] ?? humanize(state);
}

export function incidentStateColor(state?: string | null): NonNullable<ChipProps["color"]> {
  if (!state) return "default";
  return INCIDENT_STATE_COLORS[state as IncidentState] ?? "default";
}

export function incidentPriorityLabel(priority?: string | null): string {
  if (!priority) return "—";
  return INCIDENT_PRIORITY_LABELS[priority as IncidentPriority] ?? humanize(priority);
}

export function incidentPriorityColor(priority?: string | null): NonNullable<ChipProps["color"]> {
  if (!priority) return "default";
  return INCIDENT_PRIORITY_COLORS[priority as IncidentPriority] ?? "default";
}

/** All incident priorities, for the filter sheet. */
export const INCIDENT_PRIORITIES: IncidentPriority[] = ["CRITICAL", "HIGH", "MODERATE", "LOW", "PLANNING"];

// Deliberately just search + priority — the backend's IncidentSearchPayload.filters only supports
// searchQuery, priorities, and parentIds (see incident.dto.ts); there's no server-side state
// filter to build a control for (same gap the webapp's own IncidentsFilterBar documents).
export interface IncidentFilters {
  priorities: IncidentPriority[];
}

export const EMPTY_INCIDENT_FILTERS: IncidentFilters = {
  priorities: [],
};

export function countActiveIncidentFilters(filters: IncidentFilters): number {
  return filters.priorities.length > 0 ? 1 : 0;
}

export function toIncidentSearchFilters(search: string, filters: IncidentFilters): IncidentSearchPayloadDto["filters"] {
  return {
    ...(search.length > 0 && { searchQuery: search }),
    ...(filters.priorities.length > 0 && { priorities: filters.priorities }),
  };
}
