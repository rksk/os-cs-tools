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

import DOMPurify from "dompurify";
import { CaseType } from "@features/support/constants/supportConstants";
import type { AnnouncementFilterValues } from "@features/support/constants/supportConstants";
import type { CaseMetadataResponse } from "@features/support/types/cases";
import type { CaseSearchRequest } from "@features/support/types/cases";
import { mapSeverityToDisplay } from "@features/support/utils/support";
import type { FilterDefinition } from "@components/list-view/ListFiltersPanel";
import type { SortOrder } from "@/types/common";
import {
  AnnouncementFilterMetadataKey,
  AnnouncementSortField,
  type AnnouncementFilterDefinition,
  type AnnouncementFilterOption,
} from "@features/announcements/types/announcements";
import {
  ANNOUNCEMENT_CASE_STATE_ALLOWED_VALUES,
  ANNOUNCEMENTS_CLEAR_FILTERS_LABEL,
} from "@features/announcements/constants/announcementsConstants";
import { formatBackendTimestampForDisplay } from "@utils/dateTime";
import { stripLightModeInlineStyles } from "@/utils/common";

/**
 * Builds the case search payload for the announcements list (announcement case type only).
 *
 * @param filters - Filter field values from the UI.
 * @param searchTerm - Raw search string.
 * @param sortOrder - List sort direction.
 * @param sortField - List sort field.
 * @returns Search request body without pagination (offset/limit passed separately).
 */
export function buildAnnouncementCaseSearchRequest(
  filters: AnnouncementFilterValues,
  searchTerm: string,
  sortOrder: SortOrder,
  sortField: AnnouncementSortField = AnnouncementSortField.UpdatedOn,
): Omit<CaseSearchRequest, "pagination"> {
  return {
    filters: {
      caseTypes: [CaseType.ANNOUNCEMENT],
      statusIds: filters.statusIds?.length ? filters.statusIds.map(Number) : undefined,
      searchQuery: searchTerm.trim() || undefined,
    },
    sortBy: {
      field: sortField,
      order: sortOrder,
    },
  };
}

/**
 * Total page count for list pagination (at least one page when empty).
 *
 * @param totalRecords - API total count.
 * @param pageSize - Page size.
 * @returns Page count (minimum 1).
 */
export function getAnnouncementTotalPages(
  totalRecords: number,
  pageSize: number,
): number {
  return Math.max(1, Math.ceil(totalRecords / pageSize));
}

/**
 * Options for `ListFiltersPanel.resolveOptions` from project filter metadata.
 *
 * @param def - Filter definition (from `ANNOUNCEMENT_FILTER_DEFINITIONS`).
 * @param filterMetadata - Project filters API response.
 * @returns Dropdown options; case-state options restricted to allowed values.
 */
export function resolveAnnouncementListFilterOptions(
  def: FilterDefinition,
  filterMetadata: CaseMetadataResponse | undefined,
): AnnouncementFilterOption[] {
  const raw = filterMetadata?.[def.metadataKey as keyof CaseMetadataResponse];
  if (!Array.isArray(raw)) return [];
  const options: AnnouncementFilterOption[] = raw.map(
    (item: { label: string; id: string }) => ({
      label: item.label,
      value: def.useLabelAsValue ? item.label : item.id,
    }),
  );
  switch (def.metadataKey) {
    case AnnouncementFilterMetadataKey.CaseStates:
      return options.filter((o) =>
        (ANNOUNCEMENT_CASE_STATE_ALLOWED_VALUES as readonly string[]).includes(
          o.value,
        ),
      );
    case AnnouncementFilterMetadataKey.Severities:
      return options;
    default:
      return options;
  }
}

/**
 * Builds select options for `AnnouncementsFilters` (label transforms + case-state filter).
 *
 * @param def - Filter definition entry.
 * @param filterMetadata - Project filters API response.
 * @returns Options for the filter Select.
 */
export function buildAnnouncementsFilterSelectOptions(
  def: AnnouncementFilterDefinition,
  filterMetadata: CaseMetadataResponse | undefined,
): AnnouncementFilterOption[] {
  const metadataOptions = filterMetadata?.[def.metadataKey];
  let options: AnnouncementFilterOption[] = Array.isArray(metadataOptions)
    ? metadataOptions.map((item: { label: string; id: string }) => {
        let label: string;
        switch (def.metadataKey) {
          case AnnouncementFilterMetadataKey.Severities:
            label = mapSeverityToDisplay(item.label);
            break;
          case AnnouncementFilterMetadataKey.CaseStates:
          default:
            label = item.label;
            break;
        }
        return {
          label,
          value: def.useLabelAsValue ? item.label : item.id,
        };
      })
    : [];

  switch (def.metadataKey) {
    case AnnouncementFilterMetadataKey.CaseStates:
      options = options.filter((option) =>
        (ANNOUNCEMENT_CASE_STATE_ALLOWED_VALUES as readonly string[]).includes(
          option.value,
        ),
      );
      break;
    case AnnouncementFilterMetadataKey.Severities:
    default:
      break;
  }

  return options;
}

/**
 * Label for the search bar clear action including active refinement count.
 *
 * @param activeFiltersCount - From `countListSearchAndFilters`.
 * @returns e.g. `Clear Filters (2)`.
 */
export function formatAnnouncementsClearFiltersButtonLabel(
  activeFiltersCount: number,
): string {
  return `${ANNOUNCEMENTS_CLEAR_FILTERS_LABEL} (${activeFiltersCount})`;
}

/** An `<a>` tag that mentions a `target` attribute (deliberately over-inclusive). */
const LINK_WITH_TARGET = /<a\b[^>]*target\s*=/i;

/**
 * Sanitizes an announcement's HTML for display: DOMPurify's default policy (the
 * same one the description and comments used), plus links the author set to
 * open in a new tab keep doing so.
 *
 * Announcements are authored as HTML that may carry `target="_blank"` on its
 * links (a PDF download, a policy page), but DOMPurify's default policy drops
 * `target`, so they opened over the announcement. A `_blank` target is kept
 * (any other value is removed: `_top`, `_parent` and named frames) and always
 * gets `rel="noopener noreferrer"`. This is applied to the sanitised fragment,
 * not through a hook, so it touches announcement HTML only and no other
 * sanitize() call in the app.
 *
 * @param html - Announcement HTML (already normalized where that applies).
 * @returns Safe HTML for `dangerouslySetInnerHTML`.
 */
export function sanitizeAnnouncementHtml(html: string): string {
  // Only a link that carries a target needs anything different; every other
  // announcement gets exactly the sanitiser call it always did.
  if (!LINK_WITH_TARGET.test(html)) return DOMPurify.sanitize(html);
  const fragment = DOMPurify.sanitize(html, { ADD_ATTR: ["target"], RETURN_DOM_FRAGMENT: true });
  // ADD_ATTR allows `target` on every element; only a link may keep one (a form
  // or image map with a target would submit or navigate into another context).
  for (const el of Array.from(fragment.querySelectorAll("[target]"))) {
    if (el.tagName !== "A") el.removeAttribute("target");
  }
  for (const link of Array.from(fragment.querySelectorAll("a"))) {
    if (link.getAttribute("target")?.toLowerCase() === "_blank") {
      link.setAttribute("target", "_blank");
      link.setAttribute("rel", "noopener noreferrer");
    } else {
      link.removeAttribute("target");
    }
  }
  const holder = document.createElement("div");
  holder.appendChild(fragment);
  return holder.innerHTML;
}

/**
 * Normalizes announcement HTML:
 * - Strips pure-white inline backgrounds in dark mode so containers don't clash.
 * - Converts empty `<code>` blocks to a newline (`<br />`) so we do not render useless whitespace.
 * - Converts `<code><n/><code/>`-style placeholders (and bare `<n/>`) into newlines.
 *
 * @param html - Raw HTML from backend.
 * @param isDarkMode - When true, strips pure-white inline backgrounds.
 * @returns Normalized HTML.
 */
export function normalizeAnnouncementDescriptionHtml(
  html: string,
  isDarkMode = false,
): string {
  let normalized = isDarkMode ? stripLightModeInlineStyles(html) : html;

  normalized = normalized.replace(
    /<p>\s*<code>\s*(?:&nbsp;|\s|<br\s*\/?>)*<\/code>\s*<\/p>/gi,
    "<br />",
  );

  normalized = normalized.replace(
    /<code>\s*(?:&nbsp;|\s|<br\s*\/?>)*<\/code>/gi,
    "<br />",
  );

  normalized = normalized.replace(
    /<code>\s*<n\s*\/>\s*<code\s*\/>\s*/gi,
    "<br />",
  );

  normalized = normalized.replace(/<code>\s*<n\s*\/>\s*<\/code>/gi, "<br />");

  normalized = normalized.replace(/<n\s*\/>/gi, "<br />");

  return normalized;
}

/**
 * True when normalized HTML has no visible text (after stripping tags and `<br>`).
 *
 * @param html - Raw or normalized HTML.
 * @returns Whether the description should show the empty placeholder.
 */
export function isAnnouncementDescriptionEffectivelyEmpty(
  html: string,
): boolean {
  const normalizedHtml = normalizeAnnouncementDescriptionHtml(html);
  const normalizedText = normalizedHtml
    .replace(/<br\s*\/?>/gi, "")
    .replace(/&nbsp;/gi, " ")
    .replace(/<[^>]*>/g, "")
    .trim();
  return !normalizedText;
}

/**
 * The label ServiceNow's announcement flow writes ahead of the body in an
 * announcement's origin comment (`<b><u>Description</u></b><br>...`), as it
 * appears once `textForEchoComparison` has reduced the comment to its text.
 */
const ORIGIN_COMMENT_DESCRIPTION_LABEL = "description";

/**
 * Reduces HTML to just its visible characters so two snippets can be compared
 * for "same text" regardless of markup, entity encoding, or where each side
 * happens to put whitespace between blocks (a description's `</p><p>` and a
 * journal entry's newline between the same two paragraphs must not look
 * different).
 */
function textForEchoComparison(html: string): string {
  return html
    .replace(/<br\s*\/?>/gi, "")
    .replace(/&nbsp;/gi, " ")
    .replace(/<[^>]*>/g, "")
    .replace(/&amp;/gi, "&")
    .replace(/&lt;/gi, "<")
    .replace(/&gt;/gi, ">")
    .replace(/&quot;/gi, '"')
    .replace(/&#0?39;/g, "'")
    .replace(/\s+/g, "")
    .toLowerCase();
}

/**
 * True when `commentHtml` is only a repeat of the announcement description:
 * the same text (markup, entity encoding, whitespace and letter case
 * ignored), optionally under the bold "Description" label that
 * ServiceNow's announcement flow puts in front of the body.
 *
 * Creating an announcement case makes ServiceNow write an "origin" comment
 * that mirrors the description, so the details page used to show the same
 * body twice: once as the oldest comment and once as the Description card.
 * A blank description has nothing to echo, so it never counts as echoed (the
 * comment may be the only place the content lives).
 *
 * Deliberately an equality check, not "contains": a comment that quotes the
 * description and then adds something of its own (an update, a correction)
 * must never be treated as a duplicate and hidden. The worst a mismatch can
 * do here is leave the duplicate showing, which is how the page behaved
 * before this check existed.
 *
 * @param descriptionHtml - The announcement's description HTML.
 * @param commentHtml - A comment's HTML (as rendered).
 * @returns Whether the comment is only a repeat of the description.
 */
export function isDescriptionEchoedInComment(
  descriptionHtml: string | null | undefined,
  commentHtml: string | null | undefined,
): boolean {
  const description = textForEchoComparison(descriptionHtml ?? "");
  if (!description || !commentHtml) {
    return false;
  }
  const comment = textForEchoComparison(commentHtml);
  return (
    comment === description ||
    comment === `${ORIGIN_COMMENT_DESCRIPTION_LABEL}${description}`
  );
}

/**
 * Formats an API date string (ISO or ServiceNow-style `YYYY-MM-DD HH:mm:ss`) for
 * list and detail views using the runtime locale.
 *
 * @param raw - Raw timestamp from the API.
 * @returns Formatted date/time or `"--"` when missing or unparsable.
 */
export function formatAnnouncementDateDisplay(
  raw: string | null | undefined,
): string {
  if (raw == null || String(raw).trim() === "") {
    return "--";
  }
  const formatted = formatBackendTimestampForDisplay(String(raw), {
    dateStyle: "medium",
    timeStyle: "short",
  });
  return formatted ?? "--";
}
