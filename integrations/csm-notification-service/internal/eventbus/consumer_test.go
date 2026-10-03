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

package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

// testPolicy keeps the pauses between attempts to a millisecond so
// exhaustion tests don't actually wait out a real backoff schedule.
var testPolicy = RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}

// testConsumer builds a Consumer with no reader — enough for processRecord,
// which never touches it.
func testConsumer(policy RetryPolicy, park ParkFunc) *Consumer {
	return &Consumer{name: "test", policy: policy.normalized(), park: park}
}

func TestProcessRecord_SucceedsFirstAttempt(t *testing.T) {
	calls := 0
	handle := func(ctx context.Context, r Record) error {
		calls++
		return nil
	}
	c := testConsumer(testPolicy, nil)
	ok := c.processRecord(context.Background(), Record{}, handle, nil)
	if !ok {
		t.Error("processRecord() = false, want true")
	}
	if calls != 1 {
		t.Errorf("handle called %d times, want 1", calls)
	}
	if got := c.Stats().Handled.Load(); got != 1 {
		t.Errorf("Handled = %d, want 1", got)
	}
}

func TestProcessRecord_SucceedsAfterRetries(t *testing.T) {
	calls := 0
	var attempts []int
	handle := func(ctx context.Context, r Record) error {
		calls++
		attempts = append(attempts, r.Attempt)
		if calls < 3 {
			return errors.New("transient failure")
		}
		return nil
	}
	c := testConsumer(testPolicy, nil)
	ok := c.processRecord(context.Background(), Record{}, handle, nil)
	if !ok {
		t.Error("processRecord() = false, want true")
	}
	if calls != 3 {
		t.Errorf("handle called %d times, want 3", calls)
	}
	if len(attempts) != 3 || attempts[0] != 1 || attempts[1] != 2 || attempts[2] != 3 {
		t.Errorf("Record.Attempt across calls = %v, want 1, 2, 3", attempts)
	}
	if got := c.Stats().FailedAttempts.Load(); got != 2 {
		t.Errorf("FailedAttempts = %d, want 2", got)
	}
}

func TestProcessRecord_ExhaustedWithNilOnExhaustedAndNoParking_StillCommits(t *testing.T) {
	calls := 0
	handle := func(ctx context.Context, r Record) error {
		calls++
		return errors.New("persistent failure")
	}
	c := testConsumer(testPolicy, nil)
	ok := c.processRecord(context.Background(), Record{}, handle, nil)
	if !ok {
		t.Error("processRecord() = false, want true (should still commit even when dropping)")
	}
	if calls != 3 {
		t.Errorf("handle called %d times, want 3 (MaxAttempts)", calls)
	}
	if got := c.Stats().Dropped.Load(); got != 1 {
		t.Errorf("Dropped = %d, want 1 — a dropped record must be counted, never silent", got)
	}
}

func TestProcessRecord_ExhaustedCallsOnExhausted(t *testing.T) {
	handle := func(ctx context.Context, r Record) error {
		return errors.New("persistent failure")
	}
	var gotRecord Record
	var gotErr error
	onExhausted := func(ctx context.Context, record Record, handleErr error) error {
		gotRecord = record
		gotErr = handleErr
		return nil
	}
	record := Record{Topic: "case-events", Partition: 2, Offset: 42}
	c := testConsumer(testPolicy, nil)
	ok := c.processRecord(context.Background(), record, handle, onExhausted)
	if !ok {
		t.Error("processRecord() = false, want true")
	}
	if gotRecord.Topic != "case-events" || gotRecord.Partition != 2 || gotRecord.Offset != 42 {
		t.Errorf("onExhausted got record = %+v, want the original record's identity preserved", gotRecord)
	}
	if !gotRecord.IsFinalAttempt {
		t.Error("onExhausted's record.IsFinalAttempt = false, want true")
	}
	if gotErr == nil || gotErr.Error() != "persistent failure" {
		t.Errorf("onExhausted handleErr = %v, want the last handle error", gotErr)
	}
	if got := c.Stats().DeadLettered.Load(); got != 1 {
		t.Errorf("DeadLettered = %d, want 1", got)
	}
}

func TestProcessRecord_OnExhaustedFailure_StillCommits(t *testing.T) {
	handle := func(ctx context.Context, r Record) error {
		return errors.New("persistent failure")
	}
	onExhausted := func(ctx context.Context, record Record, handleErr error) error {
		return errors.New("dead-letter topic unreachable")
	}
	c := testConsumer(testPolicy, nil)
	ok := c.processRecord(context.Background(), Record{}, handle, onExhausted)
	if !ok {
		t.Error("processRecord() = false, want true (nowhere lower to fall back to, so still commit)")
	}
}

// TestProcessRecord_OnExhaustedFailure_MakesCleanupCallWithNoMoreRetries is a
// regression test for a real gap CodeRabbit flagged: when the dead-letter
// publish itself fails, there is truly no future delivery of this record's
// content coming on any topic — but without an extra call, a Handle that
// tracks content-keyed idempotency state (see dispatch.Dispatcher) would
// never learn that and would leak its tracking forever. processRecord must
// call handle one more time, with NoMoreRetries set, purely so that
// cleanup can happen.
func TestProcessRecord_OnExhaustedFailure_MakesCleanupCallWithNoMoreRetries(t *testing.T) {
	var calls []Record
	handle := func(ctx context.Context, r Record) error {
		calls = append(calls, r)
		return errors.New("persistent failure")
	}
	onExhausted := func(ctx context.Context, record Record, handleErr error) error {
		return errors.New("dead-letter topic unreachable")
	}
	c := testConsumer(testPolicy, nil)
	ok := c.processRecord(context.Background(), Record{}, handle, onExhausted)
	if !ok {
		t.Error("processRecord() = false, want true")
	}
	if len(calls) != 4 {
		t.Fatalf("handle called %d times, want 4 (3 retries + 1 cleanup call after the dead-letter publish itself failed)", len(calls))
	}
	for i, c := range calls[:3] {
		if c.NoMoreRetries {
			t.Errorf("attempt %d: NoMoreRetries = true, want false (onExhausted hadn't been tried yet)", i+1)
		}
	}
	if !calls[3].NoMoreRetries {
		t.Error("cleanup call: NoMoreRetries = false, want true (the dead-letter publish failed — nothing will ever redeliver this content)")
	}
}

// TestProcessRecord_OnExhaustedFailure_FallsBackToParking: a record whose
// dead-letter publish failed is parked rather than dropped when a ParkFunc
// is configured, after the cleanup call.
func TestProcessRecord_OnExhaustedFailure_FallsBackToParking(t *testing.T) {
	var order []string
	handle := func(ctx context.Context, r Record) error {
		if r.NoMoreRetries {
			order = append(order, "cleanup")
		}
		return errors.New("persistent failure")
	}
	onExhausted := func(ctx context.Context, record Record, handleErr error) error {
		return errors.New("dead-letter topic unreachable")
	}
	var parked Record
	park := func(ctx context.Context, record Record, handleErr error) error {
		order = append(order, "park")
		parked = record
		return nil
	}
	c := testConsumer(testPolicy, park)
	if ok := c.processRecord(context.Background(), Record{Topic: "case-events", Offset: 7}, handle, onExhausted); !ok {
		t.Fatal("processRecord() = false, want true")
	}
	if len(order) != 2 || order[0] != "cleanup" || order[1] != "park" {
		t.Errorf("sequence = %v, want the cleanup handle call and then the park", order)
	}
	if parked.Offset != 7 || parked.Attempt != 3 {
		t.Errorf("parked record = %+v, want the original coordinates with Attempt = 3", parked)
	}
	if got := c.Stats().Parked.Load(); got != 1 {
		t.Errorf("Parked = %d, want 1", got)
	}
	if got := c.Stats().Dropped.Load(); got != 0 {
		t.Errorf("Dropped = %d, want 0", got)
	}
}

// TestProcessRecord_TerminalTier_ParksAfterFinalAttempt is the dead-letter
// consumer's shape (onExhausted == nil): the last attempt already carries
// NoMoreRetries, so no cleanup call is needed, and the record is parked
// with the last handle error.
func TestProcessRecord_TerminalTier_ParksAfterFinalAttempt(t *testing.T) {
	calls := 0
	handle := func(ctx context.Context, r Record) error {
		calls++
		return errors.New("still failing")
	}
	var gotErr error
	park := func(ctx context.Context, record Record, handleErr error) error {
		gotErr = handleErr
		return nil
	}
	c := testConsumer(testPolicy, park)
	if ok := c.processRecord(context.Background(), Record{}, handle, nil); !ok {
		t.Fatal("processRecord() = false, want true")
	}
	if calls != 3 {
		t.Errorf("handle called %d times, want 3 (no extra cleanup call on a terminal tier)", calls)
	}
	if gotErr == nil || gotErr.Error() != "still failing" {
		t.Errorf("park got handleErr = %v, want the last handle error", gotErr)
	}
	if got := c.Stats().Parked.Load(); got != 1 {
		t.Errorf("Parked = %d, want 1", got)
	}
}

func TestProcessRecord_ParkFailure_CountsAsDroppedAndStillCommits(t *testing.T) {
	handle := func(ctx context.Context, r Record) error { return errors.New("still failing") }
	park := func(ctx context.Context, record Record, handleErr error) error {
		return errors.New("parking topic unreachable")
	}
	c := testConsumer(testPolicy, park)
	if ok := c.processRecord(context.Background(), Record{}, handle, nil); !ok {
		t.Fatal("processRecord() = false, want true (nothing left to fall back to)")
	}
	if got := c.Stats().Dropped.Load(); got != 1 {
		t.Errorf("Dropped = %d, want 1", got)
	}
	if got := c.Stats().Parked.Load(); got != 0 {
		t.Errorf("Parked = %d, want 0", got)
	}
}

func TestProcessRecord_ContextCanceledMidRetry_DoesNotCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	handle := func(ctx context.Context, r Record) error {
		calls++
		if calls == 1 {
			cancel()
		}
		return errors.New("failure")
	}
	c := testConsumer(RetryPolicy{MaxAttempts: 3, BaseDelay: 50 * time.Millisecond, MaxDelay: 50 * time.Millisecond}, nil)
	ok := c.processRecord(ctx, Record{}, handle, nil)
	if ok {
		t.Error("processRecord() = true, want false when ctx is canceled mid-retry-wait")
	}
	if calls != 1 {
		t.Errorf("handle called %d times, want 1 (should stop retrying once ctx is canceled)", calls)
	}
}

func TestProcessRecord_IsFinalAttemptOnlyOnLastCall(t *testing.T) {
	var finalFlags []bool
	handle := func(ctx context.Context, r Record) error {
		finalFlags = append(finalFlags, r.IsFinalAttempt)
		return errors.New("keep failing")
	}
	testConsumer(testPolicy, nil).processRecord(context.Background(), Record{}, handle, nil)
	want := []bool{false, false, true}
	if len(finalFlags) != len(want) {
		t.Fatalf("got %d attempts, want %d", len(finalFlags), len(want))
	}
	for i, w := range want {
		if finalFlags[i] != w {
			t.Errorf("attempt %d: IsFinalAttempt = %v, want %v", i+1, finalFlags[i], w)
		}
	}
}

// TestProcessRecord_NoMoreRetries_FalseWhenOnExhaustedSet verifies that a
// record with a dead-letter tier to fall back to (onExhausted != nil, the
// main topic's own Consumer.Run) never reports NoMoreRetries=true, even on
// its own final attempt — there's still a DLQ topic redelivery coming for
// the exact same content. See eventbus.Record.NoMoreRetries' doc comment
// and dispatch.recordBaseKey for why a Handle implementation that keys
// idempotency off content (not Kafka coordinates) depends on this.
func TestProcessRecord_NoMoreRetries_FalseWhenOnExhaustedSet(t *testing.T) {
	var flags []bool
	handle := func(ctx context.Context, r Record) error {
		flags = append(flags, r.NoMoreRetries)
		return errors.New("keep failing")
	}
	onExhausted := func(ctx context.Context, record Record, handleErr error) error { return nil }
	testConsumer(testPolicy, nil).processRecord(context.Background(), Record{}, handle, onExhausted)
	for i, got := range flags {
		if got {
			t.Errorf("attempt %d: NoMoreRetries = true, want false (onExhausted is set — a DLQ tier is still coming)", i+1)
		}
	}
}

// TestProcessRecord_NoMoreRetries_TrueOnFinalAttemptWhenNoOnExhausted
// verifies the DLQ topic's own Consumer.Run shape (onExhausted == nil):
// NoMoreRetries is false on every attempt except the last, where it's true
// — there really is nowhere further for this content to go (parking is
// not a retry).
func TestProcessRecord_NoMoreRetries_TrueOnFinalAttemptWhenNoOnExhausted(t *testing.T) {
	var flags []bool
	handle := func(ctx context.Context, r Record) error {
		flags = append(flags, r.NoMoreRetries)
		return errors.New("keep failing")
	}
	park := func(ctx context.Context, record Record, handleErr error) error { return nil }
	testConsumer(testPolicy, park).processRecord(context.Background(), Record{}, handle, nil)
	want := []bool{false, false, true}
	if len(flags) != len(want) {
		t.Fatalf("got %d attempts, want %d", len(flags), len(want))
	}
	for i, w := range want {
		if flags[i] != w {
			t.Errorf("attempt %d: NoMoreRetries = %v, want %v", i+1, flags[i], w)
		}
	}
}

// TestProcessRecord_UsesPolicyDelays: the pauses between attempts follow the
// policy — a 3-attempt schedule with a 20 ms base must take at least the
// schedule's guaranteed floor (10 ms + 20 ms with jitter at its minimum).
func TestProcessRecord_UsesPolicyDelays(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 3, BaseDelay: 20 * time.Millisecond, MaxDelay: time.Second}
	handle := func(ctx context.Context, r Record) error { return errors.New("keep failing") }
	start := time.Now()
	testConsumer(policy, nil).processRecord(context.Background(), Record{}, handle, nil)
	if elapsed, floor := time.Since(start), policy.MinTotalDelay(); elapsed < floor {
		t.Errorf("processRecord took %v, want at least the policy's floor %v", elapsed, floor)
	}
}

func TestRetryPolicy_Delay_ExponentialWithJitterBounds(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 5, BaseDelay: 2 * time.Second, MaxDelay: time.Minute}
	nominal := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	for i, want := range nominal {
		attempt := i + 1
		if got := p.delay(attempt, 0); got != want/2 {
			t.Errorf("attempt %d: delay at minimum jitter = %v, want %v (half the nominal)", attempt, got, want/2)
		}
		if got := p.delay(attempt, 0.999999); got < want-time.Millisecond || got >= want {
			t.Errorf("attempt %d: delay at maximum jitter = %v, want just under the nominal %v", attempt, got, want)
		}
		for range 50 {
			if got := p.Delay(attempt); got < want/2 || got >= want {
				t.Fatalf("attempt %d: Delay() = %v, outside [%v, %v)", attempt, got, want/2, want)
			}
		}
	}
}

func TestRetryPolicy_Delay_CappedAtMax(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 10, BaseDelay: 30 * time.Second, MaxDelay: 5 * time.Minute}
	// 30 s, 60 s, 120 s, 240 s, then the cap.
	if got := p.delay(5, 0.5); got != 5*time.Minute/2+5*time.Minute/4 {
		t.Errorf("attempt 5 delay = %v, want the cap (5m) with mid jitter", got)
	}
	if got := p.delay(9, 0.999); got >= 5*time.Minute {
		t.Errorf("attempt 9 delay = %v, want below the 5m cap", got)
	}
}

func TestRetryPolicy_Normalized_ToleratesZeroValues(t *testing.T) {
	p := RetryPolicy{}.normalized()
	if p.MaxAttempts != 1 || p.BaseDelay != 0 || p.MaxDelay != 0 {
		t.Errorf("normalized zero policy = %+v, want one attempt and no delays", p)
	}
	if got := (RetryPolicy{MaxAttempts: 2, BaseDelay: time.Second, MaxDelay: time.Millisecond}).normalized().MaxDelay; got != time.Second {
		t.Errorf("MaxDelay below BaseDelay normalized to %v, want the base", got)
	}
	// A single-attempt policy never sleeps.
	calls := 0
	handle := func(ctx context.Context, r Record) error { calls++; return errors.New("no") }
	testConsumer(RetryPolicy{}, nil).processRecord(context.Background(), Record{}, handle, nil)
	if calls != 1 {
		t.Errorf("zero policy made %d attempts, want 1", calls)
	}
}

// TestDefaultPolicies_CoverAMultiMinuteOutage pins the operational promise
// the defaults make: the two tiers together keep retrying for several
// minutes at the very least, even with every jitter roll at its minimum.
func TestDefaultPolicies_CoverAMultiMinuteOutage(t *testing.T) {
	if got := DefaultRetryPolicy.MinTotalDelay(); got < 15*time.Second {
		t.Errorf("main tier floor = %v, want at least 15s", got)
	}
	if got := DefaultDeadLetterRetryPolicy.MinTotalDelay(); got < 3*time.Minute {
		t.Errorf("dead-letter tier floor = %v, want at least 3m", got)
	}
}

func TestNotBefore_HeaderRoundTrip(t *testing.T) {
	want := time.Date(2026, 10, 2, 12, 34, 56, 789000000, time.UTC)
	headers := []kafka.Header{{Key: "other", Value: []byte("x")}, {Key: HeaderNotBefore, Value: []byte(NotBeforeHeader(want))}}
	if got := notBeforeOf(headers); !got.Equal(want) {
		t.Errorf("notBeforeOf() = %v, want %v", got, want)
	}
	if got := notBeforeOf(nil); !got.IsZero() {
		t.Errorf("notBeforeOf(nil) = %v, want zero", got)
	}
	if got := notBeforeOf([]kafka.Header{{Key: HeaderNotBefore, Value: []byte("not a time")}}); !got.IsZero() {
		t.Errorf("notBeforeOf(malformed) = %v, want zero (ignored, not a hold)", got)
	}
}

func TestWaitUntil(t *testing.T) {
	if !waitUntil(context.Background(), time.Time{}) {
		t.Error("waitUntil(zero) = false, want true immediately")
	}
	if !waitUntil(context.Background(), time.Now().Add(-time.Minute)) {
		t.Error("waitUntil(past) = false, want true immediately")
	}
	start := time.Now()
	if !waitUntil(context.Background(), start.Add(30*time.Millisecond)) {
		t.Error("waitUntil(near future) = false, want true")
	}
	if time.Since(start) < 30*time.Millisecond {
		t.Error("waitUntil returned before the instant")
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(10 * time.Millisecond); cancel() }()
	if waitUntil(ctx, time.Now().Add(time.Hour)) {
		t.Error("waitUntil(far future, canceled ctx) = true, want false")
	}
}

func TestParkedRecord_PreservesPayloadAndCoordinates(t *testing.T) {
	record := Record{Topic: "case-events", Partition: 1, Offset: 99, Key: []byte("CASE-1"), Value: []byte(`{"type":"case.created","entityId":"CASE-1","payload":{}}`), Attempt: 5}
	parked := NewParkedRecord("dlq", record, "dispatch: upstream returned 503")
	raw, err := json.Marshal(parked)
	if err != nil {
		t.Fatal(err)
	}
	var back ParkedRecord
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if string(back.Payload()) != string(record.Value) {
		t.Errorf("Payload() = %s, want the original value byte for byte", back.Payload())
	}
	if back.Key != "CASE-1" || back.SourceTopic != "case-events" || back.SourcePartition != 1 || back.SourceOffset != 99 || back.Attempts != 5 || back.Consumer != "dlq" {
		t.Errorf("parked envelope = %+v, want the source coordinates preserved", back)
	}
	if back.Failure != "dispatch: upstream returned 503" {
		t.Errorf("Failure = %q", back.Failure)
	}
	if back.ParkedAt.IsZero() {
		t.Error("ParkedAt is zero")
	}

	// A value that is not JSON travels base64-encoded and still comes back
	// byte for byte.
	binary := NewParkedRecord("dlq", Record{Value: []byte{0xff, 0x00, 'x'}}, "decode failed")
	raw, _ = json.Marshal(binary)
	var backBinary ParkedRecord
	_ = json.Unmarshal(raw, &backBinary)
	if got := backBinary.Payload(); len(got) != 3 || got[0] != 0xff || got[1] != 0 || got[2] != 'x' {
		t.Errorf("binary Payload() = %v, want the original bytes", got)
	}
}
