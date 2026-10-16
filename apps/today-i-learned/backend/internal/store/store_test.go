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
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/wso2-open-operations/cs-tools/apps/today-i-learned/backend/internal/entry"
)

// These tests need a MySQL server. TIL_TEST_DATABASE_DSN points at it without
// a database name, e.g. "root:root@tcp(127.0.0.1:3306)/". They create and use
// a til_test database there, never any other.
const testDatabase = "til_test"

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TIL_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TIL_TEST_DATABASE_DSN not set; skipping MySQL tests")
	}
	ctx := context.Background()

	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+testDatabase+" CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}

	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	mc.DBName = testDatabase
	s, err := openConfig(ctx, mc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// Rebuilt from the schema script every run, so the tests always exercise
	// the schema as it is checked in.
	if _, err := s.db.ExecContext(ctx, "DROP TABLE IF EXISTS til_entry"); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range schemaStatements(t) {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("create tables: %v", err)
		}
	}
	if err := s.CheckSchema(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

func schemaStatements(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../sql/create_til_tables.sql")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	var stmts []string
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		if strings.TrimSpace(stmt) != "" {
			stmts = append(stmts, stmt)
		}
	}
	return stmts
}

// clock hands out increasing times, one second apart, so creation order is
// unambiguous.
func clock(start time.Time) func() time.Time {
	t := start
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

func content(t *testing.T, title, where string, detail *string, what string) entry.Content {
	t.Helper()
	c, err := entry.Prepare(entry.Draft{Title: title, Where: where, WhereDetail: detail, What: what}, entry.MaxWhatMaxLength)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return c
}

func ptr(s string) *string { return &s }

var jane = Author{Name: "Jane Doe", Email: "jane@example.com"}

func TestCreateThenGetReturnsTheSameEntry(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	created, err := s.Create(ctx, jane, content(t, "My title", "Customer", ptr("Acme Corp"), "<p>Learned X</p>"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID || got.Title != "My title" || got.Who != "Jane Doe" || got.SubmittedByEmail != "jane@example.com" ||
		got.Where != "Customer" || got.WhereDetail == nil || *got.WhereDetail != "Acme Corp" || got.What != "<p>Learned X</p>" ||
		!got.CreatedAt.Equal(created.CreatedAt) || got.UpdatedAt != nil {
		t.Fatalf("Get = %+v, want %+v", got, created)
	}
}

func TestGetMissingIsNotFound(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.Get(context.Background(), "00000000-0000-4000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
}

func TestUpdateReplacesContentAndMarksTheEntryEdited(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	created, err := s.Create(ctx, jane, content(t, "T", "Internal", nil, "x"))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.Update(ctx, created.ID, content(t, "New title", "Partner", ptr("Globex"), "<p>Changed</p>"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "New title" || updated.Where != "Partner" || *updated.WhereDetail != "Globex" || updated.What != "<p>Changed</p>" {
		t.Fatalf("Update = %+v", updated)
	}
	if updated.UpdatedAt == nil {
		t.Fatal("UpdatedAt not set")
	}
	if updated.Who != jane.Name || updated.SubmittedByEmail != jane.Email || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("Update changed the byline or creation time: %+v", updated)
	}
}

func TestUpdateAndDeleteOfAMissingEntryAreNotFound(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	missing := "00000000-0000-4000-8000-000000000000"
	if _, err := s.Update(ctx, missing, content(t, "T", "Internal", nil, "x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete = %v, want ErrNotFound", err)
	}
}

func TestDeleteRemovesTheEntry(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	created, err := s.Create(ctx, jane, content(t, "T", "Internal", nil, "x"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func titles(p Page) []string {
	out := make([]string, len(p.Items))
	for i, e := range p.Items {
		out[i] = e.Title
	}
	return out
}

func TestListPagesNewestFirstWithTotal(t *testing.T) {
	s := openTestStore(t)
	s.now = clock(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	ctx := context.Background()
	for _, title := range []string{"one", "two", "three", "four", "five"} {
		if _, err := s.Create(ctx, jane, content(t, title, "Internal", nil, "x")); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.List(ctx, Query{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(titles(first), ","); got != "five,four" || first.Total != 5 {
		t.Fatalf("first page = %s (total %d)", got, first.Total)
	}
	last, err := s.List(ctx, Query{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(titles(last), ","); got != "one" || last.Total != 5 {
		t.Fatalf("last page = %s (total %d)", got, last.Total)
	}
	beyond, err := s.List(ctx, Query{Limit: 2, Offset: 10})
	if err != nil {
		t.Fatal(err)
	}
	if beyond.Items == nil || len(beyond.Items) != 0 || beyond.Total != 5 {
		t.Fatalf("past the end = %+v", beyond)
	}
}

func TestListFiltersByAuthor(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, jane, content(t, "mine", "Internal", nil, "x")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, Author{Name: "Sam", Email: "sam@example.com"}, content(t, "theirs", "Internal", nil, "x")); err != nil {
		t.Fatal(err)
	}
	p, err := s.List(ctx, Query{AuthorEmail: jane.Email, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(titles(p), ","); got != "mine" || p.Total != 1 {
		t.Fatalf("mine = %s (total %d)", got, p.Total)
	}
}

func TestListFiltersByAuthorName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for _, a := range []Author{
		{Name: "Alex Sample", Email: "alex@example.com"},
		{Name: "Sam Alexander", Email: "sam@example.com"},
		{Name: "Dana Lee", Email: "dana@example.com"},
		{Name: "100% Bob_Smith", Email: "bob@example.com"},
	} {
		// Every learning mentions Alex, so only a match on the name itself
		// tells the entries apart.
		if _, err := s.Create(ctx, a, content(t, a.Name, "Internal", nil, "<p>Alex said so</p>")); err != nil {
			t.Fatal(err)
		}
	}
	for author, want := range map[string]string{
		"alex":    "Sam Alexander,Alex Sample", // the start of any word, any case
		"ale sam": "Sam Alexander,Alex Sample",
		"sample":  "Alex Sample",
		"lex":     "", // not the start of a word
		"dana":    "Dana Lee",
		"100%":    "100% Bob_Smith", // LIKE wildcards are themselves
		"bob_":    "100% Bob_Smith",
		"b%b":     "",
	} {
		p, err := s.List(ctx, Query{Author: author, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(titles(p), ","); got != want || p.Total != len(p.Items) {
			t.Errorf("author %q = %q (total %d), want %q", author, got, p.Total, want)
		}
	}
}

func TestListFiltersBySourceWithTheOtherFilters(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for _, e := range []struct {
		title, source string
		detail        *string
		author        Author
	}{
		{"customer retries", "Customer", ptr("Acme Corp"), jane},
		{"partner retries", "Partner", ptr("Acme Reseller"), jane},
		{"internal retries", "Internal", nil, Author{Name: "Sam", Email: "sam@example.com"}},
		{"customer pooling", "Customer", ptr("Globex"), Author{Name: "Sam", Email: "sam@example.com"}},
	} {
		if _, err := s.Create(ctx, e.author, content(t, e.title, e.source, e.detail, "<p>x</p>")); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		q    Query
		want string
	}{
		{Query{Source: "Customer"}, "customer pooling,customer retries"},
		{Query{Source: "Internal"}, "internal retries"},
		{Query{Source: "Other"}, ""},
		{Query{Source: "Customer", Search: "retries"}, "customer retries"},
		{Query{Source: "Customer", Author: "sam"}, "customer pooling"},
		{Query{Source: "Partner", AuthorEmail: jane.Email}, "partner retries"},
	} {
		tc.q.Limit = 10
		p, err := s.List(ctx, tc.q)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(titles(p), ","); got != tc.want || p.Total != len(p.Items) {
			t.Errorf("%+v = %q (total %d), want %q", tc.q, got, p.Total, tc.want)
		}
	}
}

func TestListFiltersByCreationTime(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		title string
		at    time.Time
	}{
		{"before", time.Date(2026, 10, 3, 23, 59, 59, 0, time.UTC)},
		{"first day", time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
		{"last day", time.Date(2026, 10, 5, 23, 59, 59, 999999000, time.UTC)},
		{"after", time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)},
	} {
		at := tc.at
		s.now = func() time.Time { return at }
		if _, err := s.Create(ctx, jane, content(t, tc.title, "Internal", nil, "x")); err != nil {
			t.Fatal(err)
		}
	}
	// From is inclusive and Before exclusive.
	from := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	p, err := s.List(ctx, Query{From: &from, Before: &before, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(titles(p), ","); got != "last day,first day" {
		t.Fatalf("in range = %s", got)
	}
}

func TestSearch(t *testing.T) {
	s := openTestStore(t)
	s.now = clock(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	ctx := context.Background()
	for _, e := range []struct {
		author Author
		c      entry.Content
	}{
		{jane, content(t, "Idempotency keys", "Customer", ptr("Acme Corp"), "<p>Retries can apply a <strong>payment</strong> twice.</p>")},
		{Author{Name: "Sam Perera", Email: "sam@example.com"}, content(t, "Liveness probes", "Internal", nil, "<p>A probe shouldn't check the database.</p>")},
		{jane, content(t, "R&D notes", "Other", ptr("Conference"), "<p>Go is fun &amp; 100% fast_path</p>")},
		{jane, content(t, "Batch sizes", "Internal", nil, "<p>Batches of 10 cut latency.</p>")},
	} {
		if _, err := s.Create(ctx, e.author, e.c); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		search string
		want   string
	}{
		{"payment", "Idempotency keys"},       // a word in the text
		{"idempot", "Idempotency keys"},       // a prefix of a word in the title
		{"acme", "Idempotency keys"},          // the customer name
		{"perera", "Liveness probes"},         // the author
		{"probe database", "Liveness probes"}, // every term has to match
		{"probe payment", ""},                 // terms from different entries
		{"Go", "R&D notes"},                   // too short for the index
		{"R&D", "R&D notes"},                  // an entity decoded in the text
		{"0%", "R&D notes"},                   // a literal %, so not "10"
		{"fast_path", "R&D notes"},            // a literal _
		{"_", "R&D notes"},                    // "_" alone isn't a wildcard
		{"strong", ""},                        // markup is never searched
		{"the", "Liveness probes"},            // a stopword still matches as text
		{"keys retries", "Idempotency keys"},  // title and text together
		{"conference", "R&D notes"},           // an explanation for Other
		{"nothing-like-this", ""},
	}
	for _, tc := range tests {
		t.Run(tc.search, func(t *testing.T) {
			p, err := s.List(ctx, Query{Search: tc.search, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(titles(p), ","); got != tc.want || p.Total != len(p.Items) {
				t.Fatalf("search %q = %q (total %d), want %q", tc.search, got, p.Total, tc.want)
			}
		})
	}
}
