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

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";
import "@testing-library/jest-dom/vitest";
import { MemoryRouter, Route, Routes } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const authFetchMock = vi.fn();
const postMock = vi.fn();

// useSearchUsers (csm-users/api) is on the older useAuthApiClient + apiConfig
// convention; useSearchRoles / useSearchTeams / the group picker's
// useSearchGroups are all on useBackendApi. Both read runtime config at
// module load, which isn't present under vitest, so stub both (same approach
// as useAccountProjects.test.tsx / useQuickCaseSearch.test.tsx).
vi.mock("@config/apiConfig", () => ({
  apiConfig: { backendUrl: "https://example.test" },
}));
vi.mock("@hooks/useAuthApiClient", () => ({
  useAuthApiClient: () => authFetchMock,
}));
vi.mock("@api/backend/client", () => ({
  BackendApiError: class BackendApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
  useBackendApi: () => ({ post: postMock }),
}));

import CsmUsersPage from "@features/csm-users/pages/CsmUsersPage";

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: "OK",
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

function renderPage(initialPath: string): ReturnType<typeof render> {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <CsmUsersPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

/**
 * Same as {@link renderPage}, plus marker routes for the destinations a row
 * or a role chip can navigate to — used to assert *which* route a click
 * actually lands on, not just that some navigation happened.
 */
function renderPageWithDestinations(
  initialPath: string,
): ReturnType<typeof render> {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialPath]}>
        <Routes>
          <Route path="/admin/users" element={<CsmUsersPage />} />
          <Route path="/admin/roles/:id" element={<div>Role members page</div>} />
          <Route path="/people/:id" element={<div>User profile page</div>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("CsmUsersPage", () => {
  beforeEach(() => {
    authFetchMock.mockReset();
    postMock.mockReset();
    postMock.mockImplementation((path: string) => {
      if (path === "/roles/search") {
        return Promise.resolve({
          roles: [{ id: "agent", name: "Agent" }],
          total: 1,
          limit: 50,
          offset: 0,
        });
      }
      if (path === "/teams/search") {
        return Promise.resolve({
          teams: [{ id: "alpha", name: "Alpha" }],
          total: 1,
          limit: 50,
          offset: 0,
        });
      }
      return Promise.resolve({ groups: [], total: 0, limit: 20, offset: 0 });
    });
    authFetchMock.mockResolvedValue(
      jsonResponse({ users: [], total: 0, limit: 20, offset: 0, hasMore: false }),
    );
  });

  it("combines name/email search, role, group, team and status into one request with every key set", async () => {
    renderPage(
      "/admin/users?search=jane&roles=agent&groups=11111111-1111-1111-1111-111111111111&teams=alpha&active=active",
    );

    await waitFor(() => expect(authFetchMock).toHaveBeenCalled());

    // All filters land on a single /users/search call, combined (AND'd)
    // server-side — not split across separate requests per filter.
    expect(authFetchMock).toHaveBeenCalledTimes(1);
    const [url, requestInit] = authFetchMock.mock.calls[0];
    expect(url).toContain("/users/search");
    const body = JSON.parse(requestInit.body as string);
    expect(body.filters).toEqual({
      searchQuery: "jane",
      roleIds: ["agent"],
      groupIds: ["11111111-1111-1111-1111-111111111111"],
      teamIds: ["alpha"],
      active: true,
    });
  });

  it("omits every filter key when nothing is selected", async () => {
    renderPage("/admin/users");

    await waitFor(() => expect(authFetchMock).toHaveBeenCalled());
    const [, requestInit] = authFetchMock.mock.calls[0];
    const body = JSON.parse(requestInit.body as string);
    expect(body.filters).toEqual({});
  });
});

describe("CsmUsersPage — role truncation and row navigation", () => {
  const MANY_ROLES_USER = {
    id: "user-1",
    userName: "jane.doe",
    name: "Jane Doe",
    email: "jane.doe@example.com",
    active: true,
    roles: ["agent", "admin", "commenter", "partner", "customer_admin"],
    createdOn: "2025-01-01T00:00:00Z",
    updatedOn: "2025-06-01T00:00:00Z",
  };
  const FEW_ROLES_USER = {
    id: "user-2",
    userName: "john.smith",
    name: "John Smith",
    email: "john.smith@example.com",
    active: true,
    roles: ["agent", "admin"],
    createdOn: "2025-01-01T00:00:00Z",
    updatedOn: "2025-06-01T00:00:00Z",
  };

  beforeEach(() => {
    authFetchMock.mockReset();
    postMock.mockReset();
    postMock.mockImplementation((path: string) => {
      if (path === "/roles/search") {
        return Promise.resolve({
          roles: [
            { id: "agent", name: "Agent" },
            { id: "admin", name: "Admin" },
            { id: "commenter", name: "Commenter" },
            { id: "partner", name: "Partner" },
            { id: "customer_admin", name: "Customer Admin" },
          ],
          total: 5,
          limit: 50,
          offset: 0,
        });
      }
      if (path === "/teams/search") {
        return Promise.resolve({ teams: [], total: 0, limit: 50, offset: 0 });
      }
      return Promise.resolve({ groups: [], total: 0, limit: 20, offset: 0 });
    });
    authFetchMock.mockResolvedValue(
      jsonResponse({
        users: [MANY_ROLES_USER, FEW_ROLES_USER],
        total: 2,
        limit: 20,
        offset: 0,
      }),
    );
  });

  it("shows a legible overflow chip beyond 3 roles, and no overflow chip at 3 or fewer", async () => {
    renderPage("/admin/users");

    // Both rows carry "Agent"/"Admin" — wait until the role-name catalogue
    // has resolved them from the raw keys ("agent"/"admin") before asserting.
    await waitFor(() => expect(screen.getAllByText("Agent")).toHaveLength(2));

    const janeRow = screen.getByText("Jane Doe").closest("tr") as HTMLElement;
    const johnRow = screen.getByText("John Smith").closest("tr") as HTMLElement;

    // 5 roles: 3 visible + a legible "+2 more" overflow chip, the other 2 roles hidden.
    expect(within(janeRow).getByText("Agent")).toBeInTheDocument();
    expect(within(janeRow).getByText("Admin")).toBeInTheDocument();
    expect(within(janeRow).getByText("Commenter")).toBeInTheDocument();
    expect(within(janeRow).getByText("+2 more")).toBeInTheDocument();
    expect(within(janeRow).queryByText("Partner")).not.toBeInTheDocument();
    expect(within(janeRow).queryByText("Customer Admin")).not.toBeInTheDocument();

    // 2 roles: both visible, no overflow chip for this row.
    expect(within(johnRow).getByText("Agent")).toBeInTheDocument();
    expect(within(johnRow).getByText("Admin")).toBeInTheDocument();
    expect(within(johnRow).queryByText(/more$/)).not.toBeInTheDocument();
  });

  it("navigates a row click to that user's profile, but a nested role chip click to the role page instead", async () => {
    renderPageWithDestinations("/admin/users");

    await waitFor(() => expect(screen.getAllByText("Agent")).toHaveLength(2));
    const janeRow = screen.getByText("Jane Doe").closest("tr") as HTMLElement;

    // Clicking the role chip must land on the role page, not the profile —
    // the row itself is also clickable, so this only holds if the chip stops
    // the click from bubbling up to the row's own handler.
    fireEvent.click(within(janeRow).getByText("Agent"));
    expect(await screen.findByText("Role members page")).toBeInTheDocument();
    expect(screen.queryByText("User profile page")).not.toBeInTheDocument();
  });

  it("navigates a whole-row click (outside any nested chip/link) to the user's profile", async () => {
    renderPageWithDestinations("/admin/users");

    await waitFor(() => expect(screen.getByText("John Smith")).toBeInTheDocument());

    // Click the timezone cell — plain text, not a nested interactive element.
    const row = screen.getByText("John Smith").closest("tr");
    expect(row).not.toBeNull();
    fireEvent.click(row as HTMLElement);
    expect(await screen.findByText("User profile page")).toBeInTheDocument();
  });

  it("is keyboard-activatable: Enter on a focused row navigates to the profile", async () => {
    renderPageWithDestinations("/admin/users");

    await waitFor(() => expect(screen.getByText("John Smith")).toBeInTheDocument());
    const row = screen.getByText("John Smith").closest("tr") as HTMLElement;
    row.focus();
    fireEvent.keyDown(row, { key: "Enter" });
    expect(await screen.findByText("User profile page")).toBeInTheDocument();
  });
});
