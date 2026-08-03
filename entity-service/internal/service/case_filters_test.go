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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// fakeJWTWithEmail builds an unsigned-but-well-formed JWT (3 base64url segments)
// whose payload carries the given email claim, matching what emailFromJWT reads.
func fakeJWTWithEmail(t *testing.T, email string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadBytes, err := json.Marshal(map[string]string{"email": email})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return header + "." + payload + ".sig"
}

func TestParseCaseFieldFilters_NamedFieldTranslations(t *testing.T) {
	callerEmail, callerErr := "jane.doe@example.com", error(nil)

	cases := []struct {
		name  string
		in    []domain.CaseFieldFilter
		check func(t *testing.T, p domain.ParsedCaseFilters)
	}{
		{
			name: "type in",
			in:   []domain.CaseFieldFilter{{Field: "type", Op: "in", Values: []string{"case", "engagement"}}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if len(p.Types) != 2 || p.Types[0] != "case" || p.Types[1] != "engagement" {
					t.Fatalf("Types = %v", p.Types)
				}
			},
		},
		{
			name: "tag in maps to Tags",
			in:   []domain.CaseFieldFilter{{Field: "tag", Op: "in", Values: []string{"patch"}}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if len(p.Tags) != 1 || p.Tags[0] != "patch" {
					t.Fatalf("Tags = %v", p.Tags)
				}
			},
		},
		{
			name: "tag notIn maps to ExcludeTags",
			in:   []domain.CaseFieldFilter{{Field: "tag", Op: "notIn", Values: []string{"patch"}}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if len(p.ExcludeTags) != 1 || p.ExcludeTags[0] != "patch" {
					t.Fatalf("ExcludeTags = %v", p.ExcludeTags)
				}
			},
		},
		{
			name: "assignedUserId isEmpty maps to Unassigned",
			in:   []domain.CaseFieldFilter{{Field: "assignedUserId", Op: "isEmpty"}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if !p.Unassigned {
					t.Fatalf("expected Unassigned = true")
				}
			},
		},
		{
			name: "resolutionNotes isEmpty maps to ResolutionNotesEmpty",
			in:   []domain.CaseFieldFilter{{Field: "resolutionNotes", Op: "isEmpty"}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if !p.ResolutionNotesEmpty {
					t.Fatalf("expected ResolutionNotesEmpty = true")
				}
			},
		},
		{
			name: "createdBy in maps to literal CreatedBy list",
			in:   []domain.CaseFieldFilter{{Field: "createdBy", Op: "in", Values: []string{"a@example.com", "b@example.com"}}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if len(p.CreatedBy) != 2 {
					t.Fatalf("CreatedBy = %v", p.CreatedBy)
				}
				if p.CreatedByMe {
					t.Fatalf("expected CreatedByMe = false for a literal email list")
				}
			},
		},
		{
			name: "createdBy eq placeholder maps to CreatedByMe",
			in:   []domain.CaseFieldFilter{{Field: "createdBy", Op: "eq", Values: []string{currentUserFilterPlaceholder}}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if !p.CreatedByMe {
					t.Fatalf("expected CreatedByMe = true")
				}
				if len(p.CreatedBy) != 0 {
					t.Fatalf("expected CreatedBy left empty (SN forwards CreatedByMe as a flag, not folded in), got %v", p.CreatedBy)
				}
			},
		},
		{
			name: "createdOn gte/lte map to StartCreatedDate/EndCreatedDate",
			in: []domain.CaseFieldFilter{
				{Field: "createdOn", Op: "gte", Values: []string{"2026-01-01"}},
				{Field: "createdOn", Op: "lte", Values: []string{"2026-01-31"}},
			},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if p.StartCreatedDate == nil || p.EndCreatedDate == nil {
					t.Fatalf("expected both StartCreatedDate and EndCreatedDate set")
				}
			},
		},
		{
			name: "projectOnboardingStatus in",
			in:   []domain.CaseFieldFilter{{Field: "projectOnboardingStatus", Op: "in", Values: []string{"Completed"}}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if len(p.ProjectOnboardingStatuses) != 1 || p.ProjectOnboardingStatuses[0] != "Completed" {
					t.Fatalf("ProjectOnboardingStatuses = %v", p.ProjectOnboardingStatuses)
				}
			},
		},
		{
			name: "parentId eq",
			in:   []domain.CaseFieldFilter{{Field: "parentId", Op: "eq", Values: []string{"00000000-0000-0000-0000-000000000000"}}},
			check: func(t *testing.T, p domain.ParsedCaseFilters) {
				if p.ParentID == nil || *p.ParentID != "00000000-0000-0000-0000-000000000000" {
					t.Fatalf("ParentID = %v", p.ParentID)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParseCaseFieldFilters(tc.in, callerEmail, callerErr, time.Now().UTC())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.check(t, p)
		})
	}
}

func TestParseCaseFieldFilters_Rejections(t *testing.T) {
	cases := []struct {
		name string
		in   []domain.CaseFieldFilter
	}{
		{name: "unsupported field", in: []domain.CaseFieldFilter{{Field: "bogus", Op: "in", Values: []string{"x"}}}},
		{name: "unsupported op", in: []domain.CaseFieldFilter{{Field: "type", Op: "bogus", Values: []string{"x"}}}},
		{name: "bad field/op combo", in: []domain.CaseFieldFilter{{Field: "type", Op: "eq", Values: []string{"case"}}}},
		{name: "in with no values", in: []domain.CaseFieldFilter{{Field: "type", Op: "in"}}},
		{name: "assignedUserId isNotEmpty unsupported", in: []domain.CaseFieldFilter{{Field: "assignedUserId", Op: "isNotEmpty"}}},
		{name: "resolutionNotes isNotEmpty unsupported", in: []domain.CaseFieldFilter{{Field: "resolutionNotes", Op: "isNotEmpty"}}},
		{name: "createdBy eq non-placeholder literal", in: []domain.CaseFieldFilter{{Field: "createdBy", Op: "eq", Values: []string{"someone@example.com"}}}},
		{name: "createdOn bad date format", in: []domain.CaseFieldFilter{{Field: "createdOn", Op: "gte", Values: []string{"not-a-date"}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCaseFieldFilters(tc.in, "caller@example.com", nil, time.Now().UTC())
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			var ve *apierror.ValidationError
			if !asValidationError(err, &ve) {
				t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
			}
		})
	}
}

func TestParseCaseFieldFilters_DateOnlyLteBoundIncludesWholeDay(t *testing.T) {
	filters := []domain.CaseFieldFilter{
		{Field: "createdOn", Op: "lte", Values: []string{"2026-01-31"}},
	}

	p, err := ParseCaseFieldFilters(filters, "caller@example.com", nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.EndCreatedDate == nil {
		t.Fatalf("expected EndCreatedDate to be set")
	}

	endOfDay := time.Date(2026, 1, 31, 23, 59, 59, 999999999, time.UTC)
	if !p.EndCreatedDate.Equal(endOfDay) {
		t.Fatalf("expected EndCreatedDate %v to equal the exact inclusive boundary %v", p.EndCreatedDate, endOfDay)
	}

	startOfNextDay := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if !p.EndCreatedDate.Before(startOfNextDay) {
		t.Fatalf("expected EndCreatedDate %v to exclude 00:00:00 of the next day", p.EndCreatedDate)
	}
}

func TestParseCaseFieldFilters_CreatedByCurrentUser_RequiresCallerEmail(t *testing.T) {
	filters := []domain.CaseFieldFilter{{Field: "createdBy", Op: "eq", Values: []string{currentUserFilterPlaceholder}}}

	if _, err := ParseCaseFieldFilters(filters, "", nil, time.Now().UTC()); err == nil {
		t.Fatalf("expected an error when no caller email is available")
	} else if _, ok := err.(*apierror.UnauthorizedError); !ok {
		t.Fatalf("expected *apierror.UnauthorizedError, got %T: %v", err, err)
	}

	forwardedErr := &apierror.ValidationError{Msg: "x-user-id-token: malformed"}
	if _, err := ParseCaseFieldFilters(filters, "", forwardedErr, time.Now().UTC()); err != forwardedErr {
		t.Fatalf("expected the resolver's own error to be forwarded, got %v", err)
	}
}

// fixedNow is a fixed reference instant (a Saturday, no significance beyond
// being unambiguous) every resolveRelativeDate test below resolves against,
// so the expected outputs are exact rather than moving-target "relative to
// whenever this test happens to run."
var fixedNow = time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)

func TestResolveRelativeDate_Resolutions(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		expected string
	}{
		{"today", "__today__", "2026-08-15"},
		{"daysAgo 0", "__daysAgo:0__", "2026-08-15"},
		{"daysAgo 2", "__daysAgo:2__", "2026-08-13"},
		{"daysAgo 30 crosses month boundary", "__daysAgo:30__", "2026-07-16"},
		{"startOfMonth 0 (this month)", "__startOfMonth:0__", "2026-08-01"},
		{"startOfMonth -1 (last month)", "__startOfMonth:-1__", "2026-07-01"},
		{"startOfMonth -2 (month before last)", "__startOfMonth:-2__", "2026-06-01"},
		{"startOfMonth 1 (next month)", "__startOfMonth:1__", "2026-09-01"},
		{"endOfMonth 0 (this month, 31 days)", "__endOfMonth:0__", "2026-08-31"},
		{"endOfMonth 1 (next month, 30 days)", "__endOfMonth:1__", "2026-09-30"},
		{"startOfQuarter 0 (Q3: Jul-Sep)", "__startOfQuarter:0__", "2026-07-01"},
		{"startOfQuarter -1 (Q2: Apr-Jun)", "__startOfQuarter:-1__", "2026-04-01"},
		{"endOfQuarter 0 (Q3 ends Sep 30)", "__endOfQuarter:0__", "2026-09-30"},
		{"endOfQuarter 1 (Q4 ends Dec 31)", "__endOfQuarter:1__", "2026-12-31"},
		{"year rollover: startOfMonth 6 from August", "__startOfMonth:6__", "2027-02-01"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, matched, err := resolveRelativeDate(tc.value, fixedNow)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !matched {
				t.Fatalf("expected %q to be recognized as a relative-date placeholder", tc.value)
			}
			if resolved != tc.expected {
				t.Fatalf("resolveRelativeDate(%q) = %q, want %q", tc.value, resolved, tc.expected)
			}
		})
	}
}

func TestResolveRelativeDate_NotAPlaceholder(t *testing.T) {
	cases := []string{"2026-07-01", "2026-07-01T00:00:00Z", "__current_user_email__", "__bogus__", "__bogus:1__", "not-a-date-at-all"}

	for _, value := range cases {
		t.Run(value, func(t *testing.T) {
			resolved, matched, err := resolveRelativeDate(value, fixedNow)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if matched {
				t.Fatalf("expected %q not to be recognized as a relative-date placeholder, got resolved=%q", value, resolved)
			}
		})
	}
}

func TestResolveRelativeDate_Rejections(t *testing.T) {
	cases := []string{
		"__daysAgo__",        // missing required offset
		"__daysAgo:abc__",    // non-integer offset
		"__daysAgo:-1__",     // negative offset not allowed for daysAgo
		"__today:5__",        // today takes no argument
		"__startOfMonth__",   // missing required offset
		"__startOfQuarter__", // missing required offset
	}

	for _, value := range cases {
		t.Run(value, func(t *testing.T) {
			_, _, err := resolveRelativeDate(value, fixedNow)
			if err == nil {
				t.Fatalf("expected an error for %q, got nil", value)
			}
			var ve *apierror.ValidationError
			if !asValidationError(err, &ve) {
				t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
			}
		})
	}
}

func TestParseCaseFieldFilters_RelativeDatePlaceholders(t *testing.T) {
	filters := []domain.CaseFieldFilter{
		{Field: "createdOn", Op: "gte", Values: []string{"__daysAgo:30__"}},
		{Field: "createdOn", Op: "lte", Values: []string{"__today__"}},
	}

	p, err := ParseCaseFieldFilters(filters, "caller@example.com", nil, fixedNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.StartCreatedDate == nil || p.EndCreatedDate == nil {
		t.Fatalf("expected both StartCreatedDate and EndCreatedDate set")
	}

	wantStart := time.Date(2026, time.July, 16, 0, 0, 0, 0, time.UTC)
	if !p.StartCreatedDate.Equal(wantStart) {
		t.Fatalf("StartCreatedDate = %v, want %v", p.StartCreatedDate, wantStart)
	}

	// lte on a relative-date placeholder still gets the same inclusive-of-
	// whole-day bump a literal YYYY-MM-DD lte value gets (see
	// TestParseCaseFieldFilters_DateOnlyLteBoundIncludesWholeDay) -- resolution
	// happens before that logic runs, not around it.
	wantEnd := time.Date(2026, time.August, 15, 23, 59, 59, 999999999, time.UTC)
	if !p.EndCreatedDate.Equal(wantEnd) {
		t.Fatalf("EndCreatedDate = %v, want %v (inclusive of the whole day)", p.EndCreatedDate, wantEnd)
	}
}

// An OR-group branch containing createdBy must fail with the "not supported
// inside an OR group" validation error (400). Before the caller-email sentinel
// was introduced, ParseCaseFieldFilterGroups passed an empty callerEmail, so
// the createdBy current-user filter short-circuited with an UnauthorizedError
// and an authenticated caller saw a misleading 401 for a merely-invalid request.
func TestParseCaseFieldFilterGroups_CreatedByRejectedAsValidationError(t *testing.T) {
	cases := []struct {
		name  string
		group []domain.CaseFieldFilter
	}{
		{
			name:  "createdBy eq current-user placeholder",
			group: []domain.CaseFieldFilter{{Field: "createdBy", Op: "eq", Values: []string{currentUserFilterPlaceholder}}},
		},
		{
			name:  "createdBy in literal emails",
			group: []domain.CaseFieldFilter{{Field: "createdBy", Op: "in", Values: []string{"someone@example.com"}}},
		},
		{
			name: "createdBy alongside a supported field",
			group: []domain.CaseFieldFilter{
				{Field: "state", Op: "in", Values: []string{"open"}},
				{Field: "createdBy", Op: "eq", Values: []string{currentUserFilterPlaceholder}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCaseFieldFilterGroups([]domain.CaseFilterBranch{{Filters: tc.group}})
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			var ue *apierror.UnauthorizedError
			if errors.As(err, &ue) {
				t.Fatalf("got *apierror.UnauthorizedError (401) %q, want a 400 validation error", ue.Msg)
			}
			var ve *apierror.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %v (%T), want *apierror.ValidationError", err, err)
			}
			const want = `anyOf: field "createdBy" is not supported inside an OR group`
			if ve.Msg != want {
				t.Errorf("Msg = %q, want %q", ve.Msg, want)
			}
		})
	}
}

// The sentinel caller email must never leak into a parsed group.
func TestParseCaseFieldFilterGroups_SupportedFieldsParse(t *testing.T) {
	groups, err := ParseCaseFieldFilterGroups([]domain.CaseFilterBranch{
		{Filters: []domain.CaseFieldFilter{{Field: "state", Op: "in", Values: []string{"open", "closed"}}}},
		{Filters: []domain.CaseFieldFilter{{Field: "severity", Op: "in", Values: []string{"high"}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}
	if len(groups[0].States) != 2 || groups[0].States[0] != domain.CaseState("open") {
		t.Errorf("groups[0].States = %v, want [open closed]", groups[0].States)
	}
	if len(groups[1].Severities) != 1 || groups[1].Severities[0] != domain.CaseSeverity("high") {
		t.Errorf("groups[1].Severities = %v, want [high]", groups[1].Severities)
	}
}
