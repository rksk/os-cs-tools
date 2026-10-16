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

import type { PagingChainCode, RotaFamily, ScheduleTeam } from "../types";

/** A chain the Case Alerts tab can show: the four the server serves, and
 *  PaaS SRE, which has no rota (and so no chain) yet. The tab offers every
 *  one but CRE's (see caseAlertChainsFor). */
export type CaseAlertChain = PagingChainCode | "PAAS";

/** The chain a team's alerts go through: CRE's, SME's, or its own SRE rota's
 *  -- SaaS or IaaS -- with PaaS for an SRE team that has no rota yet. */
export function chainOfTeam(team: Pick<ScheduleTeam, "family" | "rotaCode">): CaseAlertChain {
  if (team.family === "CRE") return "CRE";
  if (team.family === "SME") return "SME";
  if (team.rotaCode === "SRE_IAAS") return "SRE_IAAS";
  if (team.rotaCode === "SRE_SAAS") return "SRE_SAAS";
  return "PAAS";
}

/**
 * The chains a reader is shown on Case Alerts: their own, not every team's.
 *
 * A Team lead or a rota admin sees the chain of each team they run -- an SRE
 * rota admin runs SaaS and IaaS, so both -- but never CRE's for running a CRE
 * team. Whoever may see a family's alert settings (settingsLadders, the
 * server's canManage.roomLadders) sees that family's chains: CRE's only so. The CRE head and the CS head look
 * across every family, as a CSM admin does, and see them all: `undefined`.
 * What they may change on each is still the server's to say.
 *
 * In the picker's order. Only what is shown -- the server still serves every
 * chain to internal staff, and checks every change.
 */
export function caseAlertChainsFor(reader: {
  /** Every team on the rota, from the catalogue. */
  teams: readonly Pick<ScheduleTeam, "key" | "family" | "rotaCode">[];
  /** The teams the reader leads or runs as a rota admin. */
  runs: readonly string[];
  /** A CSM admin. */
  portalAdmin: boolean;
  /** The CRE head or the CS head. */
  head?: boolean;
  /** The reader's own team, from their profile: what places a team the
   *  catalogue does not list -- one with no rota yet, such as PaaS. */
  ownTeam?: { key: string; family?: RotaFamily };
  /** The families whose alert settings the reader may see and change. */
  settingsLadders?: readonly string[];
}): CaseAlertChain[] | undefined {
  if (reader.portalAdmin || reader.head) return undefined;
  const byKey = new Map<string, Pick<ScheduleTeam, "family" | "rotaCode">>(
    reader.teams.map((t) => [t.key.toLowerCase(), t]),
  );
  const own = reader.ownTeam;
  if (own?.family && !byKey.has(own.key.toLowerCase())) byKey.set(own.key.toLowerCase(), { family: own.family });
  const mine = new Set<CaseAlertChain>();
  for (const key of reader.runs) {
    const team = byKey.get(key.toLowerCase());
    if (team) mine.add(chainOfTeam(team));
  }
  const mineOf: Record<string, CaseAlertChain[]> = {
    CRE: ["CRE"],
    SRE: ["SRE_SAAS", "SRE_IAAS"],
    SME: ["SME"],
  };
  mine.delete("CRE");
  for (const ladder of reader.settingsLadders ?? []) {
    for (const c of mineOf[ladder] ?? []) mine.add(c);
  }
  const order: CaseAlertChain[] = ["CRE", "SRE_SAAS", "SRE_IAAS", "PAAS", "SME"];
  return order.filter((c) => mine.has(c));
}

/**
 * Whether the reader's Case Alerts chains can be worked out yet. A lead's own
 * chains need only the teams they run (`ownKnown`: the catalogue, their lead
 * teams and their marker-only teams, all read). The CRE chain and rota access
 * reads (`extraKnown`) only add chains -- CRE's for a CRE paging editor, every
 * chain for a head or a CSM admin -- so a lead (`runsAny`) does not wait on
 * them: until they answer they add nothing. Anybody else has nothing until
 * those reads answer, so waits on them.
 */
export function caseAlertChainsReady(ownKnown: boolean, extraKnown: boolean, runsAny: boolean): boolean {
  return ownKnown && (runsAny || extraKnown);
}

/**
 * Whether Case Alerts is offered: to a reader the page would offer it to
 * (`allowed`), once their chains are known (`null` until then), and only when
 * they have one -- a lead of nothing on the rota (a change-request board the
 * directory lists as a team) gets no tab rather than a tab with nothing of
 * theirs on it. Every chain (`undefined`: a CSM admin, the CRE and CS heads)
 * is some.
 */
export function caseAlertsOffered(allowed: boolean, chains: readonly CaseAlertChain[] | undefined | null): boolean {
  if (!allowed || chains === null) return false;
  return chains === undefined || chains.length > 0;
}

