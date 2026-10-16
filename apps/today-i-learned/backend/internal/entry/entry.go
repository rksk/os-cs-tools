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

// Package entry holds a TIL entry's shape and the rules every entry has to
// meet before it is stored: field validation, HTML sanitising, and the plain
// text that search and the length limit read.
package entry

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	TitleMaxLength        = 100
	AuthorMaxLength       = 200
	SourceDetailMaxLength = 200
	// A learning's length limit counts its plain text, not its markup. It's
	// ENTRY_TEXT_MAX_LENGTH: DefaultWhatMaxLength when that isn't set, and
	// never more than MaxWhatMaxLength.
	DefaultWhatMaxLength = 500
	MaxWhatMaxLength     = 5000
	// WhatLengthMargin is how far past the limit a learning is still
	// accepted. The webapp's form counts the same way and stops at the limit;
	// the margin absorbs any tiny difference left (such as an invisible
	// character one side trims and the other doesn't), so nothing the form
	// allowed is ever rejected here.
	WhatLengthMargin = 20
)

// Sources are the values an entry's "where" can take, in the order the form
// offers them.
var Sources = []string{"Customer", "Partner", "Internal", "Other"}

// Entry is one learning, as the API returns it.
type Entry struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Who              string     `json:"who"`
	Where            string     `json:"where"`
	WhereDetail      *string    `json:"whereDetail"`
	What             string     `json:"what"`
	SubmittedByEmail string     `json:"submittedByEmail"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        *time.Time `json:"updatedAt"`
	// NotifiedAt is when Novera accepted the entry's Google Chat
	// announcement; nil while it hasn't.
	NotifiedAt *time.Time `json:"notifiedAt"`
}

// Draft is an entry's editable content as a client sent it.
type Draft struct {
	Title       string
	Where       string
	WhereDetail *string
	What        string
}

// Content is a Draft that passed validation: trimmed, sanitised, and carrying
// the plain text of the learning alongside its HTML.
type Content struct {
	Title       string
	Where       string
	WhereDetail *string // nil unless the source takes a detail
	HTML        string
	Text        string
}

// ValidationError is a rule the input broke. Its message is written for the
// person who filled in the form.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// ValidateAuthor checks the byline a new entry is shared under.
func ValidateAuthor(who string) (string, error) {
	who = strings.TrimSpace(who)
	if who == "" {
		return "", invalid("Author name is required.")
	}
	if utf8.RuneCountInString(who) > AuthorMaxLength {
		return "", invalid("Author name must be %d characters or fewer.", AuthorMaxLength)
	}
	return who, nil
}

// Prepare validates a Draft and returns the Content to store. whatMaxLength
// is the most characters of plain text the learning may have; past it,
// WhatLengthMargin more are still accepted.
func Prepare(d Draft, whatMaxLength int) (Content, error) {
	title := strings.TrimSpace(d.Title)
	if title == "" {
		return Content{}, invalid("Title is required.")
	}
	if utf8.RuneCountInString(title) > TitleMaxLength {
		return Content{}, invalid("Title must be %d characters or fewer.", TitleMaxLength)
	}

	if !isSource(d.Where) {
		return Content{}, invalid("Choose a source: Customer, Partner, Internal or Other.")
	}

	var detail *string
	if label, ok := detailLabel(d.Where); ok {
		value := ""
		if d.WhereDetail != nil {
			value = strings.TrimSpace(*d.WhereDetail)
		}
		if value == "" {
			return Content{}, invalid("%s", label.required)
		}
		if utf8.RuneCountInString(value) > SourceDetailMaxLength {
			return Content{}, invalid("%s must be %d characters or fewer.", label.name, SourceDetailMaxLength)
		}
		detail = &value
	}

	html := Sanitize(d.What)
	text := PlainText(html)
	if text == "" {
		return Content{}, invalid("Add what you learned.")
	}
	if utf8.RuneCountInString(text) > whatMaxLength+WhatLengthMargin {
		return Content{}, invalid("What you learned must be %d characters or fewer.", whatMaxLength)
	}

	return Content{Title: title, Where: d.Where, WhereDetail: detail, HTML: html, Text: text}, nil
}

// SameContent reports whether storing c would leave e as it is, so an
// untouched save doesn't mark the entry as edited.
func SameContent(e Entry, c Content) bool {
	return e.Title == c.Title && e.Where == c.Where && e.What == c.HTML && sameDetail(e.WhereDetail, c.WhereDetail)
}

func sameDetail(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func isSource(s string) bool {
	for _, source := range Sources {
		if s == source {
			return true
		}
	}
	return false
}

type sourceDetailLabel struct {
	name     string
	required string
}

// detailLabel names the detail a source takes, in the form's own words.
// Internal takes none.
func detailLabel(source string) (sourceDetailLabel, bool) {
	switch source {
	case "Customer":
		return sourceDetailLabel{name: "Customer name", required: "Customer name is required."}, true
	case "Partner":
		return sourceDetailLabel{name: "Partner name", required: "Partner name is required."}, true
	case "Other":
		return sourceDetailLabel{name: "The explanation", required: "Please explain what Other means."}, true
	}
	return sourceDetailLabel{}, false
}
