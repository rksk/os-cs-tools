/*
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

import type { ScheduleAbsence, ScheduleAssignment, ScheduleShift, ScheduleTeam } from "../types";
import { partsInZone } from "./rota";

/** The window a team's ordinary weekday is when the team names none. */
export const REGULAR_HOURS_CODE = "CRE_REGULAR";

/** The India region shift (IND). */
export const INDIA_HOURS_CODE = "CRE_REGULAR_IND";

/**
 * Which window somebody's ordinary weekday is on their team: the team's own
 * (Americas cover, the India region shift), or Regular hours when it names
 * none. One exception: a team whose ordinary day is the India region shift
 * is led by an engineer from the Sri Lankan teams on rotation, so its lead
 * keeps Regular hours. Only that team: the Americas team's leads work Americas cover like
 * the rest of it.
 */
export function ordinaryDayCode(teamDefault: string | undefined, isLead: boolean): string {
  if (isLead && teamDefault === INDIA_HOURS_CODE) return REGULAR_HOURS_CODE;
  return teamDefault ?? REGULAR_HOURS_CODE;
}

/** Roles that sit above the rota and are never on it (the roster leaves them out too). */
const MANAGEMENT_ROLES = new Set(["americas_team_lead", "cre_head", "cs_head"]);

/** Marks a row this module made up, so nothing mistakes it for a stored one. */
export const STANDING_DAY_SOURCE = "STANDING_DAY";

/**
 * The instant a wall-clock minute of a calendar day is in a time zone, as ISO.
 * A minute past 1440 runs into the next day, the way a window that crosses
 * midnight is stored (21:00-06:00 is 1260-1800).
 */
export function instantInZone(isoDate: string, minute: number, timeZone: string): string {
  const [y, m, d] = isoDate.split("-").map(Number);
  const wall = Date.UTC(y, m - 1, d, 0, minute);
  let t = wall;
  // Two passes settle the zone's offset, including on a day it changes.
  for (let i = 0; i < 2; i++) {
    const seen = partsInZone(new Date(t).toISOString(), timeZone);
    const [sy, sm, sd] = seen.date.split("-").map(Number);
    t += wall - Date.UTC(sy, sm - 1, sd, 0, seen.minutes);
  }
  return new Date(t).toISOString();
}

function isWeekdayIso(iso: string): boolean {
  const dow = new Date(`${iso}T00:00:00`).getDay();
  return dow !== 0 && dow !== 6;
}

function covers(ab: ScheduleAbsence, iso: string): boolean {
  return ab.startsOn <= iso && (!ab.endsOn || iso <= ab.endsOn);
}

/**
 * Whether a row belongs on the day view's card for its window.
 *
 * A standing window is somebody's ordinary day, not a turn, so its card lists
 * only the teams whose ordinary day it is: Americas cover lists the Americas
 * team (a Brazil rotation is filed under Americas, so it stays), and Regular
 * hours and the India region shift list the ABTs -- not Migration, not a day
 * somebody on another team was put on Americas cover by hand. Turns (morning,
 * evening, weekend, on-call, escalation) are not touched. Until the server
 * says which teams are ABTs, the regular-hours rule is not applied.
 */
export function onItsStandingCard(
  row: ScheduleAssignment,
  shifts: ReadonlyMap<string, ScheduleShift>,
  teams: readonly ScheduleTeam[],
): boolean {
  const window = shifts.get(row.shiftCode);
  if (!window || window.isRotation) return true;
  const team = teams.find((t) => t.key.toLowerCase() === row.teamKey.toLowerCase());
  if (teams.some((t) => t.defaultShiftCode === row.shiftCode)) return team?.defaultShiftCode === row.shiftCode;
  const abtKnown = teams.some((t) => t.isAbt !== undefined);
  return !abtKnown || team?.isAbt === true;
}

/**
 * Who is working their team's ordinary day without a row on the rota to say so.
 *
 * The month roster paints every blank weekday of a team member as that team's
 * ordinary hours -- Regular hours, or Americas cover on the Americas team --
 * because that is what a blank weekday means. No row is stored for it, and the
 * day view drew only stored rows, so somebody the roster showed at work was
 * missing from "Who is on today". This makes the same rows up for the day
 * view, from the same rule:
 *
 *  - a weekday, for a member of a team in scope (not a manager above the rota),
 *    with no row of their own that rota day and no leave or allocation over it;
 *  - and the crews either side of the day: somebody whose turn that day is a
 *    window that touches their team's ordinary hours -- the evening that
 *    starts as they end (18:00-21:00 after 09:00-18:00), or the morning that
 *    ends as they start (06:00-09:00, on-call included) -- works the ordinary
 *    day too, so they are listed in it as well. An escalation tier is not a
 *    turn of that kind and adds nothing.
 *
 * Someone on two teams is listed once, under the first in catalogue order.
 * Display only: nothing here is saved, and paging reads the stored rota, never
 * these rows.
 */
export function standingDayRows({
  rotaDates,
  teams,
  shifts,
  assignments,
  absences,
}: {
  rotaDates: readonly string[];
  /** The teams on screen, members included, in display order. */
  teams: readonly ScheduleTeam[];
  shifts: ReadonlyMap<string, ScheduleShift>;
  assignments: readonly ScheduleAssignment[];
  absences: readonly ScheduleAbsence[];
}): ScheduleAssignment[] {
  const out: ScheduleAssignment[] = [];
  const windowOf = (team: ScheduleTeam, isLead: boolean): ScheduleShift | undefined =>
    shifts.get(ordinaryDayCode(team.defaultShiftCode, isLead));
  const teamByKey = new Map(teams.map((t) => [t.key.toLowerCase(), t]));

  for (const iso of rotaDates) {
    if (!isWeekdayIso(iso)) continue;
    const rowsToday = assignments.filter((a) => a.rotaDate === iso);
    const withRow = new Set(rowsToday.map((a) => a.engineer.userId));
    const away = new Set(absences.filter((ab) => covers(ab, iso)).map((ab) => ab.engineer.userId));
    const listed = new Set<string>();

    const add = (team: ScheduleTeam, window: ScheduleShift, engineer: ScheduleAssignment["engineer"]): void => {
      listed.add(engineer.userId);
      const end = window.endMinute > window.startMinute ? window.endMinute : window.endMinute + 1440;
      out.push({
        id: `standing:${engineer.userId}:${iso}`,
        engineer,
        teamKey: team.key,
        shiftCode: window.code,
        rotaDate: iso,
        startsAt: instantInZone(iso, window.startMinute, window.authoringTimeZone),
        endsAt: instantInZone(iso, end, window.authoringTimeZone),
        isOnCall: false,
        source: STANDING_DAY_SOURCE,
      });
    };

    // A blank weekday: the team's ordinary day.
    for (const team of teams) {
      for (const m of team.members ?? []) {
        if (MANAGEMENT_ROLES.has(m.role) || listed.has(m.userId)) continue;
        if (withRow.has(m.userId) || away.has(m.userId)) continue;
        const window = windowOf(team, m.isLead);
        if (!window) continue;
        add(team, window, { userId: m.userId, name: m.name, email: m.email, isLead: m.isLead });
      }
    }

    // The morning and evening crews work the ordinary day around their turn.
    for (const a of rowsToday) {
      const id = a.engineer.userId;
      if (listed.has(id) || away.has(id)) continue;
      const team = teamByKey.get(a.teamKey.toLowerCase());
      const window = team ? windowOf(team, a.engineer.isLead) : undefined;
      const turn = shifts.get(a.shiftCode);
      if (!team || !window || !turn || !turn.isRotation || turn.isEscalation) continue;
      if (turn.startMinute !== window.endMinute && turn.endMinute !== window.startMinute) continue;
      // Already holding a standing window that day: nothing to add.
      const standing = rowsToday.some((r) => r.engineer.userId === id && shifts.get(r.shiftCode)?.isRotation === false);
      if (standing) continue;
      add(team, window, a.engineer);
    }
  }
  return out;
}
