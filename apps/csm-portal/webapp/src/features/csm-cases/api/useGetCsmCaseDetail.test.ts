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

import { describe, expect, it, vi } from "vitest";
import type { BeCaseView } from "@api/backend/types";

// The module also exports a hook that imports the backend client, which throws
// at import time without runtime config. This test only exercises the mapper.
vi.mock("@api/backend/client", () => ({
  useBackendApi: vi.fn(),
}));

const { detailFromBeCase } = await import("./useGetCsmCaseDetail");

const baseCase = {
  id: "case-1",
  number: "CS0000001",
  title: "Sample case",
} as unknown as BeCaseView;

describe("detailFromBeCase resolution", () => {
  it("prefills the close notes from closeNotes", () => {
    const d = detailFromBeCase({
      ...baseCase,
      resolutionCode: "SOLVED_BY_CUSTOMER",
      closeNotes: "Fixed by upgrading.",
      resolutionNotes: null,
    } as BeCaseView);
    expect(d.resolution?.notes).toBe("Fixed by upgrading.");
  });

  it("prefers closeNotes over resolutionNotes when both are present", () => {
    const d = detailFromBeCase({
      ...baseCase,
      closeNotes: "Close text",
      resolutionNotes: "Resolution text",
    } as BeCaseView);
    expect(d.resolution?.notes).toBe("Close text");
  });

  it("falls back to resolutionNotes when closeNotes is absent", () => {
    const d = detailFromBeCase({
      ...baseCase,
      resolutionNotes: "Resolution text",
    } as BeCaseView);
    expect(d.resolution?.notes).toBe("Resolution text");
  });

  it("builds a resolution from closeNotes alone", () => {
    const d = detailFromBeCase({
      ...baseCase,
      closeNotes: "Only close notes",
    } as BeCaseView);
    expect(d.resolution).toEqual({
      resolutionCode: undefined,
      cause: undefined,
      notes: "Only close notes",
    });
  });

  it("leaves resolution undefined when nothing is set", () => {
    const d = detailFromBeCase({ ...baseCase, closeNotes: null } as BeCaseView);
    expect(d.resolution).toBeUndefined();
  });
});
