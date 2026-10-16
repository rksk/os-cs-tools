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

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useBackendApi } from "@api/backend/client";
import type {
  BeProjectSearchPayload,
  BeProjectSearchResponse,
  BeSubscriptionType,
} from "@api/backend/types";

/** Matches the entity service's own per-page ceiling (see that repo's `maxLimit`). */
const AUDIENCE_PAGE_LIMIT = 50;

/**
 * Safety bound against a wrong/always-true `hasMore`, mirroring the paged-loop
 * convention used elsewhere in this app (e.g. `useGetCsmCaseComments`). 100
 * pages * 50 rows/page = 5,000 projects, comfortably above the real scale
 * this audience resolves to (a couple of thousand projects).
 */
const MAX_AUDIENCE_PAGES = 100;

export interface AudienceFilters {
  /** e.g. ["Restricted", "Suspended"]. */
  excludeClosureStates: string[];
  excludeSubscriptionTypes: BeSubscriptionType[];
}

/**
 * `key`/`accountName` are optional so this shape can also carry the
 * EOL/product-version flow's resolved audience (`BeEntityRef`-only — id/name,
 * no key or account) through the same {@link ResolvedAudienceList}, rather
 * than duplicating that component for a narrower shape.
 */
export interface ResolvedAudienceProject {
  id: string;
  name: string;
  key?: string;
  accountName?: string | null;
}

export interface ResolvedAudience {
  /**
   * Every project fetched so far. While {@link ResolvedAudience.isLoading} is
   * true this is only a prefix of the audience (rows are published as each
   * page arrives); it is the complete, definitive list only once `isLoading`
   * is false. Always empty when `isError` — a failed resolution never exposes
   * a truncated list as if it were the audience.
   */
  projects: ResolvedAudienceProject[];
  /** `projects.length` — definitive only once `isLoading` is false. */
  total: number;
  /**
   * True until the audience is fully resolved — including while a partial list
   * is already available. Anything that acts on the audience (submit gating)
   * must key off this, not off `projects.length`.
   */
  isLoading: boolean;
  /**
   * True while at least one page has arrived and the rest are still being
   * fetched, so a caller can show the rows it has plus a "loading more"
   * indicator instead of a blank spinner. A background refresh of an
   * already-resolved list is `isLoading` but not `isLoadingMore`.
   */
  isLoadingMore: boolean;
  /**
   * The server-reported match count (from the latest page), so progress can
   * read "150 of 1,956". Null until a page has arrived, and whenever the
   * server's figure can't be trusted to be at least what is already loaded.
   */
  expectedTotal: number | null;
  isError: boolean;
  error: Error | null;
}

/**
 * What the query holds: while the walk is under way a prefix of the audience
 * (`complete: false`), once it ends the whole thing (`complete: true`).
 */
interface AudienceSnapshot {
  projects: ResolvedAudienceProject[];
  expectedTotal: number | null;
  complete: boolean;
}

const NO_PROJECTS: ResolvedAudienceProject[] = [];

/**
 * Resolves the full "all customer projects" audience for an announcement:
 * pages through every match of `POST /announcements/audience/search` under
 * the given exclusion filters (there is no single call that returns
 * "everything" — see `useProjectSearch.ts`'s own paged hooks for the same
 * shape) and returns the complete, definitive list plus its count.
 *
 * The full audience is a couple of thousand projects, i.e. dozens of
 * sequential requests, so the rows that have arrived are published after each
 * page (`projects` grows page by page) instead of only once the last one
 * lands — otherwise the sender stares at a spinner for the whole walk.
 * `isLoading` stays true until the final page is in, so nothing that depends
 * on the *whole* audience (submit, CSV export) can act on a prefix.
 *
 * The walk itself stays one loop inside the query function rather than being
 * driven page by page from a React effect: the next request is sent the
 * moment the previous one returns, and rendering the rows overlaps that
 * request's network time instead of delaying it.
 *
 * This dedicated endpoint — not the general `/projects/search` also used by
 * the type-ahead pickers in `useProjectSearch.ts` — additionally, always,
 * excludes a backend-configured denylist of project keys that must never
 * receive an automated customer announcement (mirrors a hardcoded condition
 * in the real ServiceNow flow this replaces). The caller cannot see or
 * override that list; it's applied unconditionally server-side, the same
 * way the EOL/product-version flow's own mandatory exclusion is.
 *
 * Unlike the type-ahead pickers in `useProjectSearch.ts`, this is meant to
 * be enabled only once the engineer has actually chosen the "All customer
 * projects" scope — resolving the whole audience on every keystroke of a
 * narrower search would be wasteful and is never what that scope needs.
 */
export function useResolveAnnouncementAudience(
  enabled: boolean,
  filters: AudienceFilters,
): ResolvedAudience {
  const api = useBackendApi();
  const queryClient = useQueryClient();
  const queryKey = [ApiQueryKeys.CSM_PROJECTS, "resolve-announcement-audience", filters];

  const query = useQuery<AudienceSnapshot, Error>({
    queryKey,
    queryFn: async (): Promise<AudienceSnapshot> => {
      const collected: ResolvedAudienceProject[] = [];
      let expectedTotal: number | null = null;
      let offset = 0;

      // Publish a growing prefix only while there is no complete list on
      // screen yet. A background refresh of an already-resolved audience
      // keeps showing the old complete list until the new one is ready,
      // rather than shrinking it to the first page and growing it back.
      const existing = queryClient.getQueryData<AudienceSnapshot>(queryKey);
      const publishPartial = existing === undefined || !existing.complete;

      for (let page = 0; ; page++) {
        // Fail loudly rather than truncate: silently returning `collected`
        // here would report an incomplete list as the complete, definitive
        // audience (the caller derives `total` directly from its length) —
        // an announcement could then under-count real recipients with no
        // indication anything was cut off. Mirrors the same bound-and-throw
        // convention the entity service's own paged-exclude-filter loop
        // uses (see sn_project_service.go's fetchAllProjectsFiltered).
        if (page === MAX_AUDIENCE_PAGES) {
          throw new Error(
            `Too many matching projects to resolve the audience safely (exceeded ${MAX_AUDIENCE_PAGES} pages of ${AUDIENCE_PAGE_LIMIT}) — narrow the exclusion filters and try again.`,
          );
        }
        const res = await api.post<BeProjectSearchPayload, BeProjectSearchResponse>(
          "/announcements/audience/search",
          {
            pagination: { offset, limit: AUDIENCE_PAGE_LIMIT },
            excludeClosureStates:
              filters.excludeClosureStates.length > 0 ? filters.excludeClosureStates : undefined,
            excludeSubscriptionTypes:
              filters.excludeSubscriptionTypes.length > 0
                ? filters.excludeSubscriptionTypes
                : undefined,
          },
        );
        const page_ = res.projects ?? [];
        for (const p of page_) {
          collected.push({
            id: p.id,
            name: p.name ?? p.id,
            key: p.key ?? "",
            accountName: p.account?.name ?? null,
          });
        }
        if (typeof res.total === "number") expectedTotal = res.total;
        // Stop if the last page came back empty (offset wouldn't advance) or
        // the backend says there's nothing more.
        if (!res.hasMore || page_.length === 0) break;
        offset += page_.length;

        // `setQueryData` leaves the fetch running (`isFetching` stays true),
        // so this only makes the rows so far visible; the final return value
        // below replaces it.
        if (publishPartial) {
          queryClient.setQueryData<AudienceSnapshot>(queryKey, {
            projects: [...collected],
            expectedTotal,
            complete: false,
          });
        }
      }

      return { projects: collected, expectedTotal, complete: true };
    },
    enabled,
    staleTime: 30_000,
  });

  const snapshot = query.data;
  // After a failed attempt the cache can still hold the prefix published
  // before the failure: never expose it as the audience.
  const failed = query.isError;
  const projects = failed || !snapshot ? NO_PROJECTS : snapshot.projects;
  // A prefix always reads as "still loading", whatever the fetch state — it
  // must never be mistaken for the finished audience.
  const isPartial = !failed && snapshot !== undefined && !snapshot.complete;
  const isLoading = query.isFetching || isPartial;
  // A refresh of an already-complete list is loading, but not "loading more":
  // the rows on screen aren't a prefix of anything.
  const isLoadingMore = isLoading && isPartial && projects.length > 0;
  const reportedTotal = snapshot?.expectedTotal ?? null;

  return {
    projects,
    total: projects.length,
    isLoading,
    isLoadingMore,
    expectedTotal: reportedTotal !== null && reportedTotal >= projects.length ? reportedTotal : null,
    isError: failed,
    error: query.error ?? null,
  };
}
