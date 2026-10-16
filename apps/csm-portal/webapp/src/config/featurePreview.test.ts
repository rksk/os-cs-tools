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
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  previewStorageKey,
  readPreviewOverrides,
  useFeaturePreviewOverrides,
  writePreviewOverride,
} from "@config/featurePreview";

beforeEach(() => {
  localStorage.clear();
});

describe("previewStorageKey", () => {
  it("is namespaced per user", () => {
    expect(previewStorageKey("user-1")).toBe("csm.featurePreview.v1.user-1");
    expect(previewStorageKey("user-2")).toBe("csm.featurePreview.v1.user-2");
  });
});

describe("readPreviewOverrides", () => {
  it("returns an empty map with no user id", () => {
    expect(readPreviewOverrides(undefined)).toEqual({});
  });

  it("returns an empty map when nothing is stored", () => {
    expect(readPreviewOverrides("user-1")).toEqual({});
  });

  it("reads back what was written", () => {
    writePreviewOverride("user-1", "updates", true);
    expect(readPreviewOverrides("user-1")).toEqual({ updates: true });
  });

  it("never throws on malformed JSON, and returns an empty map", () => {
    localStorage.setItem(previewStorageKey("user-1"), "{not json");
    expect(readPreviewOverrides("user-1")).toEqual({});
  });

  it("never throws on a stored non-object value", () => {
    localStorage.setItem(previewStorageKey("user-1"), JSON.stringify(["updates"]));
    expect(readPreviewOverrides("user-1")).toEqual({});
  });
});

describe("writePreviewOverride", () => {
  it("merges a new entry alongside existing ones", () => {
    writePreviewOverride("user-1", "updates", true);
    writePreviewOverride("user-1", "engagements", true);
    expect(readPreviewOverrides("user-1")).toEqual({
      updates: true,
      engagements: true,
    });
  });

  it("removes the key entirely when turned off, keeping the blob minimal", () => {
    writePreviewOverride("user-1", "updates", true);
    writePreviewOverride("user-1", "engagements", true);
    writePreviewOverride("user-1", "updates", false);
    expect(readPreviewOverrides("user-1")).toEqual({ engagements: true });
    expect(localStorage.getItem(previewStorageKey("user-1"))).not.toContain("updates");
  });

  it("keeps each user's overrides independent", () => {
    writePreviewOverride("user-1", "updates", true);
    writePreviewOverride("user-2", "engagements", true);
    expect(readPreviewOverrides("user-1")).toEqual({ updates: true });
    expect(readPreviewOverrides("user-2")).toEqual({ engagements: true });
  });
});

describe("useFeaturePreviewOverrides", () => {
  it("reflects the stored overrides for the given user", () => {
    writePreviewOverride("user-1", "updates", true);
    const { result } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    expect(result.current.overrides).toEqual({ updates: true });
  });

  it("returns an empty map with no user id", () => {
    const { result } = renderHook(() => useFeaturePreviewOverrides(undefined));
    expect(result.current.overrides).toEqual({});
  });

  it("setOverride updates the live snapshot", () => {
    const { result } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    expect(result.current.overrides).toEqual({});
    act(() => {
      result.current.setOverride("updates", true);
    });
    expect(result.current.overrides).toEqual({ updates: true });
    act(() => {
      result.current.setOverride("updates", false);
    });
    expect(result.current.overrides).toEqual({});
  });

  it("setOverride is a no-op with no user id", () => {
    const { result } = renderHook(() => useFeaturePreviewOverrides(undefined));
    act(() => {
      result.current.setOverride("updates", true);
    });
    expect(result.current.overrides).toEqual({});
  });

  it("notifies a second hook instance when the first one writes", () => {
    const a = renderHook(() => useFeaturePreviewOverrides("user-1"));
    const b = renderHook(() => useFeaturePreviewOverrides("user-1"));
    act(() => {
      a.result.current.setOverride("engagements", true);
    });
    expect(b.result.current.overrides).toEqual({ engagements: true });
  });

  it("pub-sub fires on a direct writePreviewOverride call too, not just through setOverride", () => {
    const { result } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    act(() => {
      writePreviewOverride("user-1", "updates", true);
    });
    expect(result.current.overrides).toEqual({ updates: true });
  });

  it("does not return a fresh object identity when nothing changed", () => {
    writePreviewOverride("user-1", "updates", true);
    const { result, rerender } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    const first = result.current.overrides;
    rerender();
    expect(result.current.overrides).toBe(first);
  });

  it("picks up a write made by another tab, via the storage event", () => {
    const { result } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    expect(result.current.overrides).toEqual({});

    // Another tab's write never calls this tab's own writePreviewOverride --
    // it only ever reaches localStorage directly, then the browser fires a
    // `storage` event at every OTHER open document.
    act(() => {
      localStorage.setItem(
        previewStorageKey("user-1"),
        JSON.stringify({ updates: true }),
      );
      window.dispatchEvent(
        new StorageEvent("storage", {
          key: previewStorageKey("user-1"),
          newValue: JSON.stringify({ updates: true }),
        }),
      );
    });

    expect(result.current.overrides).toEqual({ updates: true });
  });

  it("ignores a storage event for an unrelated localStorage key", () => {
    const { result } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    act(() => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: "some.other.app.key" }),
      );
    });
    expect(result.current.overrides).toEqual({});
  });

  it("re-reads on a storage-cleared event (key: null) as a precaution", () => {
    writePreviewOverride("user-1", "updates", true);
    const { result } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    expect(result.current.overrides).toEqual({ updates: true });

    act(() => {
      localStorage.clear();
      window.dispatchEvent(new StorageEvent("storage", { key: null }));
    });

    expect(result.current.overrides).toEqual({});
  });
});

describe("localStorage unavailable", () => {
  it("writePreviewOverride still notifies subscribers even if persistence throws", () => {
    const original = localStorage.setItem;
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota exceeded");
    });
    const { result } = renderHook(() => useFeaturePreviewOverrides("user-1"));
    expect(() => {
      act(() => {
        result.current.setOverride("updates", true);
      });
    }).not.toThrow();
    Storage.prototype.setItem = original;
  });
});
