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

package paging

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
)

// On a rung that rings and posts (channel both), the call alone decides
// whether the rung is retried; the card is retried on its own.

const cardRoom = "WSO2 API Manager"

// triesCaller counts every dial, failed or not: fakeCaller only records the
// ones that went through, and a re-dial is what these tests are about.
type triesCaller struct {
	fakeCaller
	tries int
}

func (c *triesCaller) MakeSSMLCall(ctx context.Context, to string, s notifications.Speech) (notifications.Call, error) {
	c.tries++
	return c.fakeCaller.MakeSSMLCall(ctx, to, s)
}

func (c *triesCaller) MakeCall(ctx context.Context, to, msg string) (notifications.Call, error) {
	c.tries++
	return c.fakeCaller.MakeCall(ctx, to, msg)
}

// bothEngine pages by call and card, as ChannelBoth wires it.
func bothEngine(caller callPlacer, chat chatSender, store ladderStore, notes incidentNotes) *Engine {
	return &Engine{
		policies: DefaultPolicy,
		resolver: fullResolver(),
		notifiers: []notifier{
			voiceNotifier{calls: caller},
			chatNotifier{chat: chat, links: fakeLinks{}, audience: cardRoom},
		},
		store: store,
		notes: notes,
		cfg:   EngineConfig{CallSendingEnabled: true, Channel: ChannelBoth, Routing: incidentRouting},
		clock: func() time.Time { return testClock },
	}
}

// startP1 starts a P1 ladder and returns when its first call (LEVEL_1,
// attempt 1, at +6m) is due.
func startP1(t *testing.T, e *Engine) time.Time {
	t.Helper()
	if err := e.Handle(context.Background(), createdEvent(t, "CRITICAL", testClock)); err != nil {
		t.Fatal(err)
	}
	return testClock.Add(7 * time.Minute)
}

func firstState(t *testing.T, store *memStore) LadderState {
	t.Helper()
	st, found, _ := store.Get(context.Background(), testIncidentID)
	if !found {
		t.Fatal("no ladder state")
	}
	return st
}

func firstScheduled(store *memStore) bool {
	_, ok := store.wakes[wakeMember(testIncidentID, 0)]
	return ok
}

// A room with no space is permanent for the card, not for the call: a call
// that failed transiently is still retried, and goes through.
func TestCardRetry_PermanentCardDoesNotStopTheCallRetry(t *testing.T) {
	store, notes := newMemStore(), &fakeNotes{}
	caller := &triesCaller{fakeCaller: fakeCaller{err: &apierror.Error{StatusCode: 503, Body: "unavailable"}}}
	chat := &fakeChat{noRoom: map[string]bool{cardRoom: true}}
	e := bothEngine(caller, chat, store, notes)
	due := startP1(t, e)

	if err := e.Tick(context.Background(), due); err == nil {
		t.Fatal("a transient call failure must surface so the tick is retried")
	}
	st := firstState(t, store)
	if st.Placed[0] || st.failure(0) != "" || st.cardPending(0) {
		t.Fatalf("after a transient call failure: placed=%v failure=%q cardPending=%v; want nothing recorded",
			st.Placed[0], st.failure(0), st.cardPending(0))
	}
	if !firstScheduled(store) {
		t.Fatal("the call must stay scheduled; NO_CHAT_SPACE for the card must not give up on it")
	}

	caller.err = nil
	if err := e.Tick(context.Background(), due); err != nil {
		t.Fatalf("call through, card rejected: %v; want no error", err)
	}
	st = firstState(t, store)
	if !st.Placed[0] || st.failure(0) != "" || st.cardPending(0) {
		t.Errorf("placed=%v failure=%q cardPending=%v; want placed, no failure, no card owed",
			st.Placed[0], st.failure(0), st.cardPending(0))
	}
	if firstScheduled(store) {
		t.Error("a rejected card must not keep the rung scheduled")
	}
	if len(caller.placed) != 1 || caller.tries != 2 {
		t.Errorf("dials: %d placed in %d tries; want 1 in 2", len(caller.placed), caller.tries)
	}
}

// The call went through and the card failed transiently: the next tick posts
// the card alone and never dials again.
func TestCardRetry_TransientCardIsResentAloneNeverRedialling(t *testing.T) {
	store := newMemStore()
	caller := &triesCaller{}
	chat := &fakeChat{err: errors.New("webhook down")}
	e := bothEngine(caller, chat, store, &fakeNotes{})
	due := startP1(t, e)

	err := e.Tick(context.Background(), due)
	if err == nil || !strings.Contains(err.Error(), "card") {
		t.Fatalf("tick err = %v; want the card's failure reported", err)
	}
	st := firstState(t, store)
	if !st.Placed[0] || !st.cardPending(0) {
		t.Fatalf("placed=%v cardPending=%v; want the call recorded and the card owed", st.Placed[0], st.cardPending(0))
	}
	if !firstScheduled(store) {
		t.Fatal("an owed card must keep its wake entry")
	}

	// Still down: the card is tried again, the call is not.
	if err := e.Tick(context.Background(), due); err == nil {
		t.Fatal("a card that still fails must surface")
	}
	if caller.tries != 1 {
		t.Fatalf("dialled %d times while only the card was owed; want 1", caller.tries)
	}

	chat.err = nil
	if err := e.Tick(context.Background(), due); err != nil {
		t.Fatal(err)
	}
	if caller.tries != 1 {
		t.Errorf("dialled %d times; the card retry must never re-dial", caller.tries)
	}
	if len(chat.posted) != 1 || chat.posted[0].Rung != Level1.Label() || chat.posted[0].Attempt != 1 {
		t.Fatalf("cards posted = %d; want L2's one card", len(chat.posted))
	}
	st = firstState(t, store)
	if !st.Placed[0] || st.cardPending(0) {
		t.Errorf("placed=%v cardPending=%v; want placed and nothing owed", st.Placed[0], st.cardPending(0))
	}
	if firstScheduled(store) {
		t.Error("a delivered card must retire the wake entry")
	}

	if err := e.Tick(context.Background(), due); err != nil {
		t.Fatal(err)
	}
	if caller.tries != 1 || len(chat.posted) != 1 {
		t.Errorf("a later tick delivered again: %d dials, %d cards", caller.tries, len(chat.posted))
	}
}

// Google Chat's 429 is a rate limit, not a rejection: retried, never recorded
// as a failure. Other 4xx stay permanent.
func TestCardRetry_RateLimitIsTransient(t *testing.T) {
	if isPermanent(&apierror.Error{StatusCode: 429}) {
		t.Error("429 must be transient")
	}
	for _, code := range []int{400, 401, 403, 404} {
		if !isPermanent(&apierror.Error{StatusCode: code}) {
			t.Errorf("%d must stay permanent", code)
		}
	}
	if got := permanentReason(&apierror.Error{StatusCode: 400, Body: `{"code":21219}`}); got != "REJECTED_400_21219" {
		t.Errorf("permanentReason = %q; unchanged rendering wanted", got)
	}

	t.Run("both", func(t *testing.T) {
		store := newMemStore()
		caller := &triesCaller{}
		chat := &fakeChat{err: &apierror.Error{StatusCode: 429, Body: "rate limited"}}
		e := bothEngine(caller, chat, store, &fakeNotes{})
		due := startP1(t, e)
		if err := e.Tick(context.Background(), due); err == nil {
			t.Fatal("a 429 card must surface for retry")
		}
		if st := firstState(t, store); !st.cardPending(0) || st.failure(0) != "" {
			t.Fatalf("cardPending=%v failure=%q; want the card owed and no failure", st.cardPending(0), st.failure(0))
		}
		chat.err = nil
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatal(err)
		}
		if len(chat.posted) != 1 || caller.tries != 1 {
			t.Errorf("%d cards, %d dials; want the card posted on retry and one dial", len(chat.posted), caller.tries)
		}
	})

	t.Run("chat only", func(t *testing.T) {
		store := newMemStore()
		chat := &fakeChat{err: &apierror.Error{StatusCode: 429, Body: "rate limited"}}
		e := chatEngine(t, chat, store, &fakeNotes{})
		due := startP1(t, e)
		if err := e.Tick(context.Background(), due); err == nil {
			t.Fatal("a 429 must surface for retry")
		}
		if st := firstState(t, store); st.failure(0) != "" || st.Placed[0] || !firstScheduled(store) {
			t.Fatalf("failure=%q placed=%v; want the rung still scheduled", st.failure(0), st.Placed[0])
		}
		chat.err = nil
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatal(err)
		}
		if len(chat.posted) != 1 {
			t.Errorf("%d cards posted after the rate limit cleared; want 1", len(chat.posted))
		}
	})
}

// A call the provider rejects is given up on, as before, with the call's own
// reason on the work note whatever happened to the card.
func TestCardRetry_RejectedCallIsNotRetried(t *testing.T) {
	rejected := &apierror.Error{StatusCode: 400, Body: `{"code":21219,"message":"unverified"}`}

	t.Run("card transient", func(t *testing.T) {
		store := newMemStore()
		caller := &triesCaller{fakeCaller: fakeCaller{err: rejected}}
		chat := &fakeChat{err: errors.New("webhook down")}
		e := bothEngine(caller, chat, store, &fakeNotes{})
		due := startP1(t, e)
		if err := e.Tick(context.Background(), due); err == nil {
			t.Fatal("the owed card must surface")
		}
		st := firstState(t, store)
		if st.failure(0) != "REJECTED_400_21219" || st.Placed[0] || !st.cardPending(0) {
			t.Fatalf("failure=%q placed=%v cardPending=%v; want the call rejected and the card owed",
				st.failure(0), st.Placed[0], st.cardPending(0))
		}
		chat.err = nil
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatal(err)
		}
		if caller.tries != 1 {
			t.Errorf("a rejected call was dialled %d times; want 1", caller.tries)
		}
		st = firstState(t, store)
		if len(chat.posted) != 1 || st.cardPending(0) || st.failure(0) != "REJECTED_400_21219" || firstScheduled(store) {
			t.Errorf("cards=%d cardPending=%v failure=%q scheduled=%v; want the card posted and the rung retired",
				len(chat.posted), st.cardPending(0), st.failure(0), firstScheduled(store))
		}
	})

	t.Run("card permanent", func(t *testing.T) {
		store := newMemStore()
		caller := &triesCaller{fakeCaller: fakeCaller{err: rejected}}
		chat := &fakeChat{noRoom: map[string]bool{cardRoom: true}}
		e := bothEngine(caller, chat, store, &fakeNotes{})
		due := startP1(t, e)
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatalf("both rejected: %v; want no error, nothing to retry", err)
		}
		st := firstState(t, store)
		if st.failure(0) != "REJECTED_400_21219" || st.cardPending(0) || firstScheduled(store) {
			t.Errorf("failure=%q cardPending=%v scheduled=%v; want the call's own reason and the rung retired",
				st.failure(0), st.cardPending(0), firstScheduled(store))
		}
	})
}

// Both channels delivering is untouched: one call, one card, and nothing new
// stored.
func TestCardRetry_BothDeliveredIsUnchanged(t *testing.T) {
	store := newMemStore()
	caller := &triesCaller{}
	chat := &fakeChat{}
	e := bothEngine(caller, chat, store, &fakeNotes{})
	due := startP1(t, e)
	if err := e.Tick(context.Background(), due); err != nil {
		t.Fatal(err)
	}
	st := firstState(t, store)
	if !st.Placed[0] || st.CardPending != nil || firstScheduled(store) {
		t.Errorf("placed=%v CardPending=%v scheduled=%v; want placed, nothing stored for cards, retired",
			st.Placed[0], st.CardPending, firstScheduled(store))
	}
	if caller.tries != 1 || len(chat.posted) != 1 {
		t.Errorf("%d dials, %d cards; want 1 and 1", caller.tries, len(chat.posted))
	}
	body, _ := json.Marshal(st)
	if strings.Contains(string(body), "cardPending") {
		t.Errorf("stored state gained a cardPending field: %s", body)
	}
}

// A single-channel ladder never owes a card: its one channel decides the
// rung, transient or permanent, exactly as before.
func TestCardRetry_SingleChannelIsUnchanged(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		store := newMemStore()
		chat := &fakeChat{err: errors.New("webhook down")}
		e := chatEngine(t, chat, store, &fakeNotes{})
		due := startP1(t, e)
		if err := e.Tick(context.Background(), due); err == nil {
			t.Fatal("a transient card failure must surface")
		}
		if st := firstState(t, store); st.Placed[0] || st.failure(0) != "" || st.CardPending != nil || !firstScheduled(store) {
			t.Fatalf("placed=%v failure=%q CardPending=%v; want the rung retried whole", st.Placed[0], st.failure(0), st.CardPending)
		}
		chat.err, chat.noRoom = nil, map[string]bool{cardRoom: true}
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatal(err)
		}
		if st := firstState(t, store); st.failure(0) != "NO_CHAT_SPACE" || st.CardPending != nil || firstScheduled(store) {
			t.Errorf("failure=%q CardPending=%v; want NO_CHAT_SPACE recorded against the rung", st.failure(0), st.CardPending)
		}
	})

	t.Run("call", func(t *testing.T) {
		store := newMemStore()
		caller := &fakeCaller{err: &apierror.Error{StatusCode: 503}}
		e := testEngine(store, caller, &fakeNotes{}, enabled())
		due := startP1(t, e)
		if err := e.Tick(context.Background(), due); err == nil {
			t.Fatal("a transient call failure must surface")
		}
		if st := firstState(t, store); st.CardPending != nil || !firstScheduled(store) {
			t.Fatalf("CardPending=%v; want nothing stored for cards and the call scheduled", st.CardPending)
		}
		caller.err = &apierror.Error{StatusCode: 400}
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatal(err)
		}
		if st := firstState(t, store); st.failure(0) != "REJECTED_400" || st.CardPending != nil || firstScheduled(store) {
			t.Errorf("failure=%q CardPending=%v; want REJECTED_400 and the call retired", st.failure(0), st.CardPending)
		}
	})
}

// A held call rings nobody, so its card decides the rung, as it always has.
func TestCardRetry_HeldCallIsUnchanged(t *testing.T) {
	store := newMemStore()
	caller := &triesCaller{}
	chat := &fakeChat{err: errors.New("webhook down")}
	e := bothEngine(caller, chat, store, &fakeNotes{})
	due := startP1(t, e)
	st := firstState(t, store)
	st.Plan.Calls[0].HoldCall = true
	if err := store.Save(context.Background(), testIncidentID, st); err != nil {
		t.Fatal(err)
	}

	if err := e.Tick(context.Background(), due); err == nil {
		t.Fatal("the held rung's card failure must surface")
	}
	st = firstState(t, store)
	if st.Placed[0] || st.CardPending != nil || !firstScheduled(store) {
		t.Fatalf("placed=%v CardPending=%v; want the rung retried whole", st.Placed[0], st.CardPending)
	}
	chat.err = nil
	if err := e.Tick(context.Background(), due); err != nil {
		t.Fatal(err)
	}
	st = firstState(t, store)
	if !st.Placed[0] || st.CardPending != nil || caller.tries != 0 || len(chat.posted) != 1 {
		t.Errorf("placed=%v CardPending=%v dials=%d cards=%d; want the card posted and nobody dialled",
			st.Placed[0], st.CardPending, caller.tries, len(chat.posted))
	}
}

// A ladder stored before CardPending existed has no such field: it reads as
// nothing owed, so a leftover wake for a placed call is dropped and a pending
// call is placed, exactly as before.
func TestCardRetry_OldStoredStateWithoutTheField(t *testing.T) {
	store := newMemStore()
	caller := &triesCaller{}
	chat := &fakeChat{}
	e := bothEngine(caller, chat, store, &fakeNotes{})
	due := startP1(t, e)

	st := firstState(t, store)
	st.Placed[0] = true // placed by an older build, whose wake removal was lost
	body, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if _, has := raw["cardPending"]; has {
		t.Fatal("a ladder that owes no card must not store the field")
	}
	var old LadderState
	if err := json.Unmarshal(body, &old); err != nil {
		t.Fatal(err)
	}
	store.states[testIncidentID] = old
	if old.AllSettled() || len(pendingMembers(testIncidentID, old)) != len(old.Placed)-1 {
		t.Fatalf("old state: AllSettled=%v pending=%d; want every call but the placed one pending",
			old.AllSettled(), len(pendingMembers(testIncidentID, old)))
	}

	// The leftover wake for the placed call delivers nothing.
	if err := e.Tick(context.Background(), due); err != nil {
		t.Fatal(err)
	}
	if caller.tries != 0 || len(chat.posted) != 0 || firstScheduled(store) {
		t.Errorf("dials=%d cards=%d scheduled=%v; want the leftover dropped with nothing sent",
			caller.tries, len(chat.posted), firstScheduled(store))
	}
	// The next call is placed as usual.
	if err := e.Tick(context.Background(), testClock.Add(9*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if caller.tries != 1 {
		t.Errorf("dials=%d; want the next pending call placed", caller.tries)
	}
}

// An owed card keeps the ladder from completing, and is retired with it when
// the incident is acknowledged.
func TestCardRetry_OwedCardIsPendingUntilDeliveredOrRetired(t *testing.T) {
	st := LadderState{Placed: []bool{true, true}, CardPending: []bool{false, true}}
	if st.AllSettled() {
		t.Error("a ladder owing a card has not run its course")
	}
	if got := pendingMembers(testIncidentID, st); len(got) != 1 || got[0] != wakeMember(testIncidentID, 1) {
		t.Errorf("pendingMembers = %v; want the owed card's wake", got)
	}
	st.setCardPending(1, false)
	if !st.AllSettled() {
		t.Error("with the card delivered, the ladder has run its course")
	}

	store := newMemStore()
	chat := &fakeChat{err: errors.New("webhook down")}
	e := bothEngine(&triesCaller{}, chat, store, &fakeNotes{})
	due := startP1(t, e)
	_ = e.Tick(context.Background(), due)
	if !firstState(t, store).cardPending(0) {
		t.Fatal("setup: want the first card owed")
	}
	acknowledgeFully(t, e)
	if firstScheduled(store) {
		t.Error("acknowledgement must retire the owed card's wake")
	}
}

// A card that keeps failing transiently is retried only within
// cardRetryWindow of the rung being due; after that it is given up on, the
// rung settles, and the call is still never re-dialled.
func TestCardRetry_GivesUpOnTheCardAfterTheWindow(t *testing.T) {
	store := newMemStore()
	caller := &triesCaller{}
	chat := &fakeChat{err: errors.New("webhook down")}
	e := bothEngine(caller, chat, store, &fakeNotes{})
	now := testClock
	e.clock = func() time.Time { return now }
	due := startP1(t, e)
	dueAt := firstState(t, store).Plan.Calls[0].At

	now = due
	if err := e.Tick(context.Background(), due); err == nil {
		t.Fatal("the first failed card must surface")
	}
	if !firstState(t, store).cardPending(0) {
		t.Fatal("want the card owed inside the window")
	}

	// Still inside the window: still owed.
	now = dueAt.Add(cardRetryWindow - time.Minute)
	if err := e.Tick(context.Background(), now); err == nil {
		t.Fatal("a card failing inside the window must surface and stay owed")
	}
	if !firstState(t, store).cardPending(0) || !firstScheduled(store) {
		t.Fatal("inside the window the card stays owed and scheduled")
	}

	// Past the window: given up on. Later rungs are due by now and fail their
	// own cards inside their own windows; only LEVEL_1's is asked about here.
	now = dueAt.Add(cardRetryWindow + time.Minute)
	if err := e.Tick(context.Background(), now); err != nil && strings.Contains(err.Error(), Level1.String()+" card") {
		t.Fatalf("giving up on a late card is not an error: %v", err)
	}
	st := firstState(t, store)
	if !st.Placed[0] || st.cardPending(0) || st.failure(0) != "" {
		t.Fatalf("placed=%v cardPending=%v failure=%q; want the call placed and nothing owed", st.Placed[0], st.cardPending(0), st.failure(0))
	}
	if firstScheduled(store) {
		t.Error("a card given up on must retire its wake entry")
	}
	// One dial per rung that has come due, and none of them twice.
	dueRungs := 0
	for _, c := range st.Plan.Calls {
		if !c.At.After(now) {
			dueRungs++
		}
	}
	if caller.tries != dueRungs {
		t.Errorf("dialled %d times for %d rungs due; a card retry must never re-dial", caller.tries, dueRungs)
	}
}

// replacingChat runs replace while a card is being sent, the way a severity
// change or an elevation can land during the HTTP call.
type replacingChat struct {
	*fakeChat
	replace func()
}

func (c *replacingChat) SendEscalationAlert(ctx context.Context, a notifications.EscalationAlert) error {
	if c.replace != nil {
		c.replace()
		c.replace = nil
	}
	return c.fakeChat.SendEscalationAlert(ctx, a)
}

// replaceLadder stands in for schedule's replacement: a new chain over the
// same rung slots, with nothing placed or owed. Its wake entries are the old
// chain's member names, so they are left as they are.
func replaceLadder(t *testing.T, store *memStore) LadderState {
	t.Helper()
	st := firstState(t, store)
	st.Plan.Trigger.Kind = TriggerPriorityElevated
	st.Plan.Trigger.At = st.Plan.Trigger.At.Add(time.Minute)
	st.Placed = make([]bool, len(st.Plan.Calls))
	st.Failed, st.CardPending = nil, nil
	if err := store.Save(context.Background(), testIncidentID, st); err != nil {
		t.Fatal(err)
	}
	return st
}

// A card send that a replacement ladder overtakes must not record anything on
// the replacement, nor retire the wake entry it now owns.
func TestCardRetry_ReplacedWhileSendingLeavesTheReplacementAlone(t *testing.T) {
	t.Run("card fails transiently", func(t *testing.T) {
		store := newMemStore()
		caller := &triesCaller{}
		chat := &replacingChat{fakeChat: &fakeChat{err: errors.New("webhook down")}}
		e := bothEngine(caller, chat, store, &fakeNotes{})
		due := startP1(t, e)
		chat.replace = func() { replaceLadder(t, store) }

		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatalf("tick err = %v; a replaced chain's card is not retried", err)
		}
		st := firstState(t, store)
		if st.Placed[0] || st.cardPending(0) {
			t.Fatalf("replacement placed=%v cardPending=%v; want its rung untouched", st.Placed[0], st.cardPending(0))
		}
		if !firstScheduled(store) {
			t.Fatal("the replacement's wake entry must stay")
		}

		// The replacement's own call goes out on the next tick.
		chat.err = nil
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatal(err)
		}
		if caller.tries != 2 {
			t.Errorf("dialled %d times; want the replacement's rung dialled on its own", caller.tries)
		}
	})

	t.Run("owed card delivered", func(t *testing.T) {
		store := newMemStore()
		caller := &triesCaller{}
		chat := &replacingChat{fakeChat: &fakeChat{err: errors.New("webhook down")}}
		e := bothEngine(caller, chat, store, &fakeNotes{})
		due := startP1(t, e)
		if err := e.Tick(context.Background(), due); err == nil {
			t.Fatal("the failed card must surface")
		}
		if !firstState(t, store).cardPending(0) {
			t.Fatal("want the card owed")
		}

		chat.err = nil
		chat.replace = func() { replaceLadder(t, store) }
		if err := e.Tick(context.Background(), due); err != nil {
			t.Fatal(err)
		}
		st := firstState(t, store)
		if st.Placed[0] || st.cardPending(0) {
			t.Fatalf("replacement placed=%v cardPending=%v; want its rung untouched", st.Placed[0], st.cardPending(0))
		}
		if !firstScheduled(store) {
			t.Fatal("the old chain's delivered card must not retire the replacement's wake entry")
		}
		if caller.tries != 1 {
			t.Errorf("dialled %d times; the card-only send must not dial", caller.tries)
		}
	})
}
