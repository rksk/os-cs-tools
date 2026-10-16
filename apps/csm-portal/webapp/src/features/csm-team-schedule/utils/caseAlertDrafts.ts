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

import { createContext, useContext } from "react";
import type { SaveRoomInput, SaveRouteInput, SaveValuesInput } from "../api/useCaseAlertSettings";

/** The two families of the Alert Settings tab: CRE, and SRE (SaaS, IaaS and
 *  SME together). Each keeps its own changes until its Submit. */
export type SettingsFamily = "CRE" | "SRE";

/**
 * A change made on the tab, not saved yet: Submit saves a family's changes in
 * the order they were made. `what` is how the change reads to people.
 */
export type DraftChange = { id: number; family: SettingsFamily; what: string } & (
  | { kind: "room"; input: SaveRoomInput; url?: string; roomId?: string }
  | { kind: "deleteRoom"; roomId: string }
  | { kind: "route"; input: SaveRouteInput }
  | { kind: "deleteRoute"; routeId: string }
  | { kind: "values"; input: SaveValuesInput }
  | { kind: "deleteValue"; valueId: string }
);

export type NewChange = DraftChange extends infer C ? (C extends DraftChange ? Omit<C, "id"> : never) : never;

export interface SubmitResult {
  /** Changes saved. */
  saved: number;
  /** Why the first change that failed did; it and the rest stay pending. */
  error?: string;
}

export interface DraftsValue {
  changes: DraftChange[];
  add: (change: NewChange) => void;
  drop: (id: number) => void;
  discard: (family: SettingsFamily) => void;
  submit: (family: SettingsFamily) => Promise<SubmitResult>;
  submitting: SettingsFamily | null;
}

export const DraftsContext = createContext<DraftsValue | null>(null);

/** Which earlier change a new one replaces: the same room, route or value
 *  changed again keeps only its latest wording. */
export function sameTarget(a: DraftChange | NewChange, b: DraftChange | NewChange): boolean {
  if (a.family !== b.family || a.kind !== b.kind) return false;
  switch (a.kind) {
    case "room":
      return b.kind === "room" && a.input.ladder === b.input.ladder && (a.input.teamKey ?? "") === (b.input.teamKey ?? "");
    case "deleteRoom":
      return b.kind === "deleteRoom" && a.roomId === b.roomId;
    case "route":
      return b.kind === "route" && a.input.ladder === b.input.ladder && a.input.groupName === b.input.groupName;
    case "deleteRoute":
      return b.kind === "deleteRoute" && a.routeId === b.routeId;
    case "values":
      return (
        b.kind === "values" &&
        a.input.ladder === b.input.ladder &&
        Object.keys(a.input.values).join() === Object.keys(b.input.values).join()
      );
    case "deleteValue":
      return b.kind === "deleteValue" && a.valueId === b.valueId;
  }
}

/** The tab's pending changes: CaseAlertDraftsProvider must be above. */
export function useCaseAlertDrafts(): DraftsValue {
  const ctx = useContext(DraftsContext);
  if (!ctx) throw new Error("useCaseAlertDrafts needs a CaseAlertDraftsProvider");
  return ctx;
}
