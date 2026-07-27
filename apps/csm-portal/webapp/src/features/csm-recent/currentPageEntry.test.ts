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
import {
  currentPageEntry,
  isKnownPage,
} from "@features/csm-recent/currentPageEntry";

describe("currentPageEntry — section landing routes", () => {
  it("pins a section under its own name", () => {
    expect(currentPageEntry("/cases", "")).toEqual({
      kind: "page",
      id: "/cases",
      title: "Support",
      href: "/cases",
    });
  });

  it("treats an empty query as no query", () => {
    expect(currentPageEntry("/cases", "?")).toMatchObject({ kind: "page" });
  });

  it("pins a filtered section view as a search", () => {
    expect(currentPageEntry("/cases", "?state=open")).toEqual({
      kind: "search",
      id: "/cases?state=open",
      title: "Support: 1 filter",
      href: "/cases?state=open",
    });
  });

  it("quotes a free-text query in the title", () => {
    expect(currentPageEntry("/cases", "?q=timeout").title).toBe(
      "Support: “timeout”",
    );
  });
});

describe("currentPageEntry — route-backed tabs", () => {
  it("pins a tab under its own name, not its section's", () => {
    expect(currentPageEntry("/customers/accounts", "")).toEqual({
      kind: "page",
      id: "/customers/accounts",
      title: "Accounts",
      href: "/customers/accounts",
    });
  });
});

describe("currentPageEntry — query-backed tabs", () => {
  it("pins ?tab= as the tab itself rather than a filter of its section", () => {
    expect(currentPageEntry("/operations", "?tab=incidents")).toEqual({
      kind: "page",
      id: "/operations?tab=incidents",
      title: "Incidents",
      href: "/operations?tab=incidents",
    });
  });

  it("gives each tab a distinct pin", () => {
    const incidents = currentPageEntry("/operations", "?tab=incidents");
    const problems = currentPageEntry("/operations", "?tab=problems");
    expect(problems.title).toBe("Problem management");
    expect(problems.id).not.toBe(incidents.id);
  });

  it("counts the remaining parameters as filters, excluding the tab", () => {
    expect(currentPageEntry("/operations", "?tab=incidents&state=open")).toEqual(
      {
        kind: "search",
        id: "/operations?tab=incidents&state=open",
        title: "Incidents: 1 filter",
        href: "/operations?tab=incidents&state=open",
      },
    );
  });

  it("falls back to the section for an unrecognised tab value", () => {
    expect(currentPageEntry("/operations", "?tab=nope").title).toBe(
      "Operations: 1 filter",
    );
  });

  it("still treats a non-tab query on the section as a filter", () => {
    expect(currentPageEntry("/operations", "?state=open").title).toBe(
      "Operations: 1 filter",
    );
  });
});

describe("currentPageEntry — unknown routes", () => {
  it("labels from the first path segment", () => {
    expect(currentPageEntry("/security-center/reports/new", "")).toEqual({
      kind: "page",
      id: "/security-center/reports/new",
      title: "Security center",
      href: "/security-center/reports/new",
    });
  });

  it("falls back to a generic title at the root", () => {
    expect(currentPageEntry("/nothing-here", "").title).toBe("Nothing here");
  });
});

describe("isKnownPage", () => {
  it("accepts section landing routes and tab routes alike", () => {
    expect(isKnownPage("/cases")).toBe(true);
    expect(isKnownPage("/customers/accounts")).toBe(true);
    expect(isKnownPage("/operations/incidents")).toBe(true);
  });

  it("rejects a detail route and an unknown path", () => {
    expect(isKnownPage("/operations/incidents/INC0001")).toBe(false);
    expect(isKnownPage("/nothing-here")).toBe(false);
  });
});
