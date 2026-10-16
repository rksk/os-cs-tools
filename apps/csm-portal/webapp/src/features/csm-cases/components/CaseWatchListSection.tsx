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

import { Box, Button, IconButton, Typography } from "@wso2/oxygen-ui";
import { User, X } from "@wso2/oxygen-ui-icons-react";
import { useMemo, type JSX } from "react";
import { useSearchInternalUsersByName } from "@api/useSearchUsersByName";
import AsyncEntitySelect from "@components/AsyncEntitySelect";
import { useSearchProjectContacts } from "@features/csm-projects/api/useSearchProjectContacts";
import {
  addWatcherEdit,
  eligibleContactWatchers,
  internalUserWatcher,
  removeWatcherEdit,
  watcherKey,
  watcherLabel,
  type WatchListEdits,
  type WatcherCandidate,
} from "@features/csm-cases/utils/createCaseWatchList";

/**
 * Candidates for the add picker: this project's eligible contacts (filtered
 * locally by the typed term) followed by internal users (searched on the
 * server). External users outside the project are never offered. `projectId`
 * arrives through AsyncEntitySelect's `searchExtra`.
 */
function useSearchWatcherCandidates(
  query: string,
  enabled: boolean,
  projectId?: string,
): { data: WatcherCandidate[] | undefined; isFetching: boolean; isError: boolean } {
  const contacts = useSearchProjectContacts(projectId || undefined);
  const internal = useSearchInternalUsersByName(query, enabled);
  const data = useMemo(() => {
    const q = query.trim().toLowerCase();
    const fromContacts = eligibleContactWatchers(contacts.data).filter(
      (c) => !q || c.name.toLowerCase().includes(q) || c.email.toLowerCase().includes(q),
    );
    const known = new Set(fromContacts.map((c) => watcherKey(c.email)));
    const fromInternal = (internal.data ?? [])
      .map(internalUserWatcher)
      .filter((c): c is WatcherCandidate => c !== null && !known.has(watcherKey(c.email)));
    return [...fromContacts, ...fromInternal];
  }, [contacts.data, internal.data, query]);
  return {
    data,
    isFetching: contacts.isFetching || internal.isFetching,
    isError: contacts.isError && internal.isError,
  };
}

const getCandidateId = (c: WatcherCandidate): string => watcherKey(c.email);
const getCandidateLabel = (c: WatcherCandidate): string => watcherLabel(c);

export interface CaseWatchListSectionProps {
  /** Selected project id ("" when none). */
  projectId: string;
  /** The effective list, from `resolveWatchList`. */
  watchers: WatcherCandidate[];
  edits: WatchListEdits;
  onEditsChange: (next: WatchListEdits) => void;
  /** State of the project-contacts fetch that feeds the pre-fill. */
  contacts: { isLoading: boolean; isError: boolean; refetch: () => void };
  /** True once the contacts loaded and none of them is an eligible watcher. */
  noEligibleContacts: boolean;
  disabled?: boolean;
}

/**
 * Watch List for the create-case form. Pre-filled with the project's
 * registered contacts; the engineer removes entries and adds project contacts
 * or internal users. The creator is not added automatically.
 */
export default function CaseWatchListSection({
  projectId,
  watchers,
  edits,
  onEditsChange,
  contacts,
  noEligibleContacts,
  disabled,
}: CaseWatchListSectionProps): JSX.Element {
  const excludeIds = useMemo(() => watchers.map(getCandidateId), [watchers]);

  let status: JSX.Element | null = null;
  if (!projectId) {
    status = (
      <Typography variant="body2" color="text.secondary">
        Select a project to pre-fill the watch list with its contacts.
      </Typography>
    );
  } else if (contacts.isLoading) {
    status = (
      <Typography variant="body2" color="text.secondary">
        Loading project contacts…
      </Typography>
    );
  } else if (contacts.isError) {
    status = (
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
        <Typography variant="body2" color="error.main">
          Could not load the project's contacts.
        </Typography>
        <Button size="small" variant="outlined" onClick={contacts.refetch}>
          Retry
        </Button>
      </Box>
    );
  }

  return (
    <Box component="section" aria-labelledby="case-watch-list-heading">
      <Typography
        id="case-watch-list-heading"
        variant="caption"
        color="text.secondary"
        sx={{ display: "block", mb: 0.5 }}
      >
        Watch list{" "}
        <Typography component="span" variant="caption" color="text.secondary">
          (people notified about this case)
        </Typography>
      </Typography>

      {status}

      {watchers.length === 0 && !status && (
        <Typography variant="body2" color="text.secondary">
          {noEligibleContacts
            ? "This project has no registered contacts to pre-fill. Add watchers below, or leave the list empty."
            : "No watchers. The case will be created without a watch list."}
        </Typography>
      )}

      {watchers.length > 0 && (
        <Box
          component="ul"
          aria-label="Watch list"
          sx={{ listStyle: "none", m: 0, p: 0, display: "flex", flexDirection: "column" }}
        >
          {watchers.map((w) => {
            const label = watcherLabel(w);
            return (
              <Box
                component="li"
                key={watcherKey(w.email)}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  py: 0.5,
                  minWidth: 0,
                  borderBottom: 1,
                  borderColor: "divider",
                  "&:last-of-type": { borderBottom: 0 },
                }}
              >
                <User size={14} />
                <Typography
                  variant="body2"
                  component="span"
                  sx={{ flex: 1, minWidth: 0, overflowWrap: "anywhere" }}
                >
                  {label}
                  {w.kind === "internal" && (
                    <Typography component="span" variant="caption" color="text.secondary">
                      {" "}
                      · internal
                    </Typography>
                  )}
                </Typography>
                <IconButton
                  size="small"
                  aria-label={`Remove ${label} from the watch list`}
                  disabled={disabled}
                  onClick={() => onEditsChange(removeWatcherEdit(edits, w.email))}
                  sx={{ color: "text.secondary" }}
                >
                  <X size={14} />
                </IconButton>
              </Box>
            );
          })}
        </Box>
      )}

      <Box sx={{ mt: 1.5, maxWidth: 480 }}>
        <AsyncEntitySelect<WatcherCandidate>
          id="case-watch-list-add"
          label="Add a watcher"
          placeholder="Search project contacts or internal users…"
          // Held at "" so the field clears itself after each pick.
          value=""
          onChange={(_id, item) => {
            if (item) onEditsChange(addWatcherEdit(edits, item));
          }}
          disabled={disabled || !projectId}
          useSearch={useSearchWatcherCandidates}
          searchExtra={projectId}
          getId={getCandidateId}
          getLabel={getCandidateLabel}
          excludeIds={excludeIds}
          helperText="Offers this project's registered contacts and internal users."
        />
      </Box>
    </Box>
  );
}
