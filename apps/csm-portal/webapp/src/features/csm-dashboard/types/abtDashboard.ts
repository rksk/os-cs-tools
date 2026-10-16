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

export type Severity = "S0" | "S1" | "S2" | "S3" | "S4";

/**
 * Severity plus the "no value" state: a case whose source has no severity set
 * (empty/missing, or a value `severityFromBe` doesn't recognize — never sent
 * to us as `"medium"`) is a distinct fact from "the severity really is
 * S3/Medium" and must never render/filter as one. Not one of the S0-S4 filter
 * options by design — see `severityFromBe` in `api/backend/mappers.ts`.
 */
export type SeverityOrUnset = Severity | "unset";

export type CaseState =
  | "open"
  | "work_in_progress"
  | "solution_proposed"
  | "awaiting_info"
  | "waiting_on_wso2"
  | "closed"
  /**
   * On a closed case's `nextStates` it is only a signal that "Create related
   * case" is available, not an actual reopen (the data source has no such
   * transition). A case can still BE in this state, though: cases carried
   * over from the previous system hold it, and the dashboard tiles count
   * them, so the list's State filter must be able to select it. See
   * `CsmCaseDetail.nextStates` and `CaseActionBar`'s `reopened` handling.
   */
  | "reopened";

/**
 * Work sub-state of a `work_in_progress` case. `null` / absent when the case is
 * not in progress. Mirrors the entity-service `CaseWorkState` enum; the backend
 * gates comment posting on `work_in_progress` + `ongoing`.
 */
export type CaseWorkState = "ongoing" | "paused";

export type SlaClockType = "ack" | "first_response" | "resolution";

export interface CsmQueueCase {
  id: string;
  caseNumber: string;
  subject: string;
  customer: string;
  severity: Severity;
  state: CaseState;
  slaClockType: SlaClockType;
  // Minutes until the active SLA clock breaches. Negative => already breached.
  minutesToBreach: number;
}

export interface CsmQueueSummary {
  actionRequiredCount: number;
  inProgressCount: number;
  awaitingInfoCount: number;
  totalOpenCount: number;
  topCases: CsmQueueCase[];
}

export interface CsmSlaAtRiskCase {
  id: string;
  caseNumber: string;
  subject: string;
  customer: string;
  severity: Severity;
  assignee: string;
  slaClockType: SlaClockType;
  minutesToBreach: number;
  state: CaseState;
}

export interface CsmCustomerSummary {
  accountId: string;
  accountName: string;
  tier: string;
  openCaseCount: number;
  s0s1Count: number;
  breachedCount: number;
  lastActivityAt: string;
}

export type CsmRecentActivityType =
  | "comment"
  | "state_change"
  | "case_created"
  | "case_closed"
  | "sla_breach";

export interface CsmRecentActivity {
  id: string;
  caseId: string;
  caseNumber: string;
  customer: string;
  type: CsmRecentActivityType;
  who: string;
  whenAt: string;
  summary: string;
}

// Multi-dashboard switcher — mirrors ServiceNow Performance Analytics where
// engineers pivot between several dashboards (Engineer / Operations / IAM /
// Security / Team Performance). See DashboardsAndReportsProposal.md.
//
// Dashboard ids now come from the backend registry (GET /dashboards, see
// useDashboardList), not a fixed compile-time set, so this is just `string`.
export type DashboardKey = string;
