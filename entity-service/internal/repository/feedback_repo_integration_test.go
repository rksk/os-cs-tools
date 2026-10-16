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

// Runs against a real Postgres with every migration applied (same DSN as
// project_case_stats_repo_integration_test.go). Skipped without
// CASE_STATS_TEST_DSN.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run FeedbackReasonsIntegration

package repository_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const (
	frAccountA = "81000000-0000-0000-0000-0000000000a1"
	frAccountB = "81000000-0000-0000-0000-0000000000b1"
	frMetricID = "81000000-0000-0000-0000-0000000000c1"
)

// frCaseID/frFeedbackID/frReasonID build the fixture ids for case n.
func frCaseID(n int) string     { return fmt.Sprintf("81000000-0000-0000-0001-%012d", n) }
func frFeedbackID(n int) string { return fmt.Sprintf("81000000-0000-0000-0002-%012d", n) }
func frReasonID(n, i int) string {
	return fmt.Sprintf("81000000-0000-0000-0003-%06d%06d", n, i)
}

// seedFeedbackReasons builds six rated cases and their ticked reasons, all
// under ids this test owns and removes afterwards:
//
//	case 1 rating 2, account A, 2026-07-10: Slow response, Unclear answer
//	case 2 rating 2, account A, 2026-07-20: Slow response
//	case 3 rating 2, account B, 2026-08-05: Slow response, Rude tone
//	case 4 rating 5, account A, 2026-07-12: Fast resolution
//	case 5 rating 5, account A, 2026-07-13: Fast resolution, Clear communication
//	case 6 rating 1, account A, no reasons recorded
func seedFeedbackReasons(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	scoped := repository.NewScoped(pool)

	cleanup := func() {
		for n := 1; n <= 6; n++ {
			_, _ = scoped.Exec(ctx, `DELETE FROM work_item_feedback WHERE work_item_id = $1`, frCaseID(n))
			_, _ = scoped.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, frCaseID(n))
		}
		_, _ = pool.Exec(ctx, `DELETE FROM work_item_feedback_metric WHERE id = $1`, frMetricID)
		_, _ = pool.Exec(ctx, `DELETE FROM account WHERE id IN ($1, $2)`, frAccountA, frAccountB)
	}
	cleanup()
	t.Cleanup(cleanup)

	now := time.Now().UTC()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.80s): %v", sql, err)
		}
	}
	mustExecScoped := func(sql string, args ...any) {
		t.Helper()
		if _, err := scoped.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed scoped (%.80s): %v", sql, err)
		}
	}

	mustExec(`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number)
		VALUES ($1, $3, $3, 'test', 'test', 'Feedback Reasons Account A', 'FR-A'),
		       ($2, $3, $3, 'test', 'test', 'Feedback Reasons Account B', 'FR-B')`, frAccountA, frAccountB, now)
	mustExec(`INSERT INTO work_item_feedback_metric (id, name, datatype, is_active, created_on, updated_on, created_by, updated_by)
		VALUES ($1, 'Dissatisfied - Reasons', 'checkbox', true, $2, $2, 'test', 'test')`, frMetricID, now)

	fixtures := []struct {
		account string
		rating  int
		at      string
		reasons []string
	}{
		{frAccountA, 2, "2026-07-10T09:00:00Z", []string{"Slow response", "Unclear answer"}},
		{frAccountA, 2, "2026-07-20T09:00:00Z", []string{"Slow response"}},
		{frAccountB, 2, "2026-08-05T09:00:00Z", []string{"Slow response", "Rude tone"}},
		{frAccountA, 5, "2026-07-12T09:00:00Z", []string{"Fast resolution"}},
		{frAccountA, 5, "2026-07-13T09:00:00Z", []string{"Fast resolution", "Clear communication"}},
		{frAccountA, 1, "2026-07-14T09:00:00Z", nil},
	}
	for i, fx := range fixtures {
		n := i + 1
		at, _ := time.Parse(time.RFC3339, fx.at)
		mustExecScoped(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type, account_id)
			VALUES ($1, $2, $2, 'test', 'test', $3, $4, 'Feedback reasons fixture', 'CASE', $5)`,
			frCaseID(n), now, fmt.Sprintf("FR-TEST-%04d", n), fmt.Sprintf("FR-TEST-WSO2-%04d", n), fx.account)
		mustExec(`INSERT INTO "case" (id, state) VALUES ($1, 'CLOSED')`, frCaseID(n))
		mustExecScoped(`INSERT INTO work_item_feedback (id, work_item_id, rating, submitted_at, created_on, updated_on, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $5, 'test', 'test')`, frFeedbackID(n), frCaseID(n), fx.rating, at, now)
		for j, reason := range fx.reasons {
			mustExecScoped(`INSERT INTO work_item_feedback_reason (id, feedback_id, metric_id, option_value, reason, created_on, updated_on, created_by, updated_by)
				VALUES ($1, $2, $3, $4, $5, $6, $6, 'test', 'test')`, frReasonID(n, j), frFeedbackID(n), frMetricID, j+1, reason, now)
		}
	}
}

func TestFeedbackReasonsIntegration(t *testing.T) {
	pool := caseStatsPool(t)
	seedFeedbackReasons(t, pool)
	repo := repository.NewFeedbackRepository(repository.NewScoped(pool))
	ctx := repository.WithSystemIdentity(context.Background())

	day := func(s string) *time.Time {
		v, _ := time.Parse("2006-01-02", s)
		return &v
	}
	// Every filter is scoped to the fixture's accounts so rows already in a
	// shared test database cannot leak into the counts.
	both := []string{frAccountA, frAccountB}

	tests := []struct {
		name   string
		filter repository.FeedbackFilter
		rating int
		want   []domain.FeedbackBucketResult
	}{
		{
			name:   "rating 2 groups by reason, most-ticked first then by label",
			filter: repository.FeedbackFilter{AccountIDs: both},
			rating: 2,
			want: []domain.FeedbackBucketResult{
				{BucketStart: "Slow response", AvgRating: 2, Count: 3},
				{BucketStart: "Rude tone", AvgRating: 2, Count: 1},
				{BucketStart: "Unclear answer", AvgRating: 2, Count: 1},
			},
		},
		{
			name:   "rating 5 only counts its own reasons",
			filter: repository.FeedbackFilter{AccountIDs: both},
			rating: 5,
			want: []domain.FeedbackBucketResult{
				{BucketStart: "Fast resolution", AvgRating: 5, Count: 2},
				{BucketStart: "Clear communication", AvgRating: 5, Count: 1},
			},
		},
		{
			name:   "rating with no reasons recorded is empty",
			filter: repository.FeedbackFilter{AccountIDs: both},
			rating: 1,
			want:   []domain.FeedbackBucketResult{},
		},
		{
			name:   "rating nobody gave is empty",
			filter: repository.FeedbackFilter{AccountIDs: both},
			rating: 3,
			want:   []domain.FeedbackBucketResult{},
		},
		{
			name:   "account filter",
			filter: repository.FeedbackFilter{AccountIDs: []string{frAccountA}},
			rating: 2,
			want: []domain.FeedbackBucketResult{
				{BucketStart: "Slow response", AvgRating: 2, Count: 2},
				{BucketStart: "Unclear answer", AvgRating: 2, Count: 1},
			},
		},
		{
			name:   "date range is [from, before)",
			filter: repository.FeedbackFilter{AccountIDs: both, From: day("2026-07-15"), Before: day("2026-08-01")},
			rating: 2,
			want: []domain.FeedbackBucketResult{
				{BucketStart: "Slow response", AvgRating: 2, Count: 1},
			},
		},
		{
			name:   "date range with no feedback is empty",
			filter: repository.FeedbackFilter{AccountIDs: both, From: day("2026-09-01")},
			rating: 2,
			want:   []domain.FeedbackBucketResult{},
		},
		{
			name:   "a rating on the filter is overridden by the argument",
			filter: repository.FeedbackFilter{AccountIDs: both, Rating: intPtrFR(5)},
			rating: 2,
			want: []domain.FeedbackBucketResult{
				{BucketStart: "Slow response", AvgRating: 2, Count: 3},
				{BucketStart: "Rude tone", AvgRating: 2, Count: 1},
				{BucketStart: "Unclear answer", AvgRating: 2, Count: 1},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.AggregateFeedbackReasons(ctx, tc.filter, tc.rating)
			if err != nil {
				t.Fatalf("AggregateFeedbackReasons: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func intPtrFR(v int) *int { return &v }
