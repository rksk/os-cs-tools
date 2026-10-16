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

import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import SetFixEtaDialog from "@features/csm-cases/components/SetFixEtaDialog";

// The Product field is now a dropdown backed by the product catalogue
// (digiops-cs#3449), so every test that reveals it needs a real product list
// to pick from -- mocked directly rather than pulling in the real backend
// client (which requires window.config at module load, per this app's own
// testing convention for @api/backend/client-backed hooks).
const DEFAULT_PRODUCTS = {
  data: [
    { id: "prod-apim", name: "API Manager" },
    { id: "prod-is", name: "Identity Server" },
  ],
  isLoading: false,
};
const useSearchProductsMock = vi.fn(() => DEFAULT_PRODUCTS);
vi.mock("@features/csm-projects/api/useSearchProducts", () => ({
  useSearchProducts: () => useSearchProductsMock(),
}));

afterEach(() => {
  useSearchProductsMock.mockReturnValue(DEFAULT_PRODUCTS);
});

describe("SetFixEtaDialog — single combined save", () => {
  it("renders one Save button, disabled until at least one field is set", () => {
    render(
      <SetFixEtaDialog isSaving={false} onClose={() => {}} onSave={() => {}} />,
    );
    expect(screen.getAllByRole("button", { name: /^save$/i })).toHaveLength(1);
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();
  });

  it("enables Save once seeded with existing estimates", () => {
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        currentMostLikelyFixEta="2099-06-17"
        currentWorstCaseFixEta="2099-06-18"
        isSaving={false}
        onClose={() => {}}
        onSave={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: /^save$/i })).toBeEnabled();
  });

  it("saves all three seeded estimates together in one combined payload", () => {
    const onSave = vi.fn();
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        currentMostLikelyFixEta="2099-06-17"
        currentWorstCaseFixEta="2099-06-18"
        isSaving={false}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave).toHaveBeenCalledWith({
      bestCaseFixEta: "2099-06-16",
      mostLikelyFixEta: "2099-06-17",
      worstCaseFixEta: "2099-06-18",
    });
  });

  it("saves just one seeded estimate, independent of the other two being empty", () => {
    const onSave = vi.fn();
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        isSaving={false}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onSave).toHaveBeenCalledWith({ bestCaseFixEta: "2099-06-16" });
  });

  it("reveals product/public ticket fields and blocks Save until they're filled, when sharing with customer", async () => {
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        isSaving={false}
        onClose={() => {}}
        onSave={() => {}}
      />,
    );
    fireEvent.click(
      screen.getByRole("switch", { name: /share fix eta with customer/i }),
    );
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();

    fireEvent.mouseDown(screen.getByLabelText(/^product/i));
    fireEvent.click(await screen.findByRole("option", { name: "API Manager" }));
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/public ticket/i), {
      target: { value: "https://github.com/wso2/product-apim/issues/1" },
    });
    expect(screen.getByRole("button", { name: /^save$/i })).toBeEnabled();
  });

  it("blocks sharing with customer when no fix ETA is set, even with product/ticket filled in", async () => {
    render(
      <SetFixEtaDialog isSaving={false} onClose={() => {}} onSave={() => {}} />,
    );
    fireEvent.click(
      screen.getByRole("switch", { name: /share fix eta with customer/i }),
    );
    fireEvent.mouseDown(screen.getByLabelText(/^product/i));
    fireEvent.click(await screen.findByRole("option", { name: "API Manager" }));
    fireEvent.change(screen.getByLabelText(/public ticket/i), {
      target: { value: "https://github.com/wso2/product-apim/issues/1" },
    });
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();
    expect(
      screen.getByText(/pick at least one fix eta to share with the customer/i),
    ).toBeInTheDocument();
  });

  it("includes addPublicComment/product/publicTicket in the combined payload when sharing", async () => {
    const onSave = vi.fn();
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        isSaving={false}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(
      screen.getByRole("switch", { name: /share fix eta with customer/i }),
    );
    fireEvent.mouseDown(screen.getByLabelText(/^product/i));
    fireEvent.click(await screen.findByRole("option", { name: "API Manager" }));
    fireEvent.change(screen.getByLabelText(/public ticket/i), {
      target: { value: "https://github.com/wso2/product-apim/issues/1" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).toHaveBeenCalledWith({
      bestCaseFixEta: "2099-06-16",
      addPublicComment: true,
      product: "API Manager",
      publicTicket: "https://github.com/wso2/product-apim/issues/1",
    });
  });

  it("rejects a public ticket that isn't a GitHub issue/PR URL", async () => {
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        isSaving={false}
        onClose={() => {}}
        onSave={() => {}}
      />,
    );
    fireEvent.click(
      screen.getByRole("switch", { name: /share fix eta with customer/i }),
    );
    fireEvent.mouseDown(screen.getByLabelText(/^product/i));
    fireEvent.click(await screen.findByRole("option", { name: "API Manager" }));
    fireEvent.change(screen.getByLabelText(/public ticket/i), {
      target: { value: "not-a-url" },
    });
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();
    expect(
      screen.getAllByText(/must be a github issue or pull request url/i).length,
    ).toBeGreaterThan(0);

    fireEvent.change(screen.getByLabelText(/public ticket/i), {
      target: { value: "https://github.com/wso2/product-apim/issues/42" },
    });
    expect(screen.getByRole("button", { name: /^save$/i })).toBeEnabled();
  });

  // CodeRabbit catch: a product selected before a catalogue refetch removed
  // its entry used to still read as "set" (productId alone), so Save stayed
  // enabled and the raw product UUID fell through into the customer-visible
  // comment instead of a real name. hasProduct is now derived from a live
  // match in the fetched list, so a mid-session refetch that drops the
  // selected product blocks Save instead.
  it("blocks Save, rather than falling back to the raw product id, when the selected product disappears from a refetched catalogue", async () => {
    const onSave = vi.fn();
    const { rerender } = render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        isSaving={false}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(
      screen.getByRole("switch", { name: /share fix eta with customer/i }),
    );
    fireEvent.mouseDown(screen.getByLabelText(/^product/i));
    fireEvent.click(await screen.findByRole("option", { name: "API Manager" }));
    fireEvent.change(screen.getByLabelText(/public ticket/i), {
      target: { value: "https://github.com/wso2/product-apim/issues/1" },
    });
    expect(screen.getByRole("button", { name: /^save$/i })).toBeEnabled();

    // Simulate a refetch, in the same mounted dialog, that drops the
    // previously-selected product ("prod-apim") from the catalogue.
    useSearchProductsMock.mockReturnValue({
      data: [{ id: "prod-is", name: "Identity Server" }],
      isLoading: false,
    });
    rerender(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        isSaving={false}
        onClose={() => {}}
        onSave={onSave}
      />,
    );

    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();
    expect(
      screen.getByText(/product is required to share with the customer/i),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).not.toHaveBeenCalled();
  });

  it("blocks a past fix ETA date and shows an inline error", () => {
    const onSave = vi.fn();
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2020-01-01"
        isSaving={false}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();
    expect(screen.getByText(/this date is in the past/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).not.toHaveBeenCalled();
  });

  it("blocks Save when the three estimates are out of best <= most likely <= worst case order", () => {
    const onSave = vi.fn();
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-18"
        currentMostLikelyFixEta="2099-06-17"
        currentWorstCaseFixEta="2099-06-16"
        isSaving={false}
        onClose={() => {}}
        onSave={onSave}
      />,
    );
    expect(screen.getByRole("button", { name: /^save$/i })).toBeDisabled();
    expect(
      screen.getByText(/best case must not be later than most likely/i),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    expect(onSave).not.toHaveBeenCalled();
  });

  it("allows the estimates in correct best <= most likely <= worst case order", () => {
    render(
      <SetFixEtaDialog
        currentBestCaseFixEta="2099-06-16"
        currentMostLikelyFixEta="2099-06-17"
        currentWorstCaseFixEta="2099-06-18"
        isSaving={false}
        onClose={() => {}}
        onSave={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: /^save$/i })).toBeEnabled();
  });

  // CodeRabbit catch: sharing posts through the same comment endpoint as the
  // case's own public-reply composer, gated by the identical
  // work_in_progress + ongoing + assignee rule -- a share attempt that
  // doesn't satisfy it is guaranteed to fail after the ETA has already
  // saved, so it must be caught here rather than only in the resulting error.
  describe("gated by publicCommentDisabledReason", () => {
    const GATE_REASON =
      "Replies are locked until the case is resumed and assigned to you.";

    it("disables the share toggle and shows the reason when sharing is unavailable", () => {
      render(
        <SetFixEtaDialog
          publicCommentDisabledReason={GATE_REASON}
          currentBestCaseFixEta="2099-06-16"
          isSaving={false}
          onClose={() => {}}
          onSave={() => {}}
        />,
      );
      expect(
        screen.getByRole("switch", { name: /share fix eta with customer/i }),
      ).toBeDisabled();
      expect(screen.getByText(GATE_REASON)).toBeInTheDocument();
      // The ETA itself is unaffected -- Save stays enabled on the seeded estimate.
      expect(screen.getByRole("button", { name: /^save$/i })).toBeEnabled();
    });

    it("leaves the share toggle enabled and shows no reason when sharing is available", () => {
      render(
        <SetFixEtaDialog
          publicCommentDisabledReason={null}
          currentBestCaseFixEta="2099-06-16"
          isSaving={false}
          onClose={() => {}}
          onSave={() => {}}
        />,
      );
      expect(
        screen.getByRole("switch", { name: /share fix eta with customer/i }),
      ).toBeEnabled();
      expect(screen.queryByText(GATE_REASON)).not.toBeInTheDocument();
    });
  });

  it("calls onClose on Close without calling onSave", () => {
    const onSave = vi.fn();
    const onClose = vi.fn();
    render(
      <SetFixEtaDialog isSaving={false} onClose={onClose} onSave={onSave} />,
    );
    fireEvent.click(screen.getByRole("button", { name: /^close$/i }));
    expect(onClose).toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
  });
});
