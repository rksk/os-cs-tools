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
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

const fetchMock = vi.fn();

vi.mock("@config/apiConfig", () => ({
  apiConfig: { backendUrl: "https://example.test" },
}));

vi.mock("@hooks/useAuthApiClient", () => ({
  useAuthApiClient: () => fetchMock,
}));

import { useGetUsersMe } from "@features/settings/api/useGetUsersMe";

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe("useGetUsersMe", () => {
  beforeEach(() => {
    fetchMock.mockReset();
  });

  it("surfaces the response body's fields", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          email: "jane.doe@example.com",
          id: "00000000-0000-0000-0000-000000000000",
        }),
        { status: 200 },
      ),
    );

    const { result } = renderHook(() => useGetUsersMe(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.email).toBe("jane.doe@example.com");
    expect(result.current.data?.id).toBe(
      "00000000-0000-0000-0000-000000000000",
    );
  });

  it("throws an ApiError on a non-ok response", async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ message: "Unauthorized" }), {
        status: 401,
        statusText: "Unauthorized",
      }),
    );

    const { result } = renderHook(() => useGetUsersMe(), { wrapper });
    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(Error);
  });
});
