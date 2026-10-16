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

import type { CasesStatsDto, EntityReference, Pagination } from "@src/types";

export interface ChangeRequestsDto extends Pagination {
  changeRequests: ChangeRequestSummaryDto[];
}

interface ChangeRequestSummaryDto {
  id: string;
  number: string;
  title: string;
  case: (EntityReference & { number: string; internalId: string }) | null;
  endDate: string | null;
  impact: EntityReference | null;
  state: EntityReference | null;
  type: EntityReference | null;
  assignedTeam: EntityReference | null;
  createdOn: string;
  updatedOn: string;
}

/**
 * The time a customer proposed, and where WSO2's answer to it stands. A proposal
 * moves the START only; the change request stays in Customer Approval until WSO2
 * answers it (`agreed` -> Scheduled; `disagreed` -> the customers are asked again).
 */
export interface ChangeRequestCustomerProposalDto {
  startDate: string;
  endDate?: string | null;
  answer: "pending" | "agreed" | "disagreed" | "unanswered";
  proposedByViewer?: boolean;
}

export interface ChangeRequestDto {
  id: string;
  number: string;
  title: string;
  createdBy: string;
  case: (EntityReference & { number: string; internalId: string }) | null;
  deployment: (EntityReference & { number: string }) | null;
  endDate: string | null;
  approvedBy: EntityReference | null;
  approvedOn: string | null;
  duration: string | null;
  hasCustomerApproved: boolean;
  hasCustomerReviewed: boolean;
  /**
   * The two requirement boxes (does the change need the customer's approval / review): the REQUIREMENT, never
   * the outcome. Omitted when the backend cannot say (the previous system's data source), which is not "not
   * required".
   */
  customerApprovalRequired?: boolean | null;
  customerReviewRequired?: boolean | null;
  /** Omitted when nothing was ever proposed (or the data source cannot say). */
  customerProposal?: ChangeRequestCustomerProposalDto | null;
  impact: EntityReference | null;
  state: EntityReference | null;
  type: EntityReference | null;
  assignedTeam: EntityReference | null;
  serviceOutage: string | null;
  rollbackPlan: string | null;
  communicationPlan: string | null;
  testPlan: null | string;
  createdOn: string;
  updatedOn: string;
}

/** One approver's row on an approval stage; the status is an open upper-case set (APPROVED, REQUESTED, ...). */
export interface ChangeRequestApproverDto {
  id: string;
  name: string;
  status: string;
  respondedOn?: string | null;
}

/**
 * One approval stage of a change request as the customer reads it (`GET /change-requests/{id}/approvals`): the
 * customer's own stages come whole, WSO2's internal ones as a label and a status only.
 */
export interface ChangeRequestApprovalDto {
  stage: string;
  approverType: string;
  approverName: string;
  /** "APPROVED", "REJECTED" or "PENDING". */
  status: string;
  approvers: ChangeRequestApproverDto[];
}

export interface ChangeRequestApprovalsDto {
  approvals: ChangeRequestApprovalDto[];
}

export interface GetChangeRequestsRquestDto {
  filters?: {
    impactKey?: number;
    searchQuery?: string;
    stateKeys?: number[];
    closedEndDate?: string;
    closedStartDate?: string;
  };
  pagination?: {
    limit?: number;
    offset?: number;
  };
}

export type ChangeRequestsStatsDto = Pick<
  CasesStatsDto,
  "totalCount" | "actionRequiredCount" | "activeCount" | "outstandingCount" | "stateCount" | "resolvedCases"
> & { resolvedCount: { total: number; currentMonth: number; pastThirtyDays: number } };
