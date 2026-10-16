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
import { vulnerabilityMetaToPriorityOptions } from "./vulnerabilities";

describe("vulnerabilityMetaToPriorityOptions", () => {
  it("falls back to the static priority list with default labels when severities is undefined", () => {
    const options = vulnerabilityMetaToPriorityOptions(undefined);
    expect(options.map((o) => o.priority)).toEqual([
      "critical",
      "high",
      "medium",
      "low",
      "info",
      "unknown",
    ]);
    expect(options.find((o) => o.priority === "critical")?.label).toBe("Critical");
  });

  it("falls back to the static priority list when severities is empty", () => {
    const options = vulnerabilityMetaToPriorityOptions([]);
    expect(options).toHaveLength(6);
  });

  it("maps entity-service's numeric severity ids to the lowercase priority enum, in display order", () => {
    const options = vulnerabilityMetaToPriorityOptions([
      { id: "4", label: "Critical" },
      { id: "3", label: "High" },
      { id: "0", label: "Info" },
    ]);
    expect(options).toEqual([
      { priority: "critical", label: "Critical" },
      { priority: "high", label: "High" },
      { priority: "info", label: "Info" },
    ]);
  });

  it("uses each entry's own real label, not the static display map", () => {
    const options = vulnerabilityMetaToPriorityOptions([{ id: "4", label: "Catastrophic" }]);
    expect(options).toEqual([{ priority: "critical", label: "Catastrophic" }]);
  });

  it("skips an id it doesn't recognize rather than rendering a blank option", () => {
    const options = vulnerabilityMetaToPriorityOptions([
      { id: "4", label: "Critical" },
      { id: "99", label: "Something unknown" },
    ]);
    expect(options).toEqual([{ priority: "critical", label: "Critical" }]);
  });
});
