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
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
)

// incidentSpaceEngine is an SRE engine whose own chat space is "SRE Case Paging", with the shared client holding the incident spaces.
func incidentSpaceEngine(chat *fakeChat) *Engine {
	return &Engine{
		cfg: EngineConfig{
			Channel:            ChannelChat,
			Kind:               LadderSRE,
			Ladder:             LadderConfig{UnansweredChat: true, SMEHandoffChat: true},
			IncidentChatFanout: map[string][]string{"Default SRE Group": {"Apollo SRE Group", "Artemis SRE Group"}},
		},
		notifiers:    []notifier{chatNotifier{chat: chat, audience: "SRE Case Paging"}},
		incidentChat: chat,
		sharedRoom:   &chatRoom{chat: chat, room: "SRE Case Paging"},
	}
}

func deliverFirstRung(t *testing.T, e *Engine, trigger Trigger) {
	t.Helper()
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	plan := Plan{Trigger: trigger, Calls: []PlannedCall{{Level: Level0, Ordinal: 1, At: at, Recipient: Recipient{Name: "Jane Doe"}}}}
	for _, n := range e.notifiersFor(plan) {
		if _, err := n.Deliver(context.Background(), plan, plan.Calls[0]); err != nil {
			t.Fatalf("Deliver: %v", err)
		}
	}
}

// An incident's rung replies under its card, in the INCIDENT_CHAT_SPACES space the card went to, not in the ladder's own space.
func TestIncidentRung_RepliesInTheIncidentCardsSpace(t *testing.T) {
	chat := &fakeChat{}
	e := incidentSpaceEngine(chat)
	deliverFirstRung(t, e, Trigger{IncidentID: "inc-1", Record: RecordIncident, Team: "MS/PC SRE Group", At: time.Now()})

	if len(chat.posted) != 1 {
		t.Fatalf("posted %d cards; want 1", len(chat.posted))
	}
	got := chat.posted[0]
	if got.Audience != notifications.IncidentAudience("MS/PC SRE Group") || !got.Reply || got.ThreadKey != notifications.IncidentThreadKey("inc-1") {
		t.Errorf("card to %q reply=%v thread=%q; want a reply in the incident card's thread and space", got.Audience, got.Reply, got.ThreadKey)
	}
}

// INCIDENT_CHAT_FANOUT sends a group's card to other groups' spaces, so its rungs follow it there.
func TestIncidentRung_FollowsTheCardsFanout(t *testing.T) {
	chat := &fakeChat{}
	e := incidentSpaceEngine(chat)
	deliverFirstRung(t, e, Trigger{IncidentID: "inc-1", Record: RecordIncident, Team: "default sre group", At: time.Now()})

	want := map[string]bool{notifications.IncidentAudience("Apollo SRE Group"): true, notifications.IncidentAudience("Artemis SRE Group"): true}
	if len(chat.posted) != 2 {
		t.Fatalf("posted %d cards; want one per fanned-out space", len(chat.posted))
	}
	for _, c := range chat.posted {
		if !want[c.Audience] || !c.Reply {
			t.Errorf("card to %q reply=%v; want a reply in a fanned-out incident space", c.Audience, c.Reply)
		}
	}
}

// A group with no incident space, and a customer case, keep the ladder's own space and the full card.
func TestIncidentRung_WithoutAnIncidentSpaceKeepsTheLaddersSpace(t *testing.T) {
	for name, trigger := range map[string]Trigger{
		"unmapped group": {IncidentID: "inc-2", Record: RecordIncident, Team: "Unmapped Group", At: time.Now()},
		"customer case":  {IncidentID: "case-1", Record: RecordCase, Team: "MS/PC SRE Group", At: time.Now()},
	} {
		t.Run(name, func(t *testing.T) {
			chat := &fakeChat{noRoom: map[string]bool{notifications.IncidentAudience("Unmapped Group"): true}}
			deliverFirstRung(t, incidentSpaceEngine(chat), trigger)
			if len(chat.posted) != 1 || chat.posted[0].Audience != "SRE Case Paging" || chat.posted[0].Reply {
				t.Fatalf("posted %+v; want one full card in the ladder's own space", chat.posted)
			}
		})
	}
}

// The closing message of an unanswered incident ladder replies under the card too.
func TestIncidentUnanswered_GoesToTheIncidentCardsSpace(t *testing.T) {
	e := incidentSpaceEngine(&fakeChat{})
	rooms, _ := e.unansweredRooms(Plan{Trigger: Trigger{IncidentID: "inc-1", Record: RecordIncident, Team: "MS/PC SRE Group"}})
	if len(rooms) != 1 || rooms[0].room != notifications.IncidentAudience("MS/PC SRE Group") || !rooms[0].reply {
		t.Fatalf("closing message rooms = %+v; want a reply in the incident card's space", rooms)
	}
}

// The escalated-to-SME message replies under the card of the group the incident left.
func TestSMEHandoff_GoesToTheIncidentCardsSpace(t *testing.T) {
	chat := &fakeChat{}
	e := incidentSpaceEngine(chat)
	e.postSMEHandoff(context.Background(), specialOpsPress{IncidentID: "inc-1", Number: "INC1", PreviousGroup: "MS/PC SRE Group"}, "choreo-runtime", true)
	if len(chat.handoffs) != 1 || chat.handoffs[0].Audience != notifications.IncidentAudience("MS/PC SRE Group") || !chat.handoffs[0].Reply {
		t.Fatalf("handoffs = %+v; want one reply in the incident card's space", chat.handoffs)
	}
}

// incidentSpaceOnlyEngine is incidentSpaceEngine with every chat setting off and a call-only ladder: INCIDENT_CHAT_SPACES alone.
func incidentSpaceOnlyEngine(chat *fakeChat) *Engine {
	e := incidentSpaceEngine(chat)
	e.cfg.Channel = ChannelCall
	e.cfg.Ladder = LadderConfig{}
	e.notifiers = nil
	e.sharedRoom = nil
	return e
}

// A call-only ladder with no chat settings still replies under the incident card.
func TestIncidentSpaceOnly_RungReplies(t *testing.T) {
	chat := &fakeChat{}
	deliverFirstRung(t, incidentSpaceOnlyEngine(chat), Trigger{IncidentID: "inc-1", Record: RecordIncident, Team: "MS/PC SRE Group", At: time.Now()})
	if len(chat.posted) != 1 || chat.posted[0].Audience != notifications.IncidentAudience("MS/PC SRE Group") || !chat.posted[0].Reply {
		t.Fatalf("posted %+v; want one reply in the incident card's space", chat.posted)
	}
}

// sre.unansweredChat and sre.smeHandoffChat off do not hold back the incident space's replies.
func TestIncidentSpaceOnly_ClosingAndHandoffReplies(t *testing.T) {
	chat := &fakeChat{}
	e := incidentSpaceOnlyEngine(chat)
	if rooms, _ := e.unansweredRooms(Plan{Trigger: Trigger{IncidentID: "inc-1", Record: RecordIncident, Team: "MS/PC SRE Group"}}); len(rooms) != 1 || !rooms[0].reply {
		t.Errorf("closing message rooms = %+v; want the incident card's space", rooms)
	}
	e.postSMEHandoff(context.Background(), specialOpsPress{IncidentID: "inc-1", Number: "INC1", PreviousGroup: "MS/PC SRE Group"}, "choreo-runtime", true)
	if len(chat.handoffs) != 1 || !chat.handoffs[0].Reply {
		t.Errorf("handoffs = %+v; want one reply in the incident card's space", chat.handoffs)
	}
}

// A log ladder is a dry run, so it posts nothing even with an incident space.
func TestIncidentSpaceOnly_LogLadderPostsNothing(t *testing.T) {
	chat := &fakeChat{}
	e := incidentSpaceOnlyEngine(chat)
	e.cfg.Channel = ChannelLog
	trigger := Trigger{IncidentID: "inc-1", Record: RecordIncident, Team: "MS/PC SRE Group", At: time.Now()}
	deliverFirstRung(t, e, trigger)
	if len(chat.posted) != 0 {
		t.Errorf("posted %+v; want no rung from a log ladder", chat.posted)
	}
	if rooms, _ := e.unansweredRooms(Plan{Trigger: trigger}); len(rooms) != 0 {
		t.Errorf("closing message rooms = %+v; want none from a log ladder", rooms)
	}
}

// A team narrowed to log is a dry run too, so its incident gets no closing reply.
func TestIncidentSpaceOnly_TeamLogPostsNoClosingReply(t *testing.T) {
	e := incidentSpaceOnlyEngine(&fakeChat{})
	e.cfg.Ladder.TeamChannels = map[string]Channel{"ms/pc sre group": ChannelLog}
	if rooms, _ := e.unansweredRooms(Plan{Trigger: Trigger{IncidentID: "inc-1", Record: RecordIncident, Team: "MS/PC SRE Group"}}); len(rooms) != 0 {
		t.Errorf("closing message rooms = %+v; want none for a team set to log", rooms)
	}
}

// A log SME ladder pages nobody, so the incident space gets no handoff reply.
func TestIncidentSpaceOnly_LogSMELadderPostsNoHandoff(t *testing.T) {
	chat := &fakeChat{}
	e := incidentSpaceOnlyEngine(chat)
	e.cfg.SME.Channel = ChannelLog
	e.postSMEHandoff(context.Background(), specialOpsPress{IncidentID: "inc-1", Number: "INC1", PreviousGroup: "MS/PC SRE Group"}, "choreo-runtime", true)
	if len(chat.handoffs) != 0 {
		t.Errorf("handoffs = %+v; want none from a log SME ladder", chat.handoffs)
	}
}

// Without an incident space the old switches still decide: off means no closing or handoff message.
func TestNoIncidentSpace_SwitchesStillApply(t *testing.T) {
	chat := &fakeChat{noRoom: map[string]bool{notifications.IncidentAudience("Unmapped Group"): true}}
	e := incidentSpaceOnlyEngine(chat)
	e.sharedRoom = &chatRoom{chat: chat, room: "SRE Case Paging"}
	if rooms, _ := e.unansweredRooms(Plan{Trigger: Trigger{IncidentID: "inc-2", Record: RecordIncident, Team: "Unmapped Group"}}); len(rooms) != 0 {
		t.Errorf("closing message rooms = %+v; want none with sre.unansweredChat off", rooms)
	}
	e.postSMEHandoff(context.Background(), specialOpsPress{IncidentID: "inc-2", Number: "INC2", PreviousGroup: "Unmapped Group"}, "choreo-runtime", true)
	if len(chat.handoffs) != 0 {
		t.Errorf("handoffs = %+v; want none with sre.smeHandoffChat off", chat.handoffs)
	}
}
