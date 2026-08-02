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
import { describe, expect, it, vi, beforeEach } from "vitest";
import "@testing-library/jest-dom/vitest";
import type { ComponentProps } from "react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const authFetchMock = vi.fn();

// useSearchUsers (csm-users/api) reads runtime config via @config/apiConfig
// and the real Asgardeo-backed useAuthApiClient at module load — neither is
// present under vitest, so stub both (same approach as
// useAccountProjects.test.tsx). authFetchMock stands in for the fetch
// wrapper the hook actually calls.
vi.mock("@config/apiConfig", () => ({
  apiConfig: { backendUrl: "https://example.test" },
}));
vi.mock("@hooks/useAuthApiClient", () => ({
  useAuthApiClient: () => authFetchMock,
}));
// UserRefLink resolves an unknown id via POST /users/search through
// useBackendApi; every row here already carries an id, so resolution is never
// triggered. useSearchRoles (for role display names) goes through this same
// client, so `post` needs a real implementation -- an empty vi.fn() resolves
// to undefined, which react-query rejects ("Query data cannot be undefined").
const backendPostMock = vi.fn();
vi.mock("@api/backend/client", () => ({
  BackendApiError: class BackendApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
  useBackendApi: () => ({ post: backendPostMock }),
}));

import DirectoryMembersList from "@features/csm-admin/components/DirectoryMembersList";

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: "OK",
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

function renderList(
  props: Partial<ComponentProps<typeof DirectoryMembersList>> = {},
): ReturnType<typeof render> {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <DirectoryMembersList
          filterKey="roleIds"
          entityId="agent"
          entityNoun="role"
          {...props}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const MEMBER = {
  id: "00000000-0000-0000-0000-000000000000",
  userName: "jane.doe",
  name: "Jane Doe",
  email: "jane.doe@example.com",
  active: true,
  createdOn: "2026-01-01T00:00:00Z",
  updatedOn: "2026-01-01T00:00:00Z",
  roles: ["agent"],
};

const ROLES_RESPONSE = {
  roles: [
    { id: "agent", name: "Agent" },
    { id: "admin", name: "Admin" },
    { id: "commenter", name: "Commenter" },
    { id: "customer", name: "Customer" },
    { id: "partner", name: "Partner" },
  ],
  total: 5,
  limit: 50,
  offset: 0,
};

describe("DirectoryMembersList", () => {
  beforeEach(() => {
    authFetchMock.mockReset();
    backendPostMock.mockReset();
    backendPostMock.mockResolvedValue(ROLES_RESPONSE);
  });

  it("sends roleIds (not groupIds/teamIds) for a role's member page", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ users: [MEMBER], total: 1, limit: 20, offset: 0 }),
    );
    renderList({ filterKey: "roleIds", entityId: "agent", entityNoun: "role" });

    await waitFor(() => expect(authFetchMock).toHaveBeenCalled());
    const [, requestInit] = authFetchMock.mock.calls[0];
    const body = JSON.parse(requestInit.body as string);
    expect(body.filters).toEqual({ roleIds: ["agent"] });
  });

  it("sends groupIds (not roleIds/teamIds) for a group's member page", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ users: [MEMBER], total: 1, limit: 20, offset: 0 }),
    );
    renderList({
      filterKey: "groupIds",
      entityId: "11111111-1111-1111-1111-111111111111",
      entityNoun: "group",
    });

    await waitFor(() => expect(authFetchMock).toHaveBeenCalled());
    const [, requestInit] = authFetchMock.mock.calls[0];
    const body = JSON.parse(requestInit.body as string);
    expect(body.filters).toEqual({ groupIds: ["11111111-1111-1111-1111-111111111111"] });
  });

  it("sends teamIds (not roleIds/groupIds) for a team's member page, using the registry key as-is", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ users: [MEMBER], total: 1, limit: 20, offset: 0 }),
    );
    renderList({ filterKey: "teamIds", entityId: "alpha", entityNoun: "team" });

    await waitFor(() => expect(authFetchMock).toHaveBeenCalled());
    const [, requestInit] = authFetchMock.mock.calls[0];
    const body = JSON.parse(requestInit.body as string);
    // The team id travels through untouched — it's a registry key, not a
    // UUID, and nothing here should coerce or reformat it.
    expect(body.filters).toEqual({ teamIds: ["alpha"] });
  });

  it("renders a member row linking to the person's profile via UserRefLink", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ users: [MEMBER], total: 1, limit: 20, offset: 0 }),
    );
    renderList();

    const link = await screen.findByRole("link", { name: "jane.doe" });
    expect(link).toHaveAttribute("href", `/people/${MEMBER.id}`);
  });

  it("renders an empty state (not an error) when the filter matches nobody", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ users: [], total: 0, limit: 20, offset: 0 }),
    );
    renderList({ filterKey: "teamIds", entityId: "beta", entityNoun: "team" });

    expect(await screen.findByText(/No members found for this team/i)).toBeInTheDocument();
    expect(screen.queryByText(/Failed to load members/i)).not.toBeInTheDocument();
  });

  it("truncates a member's roles beyond 3 with a '+N more' chip, same as the user list", async () => {
    // Regression test: role truncation was implemented directly in
    // CsmUsersPage's own row rendering, not as a shared component, so every
    // role/group/team member-list page rendered every role uncapped despite
    // the user list itself being fixed.
    const memberWithManyRoles = {
      ...MEMBER,
      roles: ["agent", "admin", "commenter", "customer", "partner"],
    };
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ users: [memberWithManyRoles], total: 1, limit: 20, offset: 0 }),
    );
    renderList();

    // Names come from the roles catalogue lookup, proving it's wired, not
    // just raw ids threaded through unchanged.
    expect(await screen.findByText("Agent")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("Commenter")).toBeInTheDocument();
    expect(screen.queryByText("Customer")).not.toBeInTheDocument();
    expect(screen.queryByText("Partner")).not.toBeInTheDocument();
    expect(screen.getByText("+2 more")).toBeInTheDocument();
  });

  it("does not show a '+N more' chip at 3 roles or fewer", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ users: [MEMBER], total: 1, limit: 20, offset: 0 }),
    );
    renderList();

    expect(await screen.findByText("Agent")).toBeInTheDocument();
    expect(screen.queryByText(/more$/)).not.toBeInTheDocument();
  });

  it("renders an error state when the search fails", async () => {
    authFetchMock.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
      text: async () => "",
    } as unknown as Response);
    renderList();

    expect(await screen.findByText("Internal Server Error")).toBeInTheDocument();
  });
});
