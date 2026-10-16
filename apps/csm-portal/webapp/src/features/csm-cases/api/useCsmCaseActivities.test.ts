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
import type { BeCaseActivityEntry } from "@api/backend/types";

// The module also exports `useGetCsmCaseActivities`, which imports the
// backend client; that client throws at import time if the runtime config
// (`CSM_PORTAL_BACKEND_BASE_URL`) isn't present, which it isn't under
// vitest. Stub it out — this test only exercises the pure mapper.
vi.mock("@api/backend/client", () => ({
  useBackendApi: vi.fn(),
}));

const { auditEntryFromBeActivity } = await import("./useCsmCaseActivities");

describe("auditEntryFromBeActivity", () => {
  it("maps a field_change entry with a preferred display-name order", () => {
    const entry: BeCaseActivityEntry = {
      id: "fc-1",
      type: "field_change",
      createdOn: "2026-07-01T00:00:00Z",
      createdBy: { id: null, email: "jane.doe@example.com", name: "Jane Doe" },
      createdByFirstName: "Jane",
      createdByLastName: "Doe",
      changes: [
        {
          field: "state",
          fieldLabel: "State",
          previousValue: "In Progress",
          newValue: "Resolved",
        },
      ],
    };

    const mapped = auditEntryFromBeActivity(entry);

    expect(mapped).toEqual({
      id: "fc-1",
      kind: "field_change",
      actor: "Jane Doe",
      actorUser: { id: null, email: "jane.doe@example.com", name: "Jane Doe" },
      createdAt: "2026-07-01T00:00:00Z",
      changes: [
        {
          field: "state",
          fieldLabel: "State",
          previousValue: "In Progress",
          newValue: "Resolved",
        },
      ],
    });
  });

  it("falls back to first+last name, then the bare email, when createdBy.name is absent", () => {
    const noName: BeCaseActivityEntry = {
      id: "fc-2",
      type: "field_change",
      createdOn: "2026-07-01T00:00:00Z",
      createdByFirstName: "Jane",
      createdByLastName: "Doe",
      changes: [],
    };
    expect(auditEntryFromBeActivity(noName).actor).toBe("Jane Doe");

    const emailOnly: BeCaseActivityEntry = {
      id: "fc-3",
      type: "field_change",
      createdOn: "2026-07-01T00:00:00Z",
      createdBy: { id: null, email: "jane.doe@example.com", name: "" },
      changes: [],
    };
    expect(auditEntryFromBeActivity(emailOnly).actor).toBe(
      "jane.doe@example.com",
    );

    const nothing: BeCaseActivityEntry = {
      id: "fc-4",
      type: "field_change",
      createdOn: "2026-07-01T00:00:00Z",
      changes: [],
    };
    const mapped = auditEntryFromBeActivity(nothing);
    expect(mapped.actor).toBe("System");
    expect(mapped.actorUser).toBeUndefined();

    const blankRef: BeCaseActivityEntry = {
      ...nothing,
      createdBy: { id: null, email: " ", name: "" },
    };
    expect(auditEntryFromBeActivity(blankRef).actor).toBe("System");
  });

  it("defaults changes to an empty array when absent", () => {
    const entry: BeCaseActivityEntry = {
      id: "fc-5",
      type: "field_change",
      createdOn: "2026-07-01T00:00:00Z",
      createdBy: { id: null, email: "jane.doe@example.com", name: "Jane Doe" },
    };
    expect(auditEntryFromBeActivity(entry).changes).toEqual([]);
  });
});
