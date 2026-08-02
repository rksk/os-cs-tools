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

import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import ErrorBanner from "@components/error-banner/ErrorBanner";

describe("ErrorBanner", () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("renders the message with no copy button when no reference ID is supplied", () => {
    render(<ErrorBanner message="Something went wrong." onClose={vi.fn()} />);

    expect(screen.getByText("Something went wrong.")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /copy reference id/i }),
    ).not.toBeInTheDocument();
  });

  it("copies the reference ID to the clipboard and shows a transient confirmation, without folding it into the message text", async () => {
    render(
      <ErrorBanner
        message="Failed to save the change."
        referenceId="c3f1a9e2-1234-4abc-8def-0123456789ab"
        onClose={vi.fn()}
      />,
    );

    // The id is not appended to the visible message string.
    expect(screen.getByText("Failed to save the change.")).toBeInTheDocument();
    expect(
      screen.queryByText(/c3f1a9e2-1234-4abc-8def-0123456789ab/),
    ).not.toBeInTheDocument();

    // The aria-live status is empty before any copy attempt.
    expect(screen.queryByText("Reference ID copied.")).not.toBeInTheDocument();

    const button = screen.getByRole("button", { name: /copy reference id/i });
    fireEvent.click(button);

    await waitFor(() => {
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
        "c3f1a9e2-1234-4abc-8def-0123456789ab",
      );
    });

    // The icon swap alone isn't a completion signal for assistive tech --
    // this live-region text is. Assert it appears, not just that the button's
    // own (unchanged) aria-label is still present.
    await waitFor(() => {
      expect(screen.getByText("Reference ID copied.")).toBeInTheDocument();
    });

    // ...and clears again after the transient-confirmation window.
    await waitFor(
      () => {
        expect(screen.queryByText("Reference ID copied.")).not.toBeInTheDocument();
      },
      { timeout: 3000 },
    );
  });

  it("still renders a close button when a copy button is also present", () => {
    // MUI's Alert only auto-renders its own close (X) button when no custom
    // `action` is supplied; supplying one (the copy button) for the tracking
    // id must not silently drop the close control.
    const onClose = vi.fn();
    render(
      <ErrorBanner
        message="Something went wrong."
        referenceId="c3f1a9e2-1234-4abc-8def-0123456789ab"
        onClose={onClose}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("calls onClose when there is no reference ID too", () => {
    const onClose = vi.fn();
    render(<ErrorBanner message="Something went wrong." onClose={onClose} />);

    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalled();
  });
});
