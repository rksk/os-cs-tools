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

import type { ScheduleShift } from "../types";
import { addDays, toIsoDate } from "./rota";

/** One write that puts a run of days back: the window they held, or "" for
 *  nothing, over consecutive days. */
export interface RevertRun {
  code: string;
  from: string;
  to: string;
}

/** The little of an assignment the undo needs. */
interface HeldDay {
  engineer: { userId: string };
  rotaDate: string;
  shiftCode: string;
  tier?: string;
  zoneCode?: string;
}

/**
 * The writes that put one engineer's days back the way they were before a
 * rota write over `from`..`to`, or undefined where that cannot be done
 * exactly.
 *
 * Exact means each day held one whole-day window, or nothing: a tier, a zone,
 * or two windows on one day are not something a single apply can rebuild,
 * and an undo that guesses is worse than none. Days the write never touched
 * -- the weekend inside a weekday window's span -- are left out. Read from
 * what the page had loaded *before* the write.
 */
export function rotaRevertRuns(
  held: readonly HeldDay[],
  shifts: ReadonlyMap<string, ScheduleShift>,
  userId: string,
  from: string,
  to: string,
  appliedCode: string,
): RevertRun[] | undefined {
  const before = new Map<string, string>();
  for (const a of held) {
    if (a.engineer.userId !== userId || a.rotaDate < from || a.rotaDate > to) continue;
    if (a.tier || a.zoneCode || shifts.get(a.shiftCode)?.zoneCode || before.has(a.rotaDate)) return undefined;
    before.set(a.rotaDate, a.shiftCode);
  }
  const applied = appliedCode ? shifts.get(appliedCode) : undefined;
  const runs: RevertRun[] = [];
  for (let d = new Date(`${from}T00:00:00`); toIsoDate(d) <= to; d = addDays(d, 1)) {
    const iso = toIsoDate(d);
    const weekend = d.getDay() === 0 || d.getDay() === 6;
    if (applied?.dayScope === "WEEKDAY" && weekend) continue;
    if (applied?.dayScope === "WEEKEND" && !weekend) continue;
    const code = before.get(iso) ?? "";
    const last = runs[runs.length - 1];
    if (last && last.code === code && last.to === toIsoDate(addDays(d, -1))) last.to = iso;
    else runs.push({ code, from: iso, to: iso });
  }
  return runs.length > 0 ? runs : undefined;
}

/** An undo that put some runs back and then failed: the days before
 *  `remainingFrom` are as they were, the rest still hold the change. */
export class UndoIncomplete extends Error {
  constructor(
    readonly remainingFrom: string,
    readonly failure: unknown,
  ) {
    super("The undo did not finish.");
    this.name = "UndoIncomplete";
  }
}

/** Write the runs back in order. A failure after the first run has gone in
 *  says how far it got (`UndoIncomplete`); one on the first run is passed on
 *  as it is, since nothing was put back. */
export async function applyRevertRuns(
  runs: readonly RevertRun[],
  apply: (run: RevertRun) => Promise<unknown>,
): Promise<void> {
  for (const [i, run] of runs.entries()) {
    try {
      await apply(run);
    } catch (err) {
      if (i === 0) throw err;
      throw new UndoIncomplete(run.from, err);
    }
  }
}

/** Whether leave or an allocation marked over `from`..`to` can be taken off
 *  again exactly: only where the engineer had nothing marked on any of those
 *  days before. */
export function awayIsRevertible(
  absences: readonly { engineer: { userId: string }; startsOn: string; endsOn?: string | null }[],
  userId: string,
  from: string,
  to: string,
): boolean {
  return !absences.some(
    (ab) => ab.engineer.userId === userId && ab.startsOn <= to && (!ab.endsOn || ab.endsOn >= from),
  );
}
