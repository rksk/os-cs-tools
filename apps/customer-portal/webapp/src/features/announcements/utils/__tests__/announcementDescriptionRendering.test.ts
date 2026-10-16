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
import {
  normalizeAnnouncementDescriptionHtml,
  sanitizeAnnouncementHtml,
} from "@features/announcements/utils/announcements";

/**
 * The announcement description the CSM portal stores for a bulletin: a styled
 * box (padding, rounded border, gradient) holding one row per advisory, with
 * inline-coloured links. This is the layout ServiceNow-created announcements
 * carry, and the one the CSM portal must now be able to store unchanged.
 */
const BULLETIN_HTML = `<p>Kindly refer to the <strong>Security Advisories</strong> section below.</p>
<p>Please note that only versions affected according to our<a style="color: #ff7300 !important; text-decoration: none;" href="https://example.test/policy" target="_blank" rel="noopener noreferrer"> back porting policy</a> are included.</p>
<p><strong>Security Advisories</strong></p>
<div style="width: 100%;box-sizing: border-box;padding: 15px;border-radius: 12px;border: 1px solid rgba(128,128,128,0.25);background: linear-gradient(160deg, rgba(255,255,255,0.05) 0%, rgba(255,255,255,0) 60%), rgba(128,128,128,0.06);box-shadow: 0 2px 8px rgba(0,0,0,0.12);">
  <div style="margin: 0;">
    <div><span><strong>WSO2-2026-0001: Security Advisory for WSO2 Identity Server</strong></span></div>
    <div>Severity: Medium</div>
    <div><a style="color: #1a8038 !important; text-decoration: underline;" href="https://example.test/a.pdf">Download here</a></div>
  </div>
</div>`;

/** What AnnouncementDetailsPanel does to the description before putting it on the page. */
function renderedDescription(html: string): string {
  return sanitizeAnnouncementHtml(normalizeAnnouncementDescriptionHtml(html));
}

describe("announcement description rendering keeps a styled bulletin intact", () => {
  const out = renderedDescription(BULLETIN_HTML);

  it("keeps the bordered, padded, gradient box and the rows inside it", () => {
    const doc = new DOMParser().parseFromString(out, "text/html");
    const box = [...doc.querySelectorAll("div")].find((d) => /border-radius/.test(d.getAttribute("style") ?? ""));
    expect(box).toBeDefined();
    expect(box?.getAttribute("style")).toContain("padding: 15px");
    expect(box?.getAttribute("style")).toContain("linear-gradient");
    expect(box?.textContent).toContain("WSO2-2026-0001");
    expect(box?.textContent).toContain("Severity: Medium");
  });

  it("keeps every inline-coloured link", () => {
    const doc = new DOMParser().parseFromString(out, "text/html");
    const links = [...doc.querySelectorAll("a")];
    expect(links).toHaveLength(2);
    expect(links.map((a) => a.getAttribute("style"))).toEqual([
      expect.stringContaining("#ff7300"),
      expect.stringContaining("#1a8038"),
    ]);
  });

  it("does not lose any element or inline style", () => {
    const count = (html: string, re: RegExp): number => (html.match(re) ?? []).length;
    expect(count(out, /<div/g)).toBe(count(BULLETIN_HTML, /<div/g));
    expect(count(out, /<span/g)).toBe(count(BULLETIN_HTML, /<span/g));
    expect(count(out, /style="/g)).toBe(count(BULLETIN_HTML, /style="/g));
  });

  it("still removes script and event handlers from announcement content", () => {
    const hostile = renderedDescription('<div style="padding:1px" onclick="alert(1)">x<script>alert(2)</script><img src=x onerror="alert(3)"></div>');
    expect(hostile).not.toMatch(/<script/i);
    expect(hostile).not.toMatch(/onclick|onerror/i);
    expect(hostile).toContain("padding");
  });
});

describe("announcement links the author set to open in a new tab", () => {
  const anchors = (html: string): HTMLAnchorElement[] => [
    ...new DOMParser().parseFromString(html, "text/html").querySelectorAll("a"),
  ];

  it("keep target=_blank and always get rel=noopener noreferrer", () => {
    const [policy] = anchors(renderedDescription(BULLETIN_HTML));
    expect(policy.getAttribute("target")).toBe("_blank");
    expect(policy.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("get rel even when the source had none, or a weaker one", () => {
    const [a, b] = anchors(
      renderedDescription(
        '<a href="https://x.test/a" target="_blank">a</a><a href="https://x.test/b" target="_blank" rel="opener">b</a>',
      ),
    );
    expect(a.getAttribute("rel")).toBe("noopener noreferrer");
    expect(b.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("accept the keyword in any case", () => {
    const [a] = anchors(renderedDescription('<a href="https://x.test" target="_BLANK">a</a>'));
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("leave a link without a target alone", () => {
    const [a] = anchors(renderedDescription('<a href="https://x.test">a</a>'));
    expect(a.hasAttribute("target")).toBe(false);
  });

  it("drop any other target (_top, _parent, a frame name)", () => {
    const links = anchors(
      renderedDescription(
        '<a href="https://x.test/1" target="_top">1</a><a href="https://x.test/2" target="_parent">2</a><a href="https://x.test/3" target="somewhere">3</a>',
      ),
    );
    expect(links).toHaveLength(3);
    for (const link of links) expect(link.hasAttribute("target")).toBe(false);
  });

  it("keep a target on links only, not on forms or image maps", () => {
    const doc = new DOMParser().parseFromString(
      renderedDescription(
        '<a href="https://x.test" target="_blank">a</a><form action="https://x.test" target="_blank"><input name="q"></form>' +
          '<map name="m"><area href="https://x.test" target="_blank" shape="rect" coords="0,0,1,1"></map>',
      ),
      "text/html",
    );
    expect(doc.querySelector("a")?.getAttribute("target")).toBe("_blank");
    expect(doc.querySelectorAll("[target]")).toHaveLength(1);
  });

  it("still remove an unsafe href while keeping the visible text", () => {
    const html = renderedDescription('<a href="javascript:alert(1)" target="_blank">click</a>');
    expect(html).not.toContain("javascript:");
    expect(html).toContain("click");
  });

  it("leave HTML with no link target exactly as the default sanitizer returns it", async () => {
    const { default: DOMPurify } = await import("dompurify");
    for (const html of [
      "plain text & more",
      "a < b and c > d",
      "",
      "<p>Hi &amp; bye</p>",
      '<a href="https://x.test">no target</a>',
      '<div style="padding:15px;border-radius:12px"><span>x</span></div><script>alert(1)</script>',
      "<table><tbody><tr><td>13</td></tr></tbody></table>",
      '<p>the word target in text, <a href="https://x.test">link</a></p>',
    ]) {
      expect(sanitizeAnnouncementHtml(html)).toBe(DOMPurify.sanitize(html));
    }
  });

  it("do not change how the default sanitizer treats target elsewhere", async () => {
    const { default: DOMPurify } = await import("dompurify");
    expect(DOMPurify.sanitize('<a href="https://x.test" target="_blank">a</a>')).not.toContain("target=");
  });
});
