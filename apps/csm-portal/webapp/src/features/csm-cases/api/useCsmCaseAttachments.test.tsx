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

import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

// `vi.mock` factories are hoisted above top-level `const`s, so anything a
// factory below closes over must itself be created via `vi.hoisted`.
const { postMock, getBlobMock } = vi.hoisted(() => ({
  postMock: vi.fn(),
  getBlobMock: vi.fn(),
}));

// The real client reads runtime config at module load, which isn't present
// under vitest (same approach as useSearchTags.test.tsx).
vi.mock("@api/backend/client", () => ({
  useBackendApi: () => ({ post: postMock, getBlob: getBlobMock }),
}));

vi.mock("@utils/saveBlob", () => ({ saveBlob: vi.fn() }));

import {
  usePostCsmCaseAttachment,
  useDownloadCsmCaseAttachment,
  useGetCsmCaseAttachmentContent,
} from "@features/csm-cases/api/useCsmCaseAttachments";
import { saveBlob } from "@utils/saveBlob";
import type { CaseAttachment } from "@features/csm-cases/types/csmCases";

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

const FILE = new File(["hello world"], "hello.txt", { type: "text/plain" });

describe("usePostCsmCaseAttachment", () => {
  beforeEach(() => {
    postMock.mockReset();
    getBlobMock.mockReset();
  });

  it("sends a single POST /attachments with a base64 file payload", async () => {
    postMock.mockResolvedValue({});
    const { result } = renderHook(() => usePostCsmCaseAttachment(), {
      wrapper,
    });

    await act(() =>
      result.current.mutateAsync({
        caseId: "case-1",
        file: FILE,
        uploadedBy: "Jane Doe",
      }),
    );

    expect(postMock).toHaveBeenCalledTimes(1);
    const [path, payload] = postMock.mock.calls[0];
    expect(path).toBe("/attachments");
    expect(typeof payload.file).toBe("string");
    expect(payload.file.startsWith("data:")).toBe(true);
  });

  it.each(["change_request", "incident"] as const)(
    "referenceType %s: still uses the single base64 path",
    async (referenceType) => {
      postMock.mockResolvedValue({});

      const { result } = renderHook(() => usePostCsmCaseAttachment(), {
        wrapper,
      });

      await act(() =>
        result.current.mutateAsync({
          caseId: "case-1",
          file: FILE,
          uploadedBy: "Jane Doe",
          referenceType,
        }),
      );

      expect(postMock).toHaveBeenCalledTimes(1);
      const [path, payload] = postMock.mock.calls[0];
      expect(path).toBe("/attachments");
      expect(payload.referenceType).toBe(referenceType);
      expect(typeof payload.file).toBe("string");
      expect(payload.file.startsWith("data:")).toBe(true);
    },
  );

  it("rejects a file over the max size before ever calling the API", async () => {
    const oversized = new File(
      [new Uint8Array(10 * 1024 * 1024 + 1)],
      "big.bin",
      { type: "application/octet-stream" },
    );
    const { result } = renderHook(() => usePostCsmCaseAttachment(), {
      wrapper,
    });

    await expect(
      act(() =>
        result.current.mutateAsync({
          caseId: "case-1",
          file: oversized,
          uploadedBy: "Jane Doe",
        }),
      ),
    ).rejects.toThrow(/too large/);

    expect(postMock).not.toHaveBeenCalled();
  });
});

describe("useDownloadCsmCaseAttachment", () => {
  const ATTACHMENT: CaseAttachment = {
    id: "att-1",
    filename: "hello.txt",
    size: 11,
    contentType: "text/plain",
    uploadedBy: "Jane Doe",
    uploadedAt: "2026-01-01T00:00:00Z",
  };

  beforeEach(() => {
    postMock.mockReset();
    getBlobMock.mockReset();
    vi.mocked(saveBlob).mockReset();
  });

  it("fetches the content blob and saves it", async () => {
    getBlobMock.mockResolvedValue(new Blob(["hello"], { type: "text/plain" }));
    const { result } = renderHook(() => useDownloadCsmCaseAttachment(), {
      wrapper,
    });

    await result.current(ATTACHMENT);

    expect(postMock).not.toHaveBeenCalled();
    expect(getBlobMock).toHaveBeenCalledWith("/attachments/att-1/content");
    expect(saveBlob).toHaveBeenCalledTimes(1);
  });
});

describe("useGetCsmCaseAttachmentContent", () => {
  const ATTACHMENT: CaseAttachment = {
    id: "att-1",
    filename: "report.pdf",
    size: 2048,
    contentType: "application/pdf",
    uploadedBy: "Jane Doe",
    uploadedAt: "2026-01-01T00:00:00Z",
  };

  beforeEach(() => {
    postMock.mockReset();
    getBlobMock.mockReset();
  });

  it("fetches the content blob via the authenticated content endpoint", async () => {
    getBlobMock.mockResolvedValue(
      new Blob(["fake"], { type: "application/pdf" }),
    );

    const { result } = renderHook(() => useGetCsmCaseAttachmentContent(), {
      wrapper,
    });

    const blob = await result.current(ATTACHMENT);

    expect(postMock).not.toHaveBeenCalled();
    expect(getBlobMock).toHaveBeenCalledWith("/attachments/att-1/content");
    expect(blob.type).toBe("application/pdf");
  });
});
