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

import { useMemo, useState } from "react";
import { Autocomplete, TextField } from "@wso2/oxygen-ui";
import { useQuery } from "@tanstack/react-query";
import { useDebouncedValue } from "@utils/useDebouncedValue";
import type { EntityOption } from "@src/services/changeRequestLookups";

interface AsyncEntitySelectProps {
  label: string;
  placeholder?: string;
  value: EntityOption | null;
  onChange: (next: EntityOption | null) => void;
  disabled?: boolean;
  helperText?: string;
  /** Type-ahead search function, called with the debounced query text (and, for the Service
   * offering field, the currently-picked Service id to narrow by — see `searchExtra`). Must be a
   * stable reference (e.g. `search={changeRequestLookups.groups}`), not an inline arrow function,
   * so the query key below stays stable across renders. */
  search: (query: string, extra?: string) => Promise<EntityOption[]>;
  /** Forwarded as `search`'s second argument. */
  searchExtra?: string;
}

// Mobile counterpart of the webapp's generic AsyncEntitySelect.tsx: same one-component-for-five-
// fields idea (Service / Service offering / Configuration item / Assignment group / Assigned to /
// Requested by all reuse this with their own search function), but follows this app's own simpler
// debounced-search pattern (see ProjectSelect.tsx, LogTimeCardDialog's approver picker) instead of
// the webapp's open-state-gated infinite pagination — a single page of matches is enough for a
// mobile type-ahead field. Owns its own useQuery (keyed by label + query + extra) rather than
// taking a queryOptions factory from the caller, so it doesn't have to fight TanStack's queryKey
// generics across five differently-shaped lookups.
export function AsyncEntitySelect({
  label,
  placeholder,
  value,
  onChange,
  disabled,
  helperText,
  search,
  searchExtra,
}: AsyncEntitySelectProps) {
  const [inputValue, setInputValue] = useState("");
  const debouncedQuery = useDebouncedValue(inputValue, 300);
  const query = debouncedQuery.trim();
  const { data, isFetching, isError } = useQuery({
    queryKey: ["async-entity-select", label, query, searchExtra ?? ""],
    queryFn: () => search(query, searchExtra),
    enabled: query.length > 0,
    staleTime: 60_000,
  });

  const options = useMemo(() => {
    const results = data ?? [];
    if (value && !results.some((o) => o.id === value.id)) return [value, ...results];
    return results;
  }, [data, value]);

  return (
    <Autocomplete
      options={options}
      value={value}
      loading={isFetching}
      disabled={disabled}
      fullWidth
      size="small"
      getOptionLabel={(option) => option.label}
      isOptionEqualToValue={(option, val) => option.id === val.id}
      onChange={(_, next) => onChange(next)}
      onInputChange={(_, next) => setInputValue(next)}
      noOptionsText={
        query.length === 0 ? "Type to search…" : isError ? "Search failed. Try again." : "No matches found"
      }
      renderInput={(params) => (
        <TextField
          {...params}
          label={label}
          placeholder={value ? undefined : placeholder}
          error={isError}
          helperText={isError ? "Search failed." : helperText}
        />
      )}
    />
  );
}
