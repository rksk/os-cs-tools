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

package entry

import (
	"strings"
	"testing"
)

func TestSanitizeKeepsTheEditorsFormatting(t *testing.T) {
	in := "<p>Learned <strong>a lot</strong> from <em>this</em> and <u>that</u>.</p><ol><li>One</li></ol><ul><li>Two<br>three</li></ul>"
	if got := Sanitize(in); got != in {
		t.Fatalf("Sanitize changed allowed markup:\n got %s\nwant %s", got, in)
	}
}

func TestSanitizeRemovesScriptAndStyleWithTheirContent(t *testing.T) {
	got := Sanitize(`<p>hi</p><script>alert("x")</script><style>p{color:red}</style>`)
	if got != "<p>hi</p>" {
		t.Fatalf("Sanitize = %q", got)
	}
}

func TestSanitizeStripsDisallowedTagsButKeepsTheirText(t *testing.T) {
	if got := Sanitize("<div><span>kept</span></div>"); got != "kept" {
		t.Fatalf("Sanitize = %q", got)
	}
}

func TestSanitizeStripsDisallowedAttributes(t *testing.T) {
	got := Sanitize(`<p onclick="alert(1)" style="position:fixed" class="x">hi</p>`)
	if got != "<p>hi</p>" {
		t.Fatalf("Sanitize = %q", got)
	}
}

func TestSanitizeLinks(t *testing.T) {
	if got := Sanitize(`<a href="https://wso2.com" target="_blank">WSO2</a>`); !strings.Contains(got, `href="https://wso2.com"`) {
		t.Fatalf("safe link lost its href: %q", got)
	}
	for _, href := range []string{"javascript:alert(1)", "data:text/html,x", "vbscript:x"} {
		if got := Sanitize(`<a href="` + href + `">bad</a>`); strings.Contains(got, href) {
			t.Fatalf("Sanitize kept %q: %q", href, got)
		}
	}
	for _, href := range []string{"mailto:a@example.com", "tel:+94112345678", "/knowledge/today-i-learned"} {
		if got := Sanitize(`<a href="` + href + `">ok</a>`); !strings.Contains(got, href) {
			t.Fatalf("Sanitize dropped %q: %q", href, got)
		}
	}
}

func TestSanitizeStripsEveryPicture(t *testing.T) {
	for _, img := range []string{
		`<img src="https://example.com/x.png" alt="a screenshot">`,
		`<img src="/uploads/07ec86bdee9142da838c9a3511f780e8.webp">`,
		`<img src="data:image/png;base64,aGVsbG8=">`,
		`<img src="x" onerror="alert(1)">`,
	} {
		if got := Sanitize("<p>See:</p>" + img); got != "<p>See:</p>" {
			t.Fatalf("Sanitize(%s) = %q", img, got)
		}
	}
}

func TestSanitizeTwiceChangesNothing(t *testing.T) {
	// What the webapp's editor sends, after its own sanitiser: quotes and
	// apostrophes bare, an ampersand escaped.
	for _, in := range []string{
		`<p>It's "fine" &amp; R&amp;D &lt;ok&gt;</p>`,
		"<p>a b</p><ul><li>x</li></ul>",
		`<p><a href="https://example.com/?a=1&amp;b=2" target="_blank">link</a></p>`,
	} {
		once := Sanitize(in)
		if twice := Sanitize(once); twice != once {
			t.Fatalf("not idempotent:\n once %s\ntwice %s", once, twice)
		}
	}
}

func TestPlainText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"<p><strong>abc</strong></p>", "abc"},
		{"<p>One</p><p>Two</p>", "One Two"},
		{"<ul><li>One</li><li>Two</li></ul>", "One Two"},
		{"<p>One<br>Two</p>", "One Two"},
		{"<p><br></p>", ""},
		{"<p>R&amp;D &lt;3 &#39;quoted&#39;</p>", "R&D <3 'quoted'"},
		{"<p>a&nbsp;b c</p>", "a b c"},
		// Trimmed at least as much as a browser's trim(), so the backend never
		// counts more than the webapp's counter.
		{"<p>\ufeffzero-width\ufeff</p>", "zero-width"},
		{"<p>next-line\u0085</p>", "next-line"},
	}
	for _, tc := range tests {
		if got := PlainText(tc.in); got != tc.want {
			t.Errorf("PlainText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBulletedText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"<ul><li>First</li><li>Second</li></ul>", "• First • Second"},
		{"<p>Steps:</p><ol><li>Retry</li><li><strong>Check</strong></li></ol><p>Done</p>", "Steps: • Retry • Check Done"},
		{"<p>No list &amp; no bullets</p>", "No list & no bullets"},
	}
	for _, tc := range tests {
		if got := BulletedText(tc.in); got != tc.want {
			t.Errorf("BulletedText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
