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

package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/ledger"
	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/registry"
)

// fakeLedger is an in-memory LedgerClient. Every call is recorded so a
// test can assert on exactly which ledger writes happened and in what
// order; the per-method error fields make any stage fail on demand.
type fakeLedger struct {
	mu sync.Mutex

	allowed    bool
	attemptErr error
	// attemptFn, when set, overrides allowed/attemptErr per task.
	attemptFn func(taskName string) (ledger.Claim, error)

	completeErr error
	failErr     error

	attempts  []string
	completes []string
	fails     []failCall
}

type failCall struct {
	id     string
	errMsg string
}

func (f *fakeLedger) Attempt(_ context.Context, taskName string, _ time.Time, _ time.Duration) (ledger.Claim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts = append(f.attempts, taskName)
	if f.attemptFn != nil {
		return f.attemptFn(taskName)
	}
	if f.attemptErr != nil {
		return ledger.Claim{}, f.attemptErr
	}
	return ledger.Claim{Allowed: f.allowed, Run: ledger.Run{ID: "run-" + taskName, TaskName: taskName, AttemptCount: 1}}, nil
}

func (f *fakeLedger) Complete(_ context.Context, id string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completes = append(f.completes, id)
	return f.completeErr
}

func (f *fakeLedger) Fail(_ context.Context, id string, _ int, errMsg string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fails = append(f.fails, failCall{id: id, errMsg: errMsg})
	return f.failErr
}

type sentEmail struct {
	to, cc  []string
	subject string
	body    string
}

type fakeEmail struct {
	mu      sync.Mutex
	sendErr error
	sent    []sentEmail
}

func (f *fakeEmail) SendEmail(_ context.Context, to, cc []string, subject, htmlBody string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentEmail{to: to, cc: cc, subject: subject, body: htmlBody})
	return f.sendErr
}

func okHandler(context.Context) error { return nil }

func newEngine(tasks []registry.Task, l *fakeLedger, m *fakeEmail) *Engine {
	return New(tasks, l, m, 5*time.Minute, []string{"oncall@example.com"}, true)
}

// stagesOf flattens Tick's joined error into the set of task/stage pairs it
// names, so a test can assert on exactly which failures were reported.
func stagesOf(err error) map[string]bool {
	out := map[string]bool{}
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		var te *TaskError
		if errors.As(e, &te) {
			out[te.Task+"/"+string(te.Stage)] = true
		}
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, inner := range j.Unwrap() {
				walk(inner)
			}
		}
	}
	walk(err)
	return out
}

func TestTick_EverythingSucceedsReturnsNil(t *testing.T) {
	l := &fakeLedger{allowed: true}
	m := &fakeEmail{}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: okHandler},
		{Name: "b", Schedule: "0 3 * * *", Handler: okHandler},
	}, l, m)

	if err := eng.Tick(context.Background(), time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(l.completes) != 2 {
		t.Fatalf("expected both tasks completed, got %v", l.completes)
	}
	if len(l.fails) != 0 || len(m.sent) != 0 {
		t.Fatalf("expected no failures recorded and no email, got fails=%v sent=%d", l.fails, len(m.sent))
	}
}

func TestTick_DeniedClaimIsNotAnError(t *testing.T) {
	ran := false
	l := &fakeLedger{allowed: false}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { ran = true; return nil }},
	}, l, &fakeEmail{})

	if err := eng.Tick(context.Background(), time.Now()); err != nil {
		t.Fatalf("a denied claim must not be reported as a failure, got: %v", err)
	}
	if ran {
		t.Fatal("handler must not run on a denied claim")
	}
	if len(l.completes) != 0 || len(l.fails) != 0 {
		t.Fatalf("no ledger report-back expected on a denied claim, got completes=%v fails=%v", l.completes, l.fails)
	}
}

func TestTick_HandlerErrorIsReturnedRecordedAndAlerted(t *testing.T) {
	l := &fakeLedger{allowed: true}
	m := &fakeEmail{}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return errors.New("boom") },
			To: []string{"owner@example.com"}, Cc: []string{"cc@example.com"}},
	}, l, m)

	err := eng.Tick(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected a non-nil error when a handler fails")
	}
	if got := stagesOf(err); !got["a/handler"] {
		t.Fatalf("expected a handler-stage TaskError for task a, got %v (err: %v)", got, err)
	}
	if len(l.fails) != 1 || l.fails[0].id != "run-a" || l.fails[0].errMsg != "boom" {
		t.Fatalf("expected one Fail for run-a carrying the handler error, got %+v", l.fails)
	}
	if len(l.completes) != 0 {
		t.Fatalf("Complete must not be called on a failed handler, got %v", l.completes)
	}
	if len(m.sent) != 1 {
		t.Fatalf("expected exactly one alert email, got %d", len(m.sent))
	}
	e := m.sent[0]
	if strings.Join(e.to, ",") != "owner@example.com,oncall@example.com" {
		t.Errorf("To must merge the task's own To with AlertRecipients, got %v", e.to)
	}
	if strings.Join(e.cc, ",") != "cc@example.com" {
		t.Errorf("Cc must stay per-task, got %v", e.cc)
	}
	if !strings.Contains(e.subject, "FAILED: a") {
		t.Errorf("subject should name the task, got %q", e.subject)
	}
	if !strings.Contains(e.body, "boom") {
		t.Errorf("body should carry the handler error, got %q", e.body)
	}
}

func TestTick_AlertsDisabledStillRecordsButSendsNothing(t *testing.T) {
	l := &fakeLedger{allowed: true}
	m := &fakeEmail{}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return errors.New("boom") }},
	}, l, m)
	eng.AlertsEnabled = false

	if err := eng.Tick(context.Background(), time.Now()); err == nil {
		t.Fatal("the failure must still be returned with alerts disabled")
	}
	if len(l.fails) != 1 {
		t.Fatalf("the failure must still be recorded in the ledger, got %+v", l.fails)
	}
	if len(m.sent) != 0 {
		t.Fatalf("no email expected with alerts disabled, got %d", len(m.sent))
	}
}

func TestTick_NoRecipientsSendsNothing(t *testing.T) {
	l := &fakeLedger{allowed: true}
	m := &fakeEmail{}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return errors.New("boom") }},
	}, l, m)
	eng.AlertRecipients = nil

	_ = eng.Tick(context.Background(), time.Now())
	if len(m.sent) != 0 {
		t.Fatalf("no email expected with no recipients at all, got %d", len(m.sent))
	}
}

func TestTick_ClaimErrorIsReturned(t *testing.T) {
	ran := false
	l := &fakeLedger{attemptErr: errors.New("upstream returned 503")}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { ran = true; return nil }},
	}, l, &fakeEmail{})

	err := eng.Tick(context.Background(), time.Now())
	if got := stagesOf(err); !got["a/claim"] {
		t.Fatalf("expected a claim-stage TaskError, got %v (err: %v)", got, err)
	}
	if ran {
		t.Fatal("handler must not run when the claim itself failed")
	}
}

func TestTick_CompleteErrorIsReturned(t *testing.T) {
	l := &fakeLedger{allowed: true, completeErr: errors.New("upstream returned 500")}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: okHandler},
	}, l, &fakeEmail{})

	err := eng.Tick(context.Background(), time.Now())
	if got := stagesOf(err); !got["a/complete"] {
		t.Fatalf("a failed Complete must be reported, got %v (err: %v)", got, err)
	}
}

func TestTick_FailErrorIsReportedAlongsideHandlerError(t *testing.T) {
	l := &fakeLedger{allowed: true, failErr: errors.New("upstream returned 500")}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return errors.New("boom") }},
	}, l, &fakeEmail{})

	err := eng.Tick(context.Background(), time.Now())
	got := stagesOf(err)
	if !got["a/handler"] || !got["a/fail"] {
		t.Fatalf("expected both the handler and the Fail-stage errors, got %v (err: %v)", got, err)
	}
}

func TestTick_AlertSendFailureIsReported(t *testing.T) {
	l := &fakeLedger{allowed: true}
	m := &fakeEmail{sendErr: errors.New("mail service down")}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return errors.New("boom") }},
	}, l, m)

	err := eng.Tick(context.Background(), time.Now())
	if got := stagesOf(err); !got["a/alert"] {
		t.Fatalf("a failed alert send must be reported, got %v (err: %v)", got, err)
	}
}

func TestTick_InvalidScheduleIsReturned(t *testing.T) {
	l := &fakeLedger{allowed: true}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "not a cron", Handler: okHandler},
	}, l, &fakeEmail{})

	err := eng.Tick(context.Background(), time.Now())
	if got := stagesOf(err); !got["a/schedule"] {
		t.Fatalf("expected a schedule-stage TaskError, got %v (err: %v)", got, err)
	}
	if len(l.attempts) != 0 {
		t.Fatal("no claim must be attempted for an invalid schedule")
	}
}

func TestTick_OneTaskFailingDoesNotStopTheRest(t *testing.T) {
	l := &fakeLedger{allowed: true}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { return errors.New("boom") }},
		{Name: "b", Schedule: "*/5 * * * *", Handler: okHandler},
	}, l, &fakeEmail{})

	err := eng.Tick(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected task a's failure to be returned")
	}
	if len(l.completes) != 1 || l.completes[0] != "run-b" {
		t.Fatalf("task b must still run and complete, got completes=%v", l.completes)
	}
}

func TestTick_CancelledContextIsInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	l := &fakeLedger{allowed: true}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: okHandler},
	}, l, &fakeEmail{})

	err := eng.Tick(ctx, time.Now())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got %v", err)
	}
	if len(l.attempts) != 0 {
		t.Fatalf("no task must be claimed once the context is cancelled, got %v", l.attempts)
	}
}

func TestTick_CancellationMidTickStopsBeforeTheNextTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := &fakeLedger{allowed: true}
	eng := newEngine([]registry.Task{
		{Name: "a", Schedule: "*/5 * * * *", Handler: func(context.Context) error { cancel(); return nil }},
		{Name: "b", Schedule: "*/5 * * * *", Handler: okHandler},
	}, l, &fakeEmail{})

	err := eng.Tick(ctx, time.Now())
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got %v", err)
	}
	if strings.Join(l.attempts, ",") != "a" {
		t.Fatalf("only task a should have been claimed, got %v", l.attempts)
	}
}
