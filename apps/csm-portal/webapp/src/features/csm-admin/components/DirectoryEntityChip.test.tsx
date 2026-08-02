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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";

const navigateMock = vi.fn();
vi.mock("@hooks/useNavTransition", () => ({
  useNavTransition: () => navigateMock,
}));

import DirectoryEntityChip from "@features/csm-admin/components/DirectoryEntityChip";

describe("DirectoryEntityChip", () => {
  it("navigates to the entity's directory page with the name as router state", () => {
    render(<DirectoryEntityChip id="agent" name="Agent" routeBase="/admin/roles" />);
    fireEvent.click(screen.getByText("Agent"));
    expect(navigateMock).toHaveBeenCalledWith("/admin/roles/agent", {
      state: { name: "Agent" },
    });
  });

  it("stops the click from bubbling to a parent handler (e.g. a clickable table row)", () => {
    const parentOnClick = vi.fn();
    render(
      <div onClick={parentOnClick}>
        <DirectoryEntityChip id="alpha" name="Alpha Team" routeBase="/admin/teams" />
      </div>,
    );
    fireEvent.click(screen.getByText("Alpha Team"));
    expect(navigateMock).toHaveBeenCalledWith("/admin/teams/alpha", {
      state: { name: "Alpha Team" },
    });
    expect(parentOnClick).not.toHaveBeenCalled();
  });

  it("encodes the id in the destination path", () => {
    render(
      <DirectoryEntityChip id="a/b c" name="Weird Id" routeBase="/admin/groups" />,
    );
    fireEvent.click(screen.getByText("Weird Id"));
    expect(navigateMock).toHaveBeenCalledWith("/admin/groups/a%2Fb%20c", {
      state: { name: "Weird Id" },
    });
  });
});
