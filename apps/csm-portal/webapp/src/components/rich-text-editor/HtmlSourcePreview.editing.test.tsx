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

import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import EditorWithSourceToggle from "@components/rich-text-editor/EditorWithSourceToggle";
import HtmlSourcePreview from "@components/rich-text-editor/HtmlSourcePreview";
import { BULLETIN_HTML } from "@components/rich-text-editor/bulletinFixture";

vi.mock("@context/error-banner/ErrorBannerContext", () => ({
  useErrorBanner: () => ({ showError: vi.fn(), clearError: vi.fn(), showErrorBanner: vi.fn() }),
}));
vi.mock("@hooks/useLogger", () => ({
  useLogger: () => ({ warn: vi.fn(), error: vi.fn(), info: vi.fn(), debug: vi.fn() }),
}));

afterEach(() => {
  document.getSelection()?.removeAllRanges();
});

const host = (): HTMLElement => screen.getByTestId("html-source-preview");
const editButton = (): HTMLElement | null => screen.queryByRole("button", { name: /edit wording in preview/i });
const startEditing = (): void => {
  fireEvent.click(screen.getByRole("button", { name: /edit wording in preview/i }));
};

function textNode(fragment: string): Text {
  const walker = document.createTreeWalker(host(), NodeFilter.SHOW_TEXT);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    if ((n.nodeValue ?? "").includes(fragment)) return n as Text;
  }
  throw new Error(`no text node containing "${fragment}"`);
}

/** What the browser does after a keystroke: change the text node, then fire input. */
function typeInto(fragment: string, newValue: string): void {
  textNode(fragment).nodeValue = newValue;
  fireEvent.input(host());
}

function select(node: Node, from: number, to = from): void {
  const range = document.createRange();
  range.setStart(node, from);
  range.setEnd(node, to);
  const selection = document.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
}

/** Fire a beforeinput and report whether the component refused it. */
function refused(inputType: string, extra?: (event: Event) => void): boolean {
  const event = new InputEvent("beforeinput", { inputType, cancelable: true, bubbles: true });
  extra?.(event);
  act(() => {
    host().dispatchEvent(event);
  });
  return event.defaultPrevented;
}

describe("HtmlSourcePreview — editing text in the preview", () => {
  it("offers no editing without an onChange", () => {
    render(<HtmlSourcePreview html={BULLETIN_HTML} />);
    expect(editButton()).toBeNull();
    expect(host()).not.toHaveAttribute("contenteditable", "true");
  });

  it("starts read-only and switches editing on and off", () => {
    render(<HtmlSourcePreview html={BULLETIN_HTML} onChange={vi.fn()} />);
    expect(host()).not.toHaveAttribute("contenteditable", "true");

    startEditing();
    expect(host()).toHaveAttribute("contenteditable", "true");
    expect(screen.getByRole("textbox", { name: /announcement text, editable/i })).toBe(host());
    expect(screen.getByText(/each change is written to the html source above/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /done editing/i }));
    expect(host()).not.toHaveAttribute("contenteditable", "true");
    expect(editButton()).not.toBeNull();
  });

  it("writes an edit back as that edit alone, leaving everything else in the source as it was", () => {
    const onChange = vi.fn();
    render(<HtmlSourcePreview html={BULLETIN_HTML} onChange={onChange} />);
    startEditing();

    typeInto("Severity: Medium", "Severity: High");

    expect(onChange).toHaveBeenCalledTimes(1);
    const written = onChange.mock.calls[0][0] as string;
    expect(written).toBe(BULLETIN_HTML.replace("Severity: Medium", "Severity: High"));
    // Not in the preview, because the sanitiser drops it, but still in what is sent.
    expect(written).toContain('target="_blank" rel="noopener noreferrer"');
  });

  it("can fix spacing, writing a non-breaking space as an entity", () => {
    const onChange = vi.fn();
    render(<HtmlSourcePreview html="<p>Hello world</p>" onChange={onChange} />);
    startEditing();

    typeInto("Hello world", "Hello  world");

    expect(onChange).toHaveBeenCalledWith("<p>Hello&nbsp; world</p>");
  });

  it("does not lose text the browser puts in a new node beside the one being edited", () => {
    const onChange = vi.fn();
    render(<HtmlSourcePreview html={'<p><a href="x">Download</a> tail</p>'} onChange={onChange} />);
    startEditing();
    const link = textNode("Download");
    select(link, "Download".length);

    // What Chrome does at the end of a link: the character goes in a new node after it.
    expect(refused("insertText")).toBe(false);
    link.parentNode?.parentNode?.insertBefore(document.createTextNode("!"), link.parentNode.nextSibling);
    fireEvent.input(host());

    expect(onChange).toHaveBeenLastCalledWith('<p><a href="x">Download!</a> tail</p>');
    expect(host().querySelectorAll("a")).toHaveLength(1);
  });

  it("ignores input events when editing is off", () => {
    const onChange = vi.fn();
    render(<HtmlSourcePreview html="<p>Hello</p>" onChange={onChange} />);
    typeInto("Hello", "Changed");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("does not redraw the preview when its own edit is handed back, so the caret is not lost", () => {
    const onChange = vi.fn();
    const { rerender } = render(<HtmlSourcePreview html="<p>one</p><p>two</p>" onChange={onChange} />);
    startEditing();
    const node = textNode("one");

    typeInto("one", "ONE");
    rerender(<HtmlSourcePreview html={onChange.mock.calls[0][0] as string} onChange={onChange} />);

    expect(node.isConnected).toBe(true);
    expect(textNode("ONE")).toBe(node);
  });

  it("keeps later edits correct after earlier ones changed the length", () => {
    const onChange = vi.fn();
    const { rerender } = render(<HtmlSourcePreview html="<p>aaa</p><p>bbb</p><p>ccc</p>" onChange={onChange} />);
    startEditing();

    typeInto("aaa", "a much longer first paragraph");
    rerender(<HtmlSourcePreview html={onChange.mock.calls[0][0] as string} onChange={onChange} />);
    typeInto("ccc", "C");

    expect(onChange).toHaveBeenLastCalledWith("<p>a much longer first paragraph</p><p>bbb</p><p>C</p>");
  });

  it("redraws and keeps working when the HTML is changed from outside", () => {
    const onChange = vi.fn();
    const { rerender } = render(<HtmlSourcePreview html="<p>old</p>" onChange={onChange} />);
    startEditing();

    rerender(<HtmlSourcePreview html="<p>brand new</p><p>second</p>" onChange={onChange} />);
    expect(host()).toHaveTextContent("brand new");
    expect(host()).not.toHaveTextContent("old");

    typeInto("second", "2nd");
    expect(onChange).toHaveBeenLastCalledWith("<p>brand new</p><p>2nd</p>");
  });

  it("redraws from the new source when a piece of text is emptied, and keeps working", () => {
    const onChange = vi.fn();
    const { rerender } = render(<HtmlSourcePreview html="<p>keep</p><p>drop</p>" onChange={onChange} />);
    startEditing();

    const before = textNode("keep");
    // Browsers remove a text node once its last character is deleted.
    textNode("drop").remove();
    fireEvent.input(host());
    // The preview is drawn afresh from the new source so the map matches it again.
    expect(before.isConnected).toBe(false);
    expect(onChange).toHaveBeenLastCalledWith("<p>keep</p><p></p>");
    rerender(<HtmlSourcePreview html="<p>keep</p><p></p>" onChange={onChange} />);

    expect(host()).toHaveTextContent("keep");
    const keep = textNode("keep");
    expect(keep.isConnected).toBe(true);
    typeInto("keep", "kept");
    expect(onChange).toHaveBeenLastCalledWith("<p>kept</p><p></p>");
  });

  it("says so, and offers no editing, when the preview and the source do not line up", () => {
    render(<HtmlSourcePreview html="<p>a</p><iframe>gone</iframe><p>b</p>" onChange={vi.fn()} />);
    expect(editButton()).toBeNull();
    expect(screen.getByText(/text can't be edited in the preview for this markup/i)).toBeInTheDocument();
  });
});

describe("HtmlSourcePreview — line breaks", () => {
  /** A parent that hands the new source straight back, like the real form does. */
  function Controlled({ initial, onSource }: { initial: string; onSource: (html: string) => void }) {
    const [html, setHtml] = useState(initial);
    return (
      <HtmlSourcePreview
        html={html}
        onChange={(next) => {
          onSource(next);
          setHtml(next);
        }}
      />
    );
  }
  const caret = (): Selection => document.getSelection() as Selection;

  it("Enter adds a <br /> to the source at the caret, redraws, and puts the caret on the new line", () => {
    const sources: string[] = [];
    render(<Controlled initial={'<p><a href="x" target="_blank">Hello world</a></p>'} onSource={(h) => sources.push(h)} />);
    startEditing();
    const node = textNode("Hello world");
    select(node, 5);

    expect(refused("insertParagraph")).toBe(true);

    expect(sources.at(-1)).toBe('<p><a href="x" target="_blank">Hello<br /> world</a></p>');
    expect(host().querySelectorAll("br")).toHaveLength(1);
    expect(caret().anchorNode?.nodeValue).toBe(" world");
    expect(caret().anchorOffset).toBe(0);
  });

  it("Shift+Enter does the same", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>ab</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("ab"), 1);
    expect(refused("insertLineBreak")).toBe(true);
    expect(sources.at(-1)).toBe("<p>a<br />b</p>");
  });

  it("Enter at the end of a line leaves the caret on the empty line, and typing there is written to the source", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>one</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("one"), 3);
    refused("insertParagraph");
    expect(sources.at(-1)).toBe("<p>one<br /></p>");

    const p = host().querySelector("p") as HTMLElement;
    expect(caret().anchorNode).toBe(p);
    expect(caret().anchorOffset).toBe(2);
    // A break ending its line shows no empty line on its own, so the preview adds one of its own.
    expect(p.querySelectorAll("br[data-edit-placeholder]")).toHaveLength(1);
    expect(sources.at(-1)).not.toContain("data-edit-placeholder");

    // The browser starts a text node on the empty line.
    expect(refused("insertText")).toBe(false);
    p.insertBefore(document.createTextNode("two"), p.querySelector("br[data-edit-placeholder]"));
    fireEvent.input(host());
    expect(sources.at(-1)).toBe("<p>one<br />two</p>");

    // More typing goes into that same text, now part of the map.
    typeInto("two", "two!");
    expect(sources.at(-1)).toBe("<p>one<br />two!</p>");
  });

  it("two Enters in a row make two breaks", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>one</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("one"), 3);
    refused("insertParagraph");
    refused("insertParagraph");
    expect(sources.at(-1)).toBe("<p>one<br /><br /></p>");
  });

  it("Backspace right after a break removes it, joining the lines with the caret at the join", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>one<br />two</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("two"), 0);

    expect(refused("deleteContentBackward")).toBe(true);

    expect(sources.at(-1)).toBe("<p>onetwo</p>");
    expect(host().querySelectorAll("br")).toHaveLength(0);
    expect(caret().anchorNode?.nodeValue).toBe("onetwo");
    expect(caret().anchorOffset).toBe(3);
  });

  it("Delete at the end of a line before a break removes it", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>one<br />two</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("one"), 3);
    expect(refused("deleteContentForward")).toBe(true);
    expect(sources.at(-1)).toBe("<p>onetwo</p>");
  });

  it("Backspace on a blank line (caret in the parent beside the break) removes the break", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>one<br /><br />two</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    const p = host().querySelector("p") as HTMLElement;
    select(p, 2); // between the two breaks
    expect(refused("deleteContentBackward")).toBe(true);
    expect(sources.at(-1)).toBe("<p>one<br />two</p>");
  });

  it("keeps working on the next edit after a break was added", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>ab</p><p>other</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("ab"), 1);
    refused("insertParagraph");
    typeInto("other", "OTHER");
    expect(sources.at(-1)).toBe("<p>a<br />b</p><p>OTHER</p>");
  });

  it("applies a pending wording edit in the same text when Enter is pressed", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>Hello</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    typeInto("Hello", "Hello there");
    select(textNode("Hello there"), 5);
    refused("insertParagraph");
    expect(sources.at(-1)).toBe("<p>Hello<br /> there</p>");
  });

  it("does nothing for Enter when text is selected, or inside preformatted text", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>one two</p><pre>x y</pre>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("one two"), 1, 4);
    expect(refused("insertParagraph")).toBe(true);
    select(textNode("x y"), 1);
    expect(refused("insertParagraph")).toBe(true);
    expect(sources).toHaveLength(0);
  });

  it("leaves line breaks alone while an IME composition is in progress", () => {
    const sources: string[] = [];
    render(<Controlled initial="<p>one<br />two</p>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(textNode("one"), 3);
    const enter = new InputEvent("beforeinput", { inputType: "insertParagraph", isComposing: true, cancelable: true, bubbles: true });
    act(() => {
      host().dispatchEvent(enter);
    });
    select(textNode("two"), 0);
    const backspace = new InputEvent("beforeinput", { inputType: "deleteContentBackward", isComposing: true, cancelable: true, bubbles: true });
    act(() => {
      host().dispatchEvent(backspace);
    });
    expect(sources).toHaveLength(0);
    expect(host().querySelectorAll("br")).toHaveLength(1);
  });

  it("refuses Enter on an empty line inside preformatted text", () => {
    const sources: string[] = [];
    render(<Controlled initial="<pre>a<br />b</pre>" onSource={(h) => sources.push(h)} />);
    startEditing();
    select(host().querySelector("pre") as HTMLElement, 2);
    expect(refused("insertParagraph")).toBe(true);
    expect(sources).toHaveLength(0);
  });

  it("still refuses Backspace at the edge of a text that has no break next to it", () => {
    render(<Controlled initial="<p>one</p><p>two</p>" onSource={vi.fn()} />);
    startEditing();
    select(textNode("two"), 0);
    expect(refused("deleteContentBackward")).toBe(true);
  });
});

describe("HtmlSourcePreview — what the browser is allowed to do while editing", () => {
  const setup = (): void => {
    render(<HtmlSourcePreview html={BULLETIN_HTML} onChange={vi.fn()} />);
    startEditing();
  };

  it("lets typing and deleting inside a piece of text through", () => {
    setup();
    const node = textNode("Severity: Medium");
    select(node, 5);
    expect(refused("insertText")).toBe(false);
    expect(refused("deleteContentBackward")).toBe(false);
    select(node, 2, 6);
    expect(refused("deleteByCut")).toBe(false);
  });

  it("refuses to add, split or join elements", () => {
    setup();
    select(textNode("Severity: Medium"), 5);
    expect(refused("insertParagraph")).toBe(true);
    expect(refused("insertLineBreak")).toBe(true);
    expect(refused("formatBold")).toBe(true);
    expect(refused("insertOrderedList")).toBe(true);
    expect(refused("insertFromDrop")).toBe(true);
  });

  it("refuses to delete past the edge of a piece of text", () => {
    setup();
    const node = textNode("Severity: Medium");
    select(node, 0);
    expect(refused("deleteContentBackward")).toBe(true);
    select(node, (node.nodeValue ?? "").length);
    expect(refused("deleteContentForward")).toBe(true);
  });

  it("refuses an edit whose selection spans two pieces of text", () => {
    setup();
    const a = textNode("Severity: Medium");
    const b = textNode("Download here");
    const range = document.createRange();
    range.setStart(a, 2);
    range.setEnd(b, 3);
    document.getSelection()?.removeAllRanges();
    document.getSelection()?.addRange(range);
    expect(refused("insertText")).toBe(true);
  });

  it("refuses typing into the whitespace between elements", () => {
    setup();
    const walker = document.createTreeWalker(host(), NodeFilter.SHOW_TEXT);
    let whitespaceOnly: Text | null = null;
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      if (!(n.nodeValue ?? "").trim()) {
        whitespaceOnly = n as Text;
        break;
      }
    }
    expect(whitespaceOnly).not.toBeNull();
    select(whitespaceOnly as Text, 0);
    expect(refused("insertText")).toBe(true);
  });

  it("refuses an edit whose target range the browser reports as leaving the text", () => {
    setup();
    const a = textNode("Severity: Medium");
    const b = textNode("Download here");
    select(a, 3);
    const crossing = refused("deleteContentBackward", (event) => {
      Object.defineProperty(event, "getTargetRanges", {
        value: () => [{ startContainer: a, endContainer: b }],
      });
    });
    expect(crossing).toBe(true);
  });

  it("lets undo and redo through", () => {
    setup();
    expect(refused("historyUndo")).toBe(false);
    expect(refused("historyRedo")).toBe(false);
  });

  it("takes only the plain text of a paste, on one line", () => {
    setup();
    const exec = vi.fn();
    Object.defineProperty(document, "execCommand", { value: exec, configurable: true });
    select(textNode("Severity: Medium"), 5);

    const wasRefused = refused("insertFromPaste", (event) => {
      Object.defineProperty(event, "dataTransfer", {
        value: { getData: (type: string) => (type === "text/plain" ? "pasted\n  text" : "<b>rich</b>") },
      });
    });

    expect(wasRefused).toBe(true);
    expect(exec).toHaveBeenCalledWith("insertText", false, "pasted text");
  });

  it("refuses a drop", () => {
    setup();
    const drop = new Event("drop", { cancelable: true, bubbles: true });
    host().dispatchEvent(drop);
    expect(drop.defaultPrevented).toBe(true);
  });

  it("does not interfere with the browser when editing is off", () => {
    render(<HtmlSourcePreview html="<p>Hello</p>" onChange={vi.fn()} />);
    select(textNode("Hello"), 1);
    expect(refused("insertParagraph")).toBe(false);
  });
});

describe("EditorWithSourceToggle — editing through the preview", () => {
  function Harness({ initial }: { initial: string }) {
    const [value, setValue] = useState(initial);
    return <EditorWithSourceToggle keepStyledHtmlAsSource value={value} onChange={setValue} />;
  }
  const sourceBox = (): HTMLTextAreaElement => screen.getByPlaceholderText("<p>Type HTML here…</p>") as HTMLTextAreaElement;

  it("shows an edit made in the preview in the HTML source box, and nothing else changed", () => {
    render(<Harness initial={BULLETIN_HTML} />);
    startEditing();

    typeInto("Severity: Medium", "Severity: High");

    expect(sourceBox().value).toBe(BULLETIN_HTML.replace("Severity: Medium", "Severity: High"));
  });

  it("shows an edit made in the source box in the preview", () => {
    render(<Harness initial={BULLETIN_HTML} />);
    startEditing();

    fireEvent.change(sourceBox(), { target: { value: BULLETIN_HTML.replace("Download here", "Get the advisory") } });

    expect(host()).toHaveTextContent("Get the advisory");
    expect(host()).not.toHaveTextContent("Download here");
  });

  it("offers no preview editing when the field is disabled", () => {
    render(<EditorWithSourceToggle keepStyledHtmlAsSource disabled value={BULLETIN_HTML} onChange={vi.fn()} />);
    expect(editButton()).toBeNull();
  });
});
