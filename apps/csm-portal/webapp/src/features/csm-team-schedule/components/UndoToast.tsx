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

import { useCallback, useEffect, useRef, useState, type JSX } from "react";

/** How long the toast stays up untouched. Long enough to notice a mis-click
 *  and reach the button; held open while the pointer or focus is on it. */
export const UNDO_MS = 8000;

export interface UndoOffer {
  /** Changes with every offer, so a second save restarts the clock. */
  id: number;
  /** What was saved, in the words of the change log. */
  message: string;
  /** "Saved" unless said otherwise ("Undone"). */
  verb?: string;
  /** Puts it back. Absent where the previous state cannot be restored
   *  exactly, when the toast only confirms the save. */
  undo?: () => Promise<void>;
}

/**
 * "Saved -- Undo" after a change on the roster.
 *
 * Edits save the moment they are clicked, which leads like and keep; this is
 * the way back from the click that landed on the wrong row. Polite live
 * region, so a screen reader hears the save without losing its place.
 */
export default function UndoToast({
  offer,
  onDismiss,
  onUndone,
  onUndoFailed,
}: {
  offer: UndoOffer | null;
  onDismiss: () => void;
  onUndone: (offer: UndoOffer) => void;
  onUndoFailed: (err: unknown) => void;
}): JSX.Element | null {
  const [busy, setBusy] = useState(false);
  const held = useRef(false);
  // The pointer and focus hold it separately: one leaving must not restart
  // the clock while the other is still on the toast.
  const hovered = useRef(false);
  const focused = useRef(false);
  const timer = useRef<number | undefined>(undefined);

  /** Start (or restart) the clock. The caller passes a stable onDismiss. */
  const arm = useCallback((): void => {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      if (!held.current) onDismiss();
    }, UNDO_MS);
  }, [onDismiss]);

  const offerId = offer?.id;
  useEffect(() => {
    if (offerId === undefined) return;
    arm();
    return () => window.clearTimeout(timer.current);
  }, [offerId, arm]);

  if (!offer) return null;

  /** Held while either is on it; the clock restarts once both have left. */
  const sync = (): void => {
    held.current = hovered.current || focused.current;
    if (held.current) window.clearTimeout(timer.current);
    else arm();
  };
  const setHovered = (on: boolean) => (): void => {
    hovered.current = on;
    sync();
  };
  const setFocused = (on: boolean) => (): void => {
    focused.current = on;
    sync();
  };

  return (
    <div
      className="undotoast"
      role="status"
      aria-live="polite"
      onMouseEnter={setHovered(true)}
      onMouseLeave={setHovered(false)}
      onFocus={setFocused(true)}
      onBlur={setFocused(false)}
    >
      <span className="ut-msg">
        <b>{offer.verb ?? "Saved"}</b> · {offer.message}
      </span>
      {offer.undo ? (
        <button
          type="button"
          className="ut-undo"
          disabled={busy}
          onClick={() => {
            if (!offer.undo) return;
            setBusy(true);
            offer
              .undo()
              .then(() => onUndone(offer))
              .catch((err: unknown) => onUndoFailed(err))
              .finally(() => setBusy(false));
          }}
        >
          {busy ? "Undoing…" : "Undo"}
        </button>
      ) : null}
      <button type="button" className="ut-x" aria-label="Dismiss" onClick={onDismiss}>
        &times;
      </button>
    </div>
  );
}
