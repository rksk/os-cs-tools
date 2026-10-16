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
	"context"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

const originTestDescription = "<p>Scheduled downtime on Friday.</p><p>No action needed.</p>"

func TestIsAnnouncementOriginComment(t *testing.T) {
	tests := []struct {
		name        string
		description string
		comment     string
		want        bool
	}{
		{"exact repeat", originTestDescription, originTestDescription, true},
		{
			"ignores markup, entities, whitespace between blocks and case",
			"<p>Tom &amp; Jerry&nbsp;rule</p><p>Second</p>",
			"<div>TOM & JERRY rule\n\nsecond</div>",
			true,
		},
		{
			"body under the Description label ServiceNow's announcement flow adds",
			originTestDescription,
			"<b><u>Description</u></b><br><p></p>" + originTestDescription,
			true,
		},
		{
			"quotes the description and adds its own text",
			originTestDescription,
			"<p>Update:</p>" + originTestDescription + "<p>The window has moved to Saturday.</p>",
			false,
		},
		{
			"label followed by different text",
			originTestDescription,
			"<b><u>Description</u></b><br><p>Something else entirely.</p>",
			false,
		},
		{"different text", originTestDescription, "<p>Update: the window moved to Saturday.</p>", false},
		{"blank description never matches", "<p> </p>", "<p>anything</p>", false},
		{"empty description never matches", "", "", false},
		{"empty comment", originTestDescription, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAnnouncementOriginComment(tt.description, tt.comment); got != tt.want {
				t.Fatalf("isAnnouncementOriginComment(%q, %q) = %v, want %v", tt.description, tt.comment, got, tt.want)
			}
		})
	}
}

// mirrorContents runs the mirror for a just-created case of caseType whose
// ServiceNow-generated comments are snComments, and returns the content of
// every comment that was written to Postgres, in order.
func mirrorContents(t *testing.T, caseType, description string, snComments []domain.CaseComment) []string {
	t.Helper()
	var written []string
	repo := &stubCaseRepo{
		createCaseCommentAsSystem: func(_ context.Context, req domain.CreateCaseCommentRequest, _ *time.Time) (domain.CaseComment, error) {
			written = append(written, req.Content)
			return domain.CaseComment{}, nil
		},
	}
	svc := &caseService{repo: repo, snMirror: &mirrorSNCaseService{comments: snComments}}
	svc.mirrorInitialSNComments(context.Background(), "44444444-4444-4444-4444-444444444444", caseType, description)
	return written
}

func originTestComment(typ domain.CommentType, content string, at time.Time) domain.CaseComment {
	return domain.CaseComment{Type: typ, Content: content, CreatedOn: at}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// An announcement's origin comment only repeats the description that every
// reader already shows, so copying it into Postgres made the same body appear
// twice. Everything else ServiceNow wrote for the announcement still copies.
func TestMirrorInitialSNComments_AnnouncementDoesNotCopyTheRepeatedOriginComment(t *testing.T) {
	at := time.Date(2026, 10, 12, 10, 16, 0, 0, time.UTC)

	t.Run("exact repeat is not copied", func(t *testing.T) {
		got := mirrorContents(t, "announcement", originTestDescription, []domain.CaseComment{
			originTestComment(domain.CommentTypeComment, originTestDescription, at),
		})
		if len(got) != 0 {
			t.Fatalf("the repeated origin comment must not be copied, got %v", got)
		}
	})

	t.Run("repeat under the Description label is not copied", func(t *testing.T) {
		got := mirrorContents(t, "announcement", originTestDescription, []domain.CaseComment{
			originTestComment(domain.CommentTypeComment, "<b><u>Description</u></b><br><p></p>"+originTestDescription, at),
		})
		if len(got) != 0 {
			t.Fatalf("the repeated origin comment must not be copied, got %v", got)
		}
	})

	t.Run("a comment that differs is copied", func(t *testing.T) {
		got := mirrorContents(t, "announcement", originTestDescription, []domain.CaseComment{
			originTestComment(domain.CommentTypeComment, "<p>Something else entirely.</p>", at),
		})
		if !equalStrings(got, []string{"<p>Something else entirely.</p>"}) {
			t.Fatalf("a comment that is not a repeat must be copied, got %v", got)
		}
	})

	t.Run("a comment that quotes the description and adds text is copied", func(t *testing.T) {
		quoted := "<p>Correction:</p>" + originTestDescription + "<p>Moved to Saturday.</p>"
		got := mirrorContents(t, "announcement", originTestDescription, []domain.CaseComment{
			originTestComment(domain.CommentTypeComment, quoted, at),
		})
		if !equalStrings(got, []string{quoted}) {
			t.Fatalf("a comment with text of its own must be copied, got %v", got)
		}
	})

	t.Run("only the repeat is dropped when ServiceNow wrote several entries", func(t *testing.T) {
		got := mirrorContents(t, "announcement", originTestDescription, []domain.CaseComment{
			originTestComment(domain.CommentTypeComment, originTestDescription, at),
			originTestComment(domain.CommentTypeWorkNote, originTestDescription, at),
			originTestComment(domain.CommentTypeComment, "<p>Reminder.</p>", at.Add(time.Minute)),
		})
		// a work note is never the origin comment, even with the same text
		if !equalStrings(got, []string{originTestDescription, "<p>Reminder.</p>"}) {
			t.Fatalf("expected the work note and the reminder to be copied, got %v", got)
		}
	})

	t.Run("only the earliest repeat is dropped, a later comment with the same text is copied", func(t *testing.T) {
		// ServiceNow's search order is not relied on, so try both.
		later := originTestComment(domain.CommentTypeComment, "<div>"+originTestDescription+"</div>", at.Add(time.Hour))
		origin := originTestComment(domain.CommentTypeComment, originTestDescription, at)
		for name, order := range map[string][]domain.CaseComment{
			"newest first": {later, origin},
			"oldest first": {origin, later},
		} {
			got := mirrorContents(t, "announcement", originTestDescription, order)
			if !equalStrings(got, []string{later.Content}) {
				t.Fatalf("%s: only the later repeat must be copied, got %v", name, got)
			}
		}
	})

	t.Run("a blank description drops nothing, the comment may be the only copy", func(t *testing.T) {
		got := mirrorContents(t, "announcement", "<p> </p>", []domain.CaseComment{
			originTestComment(domain.CommentTypeComment, "<p>The only copy of the content.</p>", at),
		})
		if !equalStrings(got, []string{"<p>The only copy of the content.</p>"}) {
			t.Fatalf("with a blank description the comment must be copied, got %v", got)
		}
	})
}

// The skip is for announcements only: for every other type, the first comment
// ServiceNow writes is the case's own conversation and is copied as before.
func TestMirrorInitialSNComments_OtherTypesStillCopyAnEchoedComment(t *testing.T) {
	at := time.Date(2026, 10, 12, 10, 16, 0, 0, time.UTC)
	for _, caseType := range []string{"case", "service_request", "engagement", "security_report_analysis", ""} {
		t.Run("type="+caseType, func(t *testing.T) {
			got := mirrorContents(t, caseType, originTestDescription, []domain.CaseComment{
				originTestComment(domain.CommentTypeComment, originTestDescription, at),
			})
			if !equalStrings(got, []string{originTestDescription}) {
				t.Fatalf("type %q: the first comment must still be copied, got %v", caseType, got)
			}
		})
	}
}
