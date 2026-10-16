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

import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import CustomerHealthFilters, { type HealthFilters } from "./CustomerHealthFilters";

vi.mock("@hooks/useIdTokenClaims", () => ({
  useIdTokenClaims: () => ({ email: "viewer@example.com" }),
}));
// The sibling filter-select components each pull in their own data hooks
// (product/region/team options) -- mocked out here to a plain no-op stand-in
// so this test can isolate the health-status dropdown this change actually
// touches, same "mock the hook/component you're not testing" convention this
// codebase already uses elsewhere (see CLAUDE.md's Testing section).
vi.mock("./CustomerHealthToggle", () => ({ default: () => null }));
vi.mock("./RiskIndicatorSelect", () => ({ default: () => null }));
vi.mock("./RegionFilterSelect", () => ({ default: () => null }));
vi.mock("./ProductFilterSelect", () => ({ default: () => null }));
vi.mock("./AbtTeamFilterSelect", () => ({ default: () => null }));

const baseFilters: HealthFilters = {
  accountScope: "all-accounts",
  userEmail: null,
  riskIndicators: [],
  region: [],
  product: null,
  abtTeam: null,
  healthStatus: null,
};

describe("CustomerHealthFilters health-status dropdown", () => {
  // Every sibling filter-select is mocked to null above, so the health-status
  // dropdown is the only combobox CustomerHealthFilters renders here -- a
  // plain, unnamed getByRole("combobox") is therefore unambiguous.
  it("offers To Be Reviewed alongside At Risk and Healthy", () => {
    render(<CustomerHealthFilters currentFilters={baseFilters} onFilterChange={vi.fn()} />);

    fireEvent.mouseDown(screen.getByRole("combobox"));

    expect(screen.getByRole("option", { name: "At Risk" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Healthy" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "To Be Reviewed" })).toBeInTheDocument();
  });

  it("reports healthStatus: \"to_be_reviewed\" when that option is chosen", () => {
    const onFilterChange = vi.fn();
    render(<CustomerHealthFilters currentFilters={baseFilters} onFilterChange={onFilterChange} />);

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "To Be Reviewed" }));

    expect(onFilterChange).toHaveBeenCalledWith({ ...baseFilters, healthStatus: "to_be_reviewed" });
  });
});
