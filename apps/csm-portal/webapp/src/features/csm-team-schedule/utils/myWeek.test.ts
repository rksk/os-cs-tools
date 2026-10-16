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
import { isSettled, sitsAboveTeamLeads } from "./myWeek";

const chain = [
  { email: "lead@example.com", role: "lead" },
  { email: "eng@example.com", role: "engineer" },
  { email: "americas@example.com", role: "americas_team_lead" },
  { email: "crehead@example.com", role: "cre_head" },
  { email: "cshead@example.com", role: "cs_head" },
];

describe("sitsAboveTeamLeads", () => {
  it("is true for a CSM admin, whatever the chain says", () => {
    expect(sitsAboveTeamLeads("anyone@example.com", true, chain)).toBe(true);
    expect(sitsAboveTeamLeads("anyone@example.com", true, undefined)).toBe(true);
  });

  it("is true for the Americas team lead and the CRE and CS heads", () => {
    expect(sitsAboveTeamLeads("americas@example.com", false, chain)).toBe(true);
    expect(sitsAboveTeamLeads("crehead@example.com", false, chain)).toBe(true);
    expect(sitsAboveTeamLeads("cshead@example.com", false, chain)).toBe(true);
  });

  it("is false for a team lead and an engineer", () => {
    expect(sitsAboveTeamLeads("lead@example.com", false, chain)).toBe(false);
    expect(sitsAboveTeamLeads("eng@example.com", false, chain)).toBe(false);
  });

  it("is false for a rota admin of every group who is none of those", () => {
    // Rota admin roles are not read here at all: holding all three is not a
    // position above the leads.
    expect(sitsAboveTeamLeads("multi.admin@example.com", false, chain)).toBe(false);
  });

  it("matches the email regardless of case and spaces", () => {
    expect(sitsAboveTeamLeads("  CREHead@EXAMPLE.com ", false, chain)).toBe(true);
  });

  it("is false when either read failed or is missing", () => {
    expect(sitsAboveTeamLeads("crehead@example.com", undefined, undefined)).toBe(false);
    expect(sitsAboveTeamLeads("", false, chain)).toBe(false);
  });
});

describe("isSettled", () => {
  it("is done once the read answered or failed", () => {
    expect(isSettled({ isSuccess: false, isError: false })).toBe(false);
    expect(isSettled({ isSuccess: true, isError: false })).toBe(true);
    expect(isSettled({ isSuccess: false, isError: true })).toBe(true);
  });
});
