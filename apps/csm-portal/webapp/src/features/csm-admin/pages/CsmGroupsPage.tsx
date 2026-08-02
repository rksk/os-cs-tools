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

import { Box, Typography } from "@wso2/oxygen-ui";
import { useMemo, useState, type JSX } from "react";
import DirectoryEntityTable from "@features/csm-admin/components/DirectoryEntityTable";
import { useSearchGroupsPage } from "@features/csm-admin/api/useSearchGroupsPage";
import { useDebouncedValue } from "@hooks/useDebouncedValue";
import type { BeGroupSearchPayload } from "@api/backend/types";

const DEFAULT_ROWS_PER_PAGE = 20;

/**
 * Assignment groups from the backing data source (`POST /groups/search`) —
 * a live query, unlike the curated role/team catalogues, so this list can be
 * much larger. Each row is click-through to that group's member list.
 */
export default function CsmGroupsPage(): JSX.Element {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(DEFAULT_ROWS_PER_PAGE);
  const debouncedSearch = useDebouncedValue(search, 300);

  const request = useMemo<BeGroupSearchPayload>(
    () => ({
      filters: debouncedSearch.trim() ? { searchQuery: debouncedSearch.trim() } : undefined,
      pagination: { limit: rowsPerPage, offset: page * rowsPerPage },
    }),
    [debouncedSearch, page, rowsPerPage],
  );

  const { data, isLoading, isFetching, isError, error } = useSearchGroupsPage(request);

  const handleSearchChange = (value: string): void => {
    setSearch(value);
    setPage(0);
  };

  const handleRowsPerPageChange = (value: number): void => {
    setRowsPerPage(value);
    setPage(0);
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Typography variant="body2" color="text.secondary">
        Assignment groups from the backing data source. Select a group to see its members.
      </Typography>

      <DirectoryEntityTable
        entityNounPlural="groups"
        memberBasePath="/admin/groups"
        rows={data?.groups ?? []}
        total={data?.total ?? 0}
        isLoading={isLoading}
        isFetching={isFetching}
        isError={isError}
        error={error}
        search={search}
        onSearchChange={handleSearchChange}
        page={page}
        onPageChange={setPage}
        rowsPerPage={rowsPerPage}
        onRowsPerPageChange={handleRowsPerPageChange}
      />
    </Box>
  );
}
