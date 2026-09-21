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

const { postMock, getBlobMock } = vi.hoisted(() => ({
  postMock: vi.fn(),
  getBlobMock: vi.fn(),
}));

vi.mock("@api/backend/client", () => ({
  useBackendApi: () => ({ post: postMock, getBlob: getBlobMock }),
}));

import { useResolvedInlineImageHtml } from "@features/csm-cases/api/useResolvedInlineImageHtml";

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

const ATTACHMENT_SYSID = "0123456789abcdef0123456789abcdef";
const HTML = `<p>see <img src="/inline/${ATTACHMENT_SYSID}.iix"></p>`;

describe("useResolvedInlineImageHtml", () => {
  beforeEach(() => {
    postMock.mockReset();
    getBlobMock.mockReset();
  });

  it("resolves via GET /attachments/{id}/content into a data: URL", async () => {
    getBlobMock.mockResolvedValue(new Blob(["fake"], { type: "image/png" }));

    const { result } = renderHook(() => useResolvedInlineImageHtml(HTML), {
      wrapper,
    });

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(postMock).not.toHaveBeenCalled();
    expect(getBlobMock).toHaveBeenCalledTimes(1);
    const [calledPath] = getBlobMock.mock.calls[0];
    expect(calledPath).toContain("/content");
    expect(result.current.resolvedHtml).toContain("data:image/png;base64,");
  });

  it("does not resolve anything when the HTML has no .iix references", () => {
    const { result } = renderHook(
      () => useResolvedInlineImageHtml("<p>no images here</p>"),
      { wrapper },
    );

    expect(postMock).not.toHaveBeenCalled();
    expect(getBlobMock).not.toHaveBeenCalled();
    expect(result.current.isLoading).toBe(false);
  });
});
