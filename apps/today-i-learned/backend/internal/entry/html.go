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
	"io"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"
)

// policy is the formatting the "What did you learn?" editor offers, and
// nothing else. The webapp sanitises the same way, but an entry is written by
// one employee and read by every other, so the backend never relies on that.
// Disallowed tags are dropped with their text kept; script and style lose
// their content too. Entries are text only, so <img> goes like any other tag.
var policy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "strong", "em", "u", "ol", "ul", "li")
	p.AllowAttrs("href", "target").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	p.AllowRelativeURLs(true)
	return p
}()

// Sanitize returns html with everything outside the editor's formatting
// removed.
func Sanitize(html string) string {
	return policy.Sanitize(html)
}

// PlainText is an entry's text without markup: what search matches and what
// the length limit counts. A paragraph, list item or line break ends with a
// space, so "<p>One</p><p>Two</p>" reads "One Two", the way the webapp's own
// character counter reads it.
func PlainText(sanitized string) string {
	return plainText(sanitized, false)
}

// BulletedText is PlainText with "• " before each list item, for showing an
// entry's text where lists can't be formatted, such as a Google Chat card.
func BulletedText(sanitized string) string {
	return plainText(sanitized, true)
}

// isTrimmed is what plain text loses at either end: whitespace, and the
// zero-width no-break space (U+FEFF) a browser's trim() also removes, so the
// backend never counts more than the webapp's character counter does.
func isTrimmed(r rune) bool {
	return unicode.IsSpace(r) || r == '\uFEFF'
}

func plainText(sanitized string, bullets bool) string {
	var b strings.Builder
	z := xhtml.NewTokenizer(strings.NewReader(sanitized))
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			if z.Err() != io.EOF {
				return ""
			}
			return strings.TrimFunc(strings.ReplaceAll(b.String(), "\u00a0", " "), isTrimmed)
		case xhtml.TextToken:
			b.Write(z.Text())
		case xhtml.EndTagToken:
			if name, _ := z.TagName(); string(name) == "p" || string(name) == "li" {
				b.WriteByte(' ')
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			switch name, _ := z.TagName(); string(name) {
			case "br":
				b.WriteByte(' ')
			case "li":
				if bullets {
					b.WriteString("• ")
				}
			}
		}
	}
}
