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

import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import UndoToast, { UNDO_MS } from "./UndoToast";

describe("UndoToast", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  const props = () => ({ onDismiss: vi.fn(), onUndone: vi.fn(), onUndoFailed: vi.fn() });

  it("says what was saved, politely", () => {
    render(<UndoToast offer={{ id: 1, message: "Asela: Evening 6-9pm, Tue 22 Sept" }} {...props()} />);
    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("Saved");
    expect(status).toHaveTextContent("Asela: Evening 6-9pm");
    // No way back offered where none is exact.
    expect(screen.queryByRole("button", { name: "Undo" })).not.toBeInTheDocument();
  });

  it("undoes, then says so", async () => {
    const p = props();
    const undo = vi.fn().mockResolvedValue(undefined);
    render(<UndoToast offer={{ id: 1, message: "x", undo }} {...p} />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    });
    expect(undo).toHaveBeenCalledTimes(1);
    expect(p.onUndone).toHaveBeenCalledTimes(1);
  });

  it("reports an undo that failed", async () => {
    const p = props();
    const undo = vi.fn().mockRejectedValue(new Error("nope"));
    render(<UndoToast offer={{ id: 1, message: "x", undo }} {...p} />);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    });
    expect(p.onUndoFailed).toHaveBeenCalledTimes(1);
    expect(p.onUndone).not.toHaveBeenCalled();
  });

  it("goes after a few seconds, but not while the pointer is on it", () => {
    const p = props();
    render(<UndoToast offer={{ id: 1, message: "x", undo: vi.fn() }} {...p} />);
    fireEvent.mouseEnter(screen.getByRole("status"));
    act(() => {
      vi.advanceTimersByTime(UNDO_MS + 100);
    });
    expect(p.onDismiss).not.toHaveBeenCalled();
    fireEvent.mouseLeave(screen.getByRole("status"));
    act(() => {
      vi.advanceTimersByTime(UNDO_MS + 100);
    });
    expect(p.onDismiss).toHaveBeenCalledTimes(1);
  });

  it("stays while focus is on it after the pointer leaves, and the other way round", () => {
    const p = props();
    render(<UndoToast offer={{ id: 1, message: "x", undo: vi.fn() }} {...p} />);
    const status = screen.getByRole("status");
    const later = () =>
      act(() => {
        vi.advanceTimersByTime(UNDO_MS + 100);
      });

    // Pointer on, Tab to Undo, pointer off: focus still holds it.
    fireEvent.mouseEnter(status);
    fireEvent.focus(screen.getByRole("button", { name: "Undo" }));
    fireEvent.mouseLeave(status);
    later();
    expect(p.onDismiss).not.toHaveBeenCalled();

    // Pointer back, focus out: the pointer still holds it.
    fireEvent.mouseEnter(status);
    fireEvent.blur(screen.getByRole("button", { name: "Undo" }));
    later();
    expect(p.onDismiss).not.toHaveBeenCalled();

    // Both gone: it goes after the usual time.
    fireEvent.mouseLeave(status);
    later();
    expect(p.onDismiss).toHaveBeenCalledTimes(1);
  });
});
