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

import { describe, expect, it } from "vitest";
import type { BeProjectContact } from "@api/backend/types";
import {
  NO_WATCH_LIST_EDITS,
  addWatcherEdit,
  eligibleContactWatchers,
  isEligibleWatcher,
  removeWatcherEdit,
  resolveWatchList,
} from "./createCaseWatchList";

const jane: BeProjectContact = {
  name: "Jane Doe",
  email: "jane.doe@example.com",
  registrationState: "registered",
  roles: ["PORTAL_USER"],
};

describe("isEligibleWatcher", () => {
  it("requires a registered state, trimmed and case-insensitive", () => {
    expect(isEligibleWatcher(jane)).toBe(true);
    expect(isEligibleWatcher({ ...jane, registrationState: "  Registered " })).toBe(true);
    expect(isEligibleWatcher({ ...jane, registrationState: "invited" })).toBe(false);
    expect(isEligibleWatcher({ ...jane, registrationState: "re-invited" })).toBe(false);
    expect(isEligibleWatcher({ ...jane, registrationState: undefined })).toBe(false);
  });

  it("needs an email", () => {
    expect(isEligibleWatcher({ ...jane, email: "  " })).toBe(false);
    expect(isEligibleWatcher({ ...jane, email: undefined })).toBe(false);
  });

  it("excludes security-only contacts but keeps admins, portal users and integration users", () => {
    expect(isEligibleWatcher({ ...jane, roles: ["SECURITY_CONTACT"] })).toBe(false);
    expect(isEligibleWatcher({ ...jane, roles: ["SECURITY_CONTACT", "PORTAL_USER"] })).toBe(true);
    expect(isEligibleWatcher({ ...jane, roles: ["SECURITY_CONTACT", "ADMIN"] })).toBe(true);
    expect(
      isEligibleWatcher({ ...jane, roles: ["SECURITY_CONTACT"], isCsIntegrationUser: true }),
    ).toBe(true);
    expect(isEligibleWatcher({ ...jane, roles: [] })).toBe(true);
  });
});

describe("eligibleContactWatchers", () => {
  it("drops ineligible rows and de-duplicates by address ignoring case", () => {
    const out = eligibleContactWatchers([
      jane,
      { ...jane, email: "JANE.DOE@example.com" },
      { name: "John Smith", email: "john.smith@example.com", registrationState: "invited" },
    ]);
    expect(out.map((c) => c.email)).toEqual(["jane.doe@example.com"]);
  });
});

describe("watch list edits", () => {
  const a = { email: "a@example.com", name: "A", kind: "contact" as const };
  const b = { email: "b@example.com", name: "B", kind: "internal" as const };

  it("removal hides a pre-filled entry and re-adding restores it", () => {
    const removed = removeWatcherEdit(NO_WATCH_LIST_EDITS, "A@example.com");
    expect(resolveWatchList([a], removed)).toEqual([]);
    expect(resolveWatchList([a], addWatcherEdit(removed, a)).map((w) => w.email)).toEqual([
      "a@example.com",
    ]);
  });

  it("keeps added entries when the pre-fill arrives later", () => {
    const edits = addWatcherEdit(NO_WATCH_LIST_EDITS, b);
    expect(resolveWatchList([], edits).map((w) => w.email)).toEqual(["b@example.com"]);
    expect(resolveWatchList([a], edits).map((w) => w.email)).toEqual([
      "a@example.com",
      "b@example.com",
    ]);
  });

  it("never lists the same address twice", () => {
    const edits = addWatcherEdit(NO_WATCH_LIST_EDITS, { ...a, email: "A@Example.com" });
    expect(resolveWatchList([a], edits)).toHaveLength(1);
  });
});
