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

import "@testing-library/jest-dom/vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import CustomerHealthDetailTable from "./CustomerHealthDetailTable";
import type { AccountDetail } from "../api/customerHealthTypes";

vi.mock("./ReviewStatusCell", () => ({ default: () => <div /> }));
vi.mock("./ActionItemsSection", () => ({ default: () => <div /> }));

const mockMutate = vi.fn();
vi.mock("../api/useAccountHealthDetail", () => ({
  useAccountHealthDetail: () => ({ data: mockAccountData, isLoading: false, isError: false, error: undefined }),
  useAccountHealthStatus: () => ({ data: [], refetch: vi.fn() }),
  useInitHealthTracking: () => ({ mutate: mockMutate }),
  isNotFoundError: () => false,
}));

let mockAccountData: AccountDetail;

function baseProject(overrides: Partial<AccountDetail["customerProjects"][number]> = {}) {
  return {
    sysId: "proj-1",
    name: "ODIDO - Subscription",
    hasRecentCases: true,
    detailedRecentCases: [],
    totalRecentCases: 5,
    hasAbandonedCases: false,
    detailedAbandonedCases: [],
    isUsingEolProduct: false,
    softwareModel: [],
    hasMigrationDelays: false,
    detailedMigrationDelays: [],
    hasEscalatedCases: false,
    detailedEscalatedCases: [],
    goLiveStatus: { status: "Live", isRisk: false },
    ...overrides,
  };
}

describe("CustomerHealthDetailTable — detail list bullets", () => {
  it("marks each abandoned-migration case with a bullet, not a bare line of text", () => {
    mockAccountData = {
      accountName: "Example Corp",
      customerProjects: [
        baseProject({
          hasAbandonedCases: true,
          detailedAbandonedCases: [{ sysId: "c1", number: "CS0345735" }],
        }),
      ],
    };
    render(<CustomerHealthDetailTable accountId="acc-1" />);

    fireEvent.click(screen.getByText("Risk Indicators"));
    const row = screen.getByText("Abandoned Product Migrations").closest("tr")!;
    fireEvent.click(within(row).getByRole("button"));

    expect(within(row).getByText("CS0345735")).toBeInTheDocument();
    expect(within(row).getByTestId("risk-detail-bullet")).toBeInTheDocument();
  });

  it("marks each migration-delay case and each escalation with the same bullet", () => {
    mockAccountData = {
      accountName: "Example Corp",
      customerProjects: [
        baseProject({
          hasMigrationDelays: true,
          detailedMigrationDelays: [{ sysId: "c2", number: "CS0999999" }],
          hasEscalatedCases: true,
          detailedEscalatedCases: [{ escalationSysId: "e1", escalationNumber: "ESC001" }],
        }),
      ],
    };
    render(<CustomerHealthDetailTable accountId="acc-1" />);
    fireEvent.click(screen.getByText("Risk Indicators"));

    const delaysRow = screen.getByText("Migration Delays").closest("tr")!;
    fireEvent.click(within(delaysRow).getByRole("button"));
    expect(within(delaysRow).getByText("CS0999999")).toBeInTheDocument();
    expect(within(delaysRow).getByTestId("risk-detail-bullet")).toBeInTheDocument();

    const escalationsRow = screen.getByText("Recent Escalations (3mo)").closest("tr")!;
    fireEvent.click(within(escalationsRow).getByRole("button"));
    expect(within(escalationsRow).getByText("ESC001")).toBeInTheDocument();
    expect(within(escalationsRow).getByTestId("risk-detail-bullet")).toBeInTheDocument();
  });
});

describe("CustomerHealthDetailTable — EOL product detail", () => {
  it("renders a short calendar date, not the raw ISO instant, in the EOL chip", () => {
    mockAccountData = {
      accountName: "Example Corp",
      customerProjects: [
        baseProject({
          isUsingEolProduct: true,
          softwareModel: [
            {
              name: "WSO2 Identity Server 5.10.0",
              eolDate: "2026-03-25T00:00:00Z",
              deployments: [{ sysId: "d1", name: "Production" }],
            },
          ],
        }),
      ],
    };
    render(<CustomerHealthDetailTable accountId="acc-1" />);
    fireEvent.click(screen.getByText("Risk Indicators"));

    const row = screen.getByText("Using EOL Products").closest("tr")!;
    fireEvent.click(within(row).getByRole("button"));

    expect(within(row).getByText("WSO2 Identity Server 5.10.0")).toBeInTheDocument();
    expect(within(row).queryByText((_, el) => el?.textContent === "EOL: 2026-03-25T00:00:00Z")).not.toBeInTheDocument();
    expect(within(row).getByText(/^EOL: /)).toHaveTextContent("EOL: Mar 25, 2026");
  });

  it("falls back to the raw string for an eolDate that doesn't parse", () => {
    mockAccountData = {
      accountName: "Example Corp",
      customerProjects: [
        baseProject({
          isUsingEolProduct: true,
          softwareModel: [{ name: "Some Product", eolDate: "not-a-date", deployments: [] }],
        }),
      ],
    };
    render(<CustomerHealthDetailTable accountId="acc-1" />);
    fireEvent.click(screen.getByText("Risk Indicators"));
    const row = screen.getByText("Using EOL Products").closest("tr")!;
    fireEvent.click(within(row).getByRole("button"));
    expect(within(row).getByText("EOL: not-a-date")).toBeInTheDocument();
  });
});

describe("CustomerHealthDetailTable — project column width", () => {
  it("caps the project column's data cell to the same width as its header cell", () => {
    mockAccountData = {
      accountName: "Example Corp",
      customerProjects: [baseProject()],
    };
    render(<CustomerHealthDetailTable accountId="acc-1" />);
    fireEvent.click(screen.getByText("Risk Indicators"));

    const table = screen.getByRole("table", { name: "customer health detail table" });
    const headerCell = within(table).getByText("ODIDO - Subscription").closest("th")!;
    const headerStyle = getComputedStyle(headerCell);

    const row = screen.getByText("Cases Within Past 6 Months").closest("tr")!;
    const dataCell = within(row).getAllByRole("cell")[0];
    const dataStyle = getComputedStyle(dataCell);

    expect(headerStyle.maxWidth).toBe("250px");
    expect(dataStyle.maxWidth).toBe("250px");
  });
});
