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

import { useCallback, useMemo, useRef, useState, type JSX, type ReactNode } from "react";
import {
  useDeleteCaseAlertRoom,
  useDeleteCaseAlertRoute,
  useDeleteCaseAlertValue,
  useSaveCaseAlertRoom,
  useSaveCaseAlertRoute,
  useSaveCaseAlertValues,
  useSetCaseAlertWebhook,
} from "../api/useCaseAlertSettings";
import {
  DraftsContext,
  sameTarget,
  type DraftChange,
  type NewChange,
  type SettingsFamily,
  type SubmitResult,
} from "../utils/caseAlertDrafts";

/**
 * Holds the tab's changes until Submit, for both families at once, so moving
 * between CRE and SRE keeps what was changed on the other. Nothing reaches the
 * server until Submit; the alert service still uses a submitted change only
 * after its next restart.
 */
export default function CaseAlertDraftsProvider({ children }: { children: ReactNode }): JSX.Element {
  const [changes, setChanges] = useState<DraftChange[]>([]);
  const [submitting, setSubmitting] = useState<SettingsFamily | null>(null);
  const nextId = useRef(1);
  const saveRoom = useSaveCaseAlertRoom();
  const setWebhook = useSetCaseAlertWebhook();
  const deleteRoom = useDeleteCaseAlertRoom();
  const saveRoute = useSaveCaseAlertRoute();
  const deleteRoute = useDeleteCaseAlertRoute();
  const saveValues = useSaveCaseAlertValues();
  const deleteValue = useDeleteCaseAlertValue();

  const add = useCallback((change: NewChange) => {
    setChanges((prev) => {
      const earlier = prev.find((c) => sameTarget(c, change));
      // A room changed again keeps a URL typed before, unless a new one is typed.
      const merged =
        earlier?.kind === "room" && change.kind === "room" && !change.url && earlier.url
          ? { ...change, url: earlier.url }
          : change;
      const next = { ...merged, id: nextId.current++ } as DraftChange;
      return earlier ? prev.map((c) => (c === earlier ? next : c)) : [...prev, next];
    });
  }, []);

  const drop = useCallback((id: number) => setChanges((prev) => prev.filter((c) => c.id !== id)), []);
  const discard = useCallback(
    (family: SettingsFamily) => setChanges((prev) => prev.filter((c) => c.family !== family)),
    [],
  );

  const submit = useCallback(
    async (family: SettingsFamily): Promise<SubmitResult> => {
      const mine = changes.filter((c) => c.family === family);
      setSubmitting(family);
      let saved = 0;
      try {
        for (const c of mine) {
          switch (c.kind) {
            case "room": {
              const room = await saveRoom.mutateAsync(c.input);
              const roomId = room?.id ?? c.roomId;
              if (c.url && roomId) await setWebhook.mutateAsync({ roomId, url: c.url });
              break;
            }
            case "deleteRoom":
              await deleteRoom.mutateAsync(c.roomId);
              break;
            case "route":
              await saveRoute.mutateAsync(c.input);
              break;
            case "deleteRoute":
              await deleteRoute.mutateAsync(c.routeId);
              break;
            case "values":
              await saveValues.mutateAsync(c.input);
              break;
            case "deleteValue":
              await deleteValue.mutateAsync(c.valueId);
              break;
          }
          saved++;
          setChanges((prev) => prev.filter((p) => p.id !== c.id));
        }
        return { saved };
      } catch (err) {
        return { saved, error: (err as Error)?.message || "A change did not save." };
      } finally {
        setSubmitting(null);
      }
    },
    [changes, saveRoom, setWebhook, deleteRoom, saveRoute, deleteRoute, saveValues, deleteValue],
  );

  const value = useMemo(
    () => ({ changes, add, drop, discard, submit, submitting }),
    [changes, add, drop, discard, submit, submitting],
  );
  return <DraftsContext.Provider value={value}>{children}</DraftsContext.Provider>;
}
