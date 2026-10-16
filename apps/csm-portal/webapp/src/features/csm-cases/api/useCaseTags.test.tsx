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

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi, beforeEach } from "vitest";

const postMock = vi.fn();
const useBackendApiMock = vi.fn((): { post: typeof postMock; del?: ReturnType<typeof vi.fn> } => ({ post: postMock }));
vi.mock("@api/backend/client", () => ({
  useBackendApi: () => useBackendApiMock(),
}));

// Imported after the mock above so the module picks it up.
import { useAddCaseTag, useAddTagToCase, useRemoveCaseTag } from "@features/csm-cases/api/useCaseTags";

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

// The hooks take an optional detail query key: a change request is tagged
// through the same routes (they take a work item id), and its page passes its
// own detail key so THAT detail refetches, not a case detail's.
describe("useAddCaseTag / useRemoveCaseTag — the detail query they invalidate", () => {
  const delMock = vi.fn();
  beforeEach(() => {
    postMock.mockReset();
    delMock.mockReset();
  });

  function clientWrapper(queryClient: QueryClient) {
    return function Wrapper({ children }: { children: ReactNode }) {
      return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    };
  }

  it("invalidates the case detail by default", async () => {
    postMock.mockResolvedValue({ id: "tag-1", label: "apim-upgrade" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    const { result } = renderHook(() => useAddCaseTag("case-a"), { wrapper: clientWrapper(queryClient) });
    await result.current.mutateAsync("apim-upgrade");

    expect(postMock).toHaveBeenCalledWith("/cases/case-a/tags", { label: "apim-upgrade" });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["csm-case-detail", "case-a"] });
  });

  it("invalidates the given detail key instead, for a change request tagged through the same route", async () => {
    postMock.mockResolvedValue({ id: "tag-1", label: "apim-upgrade" });
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    const { result } = renderHook(() => useAddCaseTag("chg-1", ["change-request-details", "chg-1"]), {
      wrapper: clientWrapper(queryClient),
    });
    await result.current.mutateAsync("apim-upgrade");

    expect(postMock).toHaveBeenCalledWith("/cases/chg-1/tags", { label: "apim-upgrade" });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["change-request-details", "chg-1"] });
    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: ["csm-case-detail", "chg-1"] });
  });

  it("removes through the same route and invalidates the given detail key", async () => {
    vi.mocked(useBackendApiMock).mockReturnValue({ post: postMock, del: delMock });
    delMock.mockResolvedValue(undefined);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");

    const { result } = renderHook(() => useRemoveCaseTag("chg-1", ["change-request-details", "chg-1"]), {
      wrapper: clientWrapper(queryClient),
    });
    await result.current.mutateAsync("tag-9");

    expect(delMock).toHaveBeenCalledWith("/cases/chg-1/tags/tag-9");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["change-request-details", "chg-1"] });
  });
});

describe("useAddTagToCase", () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  it("posts the label to the given case's own tags endpoint, not a fixed one", async () => {
    postMock.mockResolvedValue({ id: "tag-1", label: "Security Announcement", color: null });

    const { result } = renderHook(() => useAddTagToCase(), { wrapper });
    await result.current.mutateAsync({ caseId: "case-a", label: "Security Announcement" });
    await result.current.mutateAsync({ caseId: "case-b", label: "Security Announcement" });

    expect(postMock).toHaveBeenNthCalledWith(1, "/cases/case-a/tags", {
      label: "Security Announcement",
    });
    expect(postMock).toHaveBeenNthCalledWith(2, "/cases/case-b/tags", {
      label: "Security Announcement",
    });
  });

  it("rejects when the backend call fails, so a caller can distinguish it from a successful create", async () => {
    postMock.mockRejectedValue(new Error("network down"));

    const { result } = renderHook(() => useAddTagToCase(), { wrapper });
    await expect(
      result.current.mutateAsync({ caseId: "case-a", label: "Security Announcement" }),
    ).rejects.toThrow("network down");
    await waitFor(() => expect(result.current.isError).toBe(true));
  });
});
