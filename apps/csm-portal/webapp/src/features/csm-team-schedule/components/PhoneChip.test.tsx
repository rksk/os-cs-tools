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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import { pagingMember } from "../test/fixtures";
import { ADD_TO_PROFILE_HINT } from "../utils/pagingPhone";
import PhoneChip, { CALLING_LABEL, type PagingPhoneControls } from "./PhoneChip";

const paging = { masked: "+94•••••123", phone: "+94771234123", setBy: "lead@example.com", setAt: "2026-10-01T09:00:00Z" };

function controls(over: Partial<PagingPhoneControls> = {}): PagingPhoneControls {
  return { edit: vi.fn(), remove: vi.fn(), test: vi.fn(), calling: () => false, ...over };
}

describe("PhoneChip", () => {
  it("says the state in words and shows only the masked number", () => {
    render(<PhoneChip member={pagingMember({ membershipId: "a", hasProfilePhone: false, pagingPhone: { ...paging, lastTestStatus: "busy" } })} />);
    expect(screen.getByText(/Alert number · test failed \(busy\)/)).toBeInTheDocument();
    expect(screen.getByText(/\+94•••••123/)).toBeInTheDocument();
    expect(screen.queryByText(/\+94771234123/)).not.toBeInTheDocument();
  });

  it("offers no actions to a reader who may not edit the number", () => {
    render(<PhoneChip member={pagingMember({ membershipId: "a", hasProfilePhone: false, canEditPhone: false })} controls={controls()} />);
    expect(screen.getByText("No number")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("says to add a missing number on the profile, and offers no way to add one here", () => {
    const m = pagingMember({ membershipId: "a", name: "Jane Doe", hasProfilePhone: false, canEditPhone: true });
    render(<PhoneChip member={m} controls={controls()} />);
    expect(screen.getByText(ADD_TO_PROFILE_HINT)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("shows nothing when the profile has a number", () => {
    const { container } = render(
      <PhoneChip member={pagingMember({ membershipId: "a", hasProfilePhone: true, canEditPhone: true })} controls={controls()} />,
    );
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText(/Profile/)).not.toBeInTheDocument();
  });

  it("keeps an older paging number's actions beside a profile number, without a status", () => {
    const m = pagingMember({ membershipId: "a", name: "Jane Doe", hasProfilePhone: true, canEditPhone: true, pagingPhone: paging });
    render(<PhoneChip member={m} controls={controls()} />);
    expect(screen.getByRole("button", { name: "Remove Jane Doe's alert number" })).toBeInTheDocument();
    expect(screen.queryByText(/Alert number/)).not.toBeInTheDocument();
  });

  it("offers Remove and Test call on an existing paging number, not Change, and says Calling while a test is awaited", () => {
    const c = controls();
    const m = pagingMember({ membershipId: "a", name: "Jane Doe", hasProfilePhone: false, canEditPhone: true, pagingPhone: paging });
    const { rerender } = render(<PhoneChip member={m} controls={c} />);
    expect(screen.queryByRole("button", { name: /Change/ })).not.toBeInTheDocument();
    expect(screen.queryByText(ADD_TO_PROFILE_HINT)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Remove Jane Doe's alert number" }));
    fireEvent.click(screen.getByRole("button", { name: "Test call Jane Doe's alert number" }));
    expect(c.remove).toHaveBeenCalledWith(m);
    expect(c.test).toHaveBeenCalledTimes(1);

    rerender(<PhoneChip member={m} controls={{ ...c, calling: () => true }} />);
    const calling = screen.getByRole("button", { name: new RegExp(CALLING_LABEL) });
    expect(calling).toHaveTextContent(CALLING_LABEL);
    expect(calling).toHaveAttribute("aria-disabled", "true");
    fireEvent.click(calling);
    expect(c.test).toHaveBeenCalledTimes(1);
  });
});
