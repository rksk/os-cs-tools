/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import { caseAlertChainsFor, caseAlertChainsReady, caseAlertsOffered, chainOfTeam } from "./pagingChains";

const TEAMS = [
  { key: "castor", family: "CRE" as const },
  { key: "americas", family: "CRE" as const },
  { key: "apollo", family: "SRE" as const, rotaCode: "SRE_SAAS" },
  { key: "artemis", family: "SRE" as const, rotaCode: "SRE_SAAS" },
  { key: "iaas", family: "SRE" as const, rotaCode: "SRE_IAAS" },
  { key: "paas", family: "SRE" as const },
  { key: "asgardeo", family: "SME" as const, rotaCode: "SME_ASGARDEO" },
];
const reader = (
  runs: string[],
  over: Partial<{ portalAdmin: boolean; head: boolean; settingsLadders: string[] }> = {},
) =>
  caseAlertChainsFor({ teams: TEAMS, runs, portalAdmin: false, ...over });

describe("chainOfTeam", () => {
  it("is the family's chain, or the SRE rota's", () => {
    expect(TEAMS.map((t) => chainOfTeam(t))).toEqual(["CRE", "CRE", "SRE_SAAS", "SRE_SAAS", "SRE_IAAS", "PAAS", "SME"]);
  });
});

describe("caseAlertChainsFor", () => {
  it("gives each lead their own team's chain only", () => {
    expect(reader(["apollo"])).toEqual(["SRE_SAAS"]);
    expect(reader(["iaas"])).toEqual(["SRE_IAAS"]);
    expect(reader(["paas"])).toEqual(["PAAS"]);
    expect(reader(["asgardeo"])).toEqual(["SME"]);
  });

  it("gives a rota admin the chains of the teams they run", () => {
    // An SRE rota admin runs every SRE team: SaaS and IaaS.
    expect(reader(["apollo", "artemis", "iaas"])).toEqual(["SRE_SAAS", "SRE_IAAS"]);
  });

  it("never offers CRE's chain: CRE alerts keep their own flow", () => {
    // A CRE lead or rota admin has no chain here, so is not offered the tab.
    expect(reader(["castor"])).toEqual([]);
    expect(reader(["castor", "americas"])).toEqual([]);
    expect(caseAlertsOffered(true, reader(["castor", "americas"]))).toBe(false);
    // Somebody who runs a CRE team and an SRE one sees the SRE chain alone.
    expect(reader(["castor", "apollo"])).toEqual(["SRE_SAAS"]);
  });

  it("gives the CRE head and the CS head every chain, as a CSM admin", () => {
    expect(reader([], { head: true })).toBeUndefined();
    expect(caseAlertsOffered(true, reader([], { head: true }))).toBe(true);
  });

  it("places a lead's team the catalogue does not list -- PaaS, with no rota -- from their profile", () => {
    const teams = TEAMS.filter((t) => t.key !== "paas");
    expect(caseAlertChainsFor({ teams, runs: ["paas"], portalAdmin: false })).toEqual([]);
    expect(
      caseAlertChainsFor({ teams, runs: ["paas"], portalAdmin: false, ownTeam: { key: "paas", family: "SRE" } }),
    ).toEqual(["PAAS"]);
  });

  it("gives each family's chains to whoever may change its alert settings, CRE's only so", () => {
    // A CRE rota admin: CRE's chain, though running CRE teams gives none.
    expect(reader(["castor", "americas"], { settingsLadders: ["CRE"] })).toEqual(["CRE"]);
    expect(caseAlertsOffered(true, reader([], { settingsLadders: ["CRE"] }))).toBe(true);
    // Somebody with all three rota admin roles.
    expect(reader([], { settingsLadders: ["CRE", "SRE", "SME"] })).toEqual(["CRE", "SRE_SAAS", "SRE_IAAS", "SME"]);
    // An SRE rota admin who also leads a CRE team still gets no CRE chain.
    expect(reader(["castor", "apollo"], { settingsLadders: ["SRE"] })).toEqual(["SRE_SAAS", "SRE_IAAS"]);
  });

  it("gives a CSM admin every chain", () => {
    expect(reader([], { portalAdmin: true })).toBeUndefined();
  });

  it("ignores a team it does not know, and gives nobody's chain to a reader who runs nothing", () => {
    expect(reader(["no-such-team"])).toEqual([]);
    expect(reader([])).toEqual([]);
  });
});

describe("caseAlertsOffered", () => {
  it("offers the tab only once the chains are known, and only with one to show", () => {
    expect(caseAlertsOffered(true, ["SRE_SAAS"])).toBe(true);
    expect(caseAlertsOffered(true, ["PAAS"])).toBe(true); // says PaaS has no rota yet
    expect(caseAlertsOffered(true, undefined)).toBe(true); // a CSM admin: every chain
    expect(caseAlertsOffered(true, [])).toBe(false); // leads nothing on the rota
    expect(caseAlertsOffered(true, null)).toBe(false); // not known yet
    expect(caseAlertsOffered(false, ["SRE_SAAS"])).toBe(false); // not a lead or rota admin
  });
});

describe("caseAlertChainsReady", () => {
  it("does not hold a lead's own chains on the reads that only add chains", () => {
    expect(caseAlertChainsReady(true, false, true)).toBe(true); // a lead: CRE chain / rota access still loading
    expect(caseAlertChainsReady(true, true, true)).toBe(true);
    expect(caseAlertChainsReady(true, false, false)).toBe(false); // a head or editor with no team: wait for them
    expect(caseAlertChainsReady(true, true, false)).toBe(true);
    expect(caseAlertChainsReady(false, true, true)).toBe(false); // own teams not read yet
  });
});
