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

import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import "@testing-library/jest-dom/vitest";
import HtmlSourcePreview from "@components/rich-text-editor/HtmlSourcePreview";
import { BULLETIN_HTML } from "@components/rich-text-editor/bulletinFixture";

afterEach(() => {
  document.documentElement.removeAttribute("data-color-scheme");
});

const preview = (): HTMLElement => screen.getByTestId("html-source-preview");

describe("HtmlSourcePreview", () => {
  it("renders the styled bulletin with its box, rows and coloured links", () => {
    render(<HtmlSourcePreview html={BULLETIN_HTML} />);

    const box = [...preview().querySelectorAll("div")].find((d) => /border-radius/.test(d.getAttribute("style") ?? ""));
    expect(box).toBeDefined();
    expect(box?.textContent).toContain("WSO2-2026-0001");
    expect(box?.textContent).toContain("Severity: Medium");
    const links = [...preview().querySelectorAll("a")];
    expect(links.map((a) => a.getAttribute("style"))).toEqual([
      expect.stringContaining("#ff7300"),
      expect.stringContaining("#1a8038"),
    ]);
  });

  it("removes scripts, event handlers and javascript: links, as it does for customers", () => {
    render(
      <HtmlSourcePreview html={'<p onclick="alert(1)">x</p><script>alert(2)</script><a href="javascript:alert(3)">y</a><img src=x onerror="alert(4)">'} />,
    );
    expect(preview().innerHTML).not.toMatch(/<script/i);
    expect(preview().innerHTML).not.toMatch(/onclick|onerror/i);
    expect(preview().innerHTML).not.toMatch(/javascript:/i);
    expect(preview()).toHaveTextContent("x");
  });

  it("shows nothing when there is no HTML yet", () => {
    const { container } = render(<HtmlSourcePreview html="   " />);
    expect(container).toBeEmptyDOMElement();
  });

  it("follows the HTML it is given", () => {
    const { rerender } = render(<HtmlSourcePreview html="<p>first</p>" />);
    expect(preview()).toHaveTextContent("first");
    rerender(<HtmlSourcePreview html="<p>second</p>" />);
    expect(preview()).toHaveTextContent("second");
    expect(preview()).not.toHaveTextContent("first");
  });

  it("leaves light backgrounds as written in dark mode, so it matches the announcement view", () => {
    document.documentElement.setAttribute("data-color-scheme", "dark");
    render(<HtmlSourcePreview html={'<div style="background: #ffffff; border: 1px solid #888; padding: 4px">x</div>'} />);
    const styled = preview().querySelector("div") as HTMLElement;
    expect(styled.getAttribute("style")).toMatch(/background/i);
    expect(styled.getAttribute("style")).toMatch(/border/i);
  });
});
