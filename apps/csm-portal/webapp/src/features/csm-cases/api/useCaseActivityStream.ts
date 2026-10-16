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

import { useEffect, useRef, useSyncExternalStore } from "react";
import EventSourcePolyfill from "@sanity/eventsource";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { apiConfig } from "@config/apiConfig";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useIsCaseTabVisible } from "@context/case-tabs/CaseTabVisibilityContext";
import { useAuthTokens } from "@hooks/useAuthTokens";
import { useLogger } from "@hooks/useLogger";

/** Base delay before the first reconnect attempt after the stream errors out or drops. */
const RECONNECT_BASE_DELAY_MS = 3_000;
/** Reconnect delay never grows past this while still within MAX_BACKOFF_ATTEMPTS. */
const RECONNECT_MAX_DELAY_MS = 30_000;
/**
 * After this many consecutive failures (the backoff curve has long since
 * maxed out at RECONNECT_MAX_DELAY_MS by then), stop retrying every ≤30s —
 * a sustained outage/misconfiguration (e.g. the backend rejecting every
 * caller with a 403) would otherwise have every open case-detail tab call
 * the backend roughly twice a minute forever.
 */
const MAX_BACKOFF_ATTEMPTS = 10;
/** Retry interval once MAX_BACKOFF_ATTEMPTS is exceeded — still eventually recovers once the backend does, at a fraction of the call volume. */
const RECONNECT_IDLE_DELAY_MS = 5 * 60_000;

/**
 * Exponential backoff with full jitter (attempt 0 is a random delay in
 * [0, base), attempt 1 in [0, base*2), ... capped at max) — a sustained
 * backend outage or misconfiguration would otherwise have every open
 * case-detail tab retry in lockstep every RECONNECT_BASE_DELAY_MS forever,
 * hammering the endpoint indefinitely instead of backing off. Past
 * MAX_BACKOFF_ATTEMPTS consecutive failures, falls back to a slow idle
 * retry instead of continuing to hammer the endpoint every ≤30s.
 */
function reconnectDelay(attempt: number): number {
  if (attempt >= MAX_BACKOFF_ATTEMPTS) return RECONNECT_IDLE_DELAY_MS;
  const capped = Math.min(RECONNECT_MAX_DELAY_MS, RECONNECT_BASE_DELAY_MS * 2 ** attempt);
  return Math.random() * capped;
}

/**
 * Refetches everything on the case page that can reflect a `case_updated`
 * event (comment, state, assignee, severity or acknowledgement change; the
 * payload carries only the case id, never the new values). Shared by the live
 * event handler and the reactivation path so the two cannot drift.
 *
 * - comments / activities / detail / SLAs: keyed by case id, so the case is
 *   invalidated precisely. Detail is declared as `[key, caseId, userEmail]`,
 *   which the `[key, caseId]` prefix covers. SLAs show the acknowledgement and
 *   response/resolution clocks that state, severity and ack changes move.
 * - CSM_CASES: every list/search/quick-search/dashboard-card variant carries
 *   filter params this hook doesn't have, so it is invalidated by prefix; only
 *   mounted queries refetch, the rest are just marked stale.
 * - CSM_CASE_CHILDREN: keyed by the *parent* id, which this hook doesn't know
 *   (the open case may be a child row on its parent's page), so prefix too.
 */
function invalidateCaseQueries(queryClient: QueryClient, caseId: string): void {
  const byCase = [
    ApiQueryKeys.CSM_CASE_COMMENTS,
    ApiQueryKeys.CSM_CASE_ACTIVITIES,
    ApiQueryKeys.CSM_CASE_DETAIL,
    ApiQueryKeys.CSM_CASE_SLAS,
  ];
  for (const key of byCase) {
    void queryClient.invalidateQueries({ queryKey: [key, caseId] });
  }
  void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.CSM_CASES] });
  void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.CSM_CASE_CHILDREN] });
}

function subscribePageVisibility(onChange: () => void): () => void {
  document.addEventListener("visibilitychange", onChange);
  return () => document.removeEventListener("visibilitychange", onChange);
}

/** Whether the browser tab itself is in the foreground. */
function usePageVisible(): boolean {
  return useSyncExternalStore(
    subscribePageVisibility,
    () => document.visibilityState !== "hidden",
    () => true,
  );
}

/**
 * Opens a live Server-Sent Events connection to csm-portal-backend's
 * `GET /cases/{id}/activities/stream` (its dedicated :9092 listener — see
 * that backend's cmd/server/main.go) and invalidates the case's comments,
 * activities, details, SLA and case-list queries (see invalidateCaseQueries)
 * whenever it emits a `case_updated` event, so the page reflects a new
 * comment or a state/assignee/severity/acknowledgement change without the
 * viewer having to wait out those queries' own staleTime or refresh manually.
 *
 * Uses `@sanity/eventsource` rather than the browser's native `EventSource`
 * because native EventSource cannot set custom headers — it only supports
 * cookies/query params for auth. Like useAuthApiClient.ts, it sends
 * `Authorization: Bearer <access token>` plus `x-user-id-token` and lets the
 * Choreo gateway validate the access token and inject `x-jwt-assertion`
 * (the gateway-signed JWT that backend's `middleware.Auth` verifies against
 * the gateway's JWKS). The hook must never set `x-jwt-assertion` itself: the
 * raw IdP access token is not signed with the gateway key, so the backend
 * rejects it with a token-validation failure. There is no separate
 * ticket/token-exchange step.
 *
 * Headers are fixed at construction time, so they can't be refreshed on the
 * library's own built-in reconnect — a token that expires mid-connection
 * would otherwise have the polyfill retry forever with the same stale
 * header. Instead, `error` closes the current connection and this hook
 * opens a fresh one with newly-fetched tokens after an exponentially
 * backed-off delay (see reconnectDelay), rather than relying on that
 * built-in retry.
 *
 * Token acquisition goes through useAuthTokens — the same recovery path
 * useAuthApiClient uses for every other backend call — rather than calling
 * useAsgardeo() directly: a genuinely dead refresh token then gets the same
 * silent-reauth-then-sign-in-redirect treatment as the rest of the app,
 * instead of this hook just retrying forever with no way to ever recover
 * and no visible signal that anything's wrong.
 *
 * The connection is held only while it can be seen: this page is the active
 * case tab (`useIsCaseTabVisible` — the workspace keeps every opened tab's page
 * mounted, hidden with CSS) AND the browser tab is in the foreground. Each
 * open stream permanently occupies one of the browser's per-host connections
 * (6 on HTTP/1.1), so one stream per mounted hidden tab starved every other
 * request once six case tabs were open. Going inactive closes the stream and
 * cancels any pending reconnect; coming back invalidates the same queries once,
 * because events emitted in the meantime were not delivered.
 *
 * A no-op when `caseId` is unset, `apiConfig.streamEnabled` is false (the
 * feature's master switch, `CSM_PORTAL_STREAM_ENABLED` — defaults off), or
 * `apiConfig.streamUrl` isn't configured (Event Hub — and therefore this
 * endpoint — is optional on the backend); callers fall back to the
 * comments/activities queries' own staleTime.
 */
export function useCaseActivityStream(caseId: string | undefined): void {
  const queryClient = useQueryClient();
  const getTokens = useAuthTokens();
  const logger = useLogger();
  // Both hooks run unconditionally (no `&&` short-circuit between the calls).
  const caseTabVisible = useIsCaseTabVisible();
  const pageVisible = usePageVisible();
  const active = caseTabVisible && pageVisible;
  // True while the stream is deliberately closed (tab hidden / case tab
  // deactivated), so the run that reactivates it knows it missed events.
  const wasInactiveRef = useRef(false);

  useEffect(() => {
    if (!caseId || !apiConfig.streamEnabled || !apiConfig.streamUrl) return;
    if (!active) {
      wasInactiveRef.current = true;
      return;
    }

    if (wasInactiveRef.current) {
      wasInactiveRef.current = false;
      invalidateCaseQueries(queryClient, caseId);
    }

    let cancelled = false;
    let source: EventSourcePolyfill | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    let attempt = 0;

    const scheduleReconnect = (): void => {
      const delay = reconnectDelay(attempt);
      attempt += 1;
      reconnectTimer = setTimeout(() => void connect(), delay);
    };

    const connect = async (): Promise<void> => {
      let token: string | undefined;
      let idToken: string | undefined;
      try {
        ({ token, idToken } = await getTokens());
      } catch (error) {
        logger.debug(
          "[case-activity-stream] failed to get tokens",
          error instanceof Error ? error.message : "Unknown token error",
        );
      }
      if (cancelled) return;
      if (!token || !idToken) {
        scheduleReconnect();
        return;
      }

      const url = `${apiConfig.streamUrl}/cases/${encodeURIComponent(caseId)}/activities/stream`;
      source = new EventSourcePolyfill(url, {
        headers: {
          Authorization: `Bearer ${token}`,
          "x-user-id-token": idToken,
        },
      });

      // A successful connection resets the backoff — only *consecutive*
      // failures should back off, not the cumulative count over the
      // component's whole lifetime.
      source.addEventListener("open", () => {
        attempt = 0;
      });

      source.addEventListener("case_updated", () => {
        invalidateCaseQueries(queryClient, caseId);
      });

      source.addEventListener("error", () => {
        logger.debug("[case-activity-stream] connection error, reconnecting");
        source?.close();
        if (!cancelled) {
          scheduleReconnect();
        }
      });
    };

    void connect();

    return () => {
      cancelled = true;
      clearTimeout(reconnectTimer);
      source?.close();
    };
  }, [caseId, active, queryClient, getTokens, logger]);
}
