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
	"encoding/json"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// prodShapedMinimal is prodShapedConfig with only what stays in the file.
const prodShapedMinimal = `
enabled: true
cre:
  enabled: true
  channel: both
sre:
  enabled: true
  channel: both
`

// valuesFrom is what "Copy what's in use" on the Alert Settings tab saves: every
// ladder value cfg runs with, as RunningCaseAlertValues reports it.
func valuesFrom(t *testing.T, cfg Config) []CaseAlertValue {
	t.Helper()
	b, err := json.Marshal(RunningCaseAlertValues(cfg, nil))
	if err != nil {
		t.Fatal(err)
	}
	var running struct {
		Ladders map[string]map[string]struct {
			Value  json.RawMessage `json:"value"`
			Source string          `json:"source"`
		} `json:"ladders"`
	}
	if err := json.Unmarshal(b, &running); err != nil {
		t.Fatal(err)
	}
	var out []CaseAlertValue
	for ladder, keys := range running.Ladders {
		for key, v := range keys {
			if v.Source != "file" {
				t.Fatalf("%s %s reported from %q with nothing saved", ladder, key, v.Source)
			}
			out = append(out, CaseAlertValue{Ladder: ladder, Key: key, Value: v.Value})
		}
	}
	return out
}

// comparable puts a configuration in a form two that page alike share:
// rooms by URL (resolvedChats), group names in lower case as the resolver
// matches them, and no list where the file has an empty one.
func comparable(cfg Config, lookup func(string) string) Config {
	cfg = resolvedChats(cfg, lookup)
	for _, l := range []*LadderConfig{&cfg.CRE, &cfg.SRE} {
		if l.Teams.Aliases != nil {
			lower := map[string]string{}
			for k, v := range l.Teams.Aliases {
				lower[strings.ToLower(k)] = v
			}
			l.Teams.Aliases = lower
		}
		groups := []string{}
		for _, g := range l.Teams.DefaultGroups {
			groups = append(groups, strings.ToLower(g))
		}
		sort.Strings(groups)
		l.Teams.DefaultGroups = groups
		if len(l.TeamlessChats) == 0 {
			l.TeamlessChats = nil
		}
	}
	return cfg
}

// The small file plus everything copied from what the full file runs with
// pages exactly as the full file does. This is what lets production move to
// the small file: save the settings, then swap the file.
func TestMergeCaseAlertSettings_SmallFilePlusSavedIsTheFullFile(t *testing.T) {
	type pair struct{ full, small Config }
	pairs := map[string]pair{
		"production-shaped": {loadConfigText(t, prodShapedConfig), loadConfigText(t, prodShapedMinimal)},
	}
	full, errFull := LoadConfig("../../../../scripts/csm-compose/paging-alert.yaml")
	small, errSmall := LoadConfig("../../../../scripts/csm-compose/paging-alert.minimal.yaml")
	if errFull != nil || errSmall != nil {
		t.Fatalf("shipped examples: %v / %v", errFull, errSmall)
	}
	pairs["shipped examples"] = pair{full, small}

	for name, p := range pairs {
		t.Run(name, func(t *testing.T) {
			saved := savedFrom(p.full)
			saved.Values = valuesFrom(t, p.full)
			if len(saved.Values) < 15 {
				t.Fatalf("only %d values copied", len(saved.Values))
			}
			merged, env, _, err := MergeCaseAlertSettings(p.small, saved)
			if err != nil {
				t.Fatalf("merge: %v", err)
			}
			want := comparable(p.full, fakeHook)
			got := comparable(merged, func(v string) string { return env[v] })
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("the small file and the saved settings page differently:\n full:  %+v\n saved: %+v\n full SRE:  %+v\n saved SRE: %+v",
					want.CRE, got.CRE, want.SRE, got.SRE)
			}
		})
	}
}

// Each key goes into its own field, and reads back the same.
func TestMergeCaseAlertSettings_EveryValueLandsInItsField(t *testing.T) {
	cfg := loadConfigText(t, prodShapedMinimal)
	values := []CaseAlertValue{
		{"CRE", "trigger.priorities", json.RawMessage(`["S0","S1"]`)},
		{"CRE", "trigger.teams", json.RawMessage(`["phoenix group"]`)},
		{"CRE", "trigger.excludeTeams", json.RawMessage(`["noisy group"]`)},
		{"CRE", "trigger.shifts", json.RawMessage(`["USA"]`)},
		{"CRE", "trigger.requireKnownTeam", json.RawMessage(`true`)},
		{"CRE", "onLeave", json.RawMessage(`"call"`)},
		{"CRE", "teams.abtType", json.RawMessage(`"cre-abt"`)},
		{"CRE", "teams.abts", json.RawMessage(`["atlas","vega"]`)},
		{"CRE", "teams.americas", json.RawMessage(`"americas"`)},
		{"CRE", "teams.leadership", json.RawMessage(`"cre-leadership"`)},
		{"CRE", "teams.teamLeadsToCall", json.RawMessage(`3`)},
		{"CRE", "teams.teamLeads", json.RawMessage(`["atlas"]`)},
		{"CRE", "teams.rotaMembersToCall", json.RawMessage(`{"LK_EVENING":7}`)},
		{"CRE", "teams.americasWeekendRotaShifts", json.RawMessage(`["CRE_WEEKEND_NIGHT"]`)},
		{"CRE", "teams.rotaTeams", json.RawMessage(`["atlas","americas"]`)},
		{"CRE", "teams.unassignedTeamLead", json.RawMessage(`"none"`)},
		{"CRE", "teams.unassignedTeamLeadCount", json.RawMessage(`2`)},
		{"CRE", "heads.cre", json.RawMessage(`{"name":"CRE Head","email":"crehead@example.test"}`)},
		{"CRE", "heads.cs", json.RawMessage(`{"name":"CS Head","email":"cshead@example.test"}`)},
		{"CRE", "alertDuty.tiers", json.RawMessage(`["T1","T2"]`)},
		{"CRE", "alertDuty.perTeam", json.RawMessage(`1`)},
		{"CRE", "acknowledgement.requireBoth", json.RawMessage(`false`)},
		{"SRE", "trigger.priorities", json.RawMessage(`["S0"]`)},
		{"SRE", "onLeave", json.RawMessage(`"skip"`)},
		{"SRE", "teams.abtType", json.RawMessage(`"sre-abt"`)},
		{"SRE", "teams.abts", json.RawMessage(`["apollo","artemis","iaas"]`)},
		{"SRE", "teams.defaultRota", json.RawMessage(`"SRE_SAAS"`)},
		{"SRE", "timing.initialWait", json.RawMessage(`"30s"`)},
		{"SRE", "timing.interval", json.RawMessage(`"10m"`)},
		{"SRE", "timing.includeL4", json.RawMessage(`true`)},
		{"SRE", "unansweredChat", json.RawMessage(`true`)},
		{"SRE", "smeHandoffChat", json.RawMessage(`true`)},
	}
	merged, _, changes, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Values: values})
	if err != nil {
		t.Fatal(err)
	}
	c, s := merged.CRE, merged.SRE
	checks := map[string]bool{
		"cre priorities":   slices.Equal(c.Start.Priorities, []string{"S0", "S1"}),
		"cre teams":        slices.Equal(c.Start.Teams, []string{"phoenix group"}),
		"cre exclude":      slices.Equal(c.Start.ExcludeTeams, []string{"noisy group"}),
		"cre shifts":       slices.Equal(c.Start.Shifts, []string{"USA"}),
		"cre known team":   c.Start.RequireKnownTeam,
		"cre on leave":     c.OnLeave == OnLeaveCall,
		"cre abt type":     c.Teams.ABTType == "cre-abt",
		"cre abts":         slices.Equal(c.Teams.ABTs, []string{"atlas", "vega"}),
		"americas":         c.Teams.Americas == "americas",
		"leadership":       c.Teams.Leadership == "cre-leadership",
		"leads to call":    c.Teams.TeamLeadsToCall == 3,
		"team leads":       slices.Equal(c.Teams.TeamLeads, []string{"atlas"}),
		"rota members":     reflect.DeepEqual(c.Teams.RotaMembersToCall, map[string]int{"LK_EVENING": 7}),
		"weekend shifts":   slices.Equal(c.Teams.AmericasWeekendRotaShifts, []string{"CRE_WEEKEND_NIGHT"}),
		"rota teams":       slices.Equal(c.Teams.RotaTeams, []string{"atlas", "americas"}),
		"unassigned":       c.Teams.UnassignedTeamLead == "none" && c.Teams.UnassignedTeamLeadCount == 2,
		"cre head":         c.Heads.CRE == Person{Name: "CRE Head", Email: "crehead@example.test"},
		"cs head":          c.Heads.CS == Person{Name: "CS Head", Email: "cshead@example.test"},
		"tiers":            slices.Equal(c.AlertTiers(), []string{"T1", "T2"}),
		"per team":         c.AlertDuty.PerTeam == 1,
		"require both":     !c.RequireBothGestures(),
		"sre priorities":   slices.Equal(s.Start.Priorities, []string{"S0"}),
		"sre on leave":     s.OnLeave == OnLeaveSkip,
		"sre abts":         slices.Equal(s.Teams.ABTs, []string{"apollo", "artemis", "iaas"}) && s.Teams.ABTType == "sre-abt",
		"default rota":     s.Teams.DefaultRota == "SRE_SAAS",
		"initial wait":     time.Duration(s.Timing.InitialWait) == 30*time.Second,
		"interval":         time.Duration(s.Timing.Interval) == 10*time.Minute,
		"include L4":       s.Timing.IncludeL4,
		"unanswered chat":  s.UnansweredChat,
		"handoff chat":     s.SMEHandoffChat,
		"no safety change": merged.CRE.Safety.MaxCallsPerLadder == cfg.CRE.Safety.MaxCallsPerLadder,
	}
	for name, ok := range checks {
		if !ok {
			t.Errorf("%s was not set", name)
		}
	}
	covered := map[string]bool{}
	for _, v := range values {
		covered[v.Key] = true
	}
	for k := range caseAlertKeys {
		if !covered[k] {
			t.Errorf("%s is not set by this test: add it", k)
		}
	}
	joined := strings.Join(changes, "\n")
	if !strings.Contains(joined, "cre.heads.cre: changed") || strings.Contains(joined, "example.test") {
		t.Errorf("a head's change names them, or is missing:\n%s", joined)
	}
	if !strings.Contains(joined, `sre.timing.interval: unset -> "10m"`) {
		t.Errorf("no change line for the interval:\n%s", joined)
	}
}

// A list saved on the tab replaces the file's; the file's own list, which the
// caller keeps using in shadow mode, is never written into.
func TestMergeCaseAlertSettings_ValuesNeverWriteIntoTheFile(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	pristine := loadConfigText(t, prodShapedConfig)
	_, _, _, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Values: []CaseAlertValue{
		{"CRE", "teams.abts", json.RawMessage(`["vega"]`)},
		{"SRE", "teams.abts", json.RawMessage(`["apollo"]`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg, pristine) {
		t.Fatal("the merge wrote into the caller's configuration")
	}
}

// A key this build does not know, or one saved for the other ladder, is
// skipped and said so; a value in the wrong shape refuses the whole merge.
func TestMergeCaseAlertSettings_UnknownAndWrongValues(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	merged, _, changes, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Values: []CaseAlertValue{
		{"CRE", "teams.somethingNew", json.RawMessage(`1`)},
		{"CRE", "timing.interval", json.RawMessage(`"10m"`)},
		{"PAAS", "onLeave", json.RawMessage(`"call"`)},
		{"SME", "heads.cre", json.RawMessage(`{"name":"x","email":"y"}`)},
	}})
	if err != nil || !reflect.DeepEqual(comparable(merged, fakeHook).CRE, comparable(cfg, fakeHook).CRE) {
		t.Fatalf("err %v, CRE changed by skipped values", err)
	}
	joined := strings.Join(changes, "\n")
	for _, want := range []string{"skipped cre.teams.somethingNew", "skipped cre.timing.interval: not a CRE setting", `skipped a value for ladder "PAAS"`, "skipped sme.heads.cre: not a setting this build knows"} {
		if !strings.Contains(joined, want) {
			t.Errorf("changes do not say %q:\n%s", want, joined)
		}
	}

	for _, bad := range []CaseAlertValue{
		{"CRE", "teams.abts", json.RawMessage(`"atlas"`)},
		{"CRE", "heads.cre", json.RawMessage(`{"name":"x","email":"y","phone":"+1"}`)},
		{"SRE", "timing.interval", json.RawMessage(`"soon"`)},
		{"CRE", "onLeave", json.RawMessage(`"maybe"`)}, // refused by the file's own checks
	} {
		if _, env, _, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Values: []CaseAlertValue{bad}}); err == nil || env != nil {
			t.Errorf("%s %s accepted", bad.Key, bad.Value)
		}
	}
	// The file's checks run on the merged result: a team on both ladders.
	if _, _, _, err := MergeCaseAlertSettings(cfg, CaseAlertSettings{Values: []CaseAlertValue{
		{"CRE", "teams.abts", json.RawMessage(`["apollo"]`)},
	}}); err == nil {
		t.Error("a team saved on both ladders was accepted")
	}
}

// What is reported as running: unset keys left out, sources marked, heads
// without a phone number.
func TestRunningCaseAlertValues(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	cfg.CRE.Heads.CS = Person{Name: "CS Head", Email: "cshead@example.test", Phone: "+10000000000"}
	cfg.CRE.Safety.AllowedNumbers = []string{"+10000000001", "+10000000002"}
	got := RunningCaseAlertValues(cfg, map[string]map[string]bool{"CRE": {"teams.abts": true}})
	b, _ := json.Marshal(got)
	var r struct {
		Ladders map[string]map[string]struct {
			Value  json.RawMessage
			Source string
		}
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	cre, sre := r.Ladders["CRE"], r.Ladders["SRE"]
	if cre["teams.abts"].Source != "saved" || cre["onLeave"].Source != "file" {
		t.Errorf("sources: abts %q onLeave %q", cre["teams.abts"].Source, cre["onLeave"].Source)
	}
	if strings.Contains(string(b), "+10000000000") {
		t.Error("a head's phone number was reported")
	}
	var fileOnly struct {
		FileOnly map[string]map[string]any `json:"fileOnly"`
	}
	_ = json.Unmarshal(b, &fileOnly)
	if strings.Contains(string(b), "+10000000001") || fileOnly.FileOnly["CRE"]["safety.allowedNumbers"] != float64(2) {
		t.Errorf("allowed numbers listed, or not counted: %v", fileOnly.FileOnly["CRE"]["safety.allowedNumbers"])
	}
	if f := fileOnly.FileOnly["SRE"]; f["channel"] != "both" || f["enabled"] != true || f["phones.source"] != "profile" {
		t.Errorf("file-only SRE settings reported %v", f)
	}
	if _, ok := cre["heads.cre"]; ok {
		t.Error("an unset head was reported")
	}
	if _, ok := cre["timing.interval"]; ok {
		t.Error("an SRE key was reported for CRE")
	}
	if _, ok := sre["timing.interval"]; ok {
		t.Error("an unset interval was reported")
	}
	keys := make([]string, 0, len(sre))
	for k := range sre {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !slices.Contains(keys, "teams.abts") || !slices.Contains(keys, "onLeave") {
		t.Errorf("SRE keys reported: %v", keys)
	}
}

// The keys the tab can save, as entity-service lists them
// (entity-service/internal/service/case_alert_ladder_values.go). A key added
// on one side only is skipped by the other, so both lists change together.
func TestCaseAlertKeysMatchTheTab(t *testing.T) {
	want := map[string][]string{
		"trigger.priorities": caseAlertBoth, "trigger.teams": caseAlertBoth, "trigger.excludeTeams": caseAlertBoth,
		"trigger.shifts": caseAlertBoth, "trigger.requireKnownTeam": caseAlertBoth, "onLeave": caseAlertBoth,
		"teams.abtType": caseAlertBoth, "teams.abts": caseAlertBoth,
		"teams.americas": caseAlertCRE, "teams.leadership": caseAlertCRE, "teams.teamLeadsToCall": caseAlertCRE,
		"teams.teamLeads": caseAlertCRE, "teams.rotaMembersToCall": caseAlertCRE, "teams.americasWeekendRotaShifts": caseAlertCRE,
		"teams.rotaTeams": caseAlertCRE, "teams.unassignedTeamLead": caseAlertCRE, "teams.unassignedTeamLeadCount": caseAlertCRE,
		"heads.cre": caseAlertCRE, "heads.cs": caseAlertCRE, "alertDuty.tiers": caseAlertCRE, "alertDuty.perTeam": caseAlertCRE,
		"acknowledgement.requireBoth": caseAlertCRE,
		"teams.defaultRota":           caseAlertSRE, "timing.initialWait": caseAlertSRE, "timing.interval": caseAlertSRE,
		"timing.includeL4": caseAlertSRE, "unansweredChat": caseAlertSRE, "smeHandoffChat": caseAlertSRE,
	}
	if len(want) != len(caseAlertKeys) {
		t.Errorf("%d keys here, %d listed", len(caseAlertKeys), len(want))
	}
	for k, ladders := range want {
		if spec, ok := caseAlertKeys[k]; !ok || !slices.Equal(spec.ladders, ladders) {
			t.Errorf("%s: %v here, want %v", k, spec.ladders, ladders)
		}
	}
}

// SME values (0238) lay over the file's sme: section, one key each, and are
// reported back with where each came from.
func TestMergeCaseAlertSettings_SMEValues(t *testing.T) {
	cfg := loadConfigText(t, prodShapedConfig)
	saved := CaseAlertSettings{Values: []CaseAlertValue{
		{"SME", "timing.interval", json.RawMessage(`"10m"`)},
		{"SME", "onLeave", json.RawMessage(`"call"`)},
		{"SME", "unansweredChat", json.RawMessage(`true`)},
		{"SME", "teams", json.RawMessage(`{"asgardeo-ops":"asgardeo"}`)},
	}}
	merged, _, changes, err := MergeCaseAlertSettings(cfg, saved)
	if err != nil {
		t.Fatal(err)
	}
	if time.Duration(merged.SME.Timing.Interval) != 10*time.Minute || merged.SME.OnLeave != OnLeaveCall ||
		!merged.SME.UnansweredChat || merged.SME.Teams["asgardeo-ops"] != "asgardeo" {
		t.Fatalf("sme = %+v", merged.SME)
	}
	if cfg.SME.Teams != nil || cfg.SME.OnLeave == OnLeaveCall {
		t.Fatal("the merge wrote into the caller's configuration")
	}
	if !strings.Contains(strings.Join(changes, "\n"), "sme.timing.interval: unset -> \"10m\"") {
		t.Errorf("changes = %v", changes)
	}
	running := RunningCaseAlertValues(merged, SavedCaseAlertKeys(saved))
	b, _ := json.Marshal(running)
	for _, want := range []string{`"SME":{`, `"timing.interval":{"source":"saved","value":"10m"}`, `"teams":{"source":"saved"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("running does not carry %s:\n%s", want, b)
		}
	}
}
