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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi, beforeEach } from "vitest";
import "@testing-library/jest-dom/vitest";
import type { ReactNode } from "react";
import { MemoryRouter } from "react-router";

const getMock = vi.fn();
const postMock = vi.fn();

vi.mock("@api/backend/client", () => ({
  useBackendApi: () => ({ get: getMock, post: postMock }),
}));
// A `shape: "list"` tile renders through widgetListConfig.tsx, which pulls in
// useTimeSheets.ts (time_card's mapper) — that module reads `window.config`
// at load via `@config/apiConfig`, unavailable under vitest.
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

import AgentsLandingPagePilot from "@features/csm-dashboard/components/AgentsLandingPagePilot";

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

const DASHBOARD_DETAIL = {
  id: "agents_pilot",
  displayName: "Engineer overview",
  isDefault: true,
  widgets: [
    {
      widgetId: "my_patches",
      displayName: "My Patches",
      resourceType: "case",
      shape: "count",
      gridWidth: 3,
      query: { assignedUserIds: ["user-1"], tags: ["patch"] },
    },
    {
      widgetId: "my_reminders",
      displayName: "My Reminders",
      resourceType: "case",
      shape: "count",
      gridWidth: 3,
      query: { assignedUserIds: ["user-1"], states: ["awaiting_info"] },
    },
    {
      widgetId: "open_incident_team",
      displayName: "Open Incidents (Team)",
      resourceType: "case",
      shape: "count",
      gridWidth: 3,
      query: { tags: ["s_dip"] },
    },
  ],
};

function searchResponseFor(total: number) {
  return { total, cases: [], limit: 1, offset: 0, hasMore: false };
}

describe("AgentsLandingPagePilot", () => {
  beforeEach(() => {
    getMock.mockReset();
    postMock.mockReset();
  });

  it("renders skeleton tiles while the template list is in flight", () => {
    getMock.mockReturnValue(new Promise(() => {}));
    const { container } = renderWithClient(<AgentsLandingPagePilot dashboardId="agents_pilot" />);
    expect(container.querySelectorAll(".MuiSkeleton-root").length).toBe(3);
  });

  it("renders one tile per widget, each resolving its own count independently", async () => {
    getMock.mockResolvedValue(DASHBOARD_DETAIL);
    postMock.mockImplementation((_path: string, body: { filters: Record<string, unknown> }) => {
      if (body.filters.tags && (body.filters.tags as string[]).includes("patch")) {
        return Promise.resolve(searchResponseFor(3));
      }
      if (body.filters.states) {
        return Promise.resolve(searchResponseFor(5));
      }
      return Promise.resolve(searchResponseFor(12));
    });

    renderWithClient(<AgentsLandingPagePilot dashboardId="agents_pilot" />);

    await waitFor(() => expect(screen.getByText("My Patches")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("3")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("5")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("12")).toBeInTheDocument());
    expect(postMock).toHaveBeenCalledTimes(3);
  });

  it("shows an error state when the template list itself fails to load", async () => {
    getMock.mockRejectedValue(new Error("boom"));

    renderWithClient(<AgentsLandingPagePilot dashboardId="agents_pilot" />);

    await waitFor(() =>
      expect(screen.getByText("Could not load the widget pilot.")).toBeInTheDocument(),
    );
    expect(postMock).not.toHaveBeenCalled();
  });

  it("isolates one widget's failed count fetch to its own tile while siblings render their real counts", async () => {
    getMock.mockResolvedValue(DASHBOARD_DETAIL);
    postMock.mockImplementation((_path: string, body: { filters: Record<string, unknown> }) => {
      if (body.filters.tags && (body.filters.tags as string[]).includes("patch")) {
        return Promise.reject(new Error("boom"));
      }
      if (body.filters.states) {
        return Promise.resolve(searchResponseFor(5));
      }
      return Promise.resolve(searchResponseFor(12));
    });

    renderWithClient(<AgentsLandingPagePilot dashboardId="agents_pilot" />);

    await waitFor(() =>
      expect(screen.getByText("Could not load this widget.")).toBeInTheDocument(),
    );
    expect(screen.getByText("My Reminders")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("Open Incidents (Team)")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.queryByText("3")).not.toBeInTheDocument();
  });

  it("re-fetches only that section's own widgets when its refresh button is clicked, without re-pulling the dashboard config", async () => {
    getMock.mockResolvedValue(DASHBOARD_DETAIL);
    postMock.mockResolvedValue(searchResponseFor(3));

    renderWithClient(<AgentsLandingPagePilot dashboardId="agents_pilot" />);
    await waitFor(() => expect(screen.getByText("My Patches")).toBeInTheDocument());
    expect(postMock).toHaveBeenCalledTimes(3);
    expect(getMock).toHaveBeenCalledTimes(1);

    // All three DASHBOARD_DETAIL widgets are unsectioned, so they share the
    // one untitled default group — its refresh button carries a plain
    // "Refresh section" label (no section name to fold into it).
    fireEvent.click(screen.getByRole("button", { name: "Refresh section" }));

    // Every widget in that section re-runs its own /cases/search — 3 more
    // calls on top of the initial 3 — but the dashboard's own metadata GET
    // is not re-pulled (a section refresh is data-only).
    await waitFor(() => expect(postMock).toHaveBeenCalledTimes(6));
    expect(getMock).toHaveBeenCalledTimes(1);
    expect(screen.getByText("My Patches")).toBeInTheDocument();
  });

  it("groups widgets sharing a `section` under a titled heading, separate from unsectioned widgets", async () => {
    getMock.mockResolvedValue({
      id: "team_performance",
      displayName: "Team performance",
      isDefault: false,
      widgets: [
        {
          widgetId: "team_open_cases",
          displayName: "Team Open P0/P1",
          resourceType: "case",
          shape: "count",
          gridWidth: 6,
          query: {},
        },
        {
          widgetId: "incident_wow",
          displayName: "Incident WOW",
          section: "SLA Violation",
          resourceType: "case",
          shape: "count",
          gridWidth: 6,
          query: {},
        },
        {
          widgetId: "query_wow",
          displayName: "Query WOW",
          section: "SLA Violation",
          resourceType: "case",
          shape: "count",
          gridWidth: 6,
          query: {},
        },
      ],
    });
    postMock.mockResolvedValue(searchResponseFor(3));

    renderWithClient(<AgentsLandingPagePilot dashboardId="team_performance" />);

    await waitFor(() => expect(screen.getByText("Team Open P0/P1")).toBeInTheDocument());
    expect(screen.getByText("Incident WOW")).toBeInTheDocument();
    expect(screen.getByText("Query WOW")).toBeInTheDocument();

    // Exactly one "SLA Violation" heading — both of its widgets share the
    // section, they don't each get their own repeated heading.
    expect(screen.getAllByText("SLA Violation")).toHaveLength(1);
  });

  it("resolves the {{currentTeam}} text token in a section heading, same as a widget's own displayName", async () => {
    getMock.mockResolvedValue({
      id: "team_performance",
      displayName: "Team performance",
      isDefault: false,
      widgets: [
        {
          widgetId: "team_open_cases",
          displayName: "Team Open P0/P1",
          section: "Overall - {{currentTeam}}",
          resourceType: "case",
          shape: "count",
          gridWidth: 6,
          query: {},
        },
      ],
    });
    postMock.mockResolvedValue(searchResponseFor(1));

    renderWithClient(
      <AgentsLandingPagePilot dashboardId="team_performance" selectedTeamLabel="Castor" />,
    );

    await waitFor(() => expect(screen.getByText("Team Open P0/P1")).toBeInTheDocument());
    expect(screen.getByText("Overall - Castor")).toBeInTheDocument();
    expect(screen.queryByText(/{{currentTeam}}/)).not.toBeInTheDocument();
  });
});
