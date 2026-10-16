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

package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStripInlineBase64Images_NoImage_ReturnsUnchanged(t *testing.T) {
	html := "<p>Steps to reproduce: click the button twice.</p>"
	got, truncated := stripInlineBase64Images(html)
	if got != html {
		t.Fatalf("expected content with no inline image to pass through unchanged, got %q", got)
	}
	if truncated {
		t.Fatal("expected truncated=false for content with no inline image")
	}
}

func TestStripInlineBase64Images_StripsEmbeddedImage(t *testing.T) {
	html := `<p>See the error below:</p><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAUA" alt="screenshot"><p>Please advise.</p>`
	got, truncated := stripInlineBase64Images(html)
	if !truncated {
		t.Fatal("expected truncated=true when an inline image is stripped")
	}
	if strings.Contains(got, "base64") || strings.Contains(got, "iVBOR") {
		t.Fatalf("expected no fragment of the base64 payload to survive, got %q", got)
	}
	if !strings.Contains(got, inlineImageUnavailablePlaceholder) {
		t.Fatalf("expected the readable placeholder in place of the image, got %q", got)
	}
	if !strings.Contains(got, "<p>See the error below:</p>") || !strings.Contains(got, "<p>Please advise.</p>") {
		t.Fatalf("expected surrounding content to survive untouched, got %q", got)
	}
}

func TestStripInlineBase64Images_StripsSingleQuotedImage(t *testing.T) {
	// A large single-quoted image: if the regex only matched double quotes,
	// this would survive stripping entirely and the byte-cap backstop below
	// would then cut straight through the middle of its base64 payload --
	// losing both the image (still present, leaving a dangling open <img>
	// tag) and the text that follows it.
	largePayload := strings.Repeat("A", maxEventTextFieldBytes)
	html := `<p>Before.</p><img src='data:image/png;base64,` + largePayload + `' alt='x'><p>After.</p>`

	got, truncated := stripInlineBase64Images(html)
	if !truncated {
		t.Fatal("expected truncated=true for a single-quoted inline image")
	}
	if strings.Contains(got, "base64") || strings.Contains(got, "AAAA") {
		t.Fatalf("expected no fragment of the base64 payload to survive, got %q", got)
	}
	if !strings.Contains(got, inlineImageUnavailablePlaceholder) {
		t.Fatalf("expected the readable placeholder in place of the single-quoted image, got %q", got)
	}
	if !strings.Contains(got, "<p>Before.</p>") || !strings.Contains(got, "<p>After.</p>") {
		t.Fatalf("expected the text surrounding the image -- including after it -- to survive, got %q", got)
	}
}

func TestStripInlineBase64Images_StripsMultipleImages(t *testing.T) {
	html := `<img src="data:image/png;base64,AAAA"><p>middle</p><img src="data:image/jpeg;base64,BBBB">`
	got, truncated := stripInlineBase64Images(html)
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if strings.Count(got, inlineImageUnavailablePlaceholder) != 2 {
		t.Fatalf("expected both images replaced, got %q", got)
	}
}

func TestStripInlineBase64Images_LeavesNonBase64ImagesAlone(t *testing.T) {
	html := `<img src="https://example.com/real-attachment.png" alt="not inline">`
	got, truncated := stripInlineBase64Images(html)
	if got != html {
		t.Fatalf("a non-base64 image src must never be touched, got %q", got)
	}
	if truncated {
		t.Fatal("expected truncated=false: nothing was actually stripped")
	}
}

func TestStripInlineBase64Images_BackstopTruncatesOversizedText(t *testing.T) {
	// No embedded image at all -- exercises the length-cap backstop on its own.
	html := strings.Repeat("a", maxEventTextFieldBytes+500)
	got, truncated := stripInlineBase64Images(html)
	if !truncated {
		t.Fatal("expected truncated=true once the backstop cap is exceeded")
	}
	if len(got) > maxEventTextFieldBytes+len(truncatedTextSuffix) {
		t.Fatalf("expected result bounded near maxEventTextFieldBytes, got %d bytes", len(got))
	}
	if !strings.HasSuffix(got, truncatedTextSuffix) {
		t.Fatalf("expected a readable truncation suffix, got tail %q", got[len(got)-40:])
	}
}

func TestStripInlineBase64Images_BackstopNeverSplitsAMultiByteRune(t *testing.T) {
	// Pad right up to the cut boundary with a multi-byte rune straddling it,
	// so a naive byte-index cut would produce invalid UTF-8.
	html := strings.Repeat("a", maxEventTextFieldBytes-1) + "é" + strings.Repeat("b", 10)
	got, truncated := stripInlineBase64Images(html)
	if !truncated {
		t.Fatal("expected truncated=true")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid UTF-8 output, got %q", got)
	}
}
