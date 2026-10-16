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

import { describe, expect, it, vi } from "vitest";
import { UndoIncomplete, applyRevertRuns, awayIsRevertible, rotaRevertRuns } from "./undo";
import { EVENING, REGULAR, TZ1_L1, WEEKEND, shiftMap } from "../test/fixtures";

const SHIFTS = shiftMap(REGULAR, EVENING, WEEKEND, TZ1_L1);
const held = (rotaDate: string, shiftCode: string, over: { tier?: string; zoneCode?: string; userId?: string } = {}) => ({
  engineer: { userId: over.userId ?? "u1" },
  rotaDate,
  shiftCode,
  tier: over.tier,
  zoneCode: over.zoneCode,
});

describe("rotaRevertRuns: putting a rota write back", () => {
  it("restores each day's window, grouped into runs", () => {
    // Mon-Wed regular hours, Thu evening, Fri nothing stored; 6-9pm set over the week.
    const runs = rotaRevertRuns(
      [held("2026-09-21", REGULAR.code), held("2026-09-22", REGULAR.code), held("2026-09-23", REGULAR.code), held("2026-09-24", EVENING.code)],
      SHIFTS,
      "u1",
      "2026-09-21",
      "2026-09-25",
      EVENING.code,
    );
    expect(runs).toEqual([
      { code: REGULAR.code, from: "2026-09-21", to: "2026-09-23" },
      { code: EVENING.code, from: "2026-09-24", to: "2026-09-24" },
      { code: "", from: "2026-09-25", to: "2026-09-25" },
    ]);
  });

  it("leaves out the weekend a weekday window skipped", () => {
    const runs = rotaRevertRuns([], shiftMap(REGULAR, { ...EVENING, dayScope: "WEEKDAY" }), "u1", "2026-09-25", "2026-09-28", EVENING.code);
    expect(runs).toEqual([
      { code: "", from: "2026-09-25", to: "2026-09-25" },
      { code: "", from: "2026-09-28", to: "2026-09-28" },
    ]);
  });

  it("offers nothing where a day held a tier, a zone, or two windows", () => {
    expect(rotaRevertRuns([held("2026-09-21", TZ1_L1.code, { tier: "L1" })], SHIFTS, "u1", "2026-09-21", "2026-09-21", EVENING.code)).toBeUndefined();
    expect(rotaRevertRuns([held("2026-09-21", REGULAR.code, { zoneCode: "TZ1" })], SHIFTS, "u1", "2026-09-21", "2026-09-21", EVENING.code)).toBeUndefined();
    expect(
      rotaRevertRuns([held("2026-09-21", REGULAR.code), held("2026-09-21", EVENING.code)], SHIFTS, "u1", "2026-09-21", "2026-09-21", WEEKEND.code),
    ).toBeUndefined();
  });

  it("reads only the engineer who was changed", () => {
    const runs = rotaRevertRuns([held("2026-09-21", EVENING.code, { userId: "someone-else" })], SHIFTS, "u1", "2026-09-21", "2026-09-21", REGULAR.code);
    expect(runs).toEqual([{ code: "", from: "2026-09-21", to: "2026-09-21" }]);
  });
});

describe("awayIsRevertible: taking marked leave off again", () => {
  const ab = (startsOn: string, endsOn: string | null, userId = "u1") => ({ engineer: { userId }, startsOn, endsOn });

  it("is exact where nothing was marked on those days", () => {
    expect(awayIsRevertible([ab("2026-09-01", "2026-09-05")], "u1", "2026-09-21", "2026-09-23")).toBe(true);
    expect(awayIsRevertible([ab("2026-09-21", "2026-09-23", "u2")], "u1", "2026-09-21", "2026-09-23")).toBe(true);
  });

  it("is not where something already covered a day, open-ended spans included", () => {
    expect(awayIsRevertible([ab("2026-09-23", "2026-09-30")], "u1", "2026-09-21", "2026-09-23")).toBe(false);
    expect(awayIsRevertible([ab("2026-09-01", null)], "u1", "2026-09-21", "2026-09-23")).toBe(false);
  });
});

describe("applyRevertRuns: how far an undo got", () => {
  const runs = [
    { code: "REGULAR", from: "2026-09-21", to: "2026-09-23" },
    { code: "EVENING", from: "2026-09-24", to: "2026-09-24" },
    { code: "", from: "2026-09-25", to: "2026-09-25" },
  ];

  it("writes every run, in order", async () => {
    const apply = vi.fn().mockResolvedValue(undefined);
    await applyRevertRuns(runs, apply);
    expect(apply.mock.calls.map(([r]) => r.from)).toEqual(["2026-09-21", "2026-09-24", "2026-09-25"]);
  });

  it("says where the days still holding the change begin when a later run fails", async () => {
    const boom = new Error("nope");
    const apply = vi.fn().mockResolvedValueOnce(undefined).mockRejectedValueOnce(boom);
    const err = await applyRevertRuns(runs, apply).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(UndoIncomplete);
    expect((err as UndoIncomplete).remainingFrom).toBe("2026-09-24");
    expect((err as UndoIncomplete).failure).toBe(boom);
    expect(apply).toHaveBeenCalledTimes(2);
  });

  it("passes a failure on the first run on as it is: nothing was put back", async () => {
    const boom = new Error("nope");
    await expect(applyRevertRuns(runs, vi.fn().mockRejectedValue(boom))).rejects.toBe(boom);
  });
});
