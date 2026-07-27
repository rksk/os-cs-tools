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
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { describe, expect, it, vi } from "vitest";
import CloseChatConfirmDialog from "../CloseChatConfirmDialog";

describe("CloseChatConfirmDialog", () => {
  it("fires onConfirm and onCancel from the dialog actions", () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    render(
      <ThemeProvider theme={createTheme()}>
        <CloseChatConfirmDialog
          open
          isClosing={false}
          onCancel={onCancel}
          onConfirm={onConfirm}
        />
      </ThemeProvider>,
    );
    expect(screen.getByText("Close this chat?")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /close chat/i }));
    expect(onConfirm).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("disables the actions while a close is in flight", () => {
    render(
      <ThemeProvider theme={createTheme()}>
        <CloseChatConfirmDialog
          open
          isClosing
          onCancel={vi.fn()}
          onConfirm={vi.fn()}
        />
      </ThemeProvider>,
    );
    expect(screen.getByRole("button", { name: /closing/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /cancel/i })).toBeDisabled();
  });
});
