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
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
)

// Each SRE sub-team (SaaS Apollo, SaaS Artemis, IaaS) and each SME team posts
// to a Google Chat space of its own. These tests go through the wiring a
// deployment uses -- NewEngine, with SRE webhook URLs from the variables
// sre.teamChats names and SME spaces from GOOGLE_CHAT_SPACES -- where the
// older teamChats tests set the notifiers by hand.

// chatSpace is one Google Chat space: a webhook that counts the cards posted to it.
type chatSpace struct {
	mu    sync.Mutex
	posts int
	url   string
}

func newChatSpace(t *testing.T) *chatSpace {
	t.Helper()
	s := &chatSpace{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		s.posts++
		s.mu.Unlock()
		_, _ = w.Write([]byte(`{"name":"spaces/x/messages/y"}`))
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

func (s *chatSpace) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.posts
}

// splitSRETeams is the sre section's teams as a deployment writes it: the
// assignment groups that are not simply a team key map to one.
var splitSRETeams = TeamKeys{
	ABTs: []string{"apollo", "artemis", "iaas"},
	Aliases: map[string]string{
		"Apollo SRE Group":  "apollo",
		"Artemis SRE Group": "artemis",
		"SRE IaaS":          "iaas",
	},
	DefaultGroups: []string{"Default SRE Group"},
}

// sreSpaces sets one webhook variable per space and returns the spaces by team
// key; "" is sre.chat, the space every incident without one of its own uses.
func sreSpaces(t *testing.T) map[string]*chatSpace {
	t.Helper()
	spaces := map[string]*chatSpace{}
	for team, env := range map[string]string{
		"":        "SRE_CHAT_WEBHOOK_URL",
		"apollo":  "SRE_CHAT_WEBHOOK_URL_APOLLO",
		"artemis": "SRE_CHAT_WEBHOOK_URL_ARTEMIS",
		"iaas":    "SRE_CHAT_WEBHOOK_URL_IAAS",
	} {
		spaces[team] = newChatSpace(t)
		t.Setenv(env, spaces[team].url)
	}
	return spaces
}

// splitSREEngine is a chat-only SRE engine built by NewEngine, with
// sre.teamChats giving Apollo, Artemis and IaaS their own spaces. edit, when
// not nil, changes the sre section first.
func splitSREEngine(reader *stubScheduleReader, edit func(*LadderConfig)) *Engine {
	cfg := EngineConfig{CallSendingEnabled: true, Channel: ChannelChat, Kind: LadderSRE}
	cfg.Ladder.Chat = Chat{WebhookURLEnv: "SRE_CHAT_WEBHOOK_URL", Audience: "SRE"}
	cfg.Ladder.TeamChats = map[string]Chat{
		"apollo":  {WebhookURLEnv: "SRE_CHAT_WEBHOOK_URL_APOLLO", Audience: "Apollo SRE"},
		"artemis": {WebhookURLEnv: "SRE_CHAT_WEBHOOK_URL_ARTEMIS", Audience: "Artemis SRE"},
		"iaas":    {WebhookURLEnv: "SRE_CHAT_WEBHOOK_URL_IAAS", Audience: "IaaS SRE"},
	}
	cfg.Ladder.Teams = splitSRETeams
	if edit != nil {
		edit(&cfg.Ladder)
	}
	// cmd/server hands the resolver the sre section's teams as SRE teams.
	resolverTeams := TeamKeys{SRE: splitSRETeams.ABTs, Aliases: splitSRETeams.Aliases, DefaultGroups: splitSRETeams.DefaultGroups}
	e := NewEngine(DefaultPolicy, NewTeamScheduleResolver(reader, resolverTeams, nil), nil, nil, PortalLinks{}, nil, nil, "", cfg)
	e.store = newMemStore()
	e.notes = &fakeNotes{}
	e.clock = func() time.Time { return testClock }
	return e
}

// splitReader is rotaReader with Artemis on the SaaS window too.
func splitReader() *stubScheduleReader {
	r := rotaReader(true)
	r.onDuty = append(r.onDuty,
		held("r-l1", "artemis", "SRE_TZ1_L1", ""),
		held("r-l2", "artemis", "SRE_TZ1", "L2"),
	)
	return r
}

// artemisOnlyReader is the SaaS window with only Artemis on it.
func artemisOnlyReader() *stubScheduleReader {
	return &stubScheduleReader{
		catalogue: rotaCatalogue(true),
		onDuty: []onDutyAssignment{
			held("r-l1", "artemis", "SRE_TZ1_L1", ""),
			held("r-l2", "artemis", "SRE_TZ1", "L2"),
		},
	}
}

// postedOnlyTo fails unless want got at least one post and every other space none.
func postedOnlyTo(t *testing.T, spaces map[string]*chatSpace, want string) {
	t.Helper()
	if spaces[want].count() == 0 {
		t.Errorf("no post in the %q space", want)
	}
	for team, s := range spaces {
		if team != want && s.count() != 0 {
			t.Errorf("%d post(s) in the %q space; want them all in %q", s.count(), team, want)
		}
	}
}

// An incident of each sub-team posts its rung cards to that team's own space
// and to no other.
func TestTeamChatSpaces_EachSRESubTeamPostsToItsOwnSpace(t *testing.T) {
	for group, space := range map[string]string{
		"Apollo SRE Group":  "apollo",
		"Artemis SRE Group": "artemis",
		"SRE IaaS":          "iaas",
	} {
		t.Run(group, func(t *testing.T) {
			spaces := sreSpaces(t)
			runUnanswered(t, splitSREEngine(splitReader(), nil), group)
			postedOnlyTo(t, spaces, space)
		})
	}
}

// bothSaaS is sre.teamlessChats: [apollo, artemis].
func bothSaaS(l *LadderConfig) { l.TeamlessChats = []string{"apollo", "artemis"} }

// An incident of a service mapped to no SRE team (it lands in a default
// group) notifies both SaaS teams: every rung card, and the closing message,
// in Apollo's space and in Artemis's, none in sre.chat or IaaS.
func TestTeamChatSpaces_ADefaultGroupIncidentNotifiesApolloAndArtemis(t *testing.T) {
	for name, reader := range map[string]func() *stubScheduleReader{
		"both SaaS teams on duty": splitReader,
		"only Artemis on duty":    artemisOnlyReader,
	} {
		t.Run(name, func(t *testing.T) {
			spaces := sreSpaces(t)
			runUnanswered(t, splitSREEngine(reader(), func(l *LadderConfig) {
				bothSaaS(l)
				l.UnansweredChat = true
			}), "Default SRE Group")
			apollo, artemis := spaces["apollo"].count(), spaces["artemis"].count()
			if apollo == 0 || apollo != artemis {
				t.Errorf("posts: Apollo %d, Artemis %d; want the same cards in both", apollo, artemis)
			}
			if n := spaces[""].count() + spaces["iaas"].count(); n != 0 {
				t.Errorf("%d post(s) in sre.chat or IaaS; want none", n)
			}
		})
	}

	t.Run("teamlessChats empty keeps sre.chat", func(t *testing.T) {
		spaces := sreSpaces(t)
		runUnanswered(t, splitSREEngine(splitReader(), nil), "Default SRE Group")
		postedOnlyTo(t, spaces, "")
	})
}

// chatTeams: the incident's own team, else every teamlessChats team for an
// incident with no SRE team of its own.
func TestTeamChatSpaces_ChatTeams(t *testing.T) {
	e := splitSREEngine(splitReader(), bothSaaS)
	plan := func(team string) Plan { return Plan{Trigger: Trigger{Team: team}} }
	for team, want := range map[string][]string{
		"Apollo SRE Group":  {"apollo"},
		"SRE IaaS":          {"iaas"},
		"Default SRE Group": {"apollo", "artemis"}, // a service mapped to no SRE team
		"Atlas":             {"apollo", "artemis"}, // a customer case at S0
		"":                  {"apollo", "artemis"}, // a monitoring alert with no team
	} {
		if got := e.chatTeams(plan(team)); !slices.Equal(got, want) {
			t.Errorf("chatTeams(%q) = %v; want %v", team, got, want)
		}
	}

	// An SRE team of its own without a space is not "teamless": it stays in
	// sre.chat rather than going to the SaaS teams.
	apolloArtemis := splitSREEngine(splitReader(), func(l *LadderConfig) {
		delete(l.TeamChats, "iaas")
		bothSaaS(l)
	})
	if got := apolloArtemis.chatTeams(plan("SRE IaaS")); got != nil {
		t.Errorf("an SRE team with no space: chatTeams = %v; want sre.chat", got)
	}
	if got := splitSREEngine(splitReader(), nil).chatTeams(plan("Default SRE Group")); got != nil {
		t.Errorf("teamlessChats empty: chatTeams = %v; want sre.chat", got)
	}
	none := splitSREEngine(splitReader(), func(l *LadderConfig) { l.TeamChats = nil })
	if got := none.chatTeams(plan("Default SRE Group")); got != nil {
		t.Errorf("no teamChats: chatTeams = %v; want sre.chat", got)
	}
}

// countingCall is a call notifier that counts its deliveries.
type countingCall struct{ n *int }

func (countingCall) Channel() Channel { return ChannelCall }
func (c countingCall) Deliver(context.Context, Plan, PlannedCall) (Delivery, error) {
	*c.n++
	return Delivery{Channel: ChannelCall}, nil
}

// With two spaces a rung still rings its engineer once: one call notifier,
// one fan-out over both chats.
func TestTeamChatSpaces_TwoSpacesCallOnce(t *testing.T) {
	calls := 0
	call := countingCall{n: &calls}
	apollo, artemis, shared := &fakeChat{}, &fakeChat{}, &fakeChat{}
	e := ladderEngine(LadderSRE, shared, newMemStore())
	e.cfg.Ladder.Teams = splitSRETeams
	bothSaaS(&e.cfg.Ladder)
	e.notifiers = []notifier{call, chatNotifier{chat: shared, links: fakeLinks{}, audience: "SRE"}}
	e.teamNotifiers = map[string][]notifier{
		"apollo":  {call, chatNotifier{chat: apollo, links: fakeLinks{}, audience: "Apollo SRE"}},
		"artemis": {call, chatNotifier{chat: artemis, links: fakeLinks{}, audience: "Artemis SRE"}},
	}

	plan := Plan{Trigger: Trigger{IncidentID: testIncidentID, Team: "Default SRE Group"}}
	rung := PlannedCall{Level: Level0, Ordinal: 1, Recipient: Recipient{Name: "a-l1"}}
	if err, cardErr := e.placeVia(context.Background(), plan, rung, e.notifiersFor(plan), ChannelBoth); err != nil || cardErr != nil {
		t.Fatal(err, cardErr)
	}
	if calls != 1 {
		t.Errorf("calls placed = %d; want 1", calls)
	}
	if len(apollo.posted) != 1 || len(artemis.posted) != 1 || len(shared.posted) != 0 {
		t.Errorf("cards: Apollo %d, Artemis %d, shared %d; want 1, 1, 0", len(apollo.posted), len(artemis.posted), len(shared.posted))
	}
}

// One team's space changes where the card goes, never whether the rung
// rings: the call comes from the ladder's own notifiers even when the team's
// set has only its Chat notifier.
func TestTeamChatSpaces_OneTeamSpaceKeepsTheLaddersCall(t *testing.T) {
	calls := 0
	call := countingCall{n: &calls}
	apollo, shared := &fakeChat{}, &fakeChat{}
	e := ladderEngine(LadderSRE, shared, newMemStore())
	e.cfg.Ladder.Teams = splitSRETeams
	e.notifiers = []notifier{call, chatNotifier{chat: shared, links: fakeLinks{}, audience: "SRE"}}
	e.teamNotifiers = map[string][]notifier{
		"apollo": {chatNotifier{chat: apollo, links: fakeLinks{}, audience: "Apollo SRE"}}, // Chat only
	}

	plan := Plan{Trigger: Trigger{IncidentID: testIncidentID, Team: "Apollo SRE Group"}}
	rung := PlannedCall{Level: Level0, Ordinal: 1, Recipient: Recipient{Name: "a-l1"}}
	if err, cardErr := e.placeVia(context.Background(), plan, rung, e.notifiersFor(plan), ChannelBoth); err != nil || cardErr != nil {
		t.Fatal(err, cardErr)
	}
	if calls != 1 {
		t.Errorf("calls placed = %d; want 1, the ladder's", calls)
	}
	if len(apollo.posted) != 1 || len(shared.posted) != 0 {
		t.Errorf("cards: Apollo %d, shared %d; want 1 in Apollo's space only", len(apollo.posted), len(shared.posted))
	}
}

// A space that fails while the other takes the card is not a failed rung (a
// retry would ring the engineer again); a rung no space took is retried.
func TestTeamChatSpaces_FanoutFailures(t *testing.T) {
	ok := newChatSpace(t)
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(down.Close)
	chat := func(url, room string) chatNotifier {
		return chatNotifier{chat: notifications.NewGoogleChatClient(notifications.GoogleChatConfig{
			AudienceSpaces: []notifications.GoogleChatAudienceSpace{{Audience: room, WebhookURL: url}},
		}), audience: room}
	}
	noSpace := chatNotifier{chat: notifications.NewGoogleChatClient(notifications.GoogleChatConfig{}), audience: "Nowhere"}
	plan := Plan{Trigger: Trigger{IncidentID: testIncidentID, Number: "INC0099001", Team: "Default SRE Group"}}
	rung := PlannedCall{Level: Level0, Ordinal: 1}
	ctx := context.Background()

	if _, err := (fanoutChat{chats: []chatNotifier{chat(down.URL, "Apollo SRE"), chat(ok.url, "Artemis SRE")}}).Deliver(ctx, plan, rung); err != nil {
		t.Errorf("one space down, one took it: err = %v; want delivered", err)
	}
	if ok.count() != 1 {
		t.Errorf("the working space got %d card(s); want 1", ok.count())
	}
	if _, err := (fanoutChat{chats: []chatNotifier{chat(down.URL, "Apollo SRE"), noSpace}}).Deliver(ctx, plan, rung); err == nil || isPermanent(err) {
		t.Errorf("no space took it, one transient: err = %v; want a transient error, so the rung retries", err)
	}
	if _, err := (fanoutChat{chats: []chatNotifier{noSpace, noSpace}}).Deliver(ctx, plan, rung); !isPermanent(err) {
		t.Errorf("no space configured anywhere: err = %v; want a permanent error", err)
	}
}

func TestTeamChatSpaces_TeamlessChatsConfig(t *testing.T) {
	base := `
enabled: true
sre:
  enabled: true
  channel: chat
  teamlessChats: %s
  chat:
    webhookUrlEnv: SRE_CHAT_WEBHOOK_URL
  teamChats:
    apollo:  { webhookUrlEnv: SRE_CHAT_WEBHOOK_URL_APOLLO }
    artemis: { webhookUrlEnv: SRE_CHAT_WEBHOOK_URL_ARTEMIS }
  teams:
    abts: [apollo, artemis]
`
	for _, v := range []string{"[]", "[apollo, artemis]", "[Apollo]"} {
		if _, err := loadYAML(t, fmt.Sprintf(base, v)); err != nil {
			t.Errorf("teamlessChats %s: %v", v, err)
		}
	}
	for v, want := range map[string]string{
		"[apollo, iaas]":   "has no space in teamChats",
		"[apollo, Apollo]": "twice",
	} {
		if _, err := loadYAML(t, fmt.Sprintf(base, v)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("teamlessChats %s: err = %v; want one containing %q", v, err, want)
		}
	}
	cre := "\nenabled: true\ncre:\n  enabled: true\n  teamlessChats: [apollo]\n"
	if _, err := loadYAML(t, cre); err == nil || !strings.Contains(err.Error(), "SRE settings") {
		t.Errorf("teamlessChats on CRE: err = %v; want it refused", err)
	}
}

// Each SME team posts to its own space, and none to the shared SME space. The
// SME teams name an audience only, so their spaces come from
// GOOGLE_CHAT_SPACES (the service's shared client): no variable per team.
func TestTeamChatSpaces_EachSMETeamPostsToItsOwnSpace(t *testing.T) {
	shared, asgardeo, choreo := newChatSpace(t), newChatSpace(t), newChatSpace(t)
	googleChatSpaces := notifications.NewGoogleChatClient(notifications.GoogleChatConfig{AudienceSpaces: []notifications.GoogleChatAudienceSpace{
		{Audience: "Special Ops", WebhookURL: shared.url},
		{Audience: "Asgardeo SME", WebhookURL: asgardeo.url},
		{Audience: "Choreo SME", WebhookURL: choreo.url},
	}})

	sme := smeOn
	sme.Chat = Chat{Audience: "Special Ops"}
	sme.TeamChats = map[string]Chat{
		"asgardeo":   {Audience: "Asgardeo SME"},
		"choreo-sme": {Audience: "Choreo SME"},
	}
	cfg := EngineConfig{CallSendingEnabled: true, Channel: ChannelChat, Kind: LadderSRE, SME: sme}
	store := newMemStore()
	e := NewEngine(DefaultPolicy, rotaResolver(rotaReader(true), "").WithCallHistory(store), nil, googleChatSpaces, PortalLinks{}, nil, nil, "", cfg)
	e.store = store
	e.notes = &fakeNotes{}
	e.clock = func() time.Time { return testClock }

	raise(t, e, handoff("Apollo", "asgardeo", "asgardeo", testClock))
	raise(t, e, handoff("Apollo", "", "choreo", testClock.Add(time.Minute)))

	if asgardeo.count() != 1 || choreo.count() != 1 {
		t.Errorf("cards: Asgardeo %d, Choreo %d; want one each, in its own space", asgardeo.count(), choreo.count())
	}
	if shared.count() != 0 {
		t.Errorf("%d card(s) in the shared SME space; with sme.teamChats set it gets none", shared.count())
	}
}

// The nine SME teams, uncommented from paging-alert.yaml's sme.teamChats
// example, load, and each is wired to its own GOOGLE_CHAT_SPACES audience.
func TestTeamChatSpaces_TheNineSMETeamsFromTheExample(t *testing.T) {
	raw, err := os.ReadFile("../../../../scripts/csm-compose/paging-alert.yaml")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	start := strings.Index(body, "To give all nine teams their own space:\n")
	end := strings.Index(body, "  # (webhookUrlEnv: <VARIABLE> per team")
	sme := strings.Index(body, "\nsme:\n")
	if start < 0 || end < start || sme < 0 {
		t.Fatal("paging-alert.yaml no longer has the sme.teamChats example this test reads")
	}
	var example []string
	for _, line := range strings.Split(body[start:end], "\n")[1:] {
		// "  #   teamChats:" is "  teamChats:", "  #     asgardeo: ..." is "    asgardeo: ...".
		if l, ok := strings.CutPrefix(line, "  #   "); ok {
			example = append(example, "  "+l)
		}
	}
	// Replace sme's own teamChats: {} with the example.
	at := sme + strings.Index(body[sme:], "\n  teamChats: {}\n")
	body = body[:at] + "\n" + strings.Join(example, "\n") + body[at+len("\n  teamChats: {}"):]

	cfg, err := loadYAML(t, body)
	if err != nil {
		t.Fatalf("load with the example uncommented: %v", err)
	}
	want := []string{"asgardeo", "b-central", "bijira", "choreo-runtime", "cloud-agent", "cloud-core", "devant", "moesif", "u2"}
	if got := sortedTeamChatKeys(cfg.SME.TeamChats); !slices.Equal(got, want) {
		t.Fatalf("sme.teamChats teams = %v; want the nine %v", got, want)
	}

	var spaces []notifications.GoogleChatAudienceSpace
	for _, c := range cfg.SME.TeamChats {
		if c.WebhookURLEnv != "" {
			t.Errorf("%q names a variable; the example keeps the URLs in GOOGLE_CHAT_SPACES", c.Audience)
		}
		spaces = append(spaces, notifications.GoogleChatAudienceSpace{Audience: c.Audience, WebhookURL: "https://chat.example/" + c.Audience})
	}
	ec := EngineConfig{CallSendingEnabled: true, Channel: ChannelChat, Kind: LadderSRE, SME: cfg.SME}
	ec.SME.Enabled, ec.SME.Channel = true, ChannelChat
	e := NewEngine(DefaultPolicy, rotaResolver(rotaReader(true), ""), nil,
		notifications.NewGoogleChatClient(notifications.GoogleChatConfig{AudienceSpaces: spaces}), PortalLinks{}, nil, nil, "", ec)

	rooms := map[string]bool{}
	for _, team := range want {
		var room string
		for _, n := range e.smeNotifiersFor(team) {
			if c, ok := n.(chatNotifier); ok {
				room = c.roomFor()
				if !c.chat.HasAudienceSpace(room) {
					t.Errorf("%s: audience %q has no space", team, room)
				}
			}
		}
		if room == "" || rooms[room] {
			t.Errorf("%s: room %q; want a space of its own", team, room)
		}
		rooms[room] = true
	}
}

// A teamChats key no incident can match is reported at startup, so a
// misspelt team does not quietly keep its cards in sre.chat.
func TestTeamChatSpaces_UnknownSRETeamKeys(t *testing.T) {
	l := LadderConfig{Teams: splitSRETeams, TeamChats: map[string]Chat{
		"apollo":            {},
		"Artemis":           {}, // case does not matter
		"iaas":              {},
		"default sre group": {}, // a default group is matched like a team
		"apolo":             {}, // misspelt
		"PaaS":              {}, // no such team
	}}
	if got, want := unknownSRETeamChats(l), []string{"PaaS", "apolo"}; !slices.Equal(got, want) {
		t.Errorf("unknown teamChats keys = %v; want %v", got, want)
	}

	l.Teams.ABTs = nil // teams from abtType: unknown here, so nothing is reported
	if got := unknownSRETeamChats(l); got != nil {
		t.Errorf("with no teams.abts, unknown keys = %v; want none", got)
	}

	if got, want := sortedTeamChatKeys(map[string]Chat{"Iaas": {}, "apollo": {}}), []string{"apollo", "iaas"}; !slices.Equal(got, want) {
		t.Errorf("sortedTeamChatKeys = %v; want %v", got, want)
	}
}
