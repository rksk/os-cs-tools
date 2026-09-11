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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { uploadFileViaTus } from "@features/csm-cases/api/attachmentStorageTus";

/** Minimal fake XMLHttpRequest that records the PATCH call and fires progress/load. */
class FakeXhr {
  static instances: FakeXhr[] = [];
  method = "";
  url = "";
  headers: Record<string, string> = {};
  upload = { onprogress: null as ((e: ProgressEvent) => void) | null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  status = 200;
  sentBody: unknown = null;

  constructor() {
    FakeXhr.instances.push(this);
  }
  open(method: string, url: string): void {
    this.method = method;
    this.url = url;
  }
  setRequestHeader(key: string, value: string): void {
    this.headers[key] = value;
  }
  send(body: unknown): void {
    this.sentBody = body;
    this.upload.onprogress?.({
      lengthComputable: true,
      loaded: 5,
      total: 10,
    } as ProgressEvent);
    this.upload.onprogress?.({
      lengthComputable: true,
      loaded: 10,
      total: 10,
    } as ProgressEvent);
    this.onload?.();
  }
  abort(): void {
    this.onabort?.();
  }
}

describe("uploadFileViaTus", () => {
  const file = new File(["hello world"], "hello.txt", { type: "text/plain" });

  beforeEach(() => {
    FakeXhr.instances = [];
    vi.stubGlobal(
      "XMLHttpRequest",
      FakeXhr as unknown as typeof XMLHttpRequest,
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("POSTs to open the upload session, then PATCHes the bytes to the returned Location", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(null, {
        status: 201,
        headers: { Location: "/api/v2/shares-chunked-uploads/abc123" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const onProgress = vi.fn();
    await uploadFileViaTus({
      sftpgoBaseUrl: "https://sftpgo.example.com",
      shareId: "share-1",
      storageKey: "/attachments/cases/case-1/att-1",
      file,
      onProgress,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [createUrl, createInit] = fetchMock.mock.calls[0];
    expect(createUrl).toBe(
      "https://sftpgo.example.com/api/v2/shares-chunked-uploads",
    );
    expect(createInit.method).toBe("POST");
    expect(createInit.headers.Authorization).toBeUndefined();
    expect(createInit.headers["Upload-Length"]).toBe(String(file.size));
    // Upload-Metadata carries the base64-encoded final path segment,
    // share id, and mkdir_parents flag.
    const metadata = createInit.headers["Upload-Metadata"] as string;
    const entries = Object.fromEntries(
      metadata.split(",").map((entry: string) => {
        const [key, value] = entry.split(" ");
        return [key, atob(value)];
      }),
    );
    expect(entries.path).toBe("att-1");
    expect(entries.share_id).toBe("share-1");
    expect(entries.mkdir_parents).toBe("true");

    expect(FakeXhr.instances).toHaveLength(1);
    const xhr = FakeXhr.instances[0];
    expect(xhr.method).toBe("PATCH");
    expect(xhr.url).toBe(
      "https://sftpgo.example.com/api/v2/shares-chunked-uploads/abc123",
    );
    expect(xhr.headers.Authorization).toBeUndefined();
    expect(xhr.headers["Upload-Offset"]).toBe("0");
    expect(xhr.headers["Content-Type"]).toBe("application/offset+octet-stream");
    expect(xhr.sentBody).toBe(file);

    expect(onProgress).toHaveBeenCalledWith(50);
    expect(onProgress).toHaveBeenLastCalledWith(100);
  });

  it("falls back to the create endpoint when the create response has no Location header", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 200 })),
    );

    await uploadFileViaTus({
      sftpgoBaseUrl: "https://sftpgo.example.com/",
      shareId: "share-1",
      storageKey: "/attachments/cases/case-1/att-1",
      file,
    });

    expect(FakeXhr.instances[0].url).toBe(
      "https://sftpgo.example.com/api/v2/shares-chunked-uploads",
    );
  });

  it("throws when opening the upload session fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response(null, { status: 401 })),
    );

    await expect(
      uploadFileViaTus({
        sftpgoBaseUrl: "https://sftpgo.example.com",
        shareId: "share-1",
        storageKey: "/attachments/cases/case-1/att-1",
        file,
      }),
    ).rejects.toThrow(/401/);

    expect(FakeXhr.instances).toHaveLength(0);
  });

  it("rejects when the PATCH itself fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 201,
          headers: { Location: "/api/v2/shares-chunked-uploads/abc123" },
        }),
      ),
    );

    class FailingXhr extends FakeXhr {
      send(body: unknown): void {
        this.sentBody = body;
        this.status = 500;
        this.onload?.();
      }
    }
    vi.stubGlobal("XMLHttpRequest", FailingXhr as unknown as typeof XMLHttpRequest);

    await expect(
      uploadFileViaTus({
        sftpgoBaseUrl: "https://sftpgo.example.com",
        shareId: "share-1",
        storageKey: "/attachments/cases/case-1/att-1",
        file,
      }),
    ).rejects.toThrow(/500/);
  });

  it("rejects a non-https sftpgoBaseUrl before opening any request", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      uploadFileViaTus({
        sftpgoBaseUrl: "http://sftpgo.example.com",
        shareId: "share-1",
        storageKey: "/attachments/cases/case-1/att-1",
        file,
      }),
    ).rejects.toThrow(/https/i);

    expect(fetchMock).not.toHaveBeenCalled();
    expect(FakeXhr.instances).toHaveLength(0);
  });

  it("rejects a Location header pointing at a foreign origin before the PATCH is opened", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 201,
          headers: { Location: "https://evil.example.com/steal-the-share" },
        }),
      ),
    );

    await expect(
      uploadFileViaTus({
        sftpgoBaseUrl: "https://sftpgo.example.com",
        shareId: "share-1",
        storageKey: "/attachments/cases/case-1/att-1",
        file,
      }),
    ).rejects.toThrow(/untrusted origin/i);

    expect(FakeXhr.instances).toHaveLength(0);
  });

  it("accepts a same-origin absolute Location header", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 201,
          headers: {
            Location:
              "https://sftpgo.example.com/api/v2/shares-chunked-uploads/abc123",
          },
        }),
      ),
    );

    await uploadFileViaTus({
      sftpgoBaseUrl: "https://sftpgo.example.com",
      shareId: "share-1",
      storageKey: "/attachments/cases/case-1/att-1",
      file,
    });

    expect(FakeXhr.instances).toHaveLength(1);
    expect(FakeXhr.instances[0].url).toBe(
      "https://sftpgo.example.com/api/v2/shares-chunked-uploads/abc123",
    );
  });
});
