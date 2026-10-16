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
import {
  absorbStrayText,
  adjacentBreak,
  adoptSlotText,
  applyTextEdits,
  breakSlotAt,
  caretAtSourcePos,
  buildTextMap,
  decodeHtmlText,
  encodeHtmlText,
  findLineBreaks,
  findTextRuns,
  insertLineBreakAtSlot,
  insertLineBreakInText,
  isTextOnlyEdit,
  removeLineBreak,
  type TextMap,
} from "@components/rich-text-editor/htmlTextEditing";
import { sanitizeRichTextHtml } from "@utils/sanitizeHtml";

const runsOf = (html: string): string[] => findTextRuns(html).map((r) => html.slice(r.start, r.end));

/** Render `source` the way the preview does, and pair it with the source. */
function previewOf(source: string): { host: HTMLElement; map: TextMap | null } {
  const host = document.createElement("div");
  host.innerHTML = sanitizeRichTextHtml(source);
  document.body.appendChild(host);
  return { host, map: buildTextMap(source, host) };
}

function textNodeContaining(host: HTMLElement, fragment: string): Text {
  const walker = document.createTreeWalker(host, NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if ((n.nodeValue ?? "").includes(fragment)) return n as Text;
  }
  throw new Error(`no text node containing "${fragment}"`);
}

describe("findTextRuns", () => {
  it("finds the text between tags, in order", () => {
    expect(runsOf("<p>hello <b>world</b>!</p>")).toEqual(["hello ", "world", "!"]);
  });

  it("returns offsets that slice back to exactly the text", () => {
    const html = '<div class="x">one</div>\n<div>two</div>';
    for (const run of findTextRuns(html)) expect(html.slice(run.start, run.end)).not.toContain("<");
  });

  it("does not treat a > inside a quoted attribute value as the end of the tag", () => {
    expect(runsOf('<a title="a > b" href=\'x>y\'>link</a>')).toEqual(["link"]);
  });

  it("skips comments, including markup inside them", () => {
    expect(runsOf("<p>a<!-- <b>hidden</b> -->b</p>")).toEqual(["a", "b"]);
  });

  it("skips doctype declarations", () => {
    expect(runsOf("<!DOCTYPE html><p>x</p>")).toEqual(["x"]);
  });

  it("skips the content of script and style elements", () => {
    expect(runsOf("<p>a</p><script>if (a < b) { x(); }</script><style>p > a { color: red }</style><p>b</p>")).toEqual(["a", "b"]);
  });

  it("matches script and style case-insensitively", () => {
    expect(runsOf("<p>a</p><SCRIPT>var x = 1;</SCRIPT><p>b</p>")).toEqual(["a", "b"]);
  });

  it("keeps a < that does not start a tag as part of the text", () => {
    expect(runsOf("<p>1 < 2 and 3 > 2</p>")).toEqual(["1 < 2 and 3 > 2"]);
  });

  it("handles uppercase and self-closing tags", () => {
    expect(runsOf("<P>Hello<BR/>there</P>")).toEqual(["Hello", "there"]);
  });

  it("keeps whitespace-only runs, which the caller filters out", () => {
    expect(runsOf("<div>\n  <p>a</p>\n</div>")).toEqual(["\n  ", "a", "\n"]);
  });

  it("copes with a tag that is never closed", () => {
    expect(runsOf("<p>text<b")).toEqual(["text"]);
  });

  it("finds no text in plain markup with none", () => {
    expect(findTextRuns("<br><hr>")).toEqual([]);
  });

  it("treats the whole string as text when it has no tags", () => {
    expect(runsOf("just text")).toEqual(["just text"]);
  });
});

describe("decodeHtmlText / encodeHtmlText", () => {
  it("decodes entities and normalises line endings", () => {
    expect(decodeHtmlText("Fish &amp; chips &copy; 2026&nbsp;ok\r\nnext")).toBe("Fish & chips © 2026 ok\nnext");
  });

  it("escapes only what must be escaped, and writes a non-breaking space as an entity", () => {
    expect(encodeHtmlText("A & B <c>  d © é")).toBe("A &amp; B &lt;c&gt; &nbsp;d © é");
  });

  it("round-trips arbitrary text", () => {
    const text = "a & b < c > d   e \"q\" 'r'";
    expect(decodeHtmlText(encodeHtmlText(text))).toBe(text);
  });

  it("does not execute markup it is given", () => {
    expect(decodeHtmlText("&lt;img src=x onerror=alert(1)&gt;")).toBe("<img src=x onerror=alert(1)>");
  });
});

describe("buildTextMap", () => {
  it("pairs every visible piece of the styled bulletin with its text node", () => {
    const { map } = previewOf(BULLETIN_HTML);
    expect(map).not.toBeNull();
    expect(map?.entries.map((e) => e.text.trim())).toEqual([
      "Kindly refer to the",
      "Security Advisories",
      "section below.",
      "Please note that only versions affected according to our",
      "back porting policy",
      "are included.",
      "Security Advisories",
      "WSO2-2026-0001: Security Advisory for WSO2 Identity Server",
      "Severity: Medium",
      "Download here",
    ]);
  });

  it("returns null when the sanitiser removed an element that had text, so the two do not line up", () => {
    expect(previewOf("<p>a</p><iframe>gone</iframe><p>b</p>").map).toBeNull();
  });

  it("returns null when the text differs between source and preview", () => {
    const host = document.createElement("div");
    host.innerHTML = "<p>different</p>";
    expect(buildTextMap("<p>original</p>", host)).toBeNull();
  });

  it("ignores whitespace between elements", () => {
    const { map } = previewOf("<div>\n  <p>a</p>\n  <p>b</p>\n</div>");
    expect(map?.entries).toHaveLength(2);
  });

  it("pairs text that is only a non-breaking space, which is visible spacing", () => {
    const { map } = previewOf("<p>a</p><p>&nbsp;</p><p>b</p>");
    expect(map?.entries).toHaveLength(3);
  });

  it("copes with a leading newline after <pre>, which the parser drops", () => {
    const { map } = previewOf("<pre>\nline</pre>");
    expect(map).not.toBeNull();
    expect(applyTextEdits(map as TextMap)).toMatchObject({ source: "<pre>\nline</pre>", changed: false });
  });
});

describe("applyTextEdits", () => {
  it("changes nothing when nothing was edited, keeping the source byte for byte", () => {
    const { map } = previewOf(BULLETIN_HTML);
    const result = applyTextEdits(map as TextMap);
    expect(result.changed).toBe(false);
    expect(result.source).toBe(BULLETIN_HTML);
  });

  it("writes an edit into the source and changes nothing else, including what the preview lost", () => {
    const { host, map } = previewOf(BULLETIN_HTML);
    textNodeContaining(host, "Severity: Medium").nodeValue = "Severity: High";

    const result = applyTextEdits(map as TextMap);

    expect(result.changed).toBe(true);
    expect(result.source).toBe(BULLETIN_HTML.replace("Severity: Medium", "Severity: High"));
    // The sanitiser drops target on links, so the preview cannot have supplied these.
    expect(result.source).toContain('target="_blank" rel="noopener noreferrer"');
    expect(result.source).toContain("border-radius: 12px");
  });

  it("handles edits to several pieces at once", () => {
    const { host, map } = previewOf("<p>one</p><p>two</p><p>three</p>");
    textNodeContaining(host, "one").nodeValue = "1";
    textNodeContaining(host, "three").nodeValue = "three and more";

    expect(applyTextEdits(map as TextMap).source).toBe("<p>1</p><p>two</p><p>three and more</p>");
  });

  it("keeps later edits lined up after an earlier one changed the length", () => {
    const { host, map } = previewOf("<p>aaa</p><p>bbb</p><p>ccc</p>");
    textNodeContaining(host, "aaa").nodeValue = "a much longer first paragraph";
    expect(applyTextEdits(map as TextMap).source).toBe("<p>a much longer first paragraph</p><p>bbb</p><p>ccc</p>");

    textNodeContaining(host, "ccc").nodeValue = "C";
    expect(applyTextEdits(map as TextMap).source).toBe("<p>a much longer first paragraph</p><p>bbb</p><p>C</p>");

    textNodeContaining(host, "much longer").nodeValue = "short";
    expect(applyTextEdits(map as TextMap).source).toBe("<p>short</p><p>bbb</p><p>C</p>");
  });

  it("escapes what it writes, and writes a non-breaking space as an entity", () => {
    const { host, map } = previewOf("<p>plain</p>");
    textNodeContaining(host, "plain").nodeValue = "A & B <c> d";
    expect(applyTextEdits(map as TextMap).source).toBe("<p>A &amp; B &lt;c&gt;&nbsp;d</p>");
  });

  it("leaves entities in pieces that were not edited exactly as written", () => {
    const { host, map } = previewOf("<p>&copy; 2026 &amp; co</p><p>edit me</p>");
    textNodeContaining(host, "edit me").nodeValue = "edited";
    expect(applyTextEdits(map as TextMap).source).toBe("<p>&copy; 2026 &amp; co</p><p>edited</p>");
  });

  it("keeps the whitespace around the text of an edited piece", () => {
    const { host, map } = previewOf("<div>\n    <p>\n      Hello there\n    </p>\n</div>");
    textNodeContaining(host, "Hello there").nodeValue = "\n      Hello, there\n    ";
    expect(applyTextEdits(map as TextMap).source).toBe("<div>\n    <p>\n      Hello, there\n    </p>\n</div>");
  });

  it("reports a piece that was emptied", () => {
    const { host, map } = previewOf("<p>keep</p><p>drop</p>");
    textNodeContaining(host, "drop").nodeValue = "";
    const result = applyTextEdits(map as TextMap);
    expect(result).toMatchObject({ source: "<p>keep</p><p></p>", changed: true, emptied: true });
  });

  it("reports a piece whose text node the browser removed", () => {
    const { host, map } = previewOf("<p>keep</p><p>drop</p>");
    textNodeContaining(host, "drop").remove();
    expect(applyTextEdits(map as TextMap)).toMatchObject({ source: "<p>keep</p><p></p>", emptied: true });
  });

  it("is repeatable: applying again with no new edit changes nothing", () => {
    const { host, map } = previewOf("<p>a</p><p>b</p>");
    textNodeContaining(host, "a").nodeValue = "A";
    applyTextEdits(map as TextMap);
    expect(applyTextEdits(map as TextMap)).toMatchObject({ source: "<p>A</p><p>b</p>", changed: false });
  });
});

describe("applyTextEdits — the newline after <pre>", () => {
  function edit(source: string, from: string, to: string): string {
    const host = document.createElement("div");
    host.innerHTML = source;
    document.body.appendChild(host);
    const map = buildTextMap(source, host);
    if (!map) throw new Error("no map");
    const entry = map.entries.find((e) => (e.node.nodeValue ?? "").includes(from));
    if (!entry) throw new Error("no entry");
    entry.node.nodeValue = (entry.node.nodeValue ?? "").replace(from, to);
    return applyTextEdits(map).source;
  }

  it("keeps the newline the parser leaves out of a <pre> text when that text is edited", () => {
    expect(edit("<pre>\nline one\nline two</pre>", "line one", "first")).toBe("<pre>\nfirst\nline two</pre>");
  });

  it("keeps a CRLF the same way", () => {
    expect(edit("<pre>\r\nline</pre>", "line", "next")).toBe("<pre>\r\nnext</pre>");
  });

  it("adds nothing to a <pre> that has no leading newline", () => {
    expect(edit("<pre>line</pre>", "line", "next")).toBe("<pre>next</pre>");
  });

  it("keeps a dropped newline that was written as a character reference", () => {
    expect(edit("<pre>&#10;line</pre>", "line", "next")).toBe("<pre>&#10;next</pre>");
  });

  it("keeps only the one dropped newline when the text starts with another", () => {
    expect(edit("<pre>\n\nline</pre>", "line", "next")).toBe("<pre>\n\nnext</pre>");
  });

  it("leaves a <pre> that is not edited byte for byte", () => {
    const host = document.createElement("div");
    const source = "<pre>\nline</pre><p>other</p>";
    host.innerHTML = source;
    document.body.appendChild(host);
    const map = buildTextMap(source, host) as TextMap;
    expect(applyTextEdits(map).source).toBe(source);
  });
});

describe("isTextOnlyEdit", () => {
  const text = document.createTextNode("Hello world");
  const other = document.createTextNode("Other");
  const mapped = new Set<Node>([text]);
  const at = (node: Node | null, a: number, f = a) => ({ anchorNode: node, anchorOffset: a, focusNode: node, focusOffset: f });

  it("allows typing and deleting inside a mapped text node", () => {
    expect(isTextOnlyEdit(mapped, at(text, 3), "insertText")).toBe(true);
    expect(isTextOnlyEdit(mapped, at(text, 3), "deleteContentBackward")).toBe(true);
    expect(isTextOnlyEdit(mapped, at(text, 3), "deleteWordForward")).toBe(true);
    expect(isTextOnlyEdit(mapped, at(text, 2, 6), "deleteByCut")).toBe(true);
    expect(isTextOnlyEdit(mapped, at(text, 2, 6), "insertText")).toBe(true);
  });

  it("refuses to delete past either end of the text node, which would join it to its neighbour", () => {
    expect(isTextOnlyEdit(mapped, at(text, 0), "deleteContentBackward")).toBe(false);
    expect(isTextOnlyEdit(mapped, at(text, 0), "deleteWordBackward")).toBe(false);
    expect(isTextOnlyEdit(mapped, at(text, 11), "deleteContentForward")).toBe(false);
    expect(isTextOnlyEdit(mapped, at(text, 0), "deleteContentForward")).toBe(true);
    expect(isTextOnlyEdit(mapped, at(text, 11), "deleteContentBackward")).toBe(true);
  });

  it("refuses a selection that is not inside one mapped text node", () => {
    expect(isTextOnlyEdit(mapped, at(other, 1), "insertText")).toBe(false);
    expect(isTextOnlyEdit(mapped, at(null, 0), "insertText")).toBe(false);
    expect(isTextOnlyEdit(mapped, { anchorNode: text, anchorOffset: 1, focusNode: other, focusOffset: 2 }, "insertText")).toBe(false);
  });

  it("refuses anything that changes structure or formatting", () => {
    for (const type of ["insertParagraph", "insertLineBreak", "insertFromPaste", "insertFromDrop", "deleteByDrag", "formatBold", "insertOrderedList", "insertLink", "somethingNew"]) {
      expect(isTextOnlyEdit(mapped, at(text, 3), type)).toBe(false);
    }
  });

  it("lets undo and redo through, since they only replay edits that were allowed", () => {
    expect(isTextOnlyEdit(mapped, at(null, 0), "historyUndo")).toBe(true);
    expect(isTextOnlyEdit(mapped, at(null, 0), "historyRedo")).toBe(true);
  });
});

describe("absorbStrayText", () => {
  function mapOf(html: string): { host: HTMLElement; map: TextMap } {
    const host = document.createElement("div");
    host.innerHTML = html;
    document.body.appendChild(host);
    const map = buildTextMap(html, host);
    if (!map) throw new Error("no map");
    return { host, map };
  }

  it("folds text a browser put after a link back into the link's text, and removes the new node", () => {
    const { host, map } = mapOf('<p><a href="x">Download</a> tail</p>');
    const link = host.querySelector("a")?.firstChild as Text;
    host.querySelector("p")?.insertBefore(document.createTextNode(" now"), link.parentNode?.nextSibling ?? null);
    // The browser typed into a new node sitting before " tail"
    const caret = absorbStrayText(host, map, link);

    expect(caret).toEqual({ node: link, offset: "Download now".length });
    expect(link.nodeValue).toBe("Download now");
    expect(applyTextEdits(map).source).toBe('<p><a href="x">Download now</a> tail</p>');
  });

  it("folds text typed before the link's text to its start", () => {
    const { host, map } = mapOf('<p>Hi <a href="x">link</a></p>');
    const link = host.querySelector("a")?.firstChild as Text;
    host.querySelector("p")?.insertBefore(document.createTextNode("go "), host.querySelector("a"));
    const caret = absorbStrayText(host, map, link);
    expect(caret?.offset).toBe(3);
    expect(link.nodeValue).toBe("go link");
  });

  it("does nothing when there is no stray text, or no mapped target", () => {
    const { host, map } = mapOf("<div>\n  <p>one</p>\n</div>");
    const target = host.querySelector("p")?.firstChild as Text;
    expect(absorbStrayText(host, map, target)).toBeNull();
    expect(absorbStrayText(host, map, null)).toBeNull();
    expect(absorbStrayText(host, map, document.createTextNode("x"))).toBeNull();
    expect(host.textContent).toContain("one");
  });
});

describe("line breaks", () => {
  function mapOf(html: string): { host: HTMLElement; map: TextMap } {
    const host = document.createElement("div");
    host.innerHTML = html;
    document.body.appendChild(host);
    const map = buildTextMap(html, host);
    if (!map) throw new Error("no map");
    return { host, map };
  }
  const textOf = (host: HTMLElement, fragment: string): Text => {
    const walker = document.createTreeWalker(host, NodeFilter.SHOW_TEXT);
    for (let n = walker.nextNode(); n; n = walker.nextNode()) if ((n.nodeValue ?? "").includes(fragment)) return n as Text;
    throw new Error("no node " + fragment);
  };

  it("finds <br> tags in the source, not in comments or text", () => {
    const html = '<p>a<br>b<br />c<BR/></p><!-- <br> --><textarea><br></textarea>';
    expect(findLineBreaks(html).map((b) => html.slice(b.start, b.end))).toEqual(["<br>", "<br />", "<BR/>"]);
  });

  it("pairs source and preview <br> tags, and gives up when they differ", () => {
    expect(mapOf("<p>a<br>b</p>").map.breaks).toHaveLength(1);
    const host = document.createElement("div");
    host.innerHTML = "<p>a</p>";
    expect(buildTextMap("<p>a<br></p>", host)?.breaks).toBeNull();
  });

  it("splits a text with a <br /> where Enter was pressed, and puts the caret after it", () => {
    const { host, map } = mapOf("<p>Hello world</p>");
    const edit = insertLineBreakInText(map, textOf(host, "Hello"), 5);
    expect(edit?.source).toBe("<p>Hello<br /> world</p>");
    expect(edit?.caretPos).toBe("<p>Hello<br />".length);
  });

  it("leaves every other piece of the source exactly as written when adding a break", () => {
    const { host, map } = mapOf('<div style="a:b"><a href="x" target="_blank">Link &copy;</a> and text &amp; more</div>');
    const node = textOf(host, "and text");
    const edit = insertLineBreakInText(map, node, (node.nodeValue ?? "").indexOf("text"));
    // The link text is untouched (entity included); only the split text is rewritten.
    expect(edit?.source).toBe('<div style="a:b"><a href="x" target="_blank">Link &copy;</a> and <br />text &amp; more</div>');
  });

  it("applies a pending edit in the same text before adding the break", () => {
    const { host, map } = mapOf("<p>one</p><p>two three</p>");
    textOf(host, "one").nodeValue = "ONE";
    const edit = insertLineBreakInText(map, textOf(host, "two"), 3);
    expect(edit?.source).toBe("<p>ONE</p><p>two<br /> three</p>");
  });

  it("is refused inside preformatted text", () => {
    const { host, map } = mapOf("<pre>a b</pre>");
    expect(insertLineBreakInText(map, textOf(host, "a b"), 1)).toBeNull();
  });

  it("is refused when the <br> tags cannot be lined up", () => {
    const host = document.createElement("div");
    host.innerHTML = "<p>a</p>";
    document.body.appendChild(host);
    const map = buildTextMap("<p>a<br></p>", host) as TextMap;
    expect(insertLineBreakInText(map, textOf(host, "a"), 1)).toBeNull();
  });

  it("adds another <br /> from a caret beside an existing one", () => {
    const { host, map } = mapOf("<p>a<br>b</p>");
    const p = host.querySelector("p") as HTMLElement;
    const after = insertLineBreakAtSlot(map, p, 2);
    expect(after?.source).toBe("<p>a<br><br />b</p>");
  });

  it("refuses Enter on an empty line inside preformatted text", () => {
    const { host, map } = mapOf("<pre>a<br>b</pre>");
    const pre = host.querySelector("pre") as HTMLElement;
    expect(insertLineBreakAtSlot(map, pre, 2)).toBeNull();
  });

  it("recognises a caret beside a <br> as a slot, and not one inside text", () => {
    const { host, map } = mapOf("<p>a<br>b</p>");
    const p = host.querySelector("p") as HTMLElement;
    expect(breakSlotAt(map, p, 2)?.side).toBe("after");
    expect(breakSlotAt(map, p, 1)?.side).toBe("before");
    expect(breakSlotAt(map, p, 0)).toBeNull();
    expect(breakSlotAt(map, textOf(host, "a"), 1)).toBeNull();
  });

  it("finds the <br> a Backspace or Delete would remove, only for a collapsed caret", () => {
    const { host, map } = mapOf("<p>a<br>b</p>");
    const a = textOf(host, "a");
    const b = textOf(host, "b");
    const point = (node: Node, offset: number) => ({ anchorNode: node, anchorOffset: offset, focusNode: node, focusOffset: offset });
    expect(adjacentBreak(map, point(b, 0), "backward")).toBe(map.brNodes[0]);
    expect(adjacentBreak(map, point(a, 1), "forward")).toBe(map.brNodes[0]);
    expect(adjacentBreak(map, point(b, 0), "forward")).toBeNull();
    expect(adjacentBreak(map, point(a, 0), "backward")).toBeNull();
    expect(adjacentBreak(map, { anchorNode: b, anchorOffset: 0, focusNode: b, focusOffset: 1 }, "backward")).toBeNull();
  });

  it("removes one <br> and leaves the rest of the source alone", () => {
    const { map } = mapOf('<p class="x">a<br>b<br />c</p>');
    const edit = removeLineBreak(map, map.brNodes[0]);
    expect(edit?.source).toBe('<p class="x">ab<br />c</p>');
    expect(edit?.caretPos).toBe('<p class="x">a'.length);
  });

  it("tracks <br> positions through earlier text edits", () => {
    const { host, map } = mapOf("<p>aa</p><p>b<br>c</p>");
    textOf(host, "aa").nodeValue = "a much longer first line";
    const edit = removeLineBreak(map, map.brNodes[0]);
    expect(edit?.source).toBe("<p>a much longer first line</p><p>bc</p>");
  });

  it("gives text typed beside a <br> its own place in the source", () => {
    const { host, map } = mapOf("<p>one<br></p>");
    const p = host.querySelector("p") as HTMLElement;
    p.appendChild(document.createTextNode("two"));
    expect(adoptSlotText(host, map)).toBe(true);
    expect(applyTextEdits(map).source).toBe("<p>one<br>two</p>");
  });

  it("puts text typed before a <br> ahead of it", () => {
    const { host, map } = mapOf("<p><br>tail</p>");
    host.querySelector("p")?.insertBefore(document.createTextNode("head"), host.querySelector("br"));
    adoptSlotText(host, map);
    expect(applyTextEdits(map).source).toBe("<p>head<br>tail</p>");
  });

  it("does not adopt text that is not beside a <br>", () => {
    const { host, map } = mapOf("<p>one<br>two</p><p>x</p>");
    host.querySelector("p:last-child")?.appendChild(document.createElement("b")).appendChild(document.createTextNode("stray"));
    expect(adoptSlotText(host, map)).toBe(false);
  });

  it("adopts nothing, and does not fail, when the <br> tags could not be lined up", () => {
    const host = document.createElement("div");
    host.innerHTML = "<p>one<br></p>";
    document.body.appendChild(host);
    const map = buildTextMap("<p>one</p>", host) as TextMap;
    expect(map.breaks).toBeNull();
    host.querySelector("p")?.appendChild(document.createTextNode("two"));
    expect(adoptSlotText(host, map)).toBe(false);
  });

  it("works out where the caret goes in the redrawn source", () => {
    const { host, map } = mapOf("<p>Hello<br />world</p>");
    const world = textOf(host, "world");
    expect(caretAtSourcePos(map, "<p>Hello<br />".length)).toEqual({ node: world, offset: 0 });
    expect(caretAtSourcePos(map, "<p>Hel".length)).toEqual({ node: textOf(host, "Hello"), offset: 3 });
    const end = mapOf("<p>Hello<br /></p>");
    const p = end.host.querySelector("p") as HTMLElement;
    expect(caretAtSourcePos(end.map, "<p>Hello<br />".length)).toEqual({ node: p, offset: 2 });
  });
});
