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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ChangeRequestWorkflow from "@features/operations/components/change-requests/ChangeRequestWorkflow";
import {
  APPROVALS_UNAVAILABLE,
  buildChangeRequestWorkflowStages,
  WORKFLOW_APPROVAL_UNAVAILABLE,
  WORKFLOW_NO_APPROVAL_RECORDED,
  WORKFLOW_NO_REVIEW_RECORDED,
  WORKFLOW_NOT_REQUIRED,
  WORKFLOW_REVIEW_UNAVAILABLE,
} from "@features/operations/utils/changeRequests";
import type {
  ChangeRequestWorkflowStage,
  ChangeRequestWorkflowStageStatus,
} from "@features/operations/types/changeRequests";

function stage(
  name: string,
  status: ChangeRequestWorkflowStageStatus,
  description = `${name} caption`,
): ChangeRequestWorkflowStage {
  return {
    name,
    description,
    status,
    completed: status === "completed",
    current: status === "current",
    disabled: status === "disabled",
  };
}

/** The rendered step named `name`. */
function step(name: string): HTMLElement {
  const el = screen
    .getAllByTestId("change-request-workflow-stage")
    .find((node) => node.getAttribute("data-stage-name") === name);
  if (!el) throw new Error(`no step named ${name}`);
  return el;
}

describe("ChangeRequestWorkflow", () => {
  it("draws the three neutral states apart from one another and from a tick", () => {
    render(
      <ChangeRequestWorkflow
        currentStateIndex={4}
        stages={[
          stage("New", "completed", "Change request created"),
          stage("Assess", "no-record", WORKFLOW_NO_APPROVAL_RECORDED),
          stage("Authorize", "completed", "Internal authorization obtained"),
          stage("Customer Approval", "not-required", WORKFLOW_NOT_REQUIRED),
          stage("Scheduled", "current", "Maintenance window scheduled"),
          stage("Implement", "pending", "Change implementation"),
          stage("Closed", "disabled", "Change request completed"),
        ]}
      />,
    );

    // A tick only where the record shows the step happened.
    expect(within(step("New")).getByRole("img", { name: "New: completed" })).toBeInTheDocument();
    expect(within(step("Authorize")).getByRole("img", { name: "Authorize: completed" })).toBeInTheDocument();

    // Passed with nothing on record: no tick, and the caption says so.
    expect(step("Assess")).toHaveAttribute("data-stage-status", "no-record");
    expect(within(step("Assess")).getByRole("img", { name: "Assess: no record of this step" })).toBeInTheDocument();
    expect(within(step("Assess")).getByText(WORKFLOW_NO_APPROVAL_RECORDED)).toBeInTheDocument();

    // Does not apply: its own marker and caption, distinct from "no record".
    expect(step("Customer Approval")).toHaveAttribute("data-stage-status", "not-required");
    expect(
      within(step("Customer Approval")).getByRole("img", { name: "Customer Approval: not required" }),
    ).toBeInTheDocument();
    expect(within(step("Customer Approval")).getByText(WORKFLOW_NOT_REQUIRED)).toBeInTheDocument();

    // Not reached yet: the ordinary grey step with its usual caption.
    expect(step("Implement")).toHaveAttribute("data-stage-status", "pending");
    expect(within(step("Implement")).getByRole("img", { name: "Implement: not reached" })).toBeInTheDocument();

    // The current step is the only one marked "Current".
    expect(screen.getAllByText("Current")).toHaveLength(1);
    expect(within(step("Scheduled")).getByText("Current")).toBeInTheDocument();
    expect(within(step("Closed")).getByRole("img", { name: "Closed: not applicable" })).toBeInTheDocument();
  });

  it("shows placeholders, and no step at all, while the evidence is still loading", () => {
    render(<ChangeRequestWorkflow currentStateIndex={-1} stages={[stage("New", "completed")]} isLoading />);
    expect(screen.getByTestId("change-request-workflow-loading")).toBeInTheDocument();
    expect(screen.queryAllByTestId("change-request-workflow-stage")).toHaveLength(0);
    expect(screen.queryByText(WORKFLOW_NO_APPROVAL_RECORDED)).not.toBeInTheDocument();
  });

  describe("fed by the builder", () => {
    const closed = { id: "3", label: "Closed" };

    it("ticks nothing it cannot vouch for on a change migrated without its approvals", () => {
      const { workflowStages, currentStateIndex } = buildChangeRequestWorkflowStages(
        { state: closed, hasCustomerApproved: false, hasCustomerReviewed: false } as never,
        [],
      );
      render(<ChangeRequestWorkflow stages={workflowStages} currentStateIndex={currentStateIndex} />);

      const ticked = screen
        .getAllByTestId("change-request-workflow-stage")
        .filter((node) => node.getAttribute("data-stage-status") === "completed")
        .map((node) => node.getAttribute("data-stage-name"));
      expect(ticked).toEqual(["New", "Scheduled", "Implement", "Review"]);
      expect(within(step("Assess")).getByText(WORKFLOW_NO_APPROVAL_RECORDED)).toBeInTheDocument();
      expect(within(step("Authorize")).getByText(WORKFLOW_NO_APPROVAL_RECORDED)).toBeInTheDocument();
      expect(within(step("Customer Approval")).getByText(WORKFLOW_NO_APPROVAL_RECORDED)).toBeInTheDocument();
      expect(within(step("Customer Review")).getByText(WORKFLOW_NO_REVIEW_RECORDED)).toBeInTheDocument();
      expect(within(step("Closed")).getByText("Current")).toBeInTheDocument();
    });

    it("ticks the steps the record vouches for", () => {
      const { workflowStages, currentStateIndex } = buildChangeRequestWorkflowStages(
        {
          state: closed,
          hasCustomerApproved: true,
          hasCustomerReviewed: false,
          customerReviewRequired: false,
        } as never,
        [
          { stage: "Peer Approval", approverType: "STATIC_GROUP", approverName: "", status: "APPROVED", approvers: [] },
          { stage: "CAB Approval", approverType: "STATIC_GROUP", approverName: "", status: "APPROVED", approvers: [] },
        ],
      );
      render(<ChangeRequestWorkflow stages={workflowStages} currentStateIndex={currentStateIndex} />);

      for (const name of ["New", "Assess", "Authorize", "Customer Approval", "Scheduled", "Implement", "Review"]) {
        expect(step(name), name).toHaveAttribute("data-stage-status", "completed");
      }
      expect(within(step("Customer Approval")).getByText("Customer approval received")).toBeInTheDocument();
      expect(step("Customer Review")).toHaveAttribute("data-stage-status", "not-required");
      expect(within(step("Customer Review")).getByText(WORKFLOW_NOT_REQUIRED)).toBeInTheDocument();
    });
  });

  // A failed read is not a negative result: the panel never says "No approval recorded" on it.
  describe("when the approvals could not be read", () => {
    const closed = { id: "3", label: "Closed" };
    const notice = () => screen.queryByTestId("change-request-workflow-approvals-unavailable");

    it("says so, offers a Retry, and shows the steps only the approvals could vouch for as unknown", () => {
      const onRetry = vi.fn();
      const { workflowStages, currentStateIndex } = buildChangeRequestWorkflowStages(
        { state: closed, hasCustomerApproved: true, hasCustomerReviewed: false } as never,
        APPROVALS_UNAVAILABLE,
      );
      render(
        <ChangeRequestWorkflow
          stages={workflowStages}
          currentStateIndex={currentStateIndex}
          approvalsUnavailable
          onRetry={onRetry}
        />,
      );

      expect(notice()).toHaveTextContent("The approval records could not be loaded.");
      expect(screen.queryByText(WORKFLOW_NO_APPROVAL_RECORDED)).not.toBeInTheDocument();
      expect(screen.queryByText(WORKFLOW_NO_REVIEW_RECORDED)).not.toBeInTheDocument();
      for (const name of ["Assess", "Authorize"]) {
        expect(step(name), name).toHaveAttribute("data-stage-status", "unavailable");
        expect(
          within(step(name)).getByRole("img", { name: `${name}: record could not be loaded` }),
        ).toBeInTheDocument();
        expect(within(step(name)).getByText(WORKFLOW_APPROVAL_UNAVAILABLE)).toBeInTheDocument();
      }
      expect(step("Customer Review")).toHaveAttribute("data-stage-status", "unavailable");
      expect(within(step("Customer Review")).getByText(WORKFLOW_REVIEW_UNAVAILABLE)).toBeInTheDocument();
      // What the detail itself records still counts.
      expect(step("Customer Approval")).toHaveAttribute("data-stage-status", "completed");

      fireEvent.click(screen.getByRole("button", { name: "Retry" }));
      expect(onRetry).toHaveBeenCalledTimes(1);
    });

    describe("while they are read again", () => {
      const unknown = [stage("Assess", "unavailable", WORKFLOW_APPROVAL_UNAVAILABLE)];

      it("keeps the notice and the button where focus is, the button reading 'Retrying…' and doing nothing", () => {
        const onRetry = vi.fn();
        const view = render(
          <ChangeRequestWorkflow currentStateIndex={4} stages={unknown} approvalsUnavailable onRetry={onRetry} />,
        );
        screen.getByRole("button", { name: "Retry" }).focus();
        fireEvent.click(screen.getByRole("button", { name: "Retry" }));
        expect(onRetry).toHaveBeenCalledTimes(1);

        view.rerender(
          <ChangeRequestWorkflow
            currentStateIndex={4}
            stages={unknown}
            approvalsUnavailable
            onRetry={onRetry}
            isRetrying
          />,
        );
        const retrying = screen.getByRole("button", { name: "Retrying…" });
        // aria-disabled, not disabled: a disabled button drops the focus it had.
        expect(retrying).toHaveAttribute("aria-disabled", "true");
        expect(retrying).not.toBeDisabled();
        expect(retrying).toHaveFocus();
        expect(notice()).toBeInTheDocument();
        expect(screen.queryByTestId("change-request-workflow-loading")).not.toBeInTheDocument();
        expect(within(step("Assess")).getByText(WORKFLOW_APPROVAL_UNAVAILABLE)).toBeInTheDocument();
        fireEvent.click(retrying);
        expect(onRetry).toHaveBeenCalledTimes(1);

        // It failed again: the same notice, the button usable again, focus still on it.
        view.rerender(
          <ChangeRequestWorkflow currentStateIndex={4} stages={unknown} approvalsUnavailable onRetry={onRetry} />,
        );
        expect(screen.getByRole("button", { name: "Retry" })).toHaveFocus();
        expect(screen.getByRole("button", { name: "Retry" })).not.toHaveAttribute("aria-disabled");
      });

      it("moves focus to the panel heading when a retry succeeds and takes the notice (and its button) away", () => {
        const onRetry = vi.fn();
        const view = render(
          <ChangeRequestWorkflow currentStateIndex={4} stages={unknown} approvalsUnavailable onRetry={onRetry} />,
        );
        screen.getByRole("button", { name: "Retry" }).focus();
        fireEvent.click(screen.getByRole("button", { name: "Retry" }));

        view.rerender(
          <ChangeRequestWorkflow
            currentStateIndex={4}
            stages={[stage("Assess", "completed")]}
            onRetry={onRetry}
          />,
        );
        expect(notice()).not.toBeInTheDocument();
        expect(screen.getByRole("heading", { name: "Change Request Workflow" })).toHaveFocus();
      });

      it("leaves focus alone when the customer has moved it since", () => {
        const onRetry = vi.fn();
        const view = render(
          <>
            <button type="button">Elsewhere</button>
            <ChangeRequestWorkflow currentStateIndex={4} stages={unknown} approvalsUnavailable onRetry={onRetry} />
          </>,
        );
        fireEvent.click(screen.getByRole("button", { name: "Retry" }));
        screen.getByRole("button", { name: "Elsewhere" }).focus();

        view.rerender(
          <>
            <button type="button">Elsewhere</button>
            <ChangeRequestWorkflow currentStateIndex={4} stages={[stage("Assess", "completed")]} onRetry={onRetry} />
          </>,
        );
        expect(screen.getByRole("button", { name: "Elsewhere" })).toHaveFocus();
      });
    });

    it("says nothing of the kind after a read that found no approval: that is 'No approval recorded'", () => {
      const { workflowStages, currentStateIndex } = buildChangeRequestWorkflowStages(
        { state: closed, hasCustomerApproved: false, hasCustomerReviewed: false } as never,
        [],
      );
      render(<ChangeRequestWorkflow stages={workflowStages} currentStateIndex={currentStateIndex} />);
      expect(notice()).not.toBeInTheDocument();
      expect(screen.queryByText(WORKFLOW_APPROVAL_UNAVAILABLE)).not.toBeInTheDocument();
      expect(within(step("Assess")).getByText(WORKFLOW_NO_APPROVAL_RECORDED)).toBeInTheDocument();
    });
  });
});
