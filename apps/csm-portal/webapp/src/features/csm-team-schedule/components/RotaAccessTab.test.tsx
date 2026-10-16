/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import type { RotaAccess, ScheduleTeam } from "../types";
import type { RotaAccessChange } from "../api/useRotaAccess";
// vi.mock below is hoisted above this import, so the tab gets the mocks.
import RotaAccessTab from "./RotaAccessTab";

const access: { data?: RotaAccess } = {};
const mutate = vi.fn();
const found: { data: { id: string; firstName: string; lastName: string; email: string }[] } = { data: [] };

vi.mock("../api/useRotaAccess", () => ({
  useRotaAccess: () => ({ data: access.data, isLoading: false, isError: false }),
  useChangeRotaAccess: () => ({ mutate, isPending: false }),
}));
vi.mock("@api/useSearchUsersByName", () => ({
  useSearchInternalUsersByName: (q: string, enabled: boolean) => ({ data: enabled && q ? found.data : [], isLoading: false }),
}));
// The search waits a moment for typing to stop; the tests do not.
vi.mock("@hooks/useDebouncedValue", () => ({ useDebouncedValue: <T,>(v: T) => v }));

const ME = "11111111-1111-4111-8111-111111111111";
const ANA = "22222222-2222-4222-8222-222222222222";
const BEN = "33333333-3333-4333-8333-333333333333";
const LEO = "44444444-4444-4444-8444-444444444444";

const member = (userId: string, name: string, isLead = false) => ({
  userId,
  name,
  email: `${name.toLowerCase()}@example.test`,
  isLead,
  role: isLead ? "lead" : "engineer",
});

const TEAMS: ScheduleTeam[] = [
  { key: "apollo", name: "Apollo", family: "SRE", sortOrder: 1, members: [member(LEO, "Leo", true), member(ANA, "Ana"), member(BEN, "Ben")] },
  { key: "lyra", name: "Lyra", family: "CRE", sortOrder: 2, members: [member(ME, "Me")] },
];

const granted = { grantedBy: "head@example.test", grantedOn: "2026-10-01T00:00:00Z", active: true };

function lastChange(): RotaAccessChange {
  return mutate.mock.calls.at(-1)?.[0] as RotaAccessChange;
}

beforeEach(() => {
  mutate.mockReset();
  found.data = [];
  access.data = {
    canManage: { rotaAdmins: true, markerFamilies: ["CRE", "SRE", "SME"] },
    rotaAdmins: [{ family: "SRE", userId: BEN, name: "Ben", email: "ben@example.test", ...granted }],
    markers: [{ teamKey: "apollo", family: "SRE", userId: ANA, name: "Ana", email: "ana@example.test", ...granted }],
  };
});

describe("RotaAccessTab", () => {
  it("lists each group's rota admins, and removes one", () => {
    render(<RotaAccessTab teams={TEAMS} />);
    const sre = screen.getByRole("list", { name: "SRE rota admins" });
    expect(within(sre).getByText("Ben")).toBeInTheDocument();
    expect(screen.getByText("No CRE rota admins")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove Ben as SRE rota admin" }));
    expect(lastChange()).toEqual({ kind: "admin", family: "SRE", userId: BEN, grant: false });
  });

  it("adds a rota admin found by name among internal staff", () => {
    found.data = [{ id: ME, firstName: "Mina", lastName: "Silva", email: "mina@example.test" }];
    render(<RotaAccessTab teams={TEAMS} />);
    fireEvent.change(screen.getByRole("searchbox", { name: "Add a CRE rota admin" }), { target: { value: "min" } });
    fireEvent.click(screen.getByRole("button", { name: /Mina Silva/ }));
    expect(lastChange()).toEqual({ kind: "admin", family: "CRE", userId: ME, grant: true });
  });

  it("names a team's rota marker from its members, leaving out the lead and anyone already one", () => {
    render(<RotaAccessTab teams={TEAMS} />);
    fireEvent.click(screen.getByRole("tab", { name: "SRE" }));
    const pick = screen.getByRole("combobox", { name: "Add rota marker for Apollo" });
    const offered = within(pick)
      .getAllByRole("option")
      .map((o) => o.textContent);
    expect(offered).toEqual(["+ Add rota marker", "Ben"]);

    fireEvent.change(pick, { target: { value: BEN } });
    expect(lastChange()).toEqual({ kind: "marker", teamKey: "apollo", userId: BEN, grant: true });

    fireEvent.click(screen.getByRole("button", { name: "Remove Ana as Apollo rota marker" }));
    expect(lastChange()).toEqual({ kind: "marker", teamKey: "apollo", userId: ANA, grant: false });
  });

  it("ties each group tab to the markers panel it shows", () => {
    render(<RotaAccessTab teams={TEAMS} />);
    const sre = screen.getByRole("tab", { name: "SRE" });
    fireEvent.click(sre);
    const panel = screen.getByRole("tabpanel", { name: "SRE" });
    expect(sre).toHaveAttribute("aria-controls", panel.id);
    expect(within(panel).getByRole("rowheader", { name: "Apollo" })).toBeInTheDocument();
  });

  it("shows a rota admin only their own group's markers, and no rota admin section", () => {
    access.data = { ...access.data!, canManage: { rotaAdmins: false, markerFamilies: ["SRE"] } };
    render(<RotaAccessTab teams={TEAMS} />);
    expect(screen.queryByRole("heading", { name: "Rota admins" })).not.toBeInTheDocument();
    // One group needs no group switch, so no tab panel either.
    expect(screen.queryByRole("tab", { name: "CRE" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tabpanel")).not.toBeInTheDocument();
    expect(screen.getByRole("rowheader", { name: "Apollo" })).toBeInTheDocument();
    expect(screen.queryByRole("rowheader", { name: "Lyra" })).not.toBeInTheDocument();
  });

  it("marks a grant that does nothing any more, so it can be removed", () => {
    access.data!.markers[0] = { ...access.data!.markers[0], active: false };
    render(<RotaAccessTab teams={TEAMS} />);
    fireEvent.click(screen.getByRole("tab", { name: "SRE" }));
    const list = screen.getByRole("list", { name: "Apollo rota markers" });
    expect(within(list).getByText("inactive")).toBeInTheDocument();
  });

  it("says why a change did not save", () => {
    mutate.mockImplementation((_c: RotaAccessChange, opts?: { onError?: (e: Error) => void }) =>
      opts?.onError?.(new Error("only a CSM admin can grant or remove rota admin")),
    );
    render(<RotaAccessTab teams={TEAMS} />);
    fireEvent.click(screen.getByRole("button", { name: "Remove Ben as SRE rota admin" }));
    expect(screen.getByRole("alert")).toHaveTextContent("only a CSM admin can grant or remove rota admin");
  });
});
