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

import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProductVulnerabilitiesTable from "@features/security/components/ProductVulnerabilitiesTable";
import { PRODUCT_VULNERABILITIES_FETCH_ALL_REQUEST } from "@features/security/constants/securityConstants";

const mockSearchHook = vi.fn();

vi.mock("@features/security/api/usePostProductVulnerabilitiesSearch", () => ({
  usePostProductVulnerabilitiesSearch: (...args: unknown[]) => mockSearchHook(...args),
}));

vi.mock("@features/security/api/useGetVulnerabilitiesMetaData", () => ({
  useGetVulnerabilitiesMetaData: () => ({ data: undefined }),
}));

describe("ProductVulnerabilitiesTable", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSearchHook.mockReturnValue({
      data: { productVulnerabilities: [], totalRecords: 0 },
      isLoading: false,
      isError: false,
    });
  });

  it("fetches the list with the shared request the details page also uses, with default freshness", () => {
    render(<ProductVulnerabilitiesTable />);

    // The details page reads this same object, so both resolve to one cache entry.
    expect(mockSearchHook.mock.calls[0][0]).toBe(PRODUCT_VULNERABILITIES_FETCH_ALL_REQUEST);
    // The list keeps the default (always-stale) behaviour: no staleTime override.
    expect(mockSearchHook.mock.calls[0][1]).toBeUndefined();
    expect(PRODUCT_VULNERABILITIES_FETCH_ALL_REQUEST).toEqual({
      filters: {},
      pagination: { offset: 0, limit: 5000 },
    });
  });
});
