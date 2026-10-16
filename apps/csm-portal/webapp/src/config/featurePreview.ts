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

import { useCallback, useSyncExternalStore } from "react";

/**
 * Per-admin, browser-local overrides for the Feature Preview dialog: turning
 * one of this deployment's hidden/WIP nav nodes on makes it visible to just
 * this admin, in just this browser — never the deployment-wide
 * `CSM_PORTAL_FEATURE_OVERRIDES` config (`public/config.js`), which this
 * module never reads or writes. See `featureFlags.ts`'s `featureState` for
 * how the override is actually applied (promote-only, capability-gated).
 *
 * Keyed per user id, same `<base>.<userId>` shape `useRecentViews.ts` uses
 * for its own per-user localStorage bucket, so a shared machine's accounts
 * never see each other's preview toggles.
 */
const STORAGE_KEY_BASE = "csm.featurePreview.v1";

export function previewStorageKey(userId: string): string {
  return `${STORAGE_KEY_BASE}.${userId}`;
}

/** Reads and JSON-parses the stored override map. Never throws: no user id,
 *  nothing stored, or a parse failure all resolve to an empty map. */
export function readPreviewOverrides(
  userId: string | undefined,
): Record<string, boolean> {
  if (!userId) return {};
  try {
    const raw = localStorage.getItem(previewStorageKey(userId));
    if (!raw) return {};
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
      return {};
    }
    return parsed as Record<string, boolean>;
  } catch {
    return {};
  }
}

/** Module-level pub-sub so every component reading overrides (sidebar, route
 *  guard, quick-nav, the dialog itself) re-renders live when a toggle fires,
 *  without lifting the state into React. */
const listeners = new Set<() => void>();

function notify(): void {
  for (const listener of listeners) listener();
}

/**
 * The `storage` event only ever fires in OTHER documents, never the one that
 * called `localStorage.setItem` — this tab's own writes already go through
 * `writePreviewOverride`'s own direct `notify()` call below, so this handler
 * only has to re-notify for a write this tab didn't make itself. Filters on
 * `event.key` starting with the shared `STORAGE_KEY_BASE` prefix so an
 * unrelated `localStorage` write elsewhere in the app doesn't trigger a
 * spurious re-render here; `key === null` (a `clear()` call) is treated the
 * same as a match, since that could plausibly have wiped this value too.
 */
function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  const onStorage = (event: StorageEvent): void => {
    if (event.key === null || event.key.startsWith(STORAGE_KEY_BASE)) {
      listener();
    }
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", onStorage);
  };
}

/**
 * Merges one override into the stored map and persists it, then notifies
 * subscribers. Turning a feature OFF removes its key entirely rather than
 * storing an explicit `false` — keeps the stored blob minimal (it only ever
 * grows for features an admin actually turned on), and `readPreviewOverrides`
 * already treats an absent key as off, so the two are indistinguishable to
 * every reader.
 */
export function writePreviewOverride(
  userId: string,
  navNodeId: string,
  enabled: boolean,
): void {
  try {
    const current = readPreviewOverrides(userId);
    const next = { ...current };
    if (enabled) {
      next[navNodeId] = true;
    } else {
      delete next[navNodeId];
    }
    localStorage.setItem(previewStorageKey(userId), JSON.stringify(next));
  } catch {
    /* localStorage may be unavailable -- the toggle just won't persist */
  }
  notify();
}

export interface FeaturePreviewOverridesResult {
  overrides: Record<string, boolean>;
  setOverride: (navNodeId: string, enabled: boolean) => void;
}

/**
 * Live view of the signed-in admin's preview overrides, backed by
 * `localStorage` and kept in sync across every component that calls this hook
 * (and across tabs, via `useSyncExternalStore`'s subscription). Caches its
 * snapshot per userId + the raw stored JSON string, so a render that hasn't
 * actually changed (same user, same underlying blob) returns the same object
 * identity instead of a fresh parse every time — `useSyncExternalStore`
 * requires that to avoid an infinite re-render loop.
 */
export function useFeaturePreviewOverrides(
  userId: string | undefined,
): FeaturePreviewOverridesResult {
  const getSnapshot = useCallback((): Record<string, boolean> => {
    return getCachedOverrides(userId);
  }, [userId]);

  const overrides = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  const setOverride = useCallback(
    (navNodeId: string, enabled: boolean): void => {
      if (!userId) return;
      writePreviewOverride(userId, navNodeId, enabled);
    },
    [userId],
  );

  return { overrides, setOverride };
}

// Snapshot cache: keyed by userId + the raw stored JSON string, so repeated
// calls that observe no actual change (same user, same underlying blob)
// return the identical object rather than a fresh parse/object every render.
// A single, stable empty-object reference for the no-user-id case --
// `useSyncExternalStore` requires `getSnapshot` to return the same identity
// when nothing has changed, and a fresh `{}` literal on every call would
// fail that and loop forever.
const EMPTY_OVERRIDES: Record<string, boolean> = {};

let cachedUserId: string | undefined;
let cachedRaw: string | null = null;
let cachedOverrides: Record<string, boolean> = EMPTY_OVERRIDES;

function getCachedOverrides(userId: string | undefined): Record<string, boolean> {
  if (!userId) {
    cachedUserId = undefined;
    cachedRaw = null;
    cachedOverrides = EMPTY_OVERRIDES;
    return cachedOverrides;
  }
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(previewStorageKey(userId));
  } catch {
    raw = null;
  }
  if (userId === cachedUserId && raw === cachedRaw) return cachedOverrides;
  cachedUserId = userId;
  cachedRaw = raw;
  cachedOverrides = readPreviewOverrides(userId);
  return cachedOverrides;
}
