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

import type { CaseMetadataResponse } from "@features/support/types/cases";
import type {
  AuditMetadata,
  IdLabelRef,
  PaginationResponse,
  SearchRequestBase,
} from "@/types/common";

// Item type for a change request.
export type ChangeRequestItem = AuditMetadata & {
  id: string;
  internalId?: string | null;
  number: string;
  title: string;
  description?: string | null;
  project: IdLabelRef | null;
  case: IdLabelRef | null;
  deployment: IdLabelRef | null;
  deployedProduct: IdLabelRef | null;
  product: IdLabelRef | null;
  assignedEngineer: IdLabelRef | null;
  assignedTeam: IdLabelRef | null;
  startDate: string;
  endDate: string;
  duration: string | null;
  hasServiceOutage: boolean;
  impact: IdLabelRef | null;
  state: IdLabelRef | null;
  type: IdLabelRef | null;
};

/**
 * Where a proposed implementation time stands. The change request stays in
 * Customer Approval through the whole conversation; only WSO2's answer moves it.
 *
 * - `pending`: proposed, waiting for WSO2 to accept it or suggest another time.
 * - `agreed`: WSO2 accepted the proposed time (the change is then Scheduled).
 * - `disagreed`: WSO2 did not accept it (it kept or changed the planned window and
 *   the customers are asked again).
 * - `unanswered`: a proposal on record that WSO2 never answered (the change moved
 *   on): history, nothing to show.
 */
export type ChangeRequestProposalAnswer =
  | "pending"
  | "agreed"
  | "disagreed"
  | "unanswered";

/**
 * The customer-proposed time of a change request (the proposed start and its
 * confirmation, `customer_updated_on` / `customer_updated_date_confirmation`, read
 * through the backend). A proposal moves the START and keeps the planned length, so it
 * carries a start and, while it waits, the end that start implies.
 */
export type ChangeRequestCustomerProposal = {
  /** The proposed start (API date-time, UTC). */
  startDate: string;
  /** The proposed start plus the planned length; present while the proposal is pending. */
  endDate?: string | null;
  answer: ChangeRequestProposalAnswer;
  /**
   * True when the signed-in customer made the pending proposal. False or omitted
   * for a colleague's: the page then says "a new time was proposed" and no more.
   * A stored time nobody is recorded as having proposed (a date a WSO2 user wrote
   * in the previous system, an old one) is never sent as pending to a customer:
   * the backend reads it `unanswered`, history, so nothing tells a customer a time
   * of theirs waits for WSO2.
   */
  proposedByViewer?: boolean;
};

// Response type for detailed change request information.
export type ChangeRequestDetails = ChangeRequestItem & {
  description: string | null;
  createdBy: string;
  justification: string | null;
  impactDescription: string | null;
  serviceOutage: string | null;
  communicationPlan: string | null;
  rollbackPlan: string | null;
  testPlan: string | null;
  hasCustomerApproved: boolean;
  hasCustomerReviewed: boolean;
  /**
   * Viewer-specific: true only when the signed-in customer has a pending answer
   * on this change request right now; false when they do not (already answered,
   * not asked, not a contact). Omitted when the data source cannot say (the
   * legacy ServiceNow source); the page then falls back to `hasCustomerApproved`
   * at Customer Approval and always offers the review answer at Customer Review.
   */
  customerCanAnswer?: boolean;
  /**
   * True while WSO2 has the change request on hold (the reason is not shared).
   * A held change refuses a proposed time, not an answer, so the page turns
   * Propose New Time off, and says why. Omitted when the data source cannot say,
   * which is not the same as "not held".
   */
  isOnHold?: boolean;
  /**
   * The time a customer proposed, and where WSO2's answer to it stands. Omitted
   * when nothing was ever proposed (or the data source cannot say).
   */
  customerProposal?: ChangeRequestCustomerProposal | null;
  /**
   * The two requirement boxes: whether the change request needs the customer's
   * approval before it is scheduled / the customer's review before it is closed.
   * The REQUIREMENT, never the outcome (`hasCustomerApproved` /
   * `hasCustomerReviewed` record that). Present only when the backend read them
   * for a customer on the current data source; omitted when it cannot say (the
   * previous system's data source), which is "unknown", never "not required".
   */
  customerApprovalRequired?: boolean | null;
  customerReviewRequired?: boolean | null;
  approvedBy: IdLabelRef | null;
  approvedOn: string | null;
};

// --- Approvals (GET /change-requests/{id}/approvals) ----------------------

/** One approver's row on an approval stage; the status is an open upper-case set (APPROVED, REQUESTED, ...). */
export type ChangeRequestApprover = {
  id: string;
  name: string;
  status: string;
  respondedOn?: string | null;
};

/**
 * One approval stage of a change request as the customer reads it. The
 * customer's own stages (Customer Approval / Customer Review) come whole, with
 * the contacts' rows by name; WSO2's internal stages (Peer, CAB, Review) come as
 * their label and status only, with no approver rows.
 */
export type ChangeRequestApproval = {
  stage: string;
  approverType: string;
  approverName: string;
  /** "APPROVED", "REJECTED" or "PENDING". */
  status: string;
  approvers: ChangeRequestApprover[];
};

export type ChangeRequestApprovalsResponse = {
  approvals: ChangeRequestApproval[];
};

/**
 * What the page has of the approvals: the stages as read (an empty list is a
 * read that found none), or "unavailable" when the read failed and nothing was
 * read before it. A failed read is not a negative result: what the approvals
 * would say is unknown, so nothing is said either way.
 */
export type ChangeRequestApprovalsRead = ReadonlyArray<ChangeRequestApproval> | "unavailable";

// Response type for change request search results.
export type ChangeRequestSearchResponse = PaginationResponse & {
  changeRequests: ChangeRequestItem[];
};

// Response type for change request statistics.
export type ChangeRequestStats = {
  totalRequests: number;
  awaitingYourAction: number;
  ongoing: number;
  completed: number;
};

// Item type for change request state count.
export type ChangeRequestStateCount = IdLabelRef & { count: number };

// Response type for change request statistics breakdown.
export type ChangeRequestStatsResponse = {
  totalCount: number;
  activeCount?: number;
  outstandingCount?: number;
  actionRequiredCount?: number;
  stateCount: ChangeRequestStateCount[];
  resolvedCount: {
    total: number;
    currentMonth: number;
    pastThirtyDays: number;
  };
};

// Response type for patching a change request.
export type PatchChangeRequestResponse = AuditMetadata & {
  id: string;
};

// Model type for change request filters state.
export type ChangeRequestFilterValues = {
  stateIds?: string[];
  impactIds?: string[];
};

/** Change request list sort field for search API `sortBy.field`. */
export enum ChangeRequestSortField {
  UpdatedOn = "updatedOn",
  CreatedOn = "createdOn",
}

// Filter type for searching change requests.
export type ChangeRequestSearchFilters = {
  impactKeys?: number[];
  searchQuery?: string;
  stateKeys?: number[];
  closedStartDate?: string;
  closedEndDate?: string;
};

// Request type for searching change requests.
export type ChangeRequestSearchRequest = SearchRequestBase & {
  filters?: ChangeRequestSearchFilters;
};

// Request type for patching a change request. The customer-portal backend takes
// either the customer's answer (isCustomerApproved / isCustomerReviewed) or a
// proposed time (plannedStartOn / plannedEndOn, "YYYY-MM-DD HH:MM:SS" in UTC),
// never both in one request. A proposed time is a new START: the planned length
// stays, so plannedEndOn, when sent, is the proposed start plus that length (the
// dialog sends it so the previous system's data source, which takes a whole window, keeps
// working; the service refuses any other end). An answer also names the planned
// window the customer was looking at (expectedPlannedStartOn /
// expectedPlannedEndOn, as the details read them): it is then recorded only while
// that is still the window.
export type PatchChangeRequestRequest = {
  plannedStartOn?: string;
  plannedEndOn?: string;
  isCustomerApproved?: boolean;
  isCustomerReviewed?: boolean;
  expectedPlannedStartOn?: string;
  expectedPlannedEndOn?: string;
};

// Enum for change request decision mode.
export enum ChangeRequestDecisionMode {
  CUSTOMER_APPROVAL = "customerApproval",
  CUSTOMER_REVIEW = "customerReview",
  NONE = "none",
}

/**
 * Whether the customer can propose a new implementation time right now, as the
 * page decides it (it is offered at Customer Approval only, and switched off
 * while WSO2 has the change on hold). Wording that points a customer at Propose
 * New Time must follow it, so that it never points at an action that is off.
 */
export type ProposeNewTimeAvailability =
  /** The Propose New Time button is on. */
  | "available"
  /** It is offered but switched off because WSO2 has the change on hold. */
  | "on_hold"
  /**
   * It is not offered at all (Customer Review), or it is switched off because
   * there is no planned window to move: either way the reject confirmation
   * says nothing about it.
   */
  | "unavailable";

/**
 * What the workflow can say of a step, from the record alone:
 *
 * - `completed`: the record shows it happened (an approval on file, a state the
 *   change has been through).
 * - `current`: the change request is there now.
 * - `not-required`: the step does not apply to this change (its requirement box
 *   is off), so it is passed without an approval.
 * - `no-record`: the change request has moved past the step, and nothing on
 *   record says the step happened (an approval that was never recorded, or a
 *   change migrated from the previous system without its approvals). Neutral:
 *   not a tick, not a failure.
 * - `unavailable`: the change request has moved past the step, and only the
 *   approvals could say whether it happened, but they could not be read. Not
 *   known either way: neither a tick nor "no record".
 * - `pending`: not reached yet.
 * - `disabled`: cannot happen on the path the change request took (Closed on a
 *   canceled change, Customer Review after a rollback).
 */
export type ChangeRequestWorkflowStageStatus =
  | "completed"
  | "current"
  | "not-required"
  | "no-record"
  | "unavailable"
  | "pending"
  | "disabled";

// Item type for a change request workflow stage.
export type ChangeRequestWorkflowStage = {
  name: string;
  description: string;
  status: ChangeRequestWorkflowStageStatus;
  /** `status === "completed"`, kept for the renderers that read the booleans. */
  completed: boolean;
  /** `status === "current"`. */
  current: boolean;
  /** `status === "disabled"`. */
  disabled: boolean;
};

/**
 * Who approved a change request and when, as the page prints it: the record's
 * own `approvedBy` / `approvedOn` when it has them, else the CAB (Authorize)
 * stage's approver from the approvals, else nothing ("Not available").
 */
export type ChangeRequestApprovalRecord = {
  approvedBy: string | null;
  approvedOn: string | null;
  /**
   * Where the two values came from; "none" when both are null. "unavailable"
   * when both are null because the record has neither and the approvals could
   * not be read: who approved it is not known, which is not the same as no
   * approval.
   */
  source: "record" | "approvals" | "none" | "unavailable";
};

// --- Change requests UI (list, calendar, filters) ---------------------------

/** List vs calendar on the change requests page. */
export enum ChangeRequestsViewMode {
  List = "list",
  Calendar = "calendar",
}

/**
 * `ListFiltersPanel` / `CHANGE_REQUEST_FILTER_DEFINITIONS` entry `id` values
 * (used when resolving metadata into select options).
 */
export enum ChangeRequestFilterDefinitionId {
  State = "state",
  Impact = "impact",
}

export type ChangeRequestsListProps = {
  changeRequests: ChangeRequestItem[];
  isLoading: boolean;
  isError?: boolean;
  hasListRefinement?: boolean;
  onChangeRequestClick?: (item: ChangeRequestItem) => void;
};

export type ChangeRequestsCalendarViewProps = {
  changeRequests: ChangeRequestItem[];
  isLoading: boolean;
  isError?: boolean;
  onChangeRequestClick?: (item: ChangeRequestItem) => void;
  legendStates?: { label: string }[];
};

export type ChangeRequestFilterOption = {
  label: string;
  value: string;
};

export type ChangeRequestsSearchBarProps = {
  searchTerm: string;
  onSearchChange: (value: string) => void;
  isFiltersOpen: boolean;
  onFiltersToggle: () => void;
  filters: ChangeRequestFilterValues;
  filterMetadata: CaseMetadataResponse | undefined;
  onFilterChange: (field: string, value: string | string[]) => void;
  onClearFilters: () => void;
};

export type ChangeRequestsStatCardsProps = {
  isLoading: boolean;
  isError?: boolean;
  stats: ChangeRequestStats | undefined;
};

export type ProposeNewImplementationTimeModalProps = {
  open: boolean;
  onClose: () => void;
  /** Called once a proposal has been accepted, just before the dialog closes. */
  onProposed?: () => void;
  /**
   * Called when the backend refused the proposal for good (the change request no
   * longer waits on this customer), just before the dialog closes: the form and
   * the button that opened it are gone, so whoever owns the page moves focus
   * somewhere stable. Not called for a refusal that keeps the dialog open.
   */
  onRefused?: () => void;
  changeRequest: ChangeRequestDetails | null;
};

export type ScheduledMaintenanceWindowCardProps = {
  changeRequest: ChangeRequestDetails | null;
};
