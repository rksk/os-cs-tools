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

/**
 * What the change request pages read of the customer's part of the process, kept free of React and of any
 * component so the rules can be tested on their own (`changeRequestProgress.test.ts`).
 */

import type { ChangeRequestApprovalDto, ChangeRequestApprovalsDto } from "@src/types";

/**
 * The API state ids a change request is in once it has moved on from Customer Approval (id 5): Scheduled (-2),
 * Implement (-1), Review (0), Customer Review (1), Rollback (2), Closed (3) and Canceled (4).
 */
export const CHANGE_REQUEST_STATUS_IDS_PAST_CUSTOMER_APPROVAL: readonly string[] = [
  "-2",
  "-1",
  "0",
  "1",
  "2",
  "3",
  "4",
];

/**
 * Whether WSO2 accepted a time the customer proposed AND the change request has moved on because of it
 * (Scheduled or later). Accepting schedules the change, so that is where it stands; the customer's approval flag
 * stays false (no staff action records a customer's approval: the proposal was their own consent).
 *
 * An `agreed` answer is only the answer WSO2 once gave. Nothing clears it when the customers are asked again, so a
 * change that is (back) in Customer Approval, or in a state not known here, with `agreed` standing was NOT
 * scheduled by that acceptance and is waiting for the customer's own answer: reading it as accepted would tell
 * the customer there is nothing left for them to do.
 */
export function isProposedTimeAccepted(answer: string | null | undefined, stateId: string | null | undefined): boolean {
  return answer === "agreed" && !!stateId && CHANGE_REQUEST_STATUS_IDS_PAST_CUSTOMER_APPROVAL.includes(stateId);
}

// --- The approval stages as evidence ----------------------------------------------------------------------------

/** The role of an approval stage, read from the label the backend gives it. */
export type ApprovalStageKind = "peer" | "cab" | "review" | "customer-approval" | "customer-review" | "other";

const APPROVAL_STAGE_KINDS: Readonly<Record<string, ApprovalStageKind>> = {
  "peer approval": "peer",
  assess: "peer",
  "cab approval": "cab",
  authorize: "cab",
  "ecab approval": "cab",
  review: "review",
  "customer approval": "customer-approval",
  "customer review": "customer-review",
};

export function classifyApprovalStage(stage: string | null | undefined): ApprovalStageKind {
  return APPROVAL_STAGE_KINDS[(stage ?? "").trim().toLowerCase()] ?? "other";
}

function isApproved(status: string | null | undefined): boolean {
  return (status ?? "").trim().toUpperCase() === "APPROVED";
}

/**
 * Whether the approvals carry an APPROVED stage of the kind: the record that the step happened. The customer's
 * own stages must also show an APPROVED approver row (the customer's colleagues, visible by name): a stage
 * migrated without a label is named "Customer Approval" by its position whether or not it was the customer's,
 * and such a stage is WSO2's, whose rows the customer is not given.
 */
export function hasApprovedStage(
  approvals: readonly ChangeRequestApprovalDto[] | null | undefined,
  kind: ApprovalStageKind,
): boolean {
  if (!approvals) return false;
  const customersOwn = kind === "customer-approval" || kind === "customer-review";
  return approvals.some(
    (stage) =>
      classifyApprovalStage(stage.stage) === kind &&
      isApproved(stage.status) &&
      (!customersOwn || (stage.approvers ?? []).some((row) => isApproved(row.status))),
  );
}

/**
 * The approvals could not be read (the request failed and nothing was read before it). Distinct from an empty list,
 * which is a read that found no approval: a failed read is not a negative result.
 */
export const APPROVALS_UNAVAILABLE = "unavailable";

/** What the timeline has of the approvals: the stages as read, or {@link APPROVALS_UNAVAILABLE}. */
export type ApprovalsRead = readonly ChangeRequestApprovalDto[] | typeof APPROVALS_UNAVAILABLE;

/**
 * The approvals as the timeline takes them from the query's data: the stages read (cached ones too, when a later
 * refetch failed), or {@link APPROVALS_UNAVAILABLE} when nothing was read. Never `null`, which would read as "no
 * approval".
 */
export function approvalsRead(data: ChangeRequestApprovalsDto | null | undefined): ApprovalsRead {
  return data ? data.approvals : APPROVALS_UNAVAILABLE;
}

/** What the record says of the steps that need one. */
export interface TimelineEvidence {
  /** The customer's approval is on record: the flag, a proposed time WSO2 accepted, or the customer's APPROVED stage. */
  customerApproved: boolean;
  /** The customer's review is on record: the flag or the customer's APPROVED stage. */
  customerReviewed: boolean;
  /** An APPROVED Peer stage (Assess). */
  peerApproved: boolean;
  /** An APPROVED CAB stage (Authorize). */
  cabApproved: boolean;
  /** The requirement boxes; absent (or null) when the backend cannot say, which is not "not required". */
  customerApprovalRequired?: boolean | null;
  customerReviewRequired?: boolean | null;
  /**
   * The approvals could not be read: a passed step only they could vouch for is unknown (`unavailable`), not
   * unrecorded. The four booleans above then hold only what the detail says.
   */
  approvalsUnavailable?: boolean;
}

export const NO_EVIDENCE: TimelineEvidence = {
  customerApproved: false,
  customerReviewed: false,
  peerApproved: false,
  cabApproved: false,
};

/**
 * The evidence a change request's detail and approval stages give. An empty list is a read that found no approval
 * (no evidence); {@link APPROVALS_UNAVAILABLE} is a read that failed, which is no answer either way
 * (`approvalsUnavailable`), while what the detail itself records still counts.
 */
export function approvalEvidence(
  detail: {
    hasCustomerApproved?: boolean;
    hasCustomerReviewed?: boolean;
    isProposedTimeAccepted?: boolean;
    customerApprovalRequired?: boolean | null;
    customerReviewRequired?: boolean | null;
  },
  read: ApprovalsRead,
): TimelineEvidence {
  const approvalsUnavailable = read === APPROVALS_UNAVAILABLE;
  const approvals = approvalsUnavailable ? null : read;
  return {
    customerApproved:
      detail.hasCustomerApproved === true ||
      detail.isProposedTimeAccepted === true ||
      hasApprovedStage(approvals, "customer-approval"),
    customerReviewed: detail.hasCustomerReviewed === true || hasApprovedStage(approvals, "customer-review"),
    peerApproved: hasApprovedStage(approvals, "peer"),
    cabApproved: hasApprovedStage(approvals, "cab"),
    customerApprovalRequired: detail.customerApprovalRequired,
    customerReviewRequired: detail.customerReviewRequired,
    approvalsUnavailable,
  };
}

// --- The progress timeline ----------------------------------------------------------------------------------------

/**
 * How a step of the progress timeline is drawn (the entry's own `status`):
 *
 * - `completed`: the record shows the step happened.
 * - `active`: the change request is there now.
 * - `pending`: not reached yet.
 * - `no-record`: the change request passed the step and nothing on record says it happened (an approval never
 *   recorded, a change migrated from the previous system without its approvals). Neutral: no tick, no colour.
 * - `not-required`: the step does not apply to this change request (its requirement box is off).
 * - `unavailable`: the change request passed the step, only the approvals could say whether it happened, and they
 *   could not be read. Not known either way: neither a tick nor "no record".
 */
export type TimelineStepStatus = "completed" | "active" | "pending" | "no-record" | "not-required" | "unavailable";

/** The captions of the neutral states. */
export const TIMELINE_NO_APPROVAL_RECORDED = "No approval recorded";
export const TIMELINE_NO_REVIEW_RECORDED = "No review recorded";
export const TIMELINE_NOT_REQUIRED = "Not required";
export const TIMELINE_APPROVAL_UNAVAILABLE = "Approval record could not be loaded";
export const TIMELINE_REVIEW_UNAVAILABLE = "Review record could not be loaded";

/**
 * The colour of the Customer Approval step's marker in the progress timeline, or `undefined` for the timeline's
 * ordinary one: green once the customer's approval is on record (or WSO2 accepted the time they proposed) and the
 * step is reached; nothing otherwise. A step the change moved past with no approval on record is neutral
 * (`no-record`), never a colour that says "not approved": the record does not say that either.
 */
export function customerApprovalStepFill(input: {
  status: TimelineStepStatus;
  customerApproved: boolean;
}): "green" | undefined {
  if (input.status !== "completed" && input.status !== "active") return undefined;
  return input.customerApproved ? "green" : undefined;
}

/** One step of the progress timeline as the view draws it (the entry's `status`, `fill`, `end` and `last`). */
export interface TimelineStepState {
  title: string;
  status: TimelineStepStatus;
  /**
   * A caption that replaces the step's usual one ("No approval recorded", "Not required", "Approval record could not
   * be loaded"); none for the usual.
   */
  description?: string;
  /** Only the Customer Approval step is coloured: see {@link customerApprovalStepFill}. */
  fill: "green" | undefined;
  /** The change request has moved past this step: the connector below it is highlighted. */
  passed: boolean;
  /** The step is one of the three closing ones (Rollback, Closed, Canceled): its connector is not highlighted. */
  end: boolean;
  last: boolean;
}

/**
 * How every step of the progress timeline is drawn, given the steps' titles in order, the index of the one the
 * change request is in (`-1` when its state is not known) and what the record says ({@link TimelineEvidence}).
 *
 * A step before the current one is done only when the record vouches for it: New once the change moved on (its
 * creation is a fact), Scheduled / Implement / Review once passed (the state is the record), Assess / Authorize
 * on an APPROVED Peer / CAB stage, Customer Approval / Customer Review on the customer's recorded answer. Passed
 * with nothing on record it reads `no-record`, or `unavailable` when the approvals could not be read (a failed read
 * is not a negative result); a customer step whose box is off reads `not-required` (before the step as well). The
 * three closing steps (Rollback, Closed, Canceled) are only ever current or pending, and a canceled change request
 * shows nothing done: the timeline has no history of where it was canceled. After a rollback, Customer Review never
 * happens, so it stays pending.
 */
export function timelineStepStates(
  titles: readonly string[],
  activeIndex: number,
  evidence: TimelineEvidence = NO_EVIDENCE,
): TimelineStepState[] {
  const activeTitle = titles[activeIndex];
  const canceled = activeTitle === "Canceled";
  const rolledBack = activeTitle === "Rollback";
  const closingFrom = titles.length - 3;

  const statusOf = (title: string, index: number): { status: TimelineStepStatus; description?: string } => {
    if (index === activeIndex) return { status: "active" };
    if (index >= closingFrom) return { status: "pending" };
    const past = !canceled && index < activeIndex;
    const byRecord = (
      recorded: boolean,
      required: boolean | null | undefined,
      noRecord: string,
      unavailable: string,
    ): { status: TimelineStepStatus; description?: string } => {
      if (canceled) return { status: "pending" };
      if (past && recorded) return { status: "completed" };
      if (required === false) return { status: "not-required", description: TIMELINE_NOT_REQUIRED };
      if (!past) return { status: "pending" };
      return evidence.approvalsUnavailable
        ? { status: "unavailable", description: unavailable }
        : { status: "no-record", description: noRecord };
    };
    const approval = [TIMELINE_NO_APPROVAL_RECORDED, TIMELINE_APPROVAL_UNAVAILABLE] as const;
    const review = [TIMELINE_NO_REVIEW_RECORDED, TIMELINE_REVIEW_UNAVAILABLE] as const;
    switch (title) {
      case "Assess":
        return byRecord(evidence.peerApproved, undefined, ...approval);
      case "Authorize":
        return byRecord(evidence.cabApproved, undefined, ...approval);
      case "Customer Approval":
        return byRecord(evidence.customerApproved, evidence.customerApprovalRequired, ...approval);
      case "Customer Review":
        if (rolledBack) return { status: "pending" };
        return byRecord(evidence.customerReviewed, evidence.customerReviewRequired, ...review);
      default:
        return { status: past ? "completed" : "pending" };
    }
  };

  return titles.map((title, index) => {
    const { status, description } = statusOf(title, index);
    return {
      title,
      status,
      description,
      fill:
        title === "Customer Approval"
          ? customerApprovalStepFill({ status, customerApproved: evidence.customerApproved })
          : undefined,
      passed: !canceled && index < activeIndex && index < closingFrom,
      end: index >= closingFrom,
      last: index === titles.length - 1,
    };
  });
}
