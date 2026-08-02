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
  buildWidgetPreviewHref,
  parseWidgetPreviewFilters,
  resolveCurrentUserSentinels,
} from "./widgetPreviewUrl";

const CURRENT_USER_ID = "11111111-aaaa-bbbb-cccc-000000000001";

describe("widgetPreviewUrl", () => {
  it("encodes each filter field as its own readable query param, not one JSON blob", () => {
    const href = buildWidgetPreviewHref({
      previewSlug: "cases",
      widgetId: "my_critical_open",
      displayName: "My Critical & High Cases",
      filters: { severities: ["critical", "high"], states: ["open"] },
    });

    expect(href.startsWith("/dashboard/cases?")).toBe(true);
    const params = new URLSearchParams(href.split("?")[1]);
    expect(params.get("w")).toBe("my_critical_open");
    expect(params.get("n")).toBe("My Critical & High Cases");
    expect(params.get("severities")).toBe("critical,high");
    expect(params.get("states")).toBe("open");
    expect(params.get("f")).toBeNull();
  });

  it("masks the current user's own id to @me instead of embedding it verbatim", () => {
    const href = buildWidgetPreviewHref({
      previewSlug: "cases",
      widgetId: "my_cases",
      displayName: "My Cases",
      filters: { assignedUserIds: [CURRENT_USER_ID] },
      currentUserId: CURRENT_USER_ID,
    });

    expect(href).not.toContain(CURRENT_USER_ID);
    const params = new URLSearchParams(href.split("?")[1]);
    expect(params.get("assignedUserIds")).toBe("@me");
  });

  it("round-trips filters through parseWidgetPreviewFilters + resolveCurrentUserSentinels", () => {
    const href = buildWidgetPreviewHref({
      previewSlug: "cases",
      widgetId: "my_cases",
      displayName: "My Cases",
      filters: { assignedUserIds: [CURRENT_USER_ID], severities: ["critical"] },
      currentUserId: CURRENT_USER_ID,
    });

    const searchParams = new URLSearchParams(href.split("?")[1]);
    const { filters, needsCurrentUser } = parseWidgetPreviewFilters(searchParams);
    expect(needsCurrentUser).toBe(true);
    expect(filters.severities).toEqual(["critical"]);
    expect(filters.assignedUserIds).toEqual(["@me"]);

    const resolved = resolveCurrentUserSentinels(filters, CURRENT_USER_ID);
    expect(resolved.assignedUserIds).toEqual([CURRENT_USER_ID]);
    expect(resolved.severities).toEqual(["critical"]);
  });

  it("leaves the @me sentinel in place when the current user id isn't known yet", () => {
    const resolved = resolveCurrentUserSentinels({ assignedUserIds: ["@me"] }, undefined);
    expect(resolved.assignedUserIds).toEqual(["@me"]);
  });

  it("ignores the reserved w/n params when parsing filters back", () => {
    const searchParams = new URLSearchParams({ w: "id", n: "Name", severities: "critical" });
    const { filters } = parseWidgetPreviewFilters(searchParams);
    expect(filters).toEqual({ severities: ["critical"] });
  });
});
