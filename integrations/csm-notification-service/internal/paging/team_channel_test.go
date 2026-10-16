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
	"strings"
	"testing"
)

// A team can do less than its ladder, never more.
func TestNarrowChannel(t *testing.T) {
	cases := []struct{ ladder, team, want Channel }{
		{ChannelBoth, "", ChannelBoth},
		{ChannelBoth, ChannelChat, ChannelChat},
		{ChannelBoth, ChannelCall, ChannelCall},
		{ChannelBoth, ChannelLog, ChannelLog},
		{ChannelChat, ChannelBoth, ChannelChat},
		{ChannelChat, ChannelCall, ChannelLog},
		{ChannelCall, ChannelChat, ChannelLog},
		{ChannelCall, ChannelBoth, ChannelCall},
		{ChannelLog, ChannelBoth, ChannelLog},
	}
	for _, c := range cases {
		if got := narrowChannel(c.ladder, c.team); got != c.want {
			t.Errorf("narrowChannel(%s, %q) = %s, want %s", c.ladder, c.team, got, c.want)
		}
	}
}

// teamChannelEngine is a both-channel SRE ladder with Apollo on chat only,
// Artemis on calls only and IaaS gone quiet; it counts what each rung does.
func teamChannelEngine(calls *int, card *fakeChat) *Engine {
	e := ladderEngine(LadderSRE, card, newMemStore())
	e.cfg.Channel = ChannelBoth
	e.cfg.Ladder.Channel = ChannelBoth
	e.cfg.Ladder.Teams = splitSRETeams
	e.cfg.Ladder.TeamChannels = map[string]Channel{"apollo": ChannelChat, "artemis": ChannelCall, "iaas": ChannelLog}
	e.notifiers = []notifier{countingCall{n: calls}, chatNotifier{chat: card, links: fakeLinks{}, audience: "SRE"}}
	return e
}

// Each team's rung goes out only the way its team is set to: Apollo's posts
// the card and rings nobody, Artemis's rings and posts nothing, IaaS's only
// writes down what it would have done. A team with nothing set keeps the
// ladder's: both.
func TestTeamChannel_EachTeamsRungGoesOutItsWay(t *testing.T) {
	cases := []struct {
		group        string
		calls, cards int
	}{
		{"Apollo SRE Group", 0, 1},
		{"Artemis SRE Group", 1, 0},
		{"SRE IaaS", 0, 0},
		{"Default SRE Group", 1, 1},
	}
	for _, c := range cases {
		t.Run(c.group, func(t *testing.T) {
			calls, card := 0, &fakeChat{}
			e := teamChannelEngine(&calls, card)
			plan := Plan{Trigger: Trigger{IncidentID: testIncidentID, Team: c.group}}
			rung := PlannedCall{Level: Level0, Ordinal: 1, Recipient: Recipient{Name: "a-l1", Phone: "+10000000000"}}
			if err, cardErr := e.place(context.Background(), plan, rung, false); err != nil || cardErr != nil {
				t.Fatal(err, cardErr)
			}
			if calls != c.calls || len(card.posted) != c.cards {
				t.Errorf("calls %d, cards %d; want %d, %d", calls, len(card.posted), c.calls, c.cards)
			}
		})
	}
}

// The call safety checks read the incident's channel: a team on chat only
// holds no calls, since it places none.
func TestTeamChannel_ChannelForReadsTheIncidentsTeam(t *testing.T) {
	calls, card := 0, &fakeChat{}
	e := teamChannelEngine(&calls, card)
	for group, want := range map[string]Channel{
		"Apollo SRE Group": ChannelChat, "apollo": ChannelChat, "Artemis SRE Group": ChannelCall,
		"SRE IaaS": ChannelLog, "Default SRE Group": ChannelBoth, "": ChannelBoth,
	} {
		if got := e.channelFor(Trigger{Team: group}); got != want {
			t.Errorf("channelFor(%q) = %s, want %s", group, got, want)
		}
	}
	e.cfg.Ladder.TeamChannels = nil
	if got := e.channelFor(Trigger{Team: "Apollo SRE Group"}); got != ChannelBoth {
		t.Errorf("with no team channels, channelFor = %s, want the ladder's", got)
	}
}

// A team's row saved for its channel alone sets the channel and keeps the
// file's room; a channel anywhere but an SRE team's row is skipped; one the
// file would not read refuses the merge.
func TestMergeCaseAlertSettings_TeamChannels(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	merged, env, changes, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Rooms: []CaseAlertRoom{
		{Ladder: "SRE", TeamKey: "Artemis", Label: "Artemis SRE", Channel: "chat"},
		{Ladder: "SRE", TeamKey: "", Label: "SRE", Channel: "call"},
		{Ladder: "CRE", Label: "CRE", Channel: "log"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if merged.SRE.TeamChannels["artemis"] != ChannelChat || len(merged.SRE.TeamChannels) != 1 {
		t.Errorf("team channels %v", merged.SRE.TeamChannels)
	}
	if merged.SRE.TeamChats["artemis"] != cfg.SRE.TeamChats["artemis"] || len(env) != 0 {
		t.Errorf("a row with no URL replaced the file's room: %+v, env %v", merged.SRE.TeamChats["artemis"], env)
	}
	if len(cfg.SRE.TeamChannels) != 0 {
		t.Error("the merge wrote into the caller's configuration")
	}
	joined := strings.Join(changes, "\n")
	if !strings.Contains(joined, "sre team artemis: alerts by chat") || strings.Count(joined, "skipped a channel") != 2 {
		t.Errorf("changes:\n%s", joined)
	}
	if _, _, _, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Rooms: []CaseAlertRoom{{Ladder: "SRE", TeamKey: "apollo", Channel: "sms"}}}); err == nil {
		t.Error("a channel the file would not read was accepted")
	}
}

// The last-loaded copy keeps a team's channel (it holds no URL, and needs
// none for that).
func TestTeamChannel_SurvivesTheLastLoadedCopy(t *testing.T) {
	b, _ := json.Marshal(CaseAlertSettings{Rooms: []CaseAlertRoom{{Ladder: "SRE", TeamKey: "apollo", Label: "Apollo SRE", Channel: "log"}}})
	var back CaseAlertSettings
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	merged, _, _, err := MergeCaseAlertSettings(loadConfigText(t, prodShapedConfig), back)
	if err != nil || merged.SRE.TeamChannels["apollo"] != ChannelLog {
		t.Fatalf("err %v, channels %v", err, merged.SRE.TeamChannels)
	}
}
