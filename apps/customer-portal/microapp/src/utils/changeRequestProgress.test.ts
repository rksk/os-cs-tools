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

import { describe, expect, it } from "vitest";
import {
  approvalEvidence,
  approvalsRead,
  APPROVALS_UNAVAILABLE,
  CHANGE_REQUEST_STATUS_IDS_PAST_CUSTOMER_APPROVAL,
  classifyApprovalStage,
  customerApprovalStepFill,
  hasApprovedStage,
  isProposedTimeAccepted,
  NO_EVIDENCE,
  TIMELINE_APPROVAL_UNAVAILABLE,
  TIMELINE_NO_APPROVAL_RECORDED,
  TIMELINE_NO_REVIEW_RECORDED,
  TIMELINE_NOT_REQUIRED,
  TIMELINE_REVIEW_UNAVAILABLE,
  timelineStepStates,
  type TimelineEvidence,
  type TimelineStepStatus,
} from "./changeRequestProgress";
import type { ChangeRequestApprovalDto } from "@src/types";

// The API state ids of a change request (the customer portal's own): New -5, Assess -4, Authorize -3, Customer Approval 5,
// Scheduled -2, Implement -1, Review 0, Customer Review 1, Rollback 2, Closed 3, Canceled 4.
const BEFORE_OR_AT_CUSTOMER_APPROVAL = ["-5", "-4", "-3", "5"];
const PAST_CUSTOMER_APPROVAL = ["-2", "-1", "0", "1", "2", "3", "4"];

describe("isProposedTimeAccepted", () => {
  it("is true when WSO2 agreed and the change request has moved on from Customer Approval, in each state after it", () => {
    expect([...CHANGE_REQUEST_STATUS_IDS_PAST_CUSTOMER_APPROVAL]).toEqual(PAST_CUSTOMER_APPROVAL);
    for (const stateId of PAST_CUSTOMER_APPROVAL) expect(isProposedTimeAccepted("agreed", stateId), stateId).toBe(true);
  });

  // An `agreed` answer is not cleared when the customers are asked again: back in Customer Approval, Approve and Reject are live.
  it("is false while the change request is in Customer Approval, or before it, whatever answer stands", () => {
    for (const stateId of BEFORE_OR_AT_CUSTOMER_APPROVAL)
      expect(isProposedTimeAccepted("agreed", stateId), stateId).toBe(false);
  });

  it("is false for a state that is not known, or none at all", () => {
    for (const stateId of ["", "99", "abc", null, undefined])
      expect(isProposedTimeAccepted("agreed", stateId), String(stateId)).toBe(false);
  });

  it("is false for every other answer, or no proposal, in any state", () => {
    for (const answer of ["pending", "disagreed", "unanswered", "", "AGREED", null, undefined]) {
      for (const stateId of [...BEFORE_OR_AT_CUSTOMER_APPROVAL, ...PAST_CUSTOMER_APPROVAL]) {
        expect(isProposedTimeAccepted(answer, stateId), `${String(answer)} in ${stateId}`).toBe(false);
      }
    }
  });
});

const STATUSES: TimelineStepStatus[] = ["completed", "active", "pending", "no-record", "not-required", "unavailable"];

describe("customerApprovalStepFill (the Customer Approval step of the progress timeline)", () => {
  it("is green once the approval counts as given and the step is reached", () => {
    for (const status of ["completed", "active"] as const) {
      expect(customerApprovalStepFill({ status, customerApproved: true }), status).toBe("green");
    }
  });

  it("is the ordinary colour everywhere else: not reached, waiting, passed with no record, not required", () => {
    for (const status of STATUSES) {
      expect(customerApprovalStepFill({ status, customerApproved: false }), status).toBeUndefined();
    }
    for (const status of ["pending", "no-record", "not-required", "unavailable"] as const) {
      expect(customerApprovalStepFill({ status, customerApproved: true }), status).toBeUndefined();
    }
  });

  // The old rule drew a red marker for a change canceled "without the customer's approval": the record does not say
  // that of a change migrated without its approvals, so nothing is ever drawn as "not approved".
  it("never says 'not approved': green or nothing", () => {
    for (const status of STATUSES) {
      for (const customerApproved of [false, true]) {
        expect([undefined, "green"]).toContain(customerApprovalStepFill({ status, customerApproved }));
      }
    }
  });
});

const APPROVED_PEER: ChangeRequestApprovalDto = {
  stage: "Peer Approval",
  approverType: "STATIC_GROUP",
  approverName: "",
  status: "APPROVED",
  approvers: [],
};
const APPROVED_CAB: ChangeRequestApprovalDto = {
  stage: "CAB Approval",
  approverType: "STATIC_GROUP",
  approverName: "",
  status: "APPROVED",
  approvers: [],
};
const APPROVED_CUSTOMER_APPROVAL: ChangeRequestApprovalDto = {
  stage: "Customer Approval",
  approverType: "STATIC_GROUP",
  approverName: "Customer Group",
  status: "APPROVED",
  approvers: [{ id: "u-1", name: "A Contact", status: "APPROVED", respondedOn: "2026-05-02T10:00:00Z" }],
};
const APPROVED_CUSTOMER_REVIEW: ChangeRequestApprovalDto = {
  ...APPROVED_CUSTOMER_APPROVAL,
  stage: "Customer Review",
};

describe("the approval stages as evidence", () => {
  it("classifies the backend's labels, whatever their case or spacing", () => {
    expect(classifyApprovalStage("Peer Approval")).toBe("peer");
    expect(classifyApprovalStage("Assess")).toBe("peer");
    expect(classifyApprovalStage("CAB Approval")).toBe("cab");
    expect(classifyApprovalStage("Authorize")).toBe("cab");
    expect(classifyApprovalStage("ECAB Approval")).toBe("cab");
    expect(classifyApprovalStage("Review")).toBe("review");
    expect(classifyApprovalStage(" customer approval ")).toBe("customer-approval");
    expect(classifyApprovalStage("Customer Review")).toBe("customer-review");
    for (const other of ["", null, undefined, "Devops Approval", "Something"]) {
      expect(classifyApprovalStage(other), String(other)).toBe("other");
    }
  });

  it("finds an APPROVED stage of a kind, and nothing on no approvals or another status", () => {
    expect(hasApprovedStage([APPROVED_PEER, APPROVED_CAB], "cab")).toBe(true);
    expect(hasApprovedStage([APPROVED_PEER], "cab")).toBe(false);
    expect(hasApprovedStage([{ ...APPROVED_CAB, status: "approved" }], "cab")).toBe(true);
    for (const status of ["PENDING", "REJECTED", ""]) {
      expect(hasApprovedStage([{ ...APPROVED_PEER, status }], "peer"), status).toBe(false);
    }
    expect(hasApprovedStage([], "peer")).toBe(false);
    expect(hasApprovedStage(null, "peer")).toBe(false);
    expect(hasApprovedStage(undefined, "peer")).toBe(false);
  });

  // A stage migrated without a label is named "Customer Approval" by its position whether or not it was the
  // customer's; such a stage is WSO2's, whose rows the customer is not given.
  it("takes the customer's own stage as approved only with a visible APPROVED row", () => {
    expect(hasApprovedStage([APPROVED_CUSTOMER_APPROVAL], "customer-approval")).toBe(true);
    expect(hasApprovedStage([APPROVED_CUSTOMER_REVIEW], "customer-review")).toBe(true);
    for (const approvers of [[], [{ id: "u-1", name: "A Contact", status: "REQUESTED" }]]) {
      expect(hasApprovedStage([{ ...APPROVED_CUSTOMER_APPROVAL, approvers }], "customer-approval")).toBe(false);
      expect(hasApprovedStage([{ ...APPROVED_CUSTOMER_REVIEW, approvers }], "customer-review")).toBe(false);
    }
  });

  it("reads the evidence from the detail and the approvals together", () => {
    expect(
      approvalEvidence({ hasCustomerApproved: false, hasCustomerReviewed: false, isProposedTimeAccepted: false }, []),
    ).toEqual({
      ...NO_EVIDENCE,
      customerApprovalRequired: undefined,
      customerReviewRequired: undefined,
      approvalsUnavailable: false,
    });
    expect(
      approvalEvidence(
        {
          hasCustomerApproved: false,
          hasCustomerReviewed: false,
          isProposedTimeAccepted: true,
          customerApprovalRequired: true,
          customerReviewRequired: false,
        },
        [APPROVED_PEER, APPROVED_CUSTOMER_REVIEW],
      ),
    ).toEqual({
      customerApproved: true,
      customerReviewed: true,
      peerApproved: true,
      cabApproved: false,
      customerApprovalRequired: true,
      customerReviewRequired: false,
      approvalsUnavailable: false,
    });
    expect(approvalEvidence({ hasCustomerApproved: true, hasCustomerReviewed: true }, [APPROVED_CAB])).toMatchObject({
      customerApproved: true,
      customerReviewed: true,
      peerApproved: false,
      cabApproved: true,
    });
  });
});

// A failed read is not a negative result: it crosses into the timeline as APPROVALS_UNAVAILABLE, never as null.
describe("approvals that could not be read", () => {
  it("passes a failed read on as APPROVALS_UNAVAILABLE, and a read that found nothing as an empty list", () => {
    expect(approvalsRead(undefined)).toBe(APPROVALS_UNAVAILABLE);
    expect(approvalsRead(null)).toBe(APPROVALS_UNAVAILABLE);
    expect(approvalsRead({ approvals: [] })).toEqual([]);
    expect(approvalsRead({ approvals: [APPROVED_CAB] })).toEqual([APPROVED_CAB]);
  });

  it("keeps what the detail records, and marks the approvals unavailable rather than empty", () => {
    expect(
      approvalEvidence(
        { hasCustomerApproved: true, hasCustomerReviewed: false, customerReviewRequired: true },
        APPROVALS_UNAVAILABLE,
      ),
    ).toEqual({
      customerApproved: true,
      customerReviewed: false,
      peerApproved: false,
      cabApproved: false,
      customerApprovalRequired: undefined,
      customerReviewRequired: true,
      approvalsUnavailable: true,
    });
  });
});

describe("timelineStepStates (what the view draws, step by step)", () => {
  // The steps of the customer portal's change request timeline, in order (`TIMELINE_META`).
  const TITLES = [
    "New",
    "Assess",
    "Authorize",
    "Customer Approval",
    "Scheduled",
    "Implement",
    "Review",
    "Customer Review",
    "Rollback",
    "Closed",
    "Canceled",
  ];
  const at = (activeTitle: string | null, evidence: TimelineEvidence = NO_EVIDENCE) =>
    timelineStepStates(TITLES, activeTitle ? TITLES.indexOf(activeTitle) : -1, evidence);
  const stepOf = (activeTitle: string | null, evidence: TimelineEvidence = NO_EVIDENCE, title = "Customer Approval") =>
    at(activeTitle, evidence).find((s) => s.title === title)!;
  const statusesOf = (activeTitle: string | null, evidence: TimelineEvidence = NO_EVIDENCE) =>
    Object.fromEntries(at(activeTitle, evidence).map((s) => [s.title, s.status]));
  const approved: TimelineEvidence = { ...NO_EVIDENCE, customerApproved: true };
  const whole: TimelineEvidence = {
    customerApproved: true,
    customerReviewed: true,
    peerApproved: true,
    cabApproved: true,
  };

  it("draws a change migrated without its approvals (Closed, no record) with a tick only where the state itself is the record", () => {
    expect(statusesOf("Closed")).toEqual({
      New: "completed",
      Assess: "no-record",
      Authorize: "no-record",
      "Customer Approval": "no-record",
      Scheduled: "completed",
      Implement: "completed",
      Review: "completed",
      "Customer Review": "no-record",
      Rollback: "pending",
      Closed: "active",
      Canceled: "pending",
    });
    expect(stepOf("Closed", NO_EVIDENCE, "Assess").description).toBe(TIMELINE_NO_APPROVAL_RECORDED);
    expect(stepOf("Closed", NO_EVIDENCE, "Customer Approval").description).toBe(TIMELINE_NO_APPROVAL_RECORDED);
    expect(stepOf("Closed", NO_EVIDENCE, "Customer Review").description).toBe(TIMELINE_NO_REVIEW_RECORDED);
    expect(stepOf("Closed", NO_EVIDENCE, "Scheduled").description).toBeUndefined();
    // The evidence defaults to none.
    expect(timelineStepStates(TITLES, TITLES.indexOf("Closed"))).toEqual(at("Closed"));
  });

  it("never draws an approval step done by position alone, in any state past it", () => {
    const past = {
      Assess: [
        "Authorize",
        "Customer Approval",
        "Scheduled",
        "Implement",
        "Review",
        "Customer Review",
        "Rollback",
        "Closed",
      ],
      Authorize: ["Customer Approval", "Scheduled", "Implement", "Review", "Customer Review", "Rollback", "Closed"],
      "Customer Approval": ["Scheduled", "Implement", "Review", "Customer Review", "Rollback", "Closed"],
      "Customer Review": ["Closed"],
    };
    for (const [title, states] of Object.entries(past)) {
      for (const active of states) {
        expect(stepOf(active, NO_EVIDENCE, title).status, `${title} in ${active}`).toBe("no-record");
      }
    }
  });

  it("draws a passed step only the approvals could vouch for as unknown when they could not be read, never 'No approval recorded'", () => {
    const unread = approvalEvidence(
      { hasCustomerApproved: false, hasCustomerReviewed: false },
      approvalsRead(undefined),
    );
    expect(statusesOf("Closed", unread)).toEqual({
      New: "completed",
      Assess: "unavailable",
      Authorize: "unavailable",
      "Customer Approval": "unavailable",
      Scheduled: "completed",
      Implement: "completed",
      Review: "completed",
      "Customer Review": "unavailable",
      Rollback: "pending",
      Closed: "active",
      Canceled: "pending",
    });
    for (const title of ["Assess", "Authorize", "Customer Approval"]) {
      expect(stepOf("Closed", unread, title), title).toMatchObject({
        description: TIMELINE_APPROVAL_UNAVAILABLE,
        fill: undefined,
      });
    }
    expect(stepOf("Closed", unread, "Customer Review").description).toBe(TIMELINE_REVIEW_UNAVAILABLE);
    expect(
      at("Closed", unread).filter(
        (s) => s.description === TIMELINE_NO_APPROVAL_RECORDED || s.description === TIMELINE_NO_REVIEW_RECORDED,
      ),
    ).toEqual([]);
    // A read that found nothing is a different answer: "No approval recorded".
    const none = approvalEvidence(
      { hasCustomerApproved: false, hasCustomerReviewed: false },
      approvalsRead({ approvals: [] }),
    );
    expect(stepOf("Closed", none, "Assess")).toMatchObject({
      status: "no-record",
      description: TIMELINE_NO_APPROVAL_RECORDED,
    });
  });

  it("still draws what the detail records, a box that is off, the current step and a canceled change as before when the approvals could not be read", () => {
    const unread = approvalEvidence(
      { hasCustomerApproved: true, hasCustomerReviewed: false, customerReviewRequired: false },
      APPROVALS_UNAVAILABLE,
    );
    expect(statusesOf("Closed", unread)).toMatchObject({
      Assess: "unavailable",
      Authorize: "unavailable",
      "Customer Approval": "completed",
      "Customer Review": "not-required",
    });
    expect(stepOf("Closed", unread).fill).toBe("green");
    expect(statusesOf("Authorize", unread)).toMatchObject({
      Assess: "unavailable",
      Authorize: "active",
      "Customer Approval": "pending",
    });
    expect(at("Canceled", unread).filter((s) => s.status === "unavailable" || s.status === "completed")).toEqual([]);
  });

  it("draws the steps the record vouches for done", () => {
    expect(statusesOf("Closed", whole)).toEqual({
      New: "completed",
      Assess: "completed",
      Authorize: "completed",
      "Customer Approval": "completed",
      Scheduled: "completed",
      Implement: "completed",
      Review: "completed",
      "Customer Review": "completed",
      Rollback: "pending",
      Closed: "active",
      Canceled: "pending",
    });
    expect(stepOf("Scheduled", { ...NO_EVIDENCE, peerApproved: true }, "Assess").status).toBe("completed");
    expect(stepOf("Scheduled", { ...NO_EVIDENCE, peerApproved: true }, "Authorize").status).toBe("no-record");
    expect(stepOf("Scheduled", { ...NO_EVIDENCE, cabApproved: true }, "Authorize").status).toBe("completed");
  });

  it("says 'Not required' of a customer step whose box is off, passed or not, and lets evidence win over it", () => {
    const off: TimelineEvidence = { ...NO_EVIDENCE, customerApprovalRequired: false, customerReviewRequired: false };
    for (const active of ["Closed", "Scheduled", "Assess"]) {
      expect(stepOf(active, off), active).toMatchObject({
        status: "not-required",
        description: TIMELINE_NOT_REQUIRED,
        fill: undefined,
      });
    }
    expect(stepOf("Closed", off, "Customer Review")).toMatchObject({
      status: "not-required",
      description: TIMELINE_NOT_REQUIRED,
    });
    expect(stepOf("Closed", { ...off, customerApproved: true, customerReviewed: true }).status).toBe("completed");
    expect(stepOf("Closed", { ...off, customerApproved: true, customerReviewed: true }, "Customer Review").status).toBe(
      "completed",
    );
    // On or unknown: "no record", never "not required".
    for (const customerApprovalRequired of [true, undefined, null]) {
      expect(
        stepOf("Closed", { ...NO_EVIDENCE, customerApprovalRequired }),
        String(customerApprovalRequired),
      ).toMatchObject({
        status: "no-record",
        description: TIMELINE_NO_APPROVAL_RECORDED,
      });
    }
  });

  it("draws a change request that has not reached Customer Approval as a grey, uncoloured step: nothing says 'not approved' yet", () => {
    for (const active of ["New", "Assess", "Authorize"]) {
      for (const evidence of [NO_EVIDENCE, approved]) {
        expect(stepOf(active, evidence), `${active} ${evidence.customerApproved}`).toMatchObject({
          status: "pending",
          fill: undefined,
        });
      }
    }
  });

  it("draws the step current and uncoloured while the change request waits for the customer, green once the approval counts as given", () => {
    expect(stepOf("Customer Approval")).toMatchObject({ status: "active", fill: undefined });
    expect(stepOf("Customer Approval", approved)).toMatchObject({ status: "active", fill: "green" });
    expect(stepOf("Customer Approval", { ...NO_EVIDENCE, customerApprovalRequired: false }).status).toBe("active");
  });

  it("draws the step done and green for every change request that moved on with the approval given (recorded, or a proposed time WSO2 accepted)", () => {
    for (const active of ["Scheduled", "Implement", "Review", "Customer Review", "Rollback", "Closed"]) {
      expect(stepOf(active, approved), active).toMatchObject({ status: "completed", fill: "green" });
    }
  });

  it("draws the step passed with no record as a neutral, uncoloured step, never red", () => {
    for (const active of ["Scheduled", "Implement", "Review", "Customer Review", "Rollback", "Closed"]) {
      expect(stepOf(active), active).toMatchObject({
        status: "no-record",
        fill: undefined,
        description: TIMELINE_NO_APPROVAL_RECORDED,
      });
    }
  });

  it("shows nothing done for a canceled change request, record or no record", () => {
    for (const evidence of [NO_EVIDENCE, whole]) {
      const states = at("Canceled", evidence);
      expect(states.filter((s) => s.status === "completed" || s.status === "no-record")).toEqual([]);
      expect(states.filter((s) => s.status === "active").map((s) => s.title)).toEqual(["Canceled"]);
      expect(states.every((s) => s.fill === undefined && !s.passed)).toBe(true);
    }
  });

  it("after a rollback: Customer Review stays pending, and the earlier steps follow the record", () => {
    expect(statusesOf("Rollback", { ...NO_EVIDENCE, peerApproved: true, customerApproved: true })).toEqual({
      New: "completed",
      Assess: "completed",
      Authorize: "no-record",
      "Customer Approval": "completed",
      Scheduled: "completed",
      Implement: "completed",
      Review: "completed",
      "Customer Review": "pending",
      Rollback: "active",
      Closed: "pending",
      Canceled: "pending",
    });
  });

  it("colours no other step, and an unknown state leaves every step pending", () => {
    for (const active of [null, ...TITLES]) {
      for (const evidence of [NO_EVIDENCE, whole]) {
        const others = at(active, evidence).filter((s) => s.title !== "Customer Approval");
        expect(
          others.every((s) => s.fill === undefined),
          `${active} ${evidence.customerApproved}`,
        ).toBe(true);
      }
    }
    expect(at(null, whole).every((s) => s.status === "pending" && !s.passed)).toBe(true);
  });

  it("keeps the closing three current or pending, never done, and highlights the connector only below a passed step", () => {
    expect(statusesOf("Review")).toMatchObject({
      Review: "active",
      "Customer Review": "pending",
      Rollback: "pending",
      Closed: "pending",
      Canceled: "pending",
    });
    expect(
      at("Closed")
        .slice(8)
        .map((s) => s.status),
    ).toEqual(["pending", "active", "pending"]);
    expect(at("Review").map((s) => s.passed)).toEqual([
      true,
      true,
      true,
      true,
      true,
      true,
      false,
      false,
      false,
      false,
      false,
    ]);
    expect(at("Closed").map((s) => s.passed)).toEqual([
      true,
      true,
      true,
      true,
      true,
      true,
      true,
      true,
      false,
      false,
      false,
    ]);
  });

  it("marks the closing three as the ends of the line, and the last step", () => {
    const states = at("New");
    expect(states.map((s) => s.end)).toEqual([
      false,
      false,
      false,
      false,
      false,
      false,
      false,
      false,
      true,
      true,
      true,
    ]);
    expect(states.map((s) => s.last)).toEqual([
      false,
      false,
      false,
      false,
      false,
      false,
      false,
      false,
      false,
      false,
      true,
    ]);
  });
});
