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

package store

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const searchColumns = "title, content_text, author_name, source_detail"

// The full-text index can't find every word. InnoDB never indexes a word
// shorter than innodb_ft_min_token_size (3 by default) or longer than
// innodb_ft_max_token_size (84), nor one on its stopword list, so a term made
// of such words is matched with LIKE instead.
const (
	minIndexedWordLength = 3
	maxIndexedWordLength = 84
)

// InnoDB's default full-text stopword list
// (information_schema.INNODB_FT_DEFAULT_STOPWORD).
var stopwords = map[string]bool{
	"a": true, "about": true, "an": true, "are": true, "as": true, "at": true, "be": true, "by": true,
	"com": true, "de": true, "en": true, "for": true, "from": true, "how": true, "i": true, "in": true,
	"is": true, "it": true, "la": true, "of": true, "on": true, "or": true, "that": true, "the": true,
	"this": true, "to": true, "was": true, "what": true, "when": true, "where": true, "who": true,
	"will": true, "with": true, "und": true, "www": true,
}

// searchConditions turns a search box query into WHERE conditions. Each
// space-separated term has to match. A term whose words are all indexable is
// matched by the full-text index, each word as a prefix ("+word*"), so a
// half-typed word already finds results. Any other term ("R&D", "Go", "the")
// is matched as a substring with LIKE, wildcards escaped.
func searchConditions(search string) ([]string, []any) {
	var prefixes []string
	var conds []string
	var args []any
	for _, term := range strings.Fields(search) {
		words := strings.FieldsFunc(term, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
		})
		if len(words) > 0 && allIndexable(words) {
			for _, w := range words {
				prefixes = append(prefixes, "+"+w+"*")
			}
			continue
		}
		like := "%" + escapeLike(term) + "%"
		conds = append(conds, "(title LIKE ? OR content_text LIKE ? OR author_name LIKE ? OR source_detail LIKE ?)")
		args = append(args, like, like, like, like)
	}
	if len(prefixes) > 0 {
		conds = append([]string{"MATCH(" + searchColumns + ") AGAINST (? IN BOOLEAN MODE)"}, conds...)
		args = append([]any{strings.Join(prefixes, " ")}, args...)
	}
	return conds, args
}

// authorConditions matches the author's name alone. Each space-separated
// term has to start the name or a word in it. Not the full-text index, which
// covers the name only together with the other searched columns.
func authorConditions(author string) ([]string, []any) {
	var conds []string
	var args []any
	for _, term := range strings.Fields(author) {
		escaped := escapeLike(term)
		conds = append(conds, "(author_name LIKE ? OR author_name LIKE ?)")
		args = append(args, escaped+"%", "% "+escaped+"%")
	}
	return conds, args
}

func allIndexable(words []string) bool {
	for _, w := range words {
		n := utf8.RuneCountInString(w)
		if n < minIndexedWordLength || n > maxIndexedWordLength || stopwords[strings.ToLower(w)] {
			return false
		}
	}
	return true
}

// escapeLike makes LIKE treat a user's "%" and "_" as themselves. Backslash,
// MySQL's LIKE escape character, is escaped first.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}
