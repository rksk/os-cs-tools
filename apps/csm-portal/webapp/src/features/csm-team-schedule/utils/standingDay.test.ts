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

import { describe, expect, it } from "vitest";
import type { ScheduleAbsence, ScheduleAssignment, ScheduleShift, ScheduleTeam, ScheduleTeamMember } from "../types";
import { instantInZone, onItsStandingCard, STANDING_DAY_SOURCE, standingDayRows } from "./standingDay";

const COLOMBO = "Asia/Colombo";

function shift(code: string, startMinute: number, endMinute: number, extra: Partial<ScheduleShift> = {}): ScheduleShift {
  return {
    id: code,
    code,
    shortCode: code,
    label: code,
    family: "CRE",
    dayScope: "WEEKDAY",
    startMinute,
    endMinute,
    authoringTimeZone: COLOMBO,
    isOnCall: false,
    isEscalation: false,
    isRotation: true,
    crossesMidnight: endMinute > 1440,
    colourToken: code,
    sortOrder: 0,
    ...extra,
  };
}

const SHIFTS = new Map(
  [
    shift("CRE_REGULAR", 540, 1080, { isRotation: false }),
    shift("CRE_AMERICAS", 1260, 1800, { isRotation: false }),
    shift("CRE_REGULAR_IND", 540, 1080, { isRotation: false, authoringTimeZone: "Asia/Kolkata" }),
    shift("CRE_EVENING", 1080, 1260),
    shift("CRE_MORNING_OC", 360, 540, { isOnCall: true }),
    shift("CRE_LATE", 1260, 1380),
    shift("CRE_ESC_EVE", 1080, 1260, { isEscalation: true }),
  ].map((s) => [s.code, s]),
);

const member = (id: string, role = "engineer"): ScheduleTeamMember => ({
  userId: id,
  name: `Engineer ${id}`,
  email: `${id}@example.com`,
  isLead: role === "lead",
  role,
});

const team = (key: string, members: ScheduleTeamMember[], defaultShiftCode?: string): ScheduleTeam => ({
  key,
  name: key,
  family: "CRE",
  sortOrder: 0,
  members,
  defaultShiftCode,
});

const row = (id: string, teamKey: string, shiftCode: string, rotaDate: string): ScheduleAssignment => ({
  id: `${id}-${shiftCode}`,
  engineer: { userId: id, name: `Engineer ${id}`, email: `${id}@example.com`, isLead: false },
  teamKey,
  shiftCode,
  rotaDate,
  startsAt: "2026-10-12T00:00:00Z",
  endsAt: "2026-10-12T01:00:00Z",
  isOnCall: false,
  source: "IMPORTED",
});

const away = (id: string, startsOn: string, endsOn?: string): ScheduleAbsence => ({
  id: `ab-${id}`,
  engineer: { userId: id, name: `Engineer ${id}`, email: `${id}@example.com`, isLead: false },
  teamKey: "draco",
  kindCode: "ANNUAL_LEAVE",
  startsOn,
  endsOn,
});

const MONDAY = "2026-10-12";
const SATURDAY = "2026-10-17";

describe("standingDayRows", () => {
  it("puts an India region team's engineers on IND and its lead on Regular hours", () => {
    const out = standingDayRows({
      rotaDates: [MONDAY],
      teams: [team("phoenix", [member("lead", "lead"), member("e")], "CRE_REGULAR_IND")],
      shifts: SHIFTS,
      assignments: [],
      absences: [],
    });
    const codeOf = (id: string) => out.find((r) => r.engineer.userId === id)?.shiftCode;
    expect(codeOf("e")).toBe("CRE_REGULAR_IND");
    expect(codeOf("lead")).toBe("CRE_REGULAR");
  });

  it("keeps the Americas team's leads on Americas cover", () => {
    const out = standingDayRows({
      rotaDates: [MONDAY],
      teams: [team("americas", [member("lead", "lead")], "CRE_AMERICAS")],
      shifts: SHIFTS,
      assignments: [],
      absences: [],
    });
    expect(out.map((r) => r.shiftCode)).toEqual(["CRE_AMERICAS"]);
  });

  it("lists a member with no row and no leave as working their team's regular hours", () => {
    const out = standingDayRows({
      rotaDates: [MONDAY],
      teams: [team("draco", [member("a"), member("b")])],
      shifts: SHIFTS,
      assignments: [row("a", "draco", "CRE_REGULAR", MONDAY)],
      absences: [],
    });
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({
      engineer: { userId: "b" },
      teamKey: "draco",
      shiftCode: "CRE_REGULAR",
      rotaDate: MONDAY,
      source: STANDING_DAY_SOURCE,
      startsAt: "2026-10-12T03:30:00.000Z",
      endsAt: "2026-10-12T12:30:00.000Z",
    });
  });

  it("leaves out people on leave, managers, and weekends", () => {
    const teams = [team("draco", [member("a"), member("h", "cre_head"), member("l", "lead")])];
    expect(
      standingDayRows({ rotaDates: [MONDAY], teams, shifts: SHIFTS, assignments: [], absences: [away("a", "2026-10-10")] })
        .map((r) => r.engineer.userId),
    ).toEqual(["l"]);
    expect(
      standingDayRows({ rotaDates: [SATURDAY], teams, shifts: SHIFTS, assignments: [], absences: [] }),
    ).toEqual([]);
  });

  it("uses the team's own ordinary window -- Americas cover on the Americas team", () => {
    const out = standingDayRows({
      rotaDates: [MONDAY],
      teams: [team("americas", [member("n")], "CRE_AMERICAS")],
      shifts: SHIFTS,
      assignments: [],
      absences: [],
    });
    expect(out[0]).toMatchObject({
      shiftCode: "CRE_AMERICAS",
      startsAt: "2026-10-12T15:30:00.000Z",
      endsAt: "2026-10-13T00:30:00.000Z",
    });
  });

  it("lists the evening and morning crews (on-call included) in regular hours too", () => {
    const out = standingDayRows({
      rotaDates: [MONDAY],
      teams: [team("draco", [member("e"), member("m"), member("x")])],
      shifts: SHIFTS,
      assignments: [
        row("e", "draco", "CRE_EVENING", MONDAY),
        row("m", "draco", "CRE_MORNING_OC", MONDAY),
        // A window that does not touch the regular hours adds nothing.
        row("x", "draco", "CRE_LATE", MONDAY),
      ],
      absences: [],
    });
    expect(out.map((r) => [r.engineer.userId, r.shiftCode])).toEqual([
      ["e", "CRE_REGULAR"],
      ["m", "CRE_REGULAR"],
    ]);
  });

  it("adds nothing for an escalation tier, even one whose window touches the regular hours", () => {
    const out = standingDayRows({
      rotaDates: [MONDAY],
      teams: [team("draco", [member("t")])],
      shifts: SHIFTS,
      assignments: [row("t", "draco", "CRE_ESC_EVE", MONDAY)],
      absences: [],
    });
    expect(out).toEqual([]);
  });

  it("does not add regular hours for an evening person who already holds them, or who is away", () => {
    const teams = [team("draco", [member("e")])];
    const evening = row("e", "draco", "CRE_EVENING", MONDAY);
    expect(
      standingDayRows({
        rotaDates: [MONDAY],
        teams,
        shifts: SHIFTS,
        assignments: [evening, row("e", "draco", "CRE_REGULAR", MONDAY)],
        absences: [],
      }),
    ).toEqual([]);
    expect(
      standingDayRows({ rotaDates: [MONDAY], teams, shifts: SHIFTS, assignments: [evening], absences: [away("e", MONDAY, MONDAY)] }),
    ).toEqual([]);
  });

  it("lists someone on two teams once, and each rota day on its own", () => {
    const out = standingDayRows({
      rotaDates: [MONDAY, "2026-10-13"],
      teams: [team("draco", [member("x")]), team("migration", [member("x")])],
      shifts: SHIFTS,
      assignments: [],
      absences: [],
    });
    expect(out.map((r) => [r.rotaDate, r.teamKey])).toEqual([
      [MONDAY, "draco"],
      ["2026-10-13", "draco"],
    ]);
  });
});

describe("instantInZone", () => {
  it("reads a wall-clock minute in the zone, including past midnight", () => {
    expect(instantInZone(MONDAY, 540, COLOMBO)).toBe("2026-10-12T03:30:00.000Z");
    expect(instantInZone(MONDAY, 1800, COLOMBO)).toBe("2026-10-13T00:30:00.000Z");
    // A zone with daylight saving: 09:00 in New York on 2026-11-02 is EST.
    expect(instantInZone("2026-11-02", 540, "America/New_York")).toBe("2026-11-02T14:00:00.000Z");
  });
});

describe("onItsStandingCard", () => {
  const TEAMS: ScheduleTeam[] = [
    { ...team("vega", []), isAbt: true },
    { ...team("americas", [], "CRE_AMERICAS"), isAbt: false },
    { ...team("migration", []), isAbt: false },
  ];
  const keep = (teamKey: string, shiftCode: string, teams = TEAMS) =>
    onItsStandingCard(row("p", teamKey, shiftCode, MONDAY), SHIFTS, teams);

  it("lists only the Americas team on Americas cover", () => {
    expect(keep("americas", "CRE_AMERICAS")).toBe(true);
    expect(keep("vega", "CRE_AMERICAS")).toBe(false);
  });

  it("lists only ABTs on regular hours", () => {
    expect(keep("vega", "CRE_REGULAR")).toBe(true);
    expect(keep("migration", "CRE_REGULAR")).toBe(false);
  });

  it("leaves turns alone, whatever the team", () => {
    expect(keep("migration", "CRE_EVENING")).toBe(true);
    expect(keep("americas", "CRE_MORNING_OC")).toBe(true);
  });

  it("does not apply the ABT rule until the server says which teams are ABTs", () => {
    const old = TEAMS.map(({ isAbt: _isAbt, ...t }) => t);
    expect(keep("migration", "CRE_REGULAR", old)).toBe(true);
    expect(keep("vega", "CRE_AMERICAS", old)).toBe(false);
  });
});
