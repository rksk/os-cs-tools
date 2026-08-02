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

import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi, beforeEach } from "vitest";
import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";

const postMock = vi.fn();

vi.mock("@api/backend/client", () => ({
  useBackendApi: () => ({ post: postMock }),
}));
// A `shape: "list"` tile now renders through widgetListConfig.tsx, which
// pulls in useTimeSheets.ts (time_card's mapper) — that module reads
// `window.config` at load via `@config/apiConfig`, unavailable under vitest.
vi.mock("@config/apiConfig", () => ({
  apiConfig: { backendUrl: "https://example.test" },
}));
vi.mock("@context/current-user/CurrentUserContext", () => ({
  useCurrentUser: () => ({
    user: { id: "11111111-aaaa-bbbb-cccc-000000000001" },
    isLoading: false,
    isError: false,
  }),
}));

import DashboardWidgetTile from "@features/csm-dashboard/components/DashboardWidgetTile";

function renderWithClient(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("DashboardWidgetTile", () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  it("renders a skeleton while its own count is in flight", () => {
    postMock.mockReturnValue(new Promise(() => {}));
    const { container } = renderWithClient(
      <DashboardWidgetTile
        widgetId="my_patches"
        displayName="My Patches"
        resourceType="case"
        shape="count"
        filters={{}}
      />,
    );
    expect(container.querySelectorAll(".MuiSkeleton-root").length).toBe(1);
  });

  it("renders the resolved count once its own /cases/search call succeeds", async () => {
    postMock.mockResolvedValue({ total: 3, cases: [], limit: 1, offset: 0, hasMore: false });

    renderWithClient(
      <DashboardWidgetTile
        widgetId="my_patches"
        displayName="My Patches"
        resourceType="case"
        shape="count"
        filters={{}}
      />,
    );

    await waitFor(() => expect(screen.getByText("3")).toBeInTheDocument());
    expect(screen.getByText("My Patches")).toBeInTheDocument();
    expect(postMock).toHaveBeenCalledWith("/cases/search", {
      filters: {},
      pagination: { offset: 0, limit: 1 },
    });
  });

  it("renders its own error state when its /cases/search call fails", async () => {
    postMock.mockRejectedValue(new Error("boom"));

    renderWithClient(
      <DashboardWidgetTile
        widgetId="my_patches"
        displayName="My Patches"
        resourceType="case"
        shape="count"
        filters={{}}
      />,
    );

    await waitFor(() =>
      expect(screen.getByText("Could not load this widget.")).toBeInTheDocument(),
    );
  });

  it("renders the same table the Cases tab uses for shape: list, capped at listLimit", async () => {
    postMock.mockResolvedValue({
      total: 2,
      cases: [
        { id: "11111111-1111-1111-1111-111111111111", number: "CS-1", subject: "Disk full", state: "open" },
        {
          id: "22222222-2222-2222-2222-222222222222",
          number: "CS-2",
          subject: "Auth failing",
          state: "work_in_progress",
        },
      ],
      limit: 5,
      offset: 0,
      hasMore: false,
    });

    renderWithClient(
      <DashboardWidgetTile
        widgetId="my_critical_open"
        displayName="My Critical & High Cases"
        resourceType="case"
        shape="list"
        filters={{}}
        listLimit={5}
      />,
    );

    await waitFor(() => expect(screen.getByText("CS-1")).toBeInTheDocument());
    expect(screen.getByText("Disk full")).toBeInTheDocument();
    expect(screen.getByText("CS-2")).toBeInTheDocument();
    expect(screen.getByText("Auth failing")).toBeInTheDocument();
    expect(postMock).toHaveBeenCalledWith("/cases/search", {
      filters: {},
      pagination: { offset: 0, limit: 5 },
    });
  });

  it("shows a 'View more' link through to the full tab only when more records exist than shown", async () => {
    postMock.mockResolvedValue({
      total: 1,
      cases: [{ id: "11111111-1111-1111-1111-111111111111", number: "CS-1", subject: "Disk full", state: "open" }],
      limit: 5,
      offset: 0,
      hasMore: false,
    });

    renderWithClient(
      <DashboardWidgetTile
        widgetId="my_critical_open"
        displayName="My Critical & High Cases"
        resourceType="case"
        shape="list"
        filters={{}}
        listLimit={5}
      />,
    );

    await waitFor(() => expect(screen.getByText("CS-1")).toBeInTheDocument());
    expect(screen.queryByRole("link", { name: /view more/i })).not.toBeInTheDocument();

    postMock.mockResolvedValue({
      total: 6,
      cases: [{ id: "11111111-1111-1111-1111-111111111111", number: "CS-1", subject: "Disk full", state: "open" }],
      limit: 5,
      offset: 0,
      hasMore: true,
    });

    renderWithClient(
      <DashboardWidgetTile
        widgetId="my_critical_open_2"
        displayName="My Critical & High Cases"
        resourceType="case"
        shape="list"
        filters={{}}
        listLimit={5}
      />,
    );

    const viewMoreLink = await screen.findByRole("link", { name: /view more/i });
    const href = viewMoreLink.getAttribute("href") ?? "";
    // Goes to the widget's own preview page (real, bookmarkable URL — see
    // widgetPreviewUrl.ts), not straight to the resource's own tab.
    expect(href.startsWith("/dashboard/cases?")).toBe(true);
    const params = new URLSearchParams(href.split("?")[1]);
    expect(params.get("w")).toBe("my_critical_open_2");
    expect(params.get("n")).toBe("My Critical & High Cases");
    expect(params.get("f")).toBeNull();
  });

  it("masks the signed-in user's own id in the 'View more' link's filter query params", async () => {
    postMock.mockResolvedValue({
      total: 6,
      cases: [{ id: "11111111-1111-1111-1111-111111111111", number: "CS-1", subject: "Disk full", state: "open" }],
      limit: 5,
      offset: 0,
      hasMore: true,
    });

    renderWithClient(
      <DashboardWidgetTile
        widgetId="my_cases"
        displayName="My Cases"
        resourceType="case"
        shape="list"
        // "11111111-aaaa-bbbb-cccc-000000000001" is the mocked signed-in
        // user's own id (see the CurrentUserContext mock above) — it must
        // never appear verbatim in the resulting URL.
        filters={{ assignedUserIds: ["11111111-aaaa-bbbb-cccc-000000000001"] }}
        listLimit={5}
      />,
    );

    const viewMoreLink = await screen.findByRole("link", { name: /view more/i });
    const href = viewMoreLink.getAttribute("href") ?? "";
    expect(href).not.toContain("11111111-aaaa-bbbb-cccc-000000000001");
    const params = new URLSearchParams(href.split("?")[1]);
    expect(params.get("assignedUserIds")).toBe("@me");
  });

  it("navigates to /cases with translated filters when a case-resource tile is clicked", async () => {
    postMock.mockResolvedValue({ total: 3, cases: [], limit: 1, offset: 0, hasMore: false });

    renderWithClient(
      <DashboardWidgetTile
        widgetId="my_patches"
        displayName="My Patches"
        resourceType="case"
        shape="count"
        filters={{ severities: ["critical"], states: ["open"] }}
      />,
    );

    await waitFor(() => expect(screen.getByText("3")).toBeInTheDocument());

    const link = screen.getByRole("link");
    const href = link.getAttribute("href") ?? "";
    expect(href.startsWith("/cases?")).toBe(true);
    const params = new URLSearchParams(href.split("?")[1]);
    expect(params.get("severities")).toBe("S1");
    expect(params.get("states")).toBe("open");
  });

  it("renders a not-yet-supported message for shape pie/bar instead of crashing", async () => {
    postMock.mockResolvedValue({ total: 0, cases: [], limit: 1, offset: 0, hasMore: false });

    renderWithClient(
      <DashboardWidgetTile
        widgetId="cases_by_severity"
        displayName="Open Cases by Severity"
        resourceType="case"
        shape="bar"
        filters={{}}
      />,
    );

    await waitFor(() =>
      expect(screen.getByText("Not yet supported.")).toBeInTheDocument(),
    );
  });

  it("renders an unsupported-widget message instead of crashing for an unrecognized resourceType", () => {
    renderWithClient(
      <DashboardWidgetTile
        widgetId="mystery_widget"
        displayName="Mystery Widget"
        // Simulates a resourceType the backend registry knows about (now
        // runtime JSON config, not compile-time checked) but this frontend
        // build doesn't yet have an entry for in WIDGET_RESOURCE_CONFIG.
        resourceType={"future_resource" as unknown as never}
        shape="count"
        filters={{}}
      />,
    );

    expect(screen.getByText("Mystery Widget")).toBeInTheDocument();
    expect(screen.getByText("Unsupported widget type.")).toBeInTheDocument();
    expect(postMock).not.toHaveBeenCalled();
  });
});
