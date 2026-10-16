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
	"html"
	"regexp"
	"strings"
	"unicode"
)

// announcementOriginCommentLabel is the label ServiceNow's announcement flow
// writes ahead of the body in an announcement's origin comment
// (<b><u>Description</u></b><br>...), as it reads once
// echoComparableText has reduced the comment to its text.
const announcementOriginCommentLabel = "description"

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// echoComparableText reduces HTML to its visible characters, so two snippets can
// be compared for "same text" regardless of markup, entity encoding, letter case
// or where each side puts whitespace between blocks.
func echoComparableText(s string) string {
	s = html.UnescapeString(htmlTagPattern.ReplaceAllString(s, ""))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !unicode.IsSpace(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// isAnnouncementOriginComment reports whether comment is only a repeat of the
// announcement's description: the same text, optionally under the label
// ServiceNow puts in front of the body.
//
// Deliberately an equality check, not "contains": a comment that quotes the
// description and then adds something of its own must never be taken for a
// duplicate and dropped. The worst a mismatch can do is leave the repeat in
// place, which is how announcements behaved before this check existed. A blank
// description has nothing to repeat, so it never matches (the comment may be
// the only copy of the content).
func isAnnouncementOriginComment(description, comment string) bool {
	d := echoComparableText(description)
	if d == "" {
		return false
	}
	c := echoComparableText(comment)
	return c == d || c == announcementOriginCommentLabel+d
}
