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

import { describe, expect, it } from "vitest";
import { BULLETIN_HTML } from "@components/rich-text-editor/bulletinFixture";
import { htmlNeedsSourceEditing } from "@components/rich-text-editor/richHtmlFidelity";

describe("htmlNeedsSourceEditing", () => {
  it("flags the bulletin layout, whose box and link colours the rich editor would drop", () => {
    expect(htmlNeedsSourceEditing(BULLETIN_HTML)).toBe(true);
  });

  it("flags a single coloured link or span, which the rich editor would also drop", () => {
    expect(htmlNeedsSourceEditing('<p><a style="color: red" href="https://example.test">x</a></p>')).toBe(true);
    expect(htmlNeedsSourceEditing('<p><span style="color: #ff7300">x</span></p>')).toBe(true);
  });

  it("flags a styled div on its own", () => {
    expect(htmlNeedsSourceEditing('<div style="padding: 8px">x</div>')).toBe(true);
  });

  it("does not flag what the rich editor writes itself", () => {
    expect(
      htmlNeedsSourceEditing(
        '<p dir="ltr"><b><strong class="editor-text-bold" style="white-space: pre-wrap;">Hello</strong></b></p>',
      ),
    ).toBe(false);
    expect(htmlNeedsSourceEditing('<p style="text-align: center;">centred</p>')).toBe(false);
    expect(htmlNeedsSourceEditing('<p style="text-align: right; white-space: pre-wrap">x</p>')).toBe(false);
  });

  it("does not flag indentation or an image's sizing style, both of which the editor writes itself", () => {
    expect(htmlNeedsSourceEditing('<p style="padding-inline-start: 40px">indented</p>')).toBe(false);
    expect(
      htmlNeedsSourceEditing('<p><img src="https://example.test/x.png" alt="x" style="max-width: 100%; border-radius: 4px; display: block"></p>'),
    ).toBe(false);
  });

  it("flags text-align mixed with anything else the editor does not write", () => {
    expect(htmlNeedsSourceEditing('<p style="text-align: center; color: red">x</p>')).toBe(true);
  });

  it("does not flag table styling, which the editor deliberately keeps across a round trip", () => {
    expect(
      htmlNeedsSourceEditing(
        '<table style="border-collapse: collapse"><tr><td style="background-color: #eee; border: 1px solid #ccc">x</td></tr></table>',
      ),
    ).toBe(false);
  });

  it("still flags a styled element inside a table cell", () => {
    expect(htmlNeedsSourceEditing('<table><tr><td><span style="color: red">x</span></td></tr></table>')).toBe(true);
  });

  it("does not flag plain structure: unstyled divs only become paragraphs, losing no appearance", () => {
    expect(htmlNeedsSourceEditing("<div>one</div><div>two</div>")).toBe(false);
    expect(htmlNeedsSourceEditing("<p>Hello <strong>world</strong></p><ul><li>a</li></ul>")).toBe(false);
  });

  it("does not flag empty or plain-text values", () => {
    expect(htmlNeedsSourceEditing("")).toBe(false);
    expect(htmlNeedsSourceEditing("   ")).toBe(false);
    expect(htmlNeedsSourceEditing("just text, no markup")).toBe(false);
  });

  it("reads a property correctly when its value contains a semicolon inside url()", () => {
    expect(htmlNeedsSourceEditing('<p style="white-space: pre; background: url(data:image/png;base64,AAAA)">x</p>')).toBe(true);
  });

  it("ignores the word style when it is not an attribute", () => {
    expect(htmlNeedsSourceEditing("<p>Use style=... in your CSS</p>")).toBe(false);
  });

  it("matches property names without regard to case and spacing", () => {
    expect(htmlNeedsSourceEditing('<p style="  COLOR :red">x</p>')).toBe(true);
    expect(htmlNeedsSourceEditing('<p style="WHITE-SPACE : pre-wrap">x</p>')).toBe(false);
  });

  it("handles an empty style attribute", () => {
    expect(htmlNeedsSourceEditing('<p style="">x</p>')).toBe(false);
    expect(htmlNeedsSourceEditing('<p style=";;">x</p>')).toBe(false);
  });
});
