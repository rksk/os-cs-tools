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
import { unwrapRedundantBoldTag, stripRedundantBoldWrapper } from "./richTextEditor";

describe("unwrapRedundantBoldTag", () => {
  const parse = (html: string) => new DOMParser().parseFromString(html, "text/html");

  it("unwraps <b><strong>...</strong></b> to just <strong>...</strong>", () => {
    const dom = parse('<p><b><strong class="editor-text-bold">BOLD</strong></b></p>');

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(true);
    expect(dom.querySelector("b")).toBeNull();
    const strong = dom.querySelector("strong");
    expect(strong?.className).toBe("editor-text-bold");
    expect(strong?.textContent).toBe("BOLD");
  });

  it("preserves <i> when bold and italic are combined (italic is class-only once bold wins the base tag)", () => {
    const dom = parse(
      '<p><i><b><strong class="editor-text-bold editor-text-italic">X</strong></b></i></p>',
    );

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(true);
    expect(dom.querySelector("b")).toBeNull();
    // <i> must survive -- it is the only thing conveying italic once posted,
    // since the render view has no CSS rule for `.editor-text-italic`.
    const i = dom.querySelector("i");
    expect(i).not.toBeNull();
    expect(i?.querySelector("strong")).not.toBeNull();
  });

  it("does not touch <u><span class=...>...</span></u> (underline is never bold's base tag)", () => {
    const dom = parse(
      '<p><u><span class="editor-text-underline">X</span></u></p>',
    );

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(false);
    expect(dom.querySelector("u")).not.toBeNull();
  });

  it("does not touch <s><span class=...>...</span></s> (strikethrough is never bold's base tag)", () => {
    const dom = parse(
      '<p><s><span class="editor-text-strikethrough">X</span></s></p>',
    );

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(false);
    expect(dom.querySelector("s")).not.toBeNull();
  });

  it("does not touch a plain italic run (<i><em>...</em></i> stays as-is per this narrow fix)", () => {
    // Note: <i> here IS technically redundant (base tag is already <em>), but
    // this fix deliberately only targets <b> -- see richTextEditor.tsx for why
    // a blanket <i> strip is unsafe when combined with other formats. A plain
    // italic run being left alone is an accepted, documented non-goal, not a
    // bug: it costs nothing (correct either way) and keeps the fix uniform
    // and simple to reason about.
    const dom = parse('<p><i><em class="editor-text-italic">X</em></i></p>');

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(false);
    expect(dom.querySelector("i")).not.toBeNull();
  });

  it("leaves a manually-authored raw <b>text</b> (no nested <strong>) untouched", () => {
    // Guards against over-matching pasted or hand-written HTML that happens to
    // use <b> directly, with no Lexical-generated <strong> inside it.
    const dom = parse("<p><b>raw bold, not from this editor</b></p>");

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(false);
    expect(dom.querySelector("b")).not.toBeNull();
  });

  it("leaves a <b> with multiple children (not just a single <strong>) untouched", () => {
    const dom = parse(
      '<p><b><strong class="editor-text-bold">A</strong><span>B</span></b></p>',
    );

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(false);
    expect(dom.querySelector("b")).not.toBeNull();
  });

  it("handles multiple independent bold runs in the same document", () => {
    const dom = parse(
      '<p><b><strong class="editor-text-bold">A</strong></b> plain <b><strong class="editor-text-bold">B</strong></b></p>',
    );

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(true);
    expect(dom.querySelectorAll("b").length).toBe(0);
    expect(dom.querySelectorAll("strong").length).toBe(2);
  });

  it("returns false on a document with no <b> at all", () => {
    const dom = parse("<p>plain text, no formatting</p>");

    const changed = unwrapRedundantBoldTag(dom);

    expect(changed).toBe(false);
  });
});

describe("stripRedundantBoldWrapper", () => {
  it("strips the wrapper and returns a serialized string", () => {
    const html = '<p><b><strong class="editor-text-bold">BOLD</strong></b></p>';

    const out = stripRedundantBoldWrapper(html);

    expect(out).not.toContain("<b>");
    expect(out).toContain('<strong class="editor-text-bold">BOLD</strong>');
  });

  it("returns the exact same string reference when there is no <b> (fast-path bail)", () => {
    const html = "<p>plain text</p>";

    const out = stripRedundantBoldWrapper(html);

    expect(out).toBe(html);
  });

  it("returns the exact same string reference when a <b> exists but nothing changed", () => {
    const html = "<p><b>raw bold, no nested strong</b></p>";

    const out = stripRedundantBoldWrapper(html);

    expect(out).toBe(html);
  });

  it("performs well on a large document with many alternating formatted runs", () => {
    const RUNS = 5000;
    const parts: string[] = [];
    for (let i = 0; i < RUNS; i++) {
      parts.push(
        `<p>Paragraph ${i} with `,
        `<b><strong class="editor-text-bold">bold ${i}</strong></b> and `,
        `<i><em class="editor-text-italic">italic ${i}</em></i> and `,
        `<u><span class="editor-text-underline">underline ${i}</span></u> and `,
        `<i><b><strong class="editor-text-bold editor-text-italic">both ${i}</strong></b></i> `,
        `plain text padding to make this a genuinely large document. `.repeat(3),
        `</p>`,
      );
    }
    const html = parts.join("");
    expect(html.length).toBeGreaterThan(1_000_000); // multi-MB realistic stress case

    const start = performance.now();
    const out = stripRedundantBoldWrapper(html);
    const elapsedMs = performance.now() - start;

    expect(elapsedMs).toBeLessThan(2000);
    expect(out).not.toContain("<b>");
    // Every combined bold+italic run's <i> must survive.
    expect((out.match(/<i>/g) ?? []).length).toBe(RUNS * 2); // plain italic + combined
    expect((out.match(/<u>/g) ?? []).length).toBe(RUNS);
    expect((out.match(/<strong/g) ?? []).length).toBe(RUNS * 2); // bold-alone + combined
  });
});
