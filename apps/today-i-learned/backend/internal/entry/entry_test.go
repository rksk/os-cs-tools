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
	"errors"
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func validDraft() Draft {
	return Draft{Title: "Retries need idempotency keys", Where: "Customer", WhereDetail: ptr("Acme Corp"), What: "<p>Learned X.</p>"}
}

func TestPrepareAcceptsAValidDraft(t *testing.T) {
	c, err := Prepare(Draft{Title: "  T  ", Where: "Customer", WhereDetail: ptr("  Acme Corp "), What: "<p>Learned X.</p>"}, DefaultWhatMaxLength)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if c.Title != "T" || *c.WhereDetail != "Acme Corp" || c.HTML != "<p>Learned X.</p>" || c.Text != "Learned X." {
		t.Fatalf("unexpected content: %+v", c)
	}
}

func TestPrepareDropsTheDetailForInternal(t *testing.T) {
	// Internal takes no detail, so one sent anyway isn't stored.
	c, err := Prepare(Draft{Title: "T", Where: "Internal", WhereDetail: ptr("ignored"), What: "x"}, DefaultWhatMaxLength)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if c.WhereDetail != nil {
		t.Fatalf("WhereDetail = %q, want nil", *c.WhereDetail)
	}
}

func TestPrepareRules(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	tests := []struct {
		name   string
		change func(*Draft)
		want   string // "" = valid
	}{
		{"missing title", func(d *Draft) { d.Title = "" }, "Title is required."},
		{"blank title", func(d *Draft) { d.Title = "   " }, "Title is required."},
		{"title at the limit", func(d *Draft) { d.Title = long(TitleMaxLength) }, ""},
		{"title over the limit", func(d *Draft) { d.Title = long(TitleMaxLength + 1) }, "Title must be 100 characters or fewer."},
		{"title counted in characters, not bytes", func(d *Draft) { d.Title = strings.Repeat("é", TitleMaxLength) }, ""},
		{"unknown source", func(d *Draft) { d.Where = "Supplier" }, "Choose a source: Customer, Partner, Internal or Other."},
		{"source is case-sensitive", func(d *Draft) { d.Where = "customer" }, "Choose a source: Customer, Partner, Internal or Other."},
		{"customer without a name", func(d *Draft) { d.WhereDetail = nil }, "Customer name is required."},
		{"customer with a blank name", func(d *Draft) { d.WhereDetail = ptr("  ") }, "Customer name is required."},
		{"partner without a name", func(d *Draft) { d.Where = "Partner"; d.WhereDetail = nil }, "Partner name is required."},
		{"other without an explanation", func(d *Draft) { d.Where = "Other"; d.WhereDetail = nil }, "Please explain what Other means."},
		{"internal without a detail", func(d *Draft) { d.Where = "Internal"; d.WhereDetail = nil }, ""},
		{"customer name over the limit", func(d *Draft) { d.WhereDetail = ptr(long(SourceDetailMaxLength + 1)) }, "Customer name must be 200 characters or fewer."},
		{"explanation over the limit", func(d *Draft) { d.Where = "Other"; d.WhereDetail = ptr(long(SourceDetailMaxLength + 1)) }, "The explanation must be 200 characters or fewer."},
		{"missing learning", func(d *Draft) { d.What = "" }, "Add what you learned."},
		{"empty editor markup", func(d *Draft) { d.What = "<p><br></p>" }, "Add what you learned."},
		{"learning that is only a script", func(d *Draft) { d.What = "<script>alert(1)</script>" }, "Add what you learned."},
		{"learning that is only a picture", func(d *Draft) { d.What = `<p><img src="https://example.com/x.png"></p>` }, "Add what you learned."},
		{"learning at the limit", func(d *Draft) { d.What = "<p>" + long(DefaultWhatMaxLength) + "</p>" }, ""},
		{"learning within the margin past the limit", func(d *Draft) { d.What = long(DefaultWhatMaxLength + WhatLengthMargin) }, ""},
		{"learning past the margin", func(d *Draft) { d.What = long(DefaultWhatMaxLength + WhatLengthMargin + 1) }, "What you learned must be 500 characters or fewer."},
		{"limit counts text, not markup", func(d *Draft) { d.What = "<p><strong>" + long(DefaultWhatMaxLength) + "</strong></p>" }, ""},
		{"limit counts an entity as one character", func(d *Draft) { d.What = strings.Repeat("&amp;", DefaultWhatMaxLength) }, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := validDraft()
			tc.change(&d)
			_, err := Prepare(d, DefaultWhatMaxLength)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Prepare: unexpected error %v", err)
				}
				return
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("Prepare error = %v, want a ValidationError", err)
			}
			if verr.Message != tc.want {
				t.Fatalf("message = %q, want %q", verr.Message, tc.want)
			}
		})
	}
}

func TestValidateAuthor(t *testing.T) {
	if who, err := ValidateAuthor("  Jane Doe "); err != nil || who != "Jane Doe" {
		t.Fatalf("ValidateAuthor = %q, %v", who, err)
	}
	if _, err := ValidateAuthor(" "); err == nil || err.Error() != "Author name is required." {
		t.Fatalf("blank author: %v", err)
	}
	if _, err := ValidateAuthor(strings.Repeat("a", AuthorMaxLength+1)); err == nil || err.Error() != "Author name must be 200 characters or fewer." {
		t.Fatalf("long author: %v", err)
	}
}

func TestSameContent(t *testing.T) {
	c, err := Prepare(validDraft(), DefaultWhatMaxLength)
	if err != nil {
		t.Fatal(err)
	}
	e := Entry{Title: c.Title, Where: c.Where, WhereDetail: ptr("Acme Corp"), What: c.HTML}
	if !SameContent(e, c) {
		t.Fatal("identical content reported as changed")
	}
	e.WhereDetail = nil
	if SameContent(e, c) {
		t.Fatal("a removed detail reported as unchanged")
	}
	e.WhereDetail = ptr("Acme Corp")
	e.What = "<p>Different</p>"
	if SameContent(e, c) {
		t.Fatal("changed text reported as unchanged")
	}
}

func TestPrepareUsesTheGivenLimit(t *testing.T) {
	d := validDraft()
	d.What = strings.Repeat("a", 800)
	if _, err := Prepare(d, 1000); err != nil {
		t.Fatalf("under a 1000 limit: %v", err)
	}
	if _, err := Prepare(d, 800-WhatLengthMargin); err != nil {
		t.Fatalf("within the margin of a %d limit: %v", 800-WhatLengthMargin, err)
	}
	_, err := Prepare(d, 600)
	var verr *ValidationError
	if !errors.As(err, &verr) || verr.Message != "What you learned must be 600 characters or fewer." {
		t.Fatalf("over a 600 limit: %v", err)
	}
}
