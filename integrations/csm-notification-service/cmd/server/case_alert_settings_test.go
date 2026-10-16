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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/paging"
)

type fakeCaseAlertReader struct {
	saved    paging.CaseAlertSettings
	err      error
	errs     []error // one per read, before err
	reads    int
	reported string
	summary  map[string]any
}

func (f *fakeCaseAlertReader) CaseAlertSettings(context.Context) (paging.CaseAlertSettings, error) {
	f.reads++
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return paging.CaseAlertSettings{}, err
		}
	}
	return f.saved, f.err
}

func (f *fakeCaseAlertReader) ReportCaseAlertSettingsApplied(_ context.Context, _ int64, mode string, summary map[string]any) error {
	f.reported, f.summary = mode, summary
	return nil
}

// fileConfig is a loaded file's configuration: an SRE ladder with a shared
// room and Apollo's room.
func fileConfig() paging.Config {
	cfg := paging.DefaultConfig()
	cfg.SRE.Chat = paging.Chat{Audience: "SRE Case Paging", WebhookURLEnv: "SRE_CHAT_WEBHOOK_URL"}
	cfg.SRE.TeamChats = map[string]paging.Chat{"apollo": {Audience: "Apollo SRE", WebhookURLEnv: "SRE_CHAT_WEBHOOK_URL_APOLLO"}}
	return cfg
}

const savedHook = "https://chat.googleapis.com/v1/spaces/SAVED/messages?key=k&token=t"

func savedApollo() paging.CaseAlertSettings {
	return paging.CaseAlertSettings{Revision: 9, Rooms: []paging.CaseAlertRoom{
		{Ladder: "SRE", TeamKey: "apollo", Label: "Apollo SRE (saved)", WebhookURL: savedHook},
	}}
}

type envRecorder map[string]string

func (e envRecorder) set(k, v string) error { e[k] = v; return nil }

func TestCaseAlertSettingsMode(t *testing.T) {
	for in, want := range map[string]string{"": "db", "rooms": "off", "off": "off", " DB ": "db", "shadow": "shadow", "true": "off", "database": "off"} {
		if got := caseAlertSettingsMode(in); got != want {
			t.Errorf("caseAlertSettingsMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// Off asks nobody and changes nothing.
func TestApplyCaseAlertSettings_OffDoesNothing(t *testing.T) {
	reader, env := &fakeCaseAlertReader{saved: savedApollo()}, envRecorder{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceOff, fileConfig(), reader, env.set, caseAlertStart{})
	if reader.reads != 0 || len(env) != 0 || reader.reported != "" || !reflect.DeepEqual(got, fileConfig()) {
		t.Fatalf("off: reads=%d env=%v reported=%q changed=%v", reader.reads, env, reader.reported, !reflect.DeepEqual(got, fileConfig()))
	}
}

// Shadow reads and reports, but runs on the file.
func TestApplyCaseAlertSettings_ShadowKeepsTheFile(t *testing.T) {
	reader, env := &fakeCaseAlertReader{saved: savedApollo()}, envRecorder{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceShadow, fileConfig(), reader, env.set, caseAlertStart{})
	if !reflect.DeepEqual(got, fileConfig()) || len(env) != 0 || reader.reported != "shadow" {
		t.Fatalf("shadow: env=%v reported=%q changed=%v", env, reader.reported, !reflect.DeepEqual(got, fileConfig()))
	}
}

// DB uses the saved room, by putting its URL where the engine reads webhooks.
func TestApplyCaseAlertSettings_DBUsesTheSavedRoom(t *testing.T) {
	reader, env := &fakeCaseAlertReader{saved: savedApollo()}, envRecorder{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), reader, env.set, caseAlertStart{})
	apollo := got.SRE.TeamChats["apollo"]
	if apollo.Audience != "Apollo SRE (saved)" || env[apollo.WebhookURLEnv] != savedHook || reader.reported != "db" {
		t.Fatalf("db: apollo=%+v env=%v reported=%q", apollo, env, reader.reported)
	}
	if got.SRE.Chat != fileConfig().SRE.Chat {
		t.Error("db: the shared room changed without being saved")
	}
}

// Unset is db: everything saved on the tab is used, for every ladder --
// CRE's values too -- each in place of the file's; what is not saved keeps
// the file's.
func TestApplyCaseAlertSettings_UnsetUsesEverythingSaved(t *testing.T) {
	cfg := fileConfig()
	cfg.SME.Enabled = true
	saved := savedLadderValues()
	saved.Rooms = append(saved.Rooms, paging.CaseAlertRoom{Ladder: "SME", TeamKey: "asgardeo", Label: "Asgardeo SME (saved)", WebhookURL: savedHook})
	saved.Values = append(saved.Values, paging.CaseAlertValue{Ladder: "SME", Key: "onLeave", Value: json.RawMessage(`"call"`)})
	reader, env := &fakeCaseAlertReader{saved: saved}, envRecorder{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSettingsMode(""), cfg, reader, env.set, caseAlertStart{})

	if a := got.SRE.TeamChats["apollo"]; a.Audience != "Apollo SRE (saved)" || env[a.WebhookURLEnv] != savedHook {
		t.Fatalf("apollo = %+v", a)
	}
	if a := got.SME.TeamChats["asgardeo"]; a.Audience != "Asgardeo SME (saved)" || env[a.WebhookURLEnv] != savedHook {
		t.Fatalf("asgardeo = %+v", a)
	}
	if !reflect.DeepEqual(got.CRE.Teams.ABTs, []string{"atlas", "vega"}) || got.SME.OnLeave != paging.OnLeaveCall {
		t.Fatalf("CRE abts %v, SME onLeave %q", got.CRE.Teams.ABTs, got.SME.OnLeave)
	}
	if got.SRE.Chat != fileConfig().SRE.Chat {
		t.Error("the SRE shared room changed without being saved")
	}
	if reader.reported != "db" {
		t.Fatalf("reported %q", reader.reported)
	}
}

// Unset with nothing saved is the file as it is.
func TestApplyCaseAlertSettings_UnsetWithNothingSavedIsTheFile(t *testing.T) {
	reader, env := &fakeCaseAlertReader{saved: paging.CaseAlertSettings{Revision: 3}}, envRecorder{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSettingsMode(""), fileConfig(), reader, env.set, caseAlertStart{})
	if !reflect.DeepEqual(got, fileConfig()) || len(env) != 0 {
		t.Fatalf("got a change: env %v", env)
	}
}

// Whatever goes wrong, the file runs -- never the defaults, which page nobody.
func TestApplyCaseAlertSettings_FallsBackToTheFile(t *testing.T) {
	refused := paging.CaseAlertSettings{Routes: []paging.CaseAlertRoute{{Ladder: "SRE", GroupName: "apollo", RotaCode: "SRE_SAAS"}}}
	withABT := fileConfig()
	withABT.SRE.Teams.ABTs = []string{"apollo"}
	withABT.SRE.Teams.DefaultRota = "SRE_SAAS"
	cases := map[string]struct {
		cfg      paging.Config
		reader   caseAlertSettingsReader
		reported string
	}{
		"no entity-service":   {fileConfig(), nil, ""},
		"entity-service down": {fileConfig(), &fakeCaseAlertReader{err: errors.New("connection refused")}, ""},
		"refused by checks":   {withABT, &fakeCaseAlertReader{saved: refused}, "off"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env := envRecorder{}
			got := applyCaseAlertSettings(context.Background(), caseAlertSourceDB, c.cfg, c.reader, env.set, caseAlertStart{})
			if !reflect.DeepEqual(got, c.cfg) || len(env) != 0 {
				t.Fatalf("got a changed configuration (env %v)", env)
			}
			if f, ok := c.reader.(*fakeCaseAlertReader); ok && f.reported != c.reported {
				t.Fatalf("reported %q, want %q", f.reported, c.reported)
			}
		})
	}
}

// A room that cannot be put in the environment leaves the whole file in
// charge, not half of each.
func TestApplyCaseAlertSettings_SetenvFailureKeepsTheFile(t *testing.T) {
	reader := &fakeCaseAlertReader{saved: savedApollo()}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), reader,
		func(string, string) error { return errors.New("no") }, caseAlertStart{})
	if !reflect.DeepEqual(got, fileConfig()) || reader.reported != "off" {
		t.Fatalf("reported %q, changed=%v", reader.reported, !reflect.DeepEqual(got, fileConfig()))
	}
}

// memSnapshots is a snapshot store in memory.
type memSnapshots struct {
	b     []byte
	saves int
}

func (m *memSnapshots) Load(context.Context) ([]byte, error) { return m.b, nil }
func (m *memSnapshots) Save(_ context.Context, b []byte) error {
	m.b, m.saves = b, m.saves+1
	return nil
}

// noSleep counts the waits instead of waiting.
type noSleep struct{ waits []time.Duration }

func (n *noSleep) sleep(_ context.Context, d time.Duration) error {
	n.waits = append(n.waits, d)
	return nil
}

// savedLadderValues is an Apollo room plus two CRE ladder values and one SRE.
func savedLadderValues() paging.CaseAlertSettings {
	s := savedApollo()
	s.Values = []paging.CaseAlertValue{
		{Ladder: "CRE", Key: "teams.abts", Value: json.RawMessage(`["atlas","vega"]`)},
		{Ladder: "CRE", Key: "heads.cre", Value: json.RawMessage(`{"name":"CRE Head","email":"crehead@example.test"}`)},
		{Ladder: "SRE", Key: "timing.interval", Value: json.RawMessage(`"10m"`)},
	}
	return s
}

func runningOf(t *testing.T, summary map[string]any) map[string]map[string]map[string]any {
	t.Helper()
	b, _ := json.Marshal(summary["running"])
	var r struct {
		Ladders map[string]map[string]map[string]any `json:"ladders"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.Ladders == nil {
		t.Fatalf("no running values in the report: %s", b)
	}
	return r.Ladders
}

// db uses the saved ladder values and reports, key by key, what it runs with
// and which came from the tab.
func TestApplyCaseAlertSettings_DBUsesSavedValuesAndReportsThem(t *testing.T) {
	reader, env := &fakeCaseAlertReader{saved: savedLadderValues()}, envRecorder{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), reader, env.set, caseAlertStart{})
	if !reflect.DeepEqual(got.CRE.Teams.ABTs, []string{"atlas", "vega"}) || got.CRE.Heads.CRE.Email != "crehead@example.test" ||
		time.Duration(got.SRE.Timing.Interval) != 10*time.Minute {
		t.Fatalf("db: abts %v head %+v interval %v", got.CRE.Teams.ABTs, got.CRE.Heads.CRE, got.SRE.Timing.Interval)
	}
	running := runningOf(t, reader.summary)
	if v := running["CRE"]["teams.abts"]; v["source"] != "saved" {
		t.Errorf("teams.abts reported %v", v)
	}
	if v := running["SRE"]["onLeave"]; v["source"] != "file" || v["value"] != "skip" {
		t.Errorf("onLeave reported %v", v)
	}
	if head, _ := json.Marshal(running["CRE"]["heads.cre"]); strings.Contains(string(head), "phone") {
		t.Errorf("a head was reported with a phone: %s", head)
	}
	if b, _ := json.Marshal(reader.summary); strings.Contains(string(b), savedHook) {
		t.Error("the report carries a webhook URL")
	}
}

// shadow reports the file's values as running, so the tab can copy them.
func TestApplyCaseAlertSettings_ShadowReportsTheFilesValues(t *testing.T) {
	cfg := fileConfig()
	cfg.CRE.Teams.ABTs = []string{"castor"}
	reader := &fakeCaseAlertReader{saved: savedLadderValues()}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceShadow, cfg, reader, envRecorder{}.set, caseAlertStart{})
	if !reflect.DeepEqual(got, cfg) {
		t.Fatal("shadow changed the configuration")
	}
	running := runningOf(t, reader.summary)
	if v := running["CRE"]["teams.abts"]; v["source"] != "file" || !reflect.DeepEqual(v["value"], []any{"castor"}) {
		t.Fatalf("teams.abts reported %v", v)
	}
}

// A db start keeps trying while entity-service is down or failing, and stops
// trying on a refusal, which waiting does not change.
func TestApplyCaseAlertSettings_DBRetriesAnOutage(t *testing.T) {
	down := errors.New("connection refused")
	reader := &fakeCaseAlertReader{saved: savedApollo(), errs: []error{down, &apierror.Error{StatusCode: 503}}}
	waits := &noSleep{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), reader, envRecorder{}.set,
		caseAlertStart{wait: time.Minute, sleep: waits.sleep})
	if reader.reads != 3 || len(waits.waits) != 2 || got.SRE.TeamChats["apollo"].Audience != "Apollo SRE (saved)" {
		t.Fatalf("reads %d, waits %v, apollo %+v", reader.reads, waits.waits, got.SRE.TeamChats["apollo"])
	}

	refused := &fakeCaseAlertReader{saved: savedApollo(), errs: []error{&apierror.Error{StatusCode: 403}}}
	waits = &noSleep{}
	applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), refused, envRecorder{}.set,
		caseAlertStart{wait: time.Minute, sleep: waits.sleep})
	if refused.reads != 1 || len(waits.waits) != 0 {
		t.Fatalf("a refusal was tried %d times", refused.reads)
	}

	// Shadow never waits: it changes nothing anyway.
	shadow := &fakeCaseAlertReader{err: down}
	waits = &noSleep{}
	applyCaseAlertSettings(context.Background(), caseAlertSourceShadow, fileConfig(), shadow, envRecorder{}.set,
		caseAlertStart{wait: time.Minute, sleep: waits.sleep})
	if shadow.reads != 1 || len(waits.waits) != 0 {
		t.Fatalf("shadow read %d times", shadow.reads)
	}
}

// A db start keeps what it loaded, without the URLs; a later start that
// cannot read entity-service runs with it, posting to the file's rooms.
func TestApplyCaseAlertSettings_DBFallsBackToTheLastLoaded(t *testing.T) {
	store := &memSnapshots{}
	first := &fakeCaseAlertReader{saved: savedLadderValues()}
	applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), first, envRecorder{}.set, caseAlertStart{snapshots: store})
	if store.saves != 1 || strings.Contains(string(store.b), "chat.googleapis.com") || strings.Contains(string(store.b), "token=") {
		t.Fatalf("kept %d times: %s", store.saves, store.b)
	}

	down := &fakeCaseAlertReader{err: errors.New("connection refused")}
	env := envRecorder{}
	got := applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), down, env.set, caseAlertStart{snapshots: store})
	if !reflect.DeepEqual(got.CRE.Teams.ABTs, []string{"atlas", "vega"}) || time.Duration(got.SRE.Timing.Interval) != 10*time.Minute {
		t.Fatalf("from the last loaded: abts %v interval %v", got.CRE.Teams.ABTs, got.SRE.Timing.Interval)
	}
	if got.SRE.TeamChats["apollo"] != fileConfig().SRE.TeamChats["apollo"] || len(env) != 0 {
		t.Fatalf("a room without its URL replaced the file's: %+v, env %v", got.SRE.TeamChats["apollo"], env)
	}
	if down.reported != "" || store.saves != 1 {
		t.Fatalf("reported %q, kept %d times", down.reported, store.saves)
	}

	// Shadow never uses it.
	shadow := &fakeCaseAlertReader{err: errors.New("connection refused")}
	if got := applyCaseAlertSettings(context.Background(), caseAlertSourceShadow, fileConfig(), shadow, envRecorder{}.set, caseAlertStart{snapshots: store}); !reflect.DeepEqual(got, fileConfig()) {
		t.Fatal("shadow ran with the last loaded settings")
	}
	// With none kept, the file.
	if got := applyCaseAlertSettings(context.Background(), caseAlertSourceDB, fileConfig(), down, envRecorder{}.set, caseAlertStart{snapshots: &memSnapshots{}}); !reflect.DeepEqual(got, fileConfig()) {
		t.Fatal("no kept settings, but the file was not used")
	}
}

// The tries run to the end of the wait: the last pause is cut short to the
// deadline, not skipped.
func TestReadCaseAlertSettings_TriesUntilTheDeadline(t *testing.T) {
	down := errors.New("connection refused")
	reader := &fakeCaseAlertReader{err: down}
	var slept time.Duration
	clock := func(_ context.Context, d time.Duration) error { slept += d; time.Sleep(d); return nil }
	start := time.Now()
	_, err := readCaseAlertSettings(context.Background(), reader, 300*time.Millisecond, clock)
	if err == nil || time.Since(start) < 300*time.Millisecond || reader.reads < 2 {
		t.Fatalf("err %v after %v, %d reads, slept %v", err, time.Since(start), reader.reads, slept)
	}
}

func TestCaseAlertWait(t *testing.T) {
	for in, want := range map[string]time.Duration{"": 2 * time.Minute, "0": 0, "0s": 0, "45s": 45 * time.Second, "soon": 2 * time.Minute, "-1m": 2 * time.Minute} {
		if got := caseAlertWait(in); got != want {
			t.Errorf("caseAlertWait(%q) = %v, want %v", in, got, want)
		}
	}
}
