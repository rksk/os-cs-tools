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
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";

// QueryErrorState imports BackendApiError from @api/backend/client, which
// reads window.config at module load -- not present under Vitest (same
// convention as every other test that transitively reaches this module; see
// this app's own CLAUDE.md "Testing" section).
vi.mock("@api/backend/client", () => ({
  useBackendApi: () => ({ get: vi.fn(), post: vi.fn() }),
  BackendApiError: class BackendApiError extends Error {},
}));
vi.mock("@config/apiConfig", () => ({ apiConfig: { backendUrl: "https://example.test" } }));

import ProductVulnerabilitiesTab from "./ProductVulnerabilitiesTab";

const searchResult = {
  data: { productVulnerabilities: [], total: 0, limit: 20, offset: 0 },
  isLoading: false,
  isError: false,
  error: null,
  isFetching: false,
  refetch: vi.fn(),
  dataUpdatedAt: 0,
};

vi.mock(
  "@features/csm-security-center/api/useSearchProductVulnerabilities",
  () => ({
    useSearchProductVulnerabilities: () => searchResult,
  }),
);

const metaMock = vi.fn();
vi.mock("@features/csm-security-center/api/useGetVulnerabilityMeta", () => ({
  useGetVulnerabilityMeta: () => metaMock(),
}));

function renderTab(): ReturnType<typeof render> {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ProductVulnerabilitiesTab />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("ProductVulnerabilitiesTab — Priority filter", () => {
  it("renders the fetched severity labels, not the static display map, when meta has loaded", () => {
    metaMock.mockReturnValue({
      data: {
        severities: [
          { id: "4", label: "Catastrophic" },
          { id: "3", label: "High" },
        ],
      },
      isLoading: false,
    });
    renderTab();

    fireEvent.mouseDown(screen.getByLabelText(/filter by priority/i));
    const listbox = screen.getByRole("listbox");
    expect(within(listbox).getByText("Catastrophic")).toBeInTheDocument();
    expect(within(listbox).getByText("High")).toBeInTheDocument();
    // Medium/Low/Info/Unknown were never returned by meta, so they must not render.
    expect(within(listbox).queryByText("Medium")).not.toBeInTheDocument();
  });

  it("falls back to the static priority list while meta is loading", () => {
    metaMock.mockReturnValue({ data: undefined, isLoading: true });
    renderTab();

    fireEvent.mouseDown(screen.getByLabelText(/filter by priority/i));
    const listbox = screen.getByRole("listbox");
    expect(within(listbox).getByText("Critical")).toBeInTheDocument();
    expect(within(listbox).getByText("Unknown")).toBeInTheDocument();
  });
});
