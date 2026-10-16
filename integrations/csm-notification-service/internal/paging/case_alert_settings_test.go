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
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// prodShapedConfig is a file shaped like Production's: a room per ladder, a
// room per SRE team, two teamless rooms, aliases and default groups. No real
// value is in it -- every webhook is a variable name.
const prodShapedConfig = `
enabled: true
cre:
  enabled: true
  channel: both
  teams:
    abts: [atlas, castor]
    americas: americas_cre_team
    aliases:
      americas: americas_cre_team
  chat:
    webhookUrlEnv: CRE_CHAT_WEBHOOK_URL
    audience: "CRE Case Paging"
sre:
  enabled: true
  channel: both
  chat:
    webhookUrlEnv: SRE_CHAT_WEBHOOK_URL
    audience: "SRE Case Paging"
  teamChats:
    apollo:  { webhookUrlEnv: SRE_CHAT_WEBHOOK_URL_APOLLO,  audience: "Apollo SRE" }
    artemis: { webhookUrlEnv: SRE_CHAT_WEBHOOK_URL_ARTEMIS, audience: "Artemis SRE" }
    iaas:    { webhookUrlEnv: SRE_CHAT_WEBHOOK_URL_IAAS,    audience: "IaaS SRE" }
  teamlessChats: [apollo, artemis]
  teams:
    abts: [apollo, artemis, iaas]
    aliases:
      "apollo sre group": apollo
      "artemis sre group": artemis
      "sre iaas": iaas
    defaultGroups: ["default sre group", "ms/pc sre group"]
    defaultRota: SRE_SAAS
`

func loadConfigText(t *testing.T, text string) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "paging-alert.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

// fakeHook is a made-up webhook URL standing for the value of variable name.
func fakeHook(name string) string {
	return "https://chat.googleapis.com/v1/spaces/" + strings.ToLower(name) + "/messages?key=k&token=t"
}

// savedFrom is what an admin would save to reproduce cfg exactly: every room
// that names a variable, with that variable's (fake) URL, and every alias and
// default group.
func savedFrom(cfg Config) CaseAlertSettings {
	s := CaseAlertSettings{Revision: 7}
	room := func(ladder, team string, c Chat, teamless bool) {
		if c.WebhookURLEnv != "" {
			s.Rooms = append(s.Rooms, CaseAlertRoom{Ladder: ladder, TeamKey: team, Label: c.Audience, Teamless: teamless, WebhookURL: fakeHook(c.WebhookURLEnv)})
		}
	}
	room("CRE", "", cfg.CRE.Chat, false)
	room("SRE", "", cfg.SRE.Chat, false)
	for team, c := range cfg.SRE.TeamChats {
		room("SRE", team, c, slices.Contains(cfg.SRE.TeamlessChats, team))
	}
	for g, team := range cfg.CRE.Teams.Aliases {
		s.Routes = append(s.Routes, CaseAlertRoute{Ladder: "CRE", GroupName: g, TeamKey: team})
	}
	for g, team := range cfg.SRE.Teams.Aliases {
		s.Routes = append(s.Routes, CaseAlertRoute{Ladder: "SRE", GroupName: g, TeamKey: team})
	}
	for _, g := range cfg.SRE.Teams.DefaultGroups {
		s.Routes = append(s.Routes, CaseAlertRoute{Ladder: "SRE", GroupName: g, RotaCode: cfg.SRE.Teams.DefaultRota})
	}
	return s
}

// resolvedChats replaces each room's variable with the URL it resolves to,
// and sorts the lists a merge may reorder, so two configurations that page
// alike compare equal.
func resolvedChats(cfg Config, lookup func(string) string) Config {
	resolve := func(c Chat) Chat {
		if c.WebhookURLEnv != "" {
			c.WebhookURLEnv = lookup(c.WebhookURLEnv)
		}
		return c
	}
	for _, l := range []*LadderConfig{&cfg.CRE, &cfg.SRE} {
		*l = cloneLadderChats(*l)
		l.Chat = resolve(l.Chat)
		for k, c := range l.TeamChats {
			l.TeamChats[k] = resolve(c)
		}
		slices.Sort(l.TeamlessChats)
		slices.Sort(l.Teams.DefaultGroups)
	}
	return cfg
}

// Settings saved to match the file give a configuration that pages exactly as
// the file does: same rooms (by URL), same labels, same routing, same
// everything else. This is what makes switching to db a no-op until an admin
// changes something.
func TestMergeCaseAlertSettings_SavedLikeTheFileChangesNothing(t *testing.T) {
	files := map[string]Config{"production-shaped": loadConfigText(t, prodShapedConfig)}
	if shipped, err := LoadConfig("../../../../scripts/csm-compose/paging-alert.yaml"); err == nil {
		files["shipped example"] = shipped
	}
	for name, cfg := range files {
		t.Run(name, func(t *testing.T) {
			merged, env, _, err := MergeCaseAlertSettings(cfg, savedFrom(cfg))
			if err != nil {
				t.Fatalf("merge: %v", err)
			}
			for v, url := range env {
				if !strings.HasPrefix(v, "CASE_ALERTS_ROOM_") || !strings.HasPrefix(url, "https://chat.googleapis.com/") {
					t.Fatalf("env %s=%q", v, url)
				}
			}
			before := resolvedChats(cfg, fakeHook)
			after := resolvedChats(merged, func(v string) string { return env[v] })
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("the merge changed what is paged:\n file:  %+v\n saved: %+v", before.SRE, after.SRE)
			}
		})
	}
}

func TestMergeCaseAlertSettings_NothingSavedIsTheFile(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	merged, env, changes, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Revision: 3})
	if err != nil || len(env) != 0 || len(changes) != 0 || !reflect.DeepEqual(merged, cfg) {
		t.Fatalf("merged=%v env=%v changes=%v err=%v", reflect.DeepEqual(merged, cfg), env, changes, err)
	}
}

// A saved room replaces only what it names; a saved route only its group.
// The caller's configuration is never written into (shadow mode keeps using it).
func TestMergeCaseAlertSettings_ReplacesOnlyWhatIsSaved(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	pristine := loadConfigText(t, prodShapedConfig)
	saved := CaseAlertSettings{
		Rooms: []CaseAlertRoom{
			{Ladder: "SRE", TeamKey: "Artemis", Label: "Artemis SRE (new)", Teamless: false, WebhookURL: "https://chat.googleapis.com/v1/spaces/NEW/messages?key=k&token=t"},
			{Ladder: "SRE", TeamKey: "lyra", Label: "Lyra SRE", Teamless: true, WebhookURL: "https://chat.googleapis.com/v1/spaces/LYRA/messages?key=k&token=t"},
		},
		Routes: []CaseAlertRoute{
			{Ladder: "SRE", GroupName: "MS/PC SRE Group", TeamKey: "apollo"},
			{Ladder: "SRE", GroupName: "other group", RotaCode: "SRE_IAAS"},
			{Ladder: "CRE", GroupName: "Phoenix", TeamKey: "castor"},
		},
	}
	merged, env, changes, err := MergeCaseAlertSettings(cfg, saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, pristine) {
		t.Fatal("the merge wrote into the caller's configuration")
	}

	if got := merged.SRE.TeamChats["artemis"]; got.Audience != "Artemis SRE (new)" || env[got.WebhookURLEnv] != saved.Rooms[0].WebhookURL {
		t.Errorf("artemis room = %+v", got)
	}
	if got := merged.SRE.TeamChats["apollo"]; got != cfg.SRE.TeamChats["apollo"] {
		t.Errorf("apollo's room changed without being saved: %+v", got)
	}
	if merged.SRE.Chat != cfg.SRE.Chat || merged.CRE.Chat != cfg.CRE.Chat {
		t.Error("a shared room changed without being saved")
	}
	slices.Sort(merged.SRE.TeamlessChats)
	if want := []string{"apollo", "lyra"}; !slices.Equal(merged.SRE.TeamlessChats, want) {
		t.Errorf("teamlessChats = %v, want %v", merged.SRE.TeamlessChats, want)
	}
	if merged.SRE.Teams.Aliases["ms/pc sre group"] != "apollo" || slices.Contains(merged.SRE.Teams.DefaultGroups, "ms/pc sre group") {
		t.Errorf("ms/pc sre group: alias %q, default groups %v", merged.SRE.Teams.Aliases["ms/pc sre group"], merged.SRE.Teams.DefaultGroups)
	}
	if _, ok := merged.SRE.Teams.Aliases["other group"]; ok || slices.Contains(merged.SRE.Teams.DefaultGroups, "other group") {
		t.Error("a route to a rota other than the default rota was applied")
	}
	if merged.CRE.Teams.Aliases["phoenix"] != "castor" || merged.CRE.Teams.Aliases["americas"] != "americas_cre_team" {
		t.Errorf("CRE aliases = %v", merged.CRE.Teams.Aliases)
	}
	joined := strings.Join(changes, "\n")
	for _, want := range []string{"sre.teamChats.artemis", "sre.teamChats.lyra: none (used sre.chat)", "sre.teamlessChats: - artemis", "sre.teamlessChats: + lyra", "skipped route"} {
		if !strings.Contains(joined, want) {
			t.Errorf("changes do not mention %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "googleapis") || strings.Contains(joined, "token=") {
		t.Error("a change line quotes a URL")
	}
}

// Saved settings the file's own checks would refuse are refused: the caller
// keeps the file, never the defaults.
func TestMergeCaseAlertSettings_RefusesWhatTheFileWouldRefuse(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	// apollo is an SRE team; it cannot also be a default group.
	_, env, _, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Routes: []CaseAlertRoute{{Ladder: "SRE", GroupName: "apollo", RotaCode: "SRE_SAAS"}}})
	if err == nil || env != nil {
		t.Fatalf("err=%v env=%v; want refused", err, env)
	}
}

func TestCaseAlertRoomEnv(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"CRE", ""}:          "CASE_ALERTS_ROOM_CRE",
		{"SRE", ""}:          "CASE_ALERTS_ROOM_SRE",
		{"SRE", "apollo"}:    "CASE_ALERTS_ROOM_SRE_APOLLO",
		{"SRE", "cloud-x.1"}: "CASE_ALERTS_ROOM_SRE_CLOUD_X_1",
	} {
		if got := caseAlertRoomEnv(in[0], in[1]); got != want || !envVarName.MatchString(got) {
			t.Errorf("caseAlertRoomEnv(%v) = %q, want %q", in, got, want)
		}
	}
}

// smeShapedConfig is prodShapedConfig with SME teams' rooms, as Production
// names them: an audience each, the URLs in GOOGLE_CHAT_SPACES.
const smeShapedConfig = prodShapedConfig + `
sme:
  enabled: true
  channel: chat
  teamChats:
    asgardeo: { audience: "Asgardeo SME" }
    u2:       { audience: "U2 SME" }
`

// A saved SME room replaces that SME team's room, or gives a team the file
// leaves out one of its own; the other SME teams and every SRE and CRE room
// keep the file's.
func TestMergeCaseAlertSettings_SMETeamRooms(t *testing.T) {
	cfg := loadConfigText(t, smeShapedConfig)
	pristine := loadConfigText(t, smeShapedConfig)
	saved := CaseAlertSettings{Rooms: []CaseAlertRoom{
		{Ladder: "SME", TeamKey: "Asgardeo", Label: "Asgardeo SME (saved)", WebhookURL: "https://chat.googleapis.com/v1/spaces/ASG/messages?key=k&token=t"},
		{Ladder: "SME", TeamKey: "moesif", Label: "Moesif SME", WebhookURL: "https://chat.googleapis.com/v1/spaces/MOE/messages?key=k&token=t"},
	}}
	merged, env, changes, err := MergeCaseAlertSettings(cfg, saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, pristine) {
		t.Fatal("the merge wrote into the caller's configuration")
	}
	if got := merged.SME.TeamChats["asgardeo"]; got.Audience != "Asgardeo SME (saved)" || got.WebhookURLEnv != "CASE_ALERTS_ROOM_SME_ASGARDEO" || env[got.WebhookURLEnv] != saved.Rooms[0].WebhookURL {
		t.Errorf("asgardeo room = %+v", got)
	}
	if got := merged.SME.TeamChats["moesif"]; got.Audience != "Moesif SME" || env[got.WebhookURLEnv] != saved.Rooms[1].WebhookURL {
		t.Errorf("moesif room = %+v", got)
	}
	if got := merged.SME.TeamChats["u2"]; got != cfg.SME.TeamChats["u2"] {
		t.Errorf("u2's room changed without being saved: %+v", got)
	}
	if !reflect.DeepEqual(merged.SRE.TeamChats, cfg.SRE.TeamChats) || merged.SRE.Chat != cfg.SRE.Chat || merged.CRE.Chat != cfg.CRE.Chat {
		t.Error("an SRE or CRE room changed when only SME rooms were saved")
	}
	joined := strings.Join(changes, "\n")
	for _, want := range []string{`sme.teamChats.asgardeo: "Asgardeo SME" via GOOGLE_CHAT_SPACES`, "sme.teamChats.moesif: none (used sme.chat)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("changes do not mention %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "googleapis") || strings.Contains(joined, "token=") {
		t.Error("a change line quotes a URL")
	}
}

// With no SME team rooms in the file, a saved one is used for its team; every
// other SME team keeps sme.chat (the engine's fallback).
func TestMergeCaseAlertSettings_SMERoomWithoutTheFilesTeamRooms(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	merged, env, _, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Rooms: []CaseAlertRoom{
		{Ladder: "SME", TeamKey: "asgardeo", Label: "Asgardeo SME", WebhookURL: "https://chat.googleapis.com/v1/spaces/ASG/messages?key=k&token=t"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if a := merged.SME.TeamChats["asgardeo"]; a.Audience != "Asgardeo SME" || env[a.WebhookURLEnv] == "" || len(merged.SME.TeamChats) != 1 {
		t.Fatalf("SME rooms = %v, env = %v", merged.SME.TeamChats, env)
	}
}
