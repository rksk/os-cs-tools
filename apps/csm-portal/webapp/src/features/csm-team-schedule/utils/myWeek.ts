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

/**
 * The positions above a team lead, as the CRE alert chain lists them: the
 * Americas team lead and the CRE and CS heads. They sit above the teams rather
 * than working a rota on one.
 */
export const ABOVE_TEAM_LEAD_ROLES: readonly string[] = ["americas_team_lead", "cre_head", "cs_head"];

/** A read the page waits on: done once it has answered or failed. */
export function isSettled(q: { isSuccess: boolean; isError: boolean }): boolean {
  return q.isSuccess || q.isError;
}

/**
 * Whether the reader sits above the team leads -- a CSM admin, or the Americas
 * team lead, the CRE head or the CS head -- and so has no My week of their
 * own. Everyone else keeps My week, whatever rota roles they hold: a rota admin
 * of all three groups still works the rota of their own team.
 *
 * `csmAdmin` is the rota-access read's `canManage.rotaAdmins`, true for a CSM
 * admin only. `chainMembers` is the CRE alert chain, which lists who holds each
 * of the positions above. A read that failed counts as "not above the leads":
 * the safe side is to keep a tab, not to take one away.
 */
export function sitsAboveTeamLeads(
  readerEmail: string,
  csmAdmin: boolean | undefined,
  chainMembers: readonly { email: string; role: string }[] | undefined,
): boolean {
  if (csmAdmin) return true;
  const email = readerEmail.trim().toLowerCase();
  if (!email) return false;
  return (chainMembers ?? []).some(
    (m) => m.email.trim().toLowerCase() === email && ABOVE_TEAM_LEAD_ROLES.includes(m.role),
  );
}
