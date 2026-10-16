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

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import "@testing-library/jest-dom/vitest";
import ResolvedAudienceList from "@features/csm-announcements/components/ResolvedAudienceList";

const PROJECTS = [
  { id: "p1", name: "Project One", key: "ONE", accountName: "Acme" },
  { id: "p2", name: "Project Two", key: "TWO", accountName: null },
];

describe("ResolvedAudienceList", () => {
  it("shows the blank spinner state while the first page is still loading", () => {
    render(<ResolvedAudienceList projects={[]} total={0} isLoading isError={false} />);

    expect(screen.getByText("Resolving audience…")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /export csv/i })).toBeDisabled();
  });

  it("shows the rows already loaded, with progress and a loading row, while more pages arrive", () => {
    render(
      <ResolvedAudienceList
        projects={PROJECTS}
        total={2}
        isLoading
        isLoadingMore
        expectedTotal={5}
        isError={false}
      />,
    );

    expect(screen.getByText("Project One — Acme")).toBeInTheDocument();
    expect(screen.getByText("TWO")).toBeInTheDocument();
    expect(screen.getByText(/of 5 projects so far/)).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Loading remaining projects…");
    expect(screen.queryByText("Resolving audience…")).not.toBeInTheDocument();
    // A partial file would read as the whole audience.
    expect(screen.getByRole("button", { name: /export csv/i })).toBeDisabled();
    // The bar under the header tracks loaded / expected (2 of 5), so progress
    // is visible without scrolling the list to its loading row.
    const bars = screen.getAllByRole("progressbar", { hidden: true });
    expect(bars.some((bar) => bar.getAttribute("aria-valuenow") === "40")).toBe(true);
  });

  it("words the progress without a denominator when the server total is unknown", () => {
    render(
      <ResolvedAudienceList
        projects={PROJECTS}
        total={2}
        isLoading
        isLoadingMore
        expectedTotal={null}
        isError={false}
      />,
    );

    expect(screen.getByText(/projects so far/)).toBeInTheDocument();
    expect(screen.queryByText(/ of /)).not.toBeInTheDocument();
  });

  it("reports a definitive match count and enables export once loading finishes", () => {
    render(<ResolvedAudienceList projects={PROJECTS} total={2} isLoading={false} isError={false} />);

    expect(screen.getByText(/projects match this scope/)).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryAllByRole("progressbar", { hidden: true })).toHaveLength(0);
    expect(screen.getByRole("button", { name: /export csv/i })).toBeEnabled();
  });

  it("hides the rows and says so when resolution failed", () => {
    render(<ResolvedAudienceList projects={[]} total={0} isLoading={false} isError />);

    expect(screen.getByText("Couldn't resolve the audience. Try again.")).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("leaves a source that never reports partial progress unchanged: a spinner, not rows", () => {
    // The product-version audience and the publish dialog's preview only have
    // their rows once they have all of them, and never pass isLoadingMore.
    render(<ResolvedAudienceList projects={PROJECTS} total={2} isLoading isError={false} />);

    expect(screen.getByText("Resolving audience…")).toBeInTheDocument();
    expect(screen.queryByText("Project One — Acme")).not.toBeInTheDocument();
  });
});
