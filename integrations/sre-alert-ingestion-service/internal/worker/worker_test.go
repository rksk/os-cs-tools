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

// This package's tests exercise the retry/escalation logic entirely against
// hand-rolled mocks of Store, IncidentCreator, and Escalator (matching this
// repo's established no-mocking-library convention, e.g.
// acp-closure-service's internal/sweep tests) — no real Postgres or Twilio
// involved. This is deliberate, not a shortcut taken because a database
// wasn't available: RunOnce's branching (deliver / retry / escalate /
// terminal-fail) is pure decision logic over the Store/IncidentCreator/
// Escalator interfaces, so it's fully covered this way regardless of
// whether internal/store's own Postgres-backed test (postgres_test.go,
// which itself needs SRE_ALERT_TEST_DATABASE_URL) runs in a given
// environment.
package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/csmclient"
	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/store"
)

// mockStore is a hand-rolled Store double recording every call it receives.
type mockStore struct {
	pendingBatchFn func(ctx context.Context, limit int) ([]store.AlertRecord, error)

	delivered     []struct{ id, incidentID string }
	attemptFailed []struct{ id, lastError string }
	escalated     []struct{ id, lastError string }
	failed        []struct{ id, lastError string }
}

func (m *mockStore) PendingBatch(ctx context.Context, limit int) ([]store.AlertRecord, error) {
	return m.pendingBatchFn(ctx, limit)
}

func (m *mockStore) MarkDelivered(ctx context.Context, id, incidentID string) error {
	m.delivered = append(m.delivered, struct{ id, incidentID string }{id, incidentID})
	return nil
}

func (m *mockStore) MarkAttemptFailed(ctx context.Context, id, lastError string) error {
	m.attemptFailed = append(m.attemptFailed, struct{ id, lastError string }{id, lastError})
	return nil
}

func (m *mockStore) MarkEscalated(ctx context.Context, id, lastError string) error {
	m.escalated = append(m.escalated, struct{ id, lastError string }{id, lastError})
	return nil
}

func (m *mockStore) MarkFailed(ctx context.Context, id, lastError string) error {
	m.failed = append(m.failed, struct{ id, lastError string }{id, lastError})
	return nil
}

// mockIncidentCreator is a hand-rolled IncidentCreator double.
type mockIncidentCreator struct {
	createFn func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error)
	calls    int

	// searchFn is optional; when nil, SearchIncidentByTag reports "no match,
	// no error" (found=false, err=nil) — the common case for tests that
	// don't care about the dedup check at all (e.g. every first-attempt
	// test, where it's never even called since row.RetryCount == 0).
	searchFn    func(ctx context.Context, tag string) (*csmclient.CreateIncidentResult, bool, error)
	searchCalls int
	searchTags  []string
}

func (m *mockIncidentCreator) CreateIncident(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
	m.calls++
	return m.createFn(ctx, req)
}

func (m *mockIncidentCreator) SearchIncidentByTag(ctx context.Context, tag string) (*csmclient.CreateIncidentResult, bool, error) {
	m.searchCalls++
	m.searchTags = append(m.searchTags, tag)
	if m.searchFn != nil {
		return m.searchFn(ctx, tag)
	}
	return nil, false, nil
}

// mockEscalator is a hand-rolled Escalator double.
type mockEscalator struct {
	err      error
	messages []string
}

func (m *mockEscalator) Escalate(ctx context.Context, message string) error {
	m.messages = append(m.messages, message)
	return m.err
}

func rowWithPayload(t *testing.T, id string, retryCount int, lastAttemptAt *time.Time) store.AlertRecord {
	t.Helper()
	return store.AlertRecord{
		ID:            id,
		Status:        store.StatusPending,
		RetryCount:    retryCount,
		LastAttemptAt: lastAttemptAt,
		Payload:       []byte(`{"callerId":"caller-1","category":"SERVICE_INTERRUPTION","serviceId":"svc-1","impact":"HIGH","urgency":"HIGH","subject":"test"}`),
	}
}

func TestRunOnce_DeliversSuccessfully(t *testing.T) {
	row := rowWithPayload(t, "alert-1", 0, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return &csmclient.CreateIncidentResult{IncidentID: "inc-1", IncidentNumber: "INC0001"}, nil
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 3})
	w.RunOnce(context.Background())

	if len(s.delivered) != 1 || s.delivered[0].id != "alert-1" || s.delivered[0].incidentID != "inc-1" {
		t.Errorf("delivered = %+v, want one row for alert-1/inc-1", s.delivered)
	}
	if len(s.attemptFailed) != 0 || len(s.escalated) != 0 || len(s.failed) != 0 {
		t.Errorf("unexpected non-delivered transitions: attemptFailed=%v escalated=%v failed=%v", s.attemptFailed, s.escalated, s.failed)
	}
	if len(tw.messages) != 0 {
		t.Error("Twilio should not be called on a successful delivery")
	}
}

func TestRunOnce_RetriesOnTransientErrorBelowThreshold(t *testing.T) {
	row := rowWithPayload(t, "alert-1", 1, nil) // retryCount=1, MaxRetries=5 -> nextRetryCount=2, still below threshold
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return nil, errors.New("connection refused")
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.RunOnce(context.Background())

	if len(s.attemptFailed) != 1 || s.attemptFailed[0].id != "alert-1" {
		t.Errorf("attemptFailed = %+v, want one row for alert-1", s.attemptFailed)
	}
	if len(s.escalated) != 0 || len(s.failed) != 0 || len(s.delivered) != 0 {
		t.Errorf("unexpected transitions: escalated=%v failed=%v delivered=%v", s.escalated, s.failed, s.delivered)
	}
	if len(tw.messages) != 0 {
		t.Error("Twilio should not be called before the retry threshold is reached")
	}
}

// The upstream 401 case is the load-bearing test: csm-integration-service's
// CreateIncident always 401s today (missing end-user identity forwarding),
// and that must be treated exactly like any other transient
// CSM-unavailability signal — retried, not treated as a permanent failure.
func TestRunOnce_401IsRetryableNotTerminal(t *testing.T) {
	row := rowWithPayload(t, "alert-1", 0, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return nil, &apierror.Error{StatusCode: 401, Body: "Missing or invalid user ID token header."}
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.RunOnce(context.Background())

	if len(s.attemptFailed) != 1 {
		t.Fatalf("attemptFailed = %+v, want exactly one retryable-failure transition for a 401", s.attemptFailed)
	}
	if len(s.failed) != 0 {
		t.Errorf("failed = %+v, want zero — a 401 must not be treated as a terminal, non-retryable error", s.failed)
	}
}

func TestRunOnce_400IsNonRetryableTerminal(t *testing.T) {
	row := rowWithPayload(t, "alert-1", 0, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return nil, &apierror.Error{StatusCode: 400, Body: "invalid payload"}
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.RunOnce(context.Background())

	if len(s.failed) != 1 || s.failed[0].id != "alert-1" {
		t.Errorf("failed = %+v, want one terminal-failure row for alert-1", s.failed)
	}
	if len(s.attemptFailed) != 0 || len(s.escalated) != 0 {
		t.Errorf("a 400 must not be retried or escalated: attemptFailed=%v escalated=%v", s.attemptFailed, s.escalated)
	}
	if len(tw.messages) != 0 {
		t.Error("Twilio should not be called for a non-retryable 400")
	}
}

func TestRunOnce_EscalatesAfterMaxRetries(t *testing.T) {
	// retryCount=4, MaxRetries=5 -> nextRetryCount=5 >= 5 -> escalate on this attempt.
	row := rowWithPayload(t, "alert-1", 4, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return nil, errors.New("dial tcp: connection refused")
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.RunOnce(context.Background())

	if len(s.escalated) != 1 || s.escalated[0].id != "alert-1" {
		t.Fatalf("escalated = %+v, want one row for alert-1", s.escalated)
	}
	if len(s.attemptFailed) != 0 {
		t.Errorf("attemptFailed = %+v, want zero once escalation fires", s.attemptFailed)
	}
	if len(tw.messages) != 1 {
		t.Fatalf("Twilio Escalate called %d times, want 1", len(tw.messages))
	}
	if tw.messages[0] == "" {
		t.Error("escalation message is empty")
	}
}

func TestRunOnce_EscalationCallFailureDoesNotBlockStoreTransition(t *testing.T) {
	row := rowWithPayload(t, "alert-1", 4, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return nil, errors.New("connection refused")
	}}
	tw := &mockEscalator{err: errors.New("twilio: 500 internal error")}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.RunOnce(context.Background())

	if len(s.escalated) != 1 {
		t.Fatalf("escalated = %+v, want the row still marked escalated even though the Twilio call itself failed", s.escalated)
	}
}

func TestRunOnce_SkipsRowsNotYetDue(t *testing.T) {
	fixedNow := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	recentAttempt := fixedNow.Add(-1 * time.Second) // far inside the 30s base delay for retryCount 0
	row := rowWithPayload(t, "alert-1", 0, &recentAttempt)

	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return &csmclient.CreateIncidentResult{IncidentID: "inc-1"}, nil
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.now = func() time.Time { return fixedNow }
	w.RunOnce(context.Background())

	if csm.calls != 0 {
		t.Errorf("CreateIncident called %d times, want 0 — row is not yet due per backoff", csm.calls)
	}
	if len(s.delivered) != 0 && len(s.attemptFailed) != 0 {
		t.Error("no store transition should occur for a row that isn't due")
	}
}

func TestRunOnce_AttemptsRowsPastTheirBackoffWindow(t *testing.T) {
	fixedNow := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	oldAttempt := fixedNow.Add(-1 * time.Hour) // well past even the max backoff delay
	row := rowWithPayload(t, "alert-1", 2, &oldAttempt)

	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return &csmclient.CreateIncidentResult{IncidentID: "inc-1"}, nil
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.now = func() time.Time { return fixedNow }
	w.RunOnce(context.Background())

	if csm.calls != 1 {
		t.Errorf("CreateIncident called %d times, want 1 — row is well past its backoff window", csm.calls)
	}
}

func TestRunOnce_CorruptPayloadIsMarkedFailedWithoutCallingCSM(t *testing.T) {
	row := store.AlertRecord{ID: "alert-1", Status: store.StatusPending, Payload: []byte(`not json`)}
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return &csmclient.CreateIncidentResult{IncidentID: "inc-1"}, nil
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 5})
	w.RunOnce(context.Background())

	if csm.calls != 0 {
		t.Errorf("CreateIncident called %d times, want 0 for a corrupt buffered payload", csm.calls)
	}
	if len(s.failed) != 1 {
		t.Fatalf("failed = %+v, want one terminal-failure row for the corrupt payload", s.failed)
	}
	if len(tw.messages) != 0 {
		t.Error("Twilio should not be called for a corrupt payload")
	}
}

func TestRunOnce_StoreLoadErrorIsLoggedNotPanicked(t *testing.T) {
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return nil, errors.New("connection reset by peer")
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		t.Fatal("CreateIncident should not be called when PendingBatch itself fails")
		return nil, nil
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{})
	w.RunOnce(context.Background()) // must not panic
}

// TestRunOnce_FirstAttemptNeverSearchesForDuplicate is the "nothing could
// exist yet on attempt 1" half of the pre-retry dedup contract: RetryCount
// == 0 must skip SearchIncidentByTag entirely, not just tolerate a
// not-found result from it.
func TestRunOnce_FirstAttemptNeverSearchesForDuplicate(t *testing.T) {
	row := rowWithPayload(t, "alert-1", 0, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return &csmclient.CreateIncidentResult{IncidentID: "inc-1"}, nil
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 3})
	w.RunOnce(context.Background())

	if csm.searchCalls != 0 {
		t.Errorf("SearchIncidentByTag called %d times on a first attempt, want 0", csm.searchCalls)
	}
	if csm.calls != 1 {
		t.Errorf("CreateIncident called %d times, want 1", csm.calls)
	}
}

// TestRunOnce_RetryFindsExistingIncident_SkipsDuplicateCreate is the "search
// finds a match -> treat as delivered, no duplicate create" branch: a prior
// attempt's response was lost, but the incident it created is found by the
// dedup search, so this retry must not call CreateIncident again.
func TestRunOnce_RetryFindsExistingIncident_SkipsDuplicateCreate(t *testing.T) {
	row := rowWithPayload(t, "alert-1", 1, nil) // retryCount=1 -> this is a retry
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{
		createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
			t.Fatal("CreateIncident should not be called once the dedup search finds an existing incident")
			return nil, nil
		},
		searchFn: func(ctx context.Context, tag string) (*csmclient.CreateIncidentResult, bool, error) {
			return &csmclient.CreateIncidentResult{IncidentID: "inc-existing", IncidentNumber: "INC0009999"}, true, nil
		},
	}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{MaxRetries: 3})
	w.RunOnce(context.Background())

	if csm.searchCalls != 1 {
		t.Fatalf("SearchIncidentByTag called %d times, want 1", csm.searchCalls)
	}
	wantTag := csmclient.DedupTag("alert-1")
	if csm.searchTags[0] != wantTag {
		t.Errorf("search tag = %q, want %q", csm.searchTags[0], wantTag)
	}
	if csm.calls != 0 {
		t.Errorf("CreateIncident called %d times, want 0", csm.calls)
	}
	if len(s.delivered) != 1 || s.delivered[0].id != "alert-1" || s.delivered[0].incidentID != "inc-existing" {
		t.Errorf("delivered = %+v, want one row for alert-1/inc-existing", s.delivered)
	}
	if len(s.attemptFailed) != 0 || len(s.escalated) != 0 || len(s.failed) != 0 {
		t.Errorf("unexpected non-delivered transitions: attemptFailed=%v escalated=%v failed=%v", s.attemptFailed, s.escalated, s.failed)
	}
}

// TestRunOnce_RetrySearchFailsOpen_ProceedsToCreate covers both "no match"
// and "the search call itself errored" (e.g. the same 401 CreateIncident
// gets today) — both must fail open toward attempting delivery, not toward
// silently giving up.
func TestRunOnce_RetrySearchFailsOpen_ProceedsToCreate(t *testing.T) {
	cases := []struct {
		name     string
		searchFn func(ctx context.Context, tag string) (*csmclient.CreateIncidentResult, bool, error)
	}{
		{
			name: "no match found",
			searchFn: func(ctx context.Context, tag string) (*csmclient.CreateIncidentResult, bool, error) {
				return nil, false, nil
			},
		},
		{
			name: "search call itself errors (e.g. the same 401 CreateIncident gets today)",
			searchFn: func(ctx context.Context, tag string) (*csmclient.CreateIncidentResult, bool, error) {
				return nil, false, &apierror.Error{StatusCode: 401, Body: "Missing or invalid user ID token header."}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := rowWithPayload(t, "alert-1", 1, nil) // retryCount=1 -> this is a retry
			s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
				return []store.AlertRecord{row}, nil
			}}
			csm := &mockIncidentCreator{
				createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
					return &csmclient.CreateIncidentResult{IncidentID: "inc-new"}, nil
				},
				searchFn: tc.searchFn,
			}
			tw := &mockEscalator{}

			w := New(s, csm, tw, Config{MaxRetries: 3})
			w.RunOnce(context.Background())

			if csm.searchCalls != 1 {
				t.Fatalf("SearchIncidentByTag called %d times, want 1", csm.searchCalls)
			}
			if csm.calls != 1 {
				t.Errorf("CreateIncident called %d times, want 1 (fail-open must still attempt delivery)", csm.calls)
			}
			if len(s.delivered) != 1 || s.delivered[0].incidentID != "inc-new" {
				t.Errorf("delivered = %+v, want one row for alert-1/inc-new", s.delivered)
			}
		})
	}
}

// TestRunOnce_DefaultRetryBudgetEscalatesOnThirdFailure pins the reduced
// retry budget: with Config{} (defaults applied, MaxRetries=3), a row must
// escalate once its 3rd failure is reached, not its 5th.
func TestRunOnce_DefaultRetryBudgetEscalatesOnThirdFailure(t *testing.T) {
	// retryCount=2 -> nextRetryCount=3 >= default MaxRetries(3) -> escalate.
	row := rowWithPayload(t, "alert-1", 2, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return nil, errors.New("connection refused")
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{}) // defaults: MaxRetries=3
	w.RunOnce(context.Background())

	if len(s.escalated) != 1 || s.escalated[0].id != "alert-1" {
		t.Fatalf("escalated = %+v, want one row for alert-1 on the 3rd failure with the default retry budget", s.escalated)
	}
	if len(tw.messages) != 1 {
		t.Fatalf("Twilio Escalate called %d times, want 1", len(tw.messages))
	}
}

// TestRunOnce_DefaultRetryBudgetDoesNotEscalateOnSecondFailure is the
// complement of the above: one failure short of the default budget must
// still just retry, not escalate.
func TestRunOnce_DefaultRetryBudgetDoesNotEscalateOnSecondFailure(t *testing.T) {
	// retryCount=1 -> nextRetryCount=2 < default MaxRetries(3) -> retry, not escalate.
	row := rowWithPayload(t, "alert-1", 1, nil)
	s := &mockStore{pendingBatchFn: func(ctx context.Context, limit int) ([]store.AlertRecord, error) {
		return []store.AlertRecord{row}, nil
	}}
	csm := &mockIncidentCreator{createFn: func(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error) {
		return nil, errors.New("connection refused")
	}}
	tw := &mockEscalator{}

	w := New(s, csm, tw, Config{}) // defaults: MaxRetries=3
	w.RunOnce(context.Background())

	if len(s.attemptFailed) != 1 {
		t.Fatalf("attemptFailed = %+v, want one row for alert-1", s.attemptFailed)
	}
	if len(s.escalated) != 0 {
		t.Errorf("escalated = %+v, want zero — one failure short of the default retry budget", s.escalated)
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.BatchSize != 50 {
		t.Errorf("default BatchSize = %d, want 50", cfg.BatchSize)
	}
	if cfg.PollInterval != 15*time.Second {
		t.Errorf("default PollInterval = %v, want 15s", cfg.PollInterval)
	}
}
