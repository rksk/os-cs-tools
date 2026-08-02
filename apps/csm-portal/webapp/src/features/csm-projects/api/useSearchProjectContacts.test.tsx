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

import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const backendPostMock = vi.fn();
vi.mock("@api/backend/client", () => ({
  useBackendApi: () => ({ post: backendPostMock }),
}));

import { useSearchProjectContacts } from "@features/csm-projects/api/useSearchProjectContacts";

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("useSearchProjectContacts", () => {
  beforeEach(() => {
    backendPostMock.mockReset();
  });

  it("stops paging once total is reached", async () => {
    backendPostMock
      .mockResolvedValueOnce({ contacts: [{ name: "A", email: "a@example.com" }], total: 2 })
      .mockResolvedValueOnce({ contacts: [{ name: "B", email: "b@example.com" }], total: 2 });

    const { result } = renderHook(() => useSearchProjectContacts("proj-1"), { wrapper });

    await waitFor(() => expect(result.current.data).toHaveLength(2));
    expect(backendPostMock).toHaveBeenCalledTimes(2);
  });

  it("stops after a bounded number of pages even if total never seems reached", async () => {
    // Regression test: total previously fell back to `all.length` on a
    // missing value, which combined with no independent page cap meant a
    // misbehaving `total` (or a backend that always reports more than it's
    // actually returned) could turn this into an effectively unbounded loop.
    backendPostMock.mockImplementation(() =>
      Promise.resolve({ contacts: [{ name: "X", email: "x@example.com" }], total: 999_999 }),
    );

    const { result } = renderHook(() => useSearchProjectContacts("proj-1"), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true), { timeout: 5000 });
    // Capped well below what `total: 999_999` would otherwise demand.
    expect(backendPostMock.mock.calls.length).toBeLessThanOrEqual(100);
  });

  it("is disabled until a project id is provided", () => {
    const { result } = renderHook(() => useSearchProjectContacts(undefined), { wrapper });
    expect(result.current.fetchStatus).toBe("idle");
    expect(backendPostMock).not.toHaveBeenCalled();
  });
});
