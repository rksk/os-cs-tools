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

import { render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import Editor from "@components/rich-text-editor/Editor";
import { htmlNeedsSourceEditing } from "@components/rich-text-editor/richHtmlFidelity";

vi.mock("@context/error-banner/ErrorBannerContext", () => ({
  useErrorBanner: () => ({ showError: vi.fn(), clearError: vi.fn(), showErrorBanner: vi.fn() }),
}));
vi.mock("@hooks/useLogger", () => ({
  useLogger: () => ({ warn: vi.fn(), error: vi.fn(), info: vi.fn(), debug: vi.fn() }),
}));

/**
 * The guard in EditorWithSourceToggle sends content to HTML source only when the
 * rich editor would destroy styling. If it ever flagged markup the editor
 * itself produces, ordinary announcements (indented text, an image) would open
 * in source mode after being saved. So: whatever the real editor emits for each
 * kind of content it supports must not be flagged.
 */
const SAMPLES: Record<string, string> = {
  inline: "<p>Hello <strong>bold</strong> <em>it</em> <u>u</u> <s>s</s> <code>c</code></p>",
  headings: "<h1>One</h1><h2>Two</h2><h3>Three</h3>",
  lists: "<ul><li>a</li><li>b<ul><li>nested</li></ul></li></ul><ol><li>1</li><li>2</li></ol>",
  indentation: '<p style="padding-inline-start: 40px">indented</p>',
  alignment: '<p style="text-align: center">c</p><p style="text-align: right">r</p><p style="text-align: justify">j</p>',
  quote: "<blockquote>quoted</blockquote>",
  codeBlock: "<pre><code>line1\nline2</code></pre>",
  link: '<p><a href="https://example.test" target="_blank" rel="noopener noreferrer">link</a></p>',
  image: '<p><img src="https://example.test/x.png" alt="x" width="200" height="100"></p>',
  table: "<table><tr><th>H</th><th>H2</th></tr><tr><td>a</td><td>b</td></tr></table>",
  styledTable: '<table style="border-collapse: collapse"><tr><td style="background-color: #eee; border: 1px solid #ccc">a</td></tr></table>',
  preWrapSpan: '<p dir="ltr"><span style="white-space: pre-wrap;">plain</span></p>',
};

describe("htmlNeedsSourceEditing against the rich editor's own output", () => {
  for (const [name, html] of Object.entries(SAMPLES)) {
    it(`does not flag what the editor emits for: ${name}`, async () => {
      const emitted: string[] = [];
      const { unmount } = render(<Editor value={html} onChange={(h) => emitted.push(h)} />);
      await waitFor(() => expect(emitted.length).toBeGreaterThan(0));
      expect(htmlNeedsSourceEditing(emitted[emitted.length - 1])).toBe(false);
      unmount();
    });
  }
});
