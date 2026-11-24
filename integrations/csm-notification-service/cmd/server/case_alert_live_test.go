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
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/paging"
)

// liveReader is a settings reader whose answer a test changes between
// refreshes. Reads and reports are counted.
type liveReader struct {
	mu       sync.Mutex
	saved    paging.CaseAlertSettings
	err      error
	reads    int
	reports  []liveReport
	reportFn func()
}

type liveReport struct {
	revision int64
	mode     string
	summary  map[string]any
}

func (r *liveReader) set(s paging.CaseAlertSettings, err error) {
	r.mu.Lock()
	r.saved, r.err = s, err
	r.mu.Unlock()
}

func (r *liveReader) CaseAlertSettings(context.Context) (paging.CaseAlertSettings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	return r.saved, r.err
}

func (r *liveReader) ReportCaseAlertSettingsApplied(_ context.Context, revision int64, mode string, summary map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reports = append(r.reports, liveReport{revision, mode, summary})
	return nil
}

func (r *liveReader) lastReport() liveReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.reports) == 0 {
		return liveReport{}
	}
	return r.reports[len(r.reports)-1]
}

// builds counts what rebuild was asked to prepare and commit.
type builds struct {
	asked, committed int
	last             paging.Config
	err              error
}

func (b *builds) rebuild(cfg paging.Config, _ func(string) string) (func(), error) {
	b.asked++
	if b.err != nil {
		return nil, b.err
	}
	return func() { b.committed++; b.last = cfg }, nil
}

func newLive(mode string, r *liveReader, b *builds, interval time.Duration) *caseAlertLive {
	l := &caseAlertLive{mode: mode, base: fileConfig(), entity: r, interval: interval, rebuild: b.rebuild,
		jitter: func() float64 { return 0.5 }}
	l.startFrom(fileConfig(), map[string]string{}, caseAlertStartResult{})
	return l
}

func intervalOf(c paging.Config) time.Duration { return time.Duration(c.SRE.Timing.Interval) }

// A change saved while the service runs is applied by the next refresh, which
// is one interval (plus jitter) after the last: no restart.
func TestCaseAlertLive_ChangeIsPickedUpWithinOneInterval(t *testing.T) {
	r, b := &liveReader{saved: paging.CaseAlertSettings{Revision: 3}}, &builds{}
	l := newLive(caseAlertSourceDB, r, b, 30*time.Second)
	// Nothing saved at start: the file's configuration is running, and the
	// start has already dealt with this revision.
	l.startFrom(fileConfig(), map[string]string{}, caseAlertStartResult{Settled: true, Revision: 3, Fingerprint: caseAlertFingerprint(r.saved)})
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	l.sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		switch len(waits) {
		case 2: // saved between the first refresh and the second
			r.set(savedLadderValues(), nil)
		case 3:
			cancel()
			return ctx.Err()
		}
		return nil
	}
	l.run(ctx)

	if len(waits) != 3 || waits[0] != 33*time.Second { // 30s + 20% * 0.5
		t.Fatalf("waits %v, want 3 waits of interval plus jitter", waits)
	}
	if b.asked != 1 || b.committed != 1 {
		t.Fatalf("rebuild asked %d committed %d, want 1 and 1: the first refresh saw nothing new", b.asked, b.committed)
	}
	if got := intervalOf(l.Effective().Config); got != 10*time.Minute {
		t.Fatalf("effective SRE interval %v, want the saved 10m", got)
	}
	if l.Effective().Revision != 9 || r.lastReport().revision != 9 || r.lastReport().mode != "db" {
		t.Fatalf("revision %d, reported %+v", l.Effective().Revision, r.lastReport())
	}
	if b.last.CRE.Teams.ABTs[0] != "atlas" {
		t.Fatalf("committed config has abts %v", b.last.CRE.Teams.ABTs)
	}
}

// Content the file's checks refuse never replaces the last good settings.
func TestCaseAlertLive_InvalidSettingsKeepLastGood(t *testing.T) {
	r, b := &liveReader{saved: savedLadderValues()}, &builds{}
	l := newLive(caseAlertSourceDB, r, b, time.Minute)
	l.refresh(context.Background())
	good := l.Effective()
	if good.Revision != 9 || b.committed != 1 {
		t.Fatalf("setup: revision %d committed %d", good.Revision, b.committed)
	}

	bad := savedLadderValues()
	bad.Revision = 10
	bad.Rooms = append(bad.Rooms, paging.CaseAlertRoom{Ladder: "SRE", TeamKey: "apollo", Channel: "carrier-pigeon"})
	r.set(bad, nil)
	l.refresh(context.Background())
	l.refresh(context.Background()) // the same refused content is not dealt with twice

	if l.Effective() != good || b.committed != 1 || b.asked != 1 {
		t.Fatalf("a refused setting changed what runs: committed %d asked %d", b.committed, b.asked)
	}
	rep := r.lastReport()
	if rep.revision != 9 || rep.mode != "db" || rep.summary["error"] == nil || rep.summary["rejectedRevision"] != int64(10) {
		t.Fatalf("report %+v: want the running revision with the refusal named", rep)
	}

	// A fixed save is picked up.
	bad.Revision, bad.Rooms = 11, bad.Rooms[:1]
	r.set(bad, nil)
	l.refresh(context.Background())
	if l.Effective().Revision != 11 || b.committed != 2 {
		t.Fatalf("a good save after a refused one was not applied: revision %d", l.Effective().Revision)
	}
}

// A setting the ladders cannot be built for is refused the same way.
func TestCaseAlertLive_RebuildFailureKeepsLastGood(t *testing.T) {
	r, b := &liveReader{saved: savedLadderValues()}, &builds{err: errors.New("it would turn off a running ladder")}
	l := newLive(caseAlertSourceDB, r, b, time.Minute)
	before := l.Effective()
	l.refresh(context.Background())
	if l.Effective() != before || b.committed != 0 {
		t.Fatal("a failed rebuild changed what runs")
	}
	if rep := r.lastReport(); rep.mode != "off" || rep.summary["error"] == nil {
		t.Fatalf("report %+v: the file's configuration is running, so off, with the error", rep)
	}
}

// entity-service being down changes nothing, and the refresh that follows
// its return applies what was saved meanwhile.
func TestCaseAlertLive_EntityDownKeepsLastGood(t *testing.T) {
	r, b := &liveReader{saved: savedLadderValues()}, &builds{}
	l := newLive(caseAlertSourceDB, r, b, time.Minute)
	l.refresh(context.Background())
	good := l.Effective()

	r.set(paging.CaseAlertSettings{}, errors.New("connection refused"))
	l.refresh(context.Background())
	l.refresh(context.Background())
	if l.Effective() != good || b.asked != 1 {
		t.Fatal("entity-service being down changed what runs")
	}

	next := savedLadderValues()
	next.Revision = 12
	r.set(next, nil)
	l.refresh(context.Background())
	if l.Effective().Revision != 12 {
		t.Fatalf("revision %d after entity-service returned, want 12", l.Effective().Revision)
	}
}

// A start that already applied a revision does not apply it again; one that
// could not read entity-service applies whatever the first refresh reads.
func TestCaseAlertLive_StartSettledIsNotReapplied(t *testing.T) {
	saved := savedLadderValues()
	r, b := &liveReader{saved: saved}, &builds{}
	l := &caseAlertLive{mode: caseAlertSourceDB, base: fileConfig(), entity: r, interval: time.Minute, rebuild: b.rebuild}
	l.startFrom(fileConfig(), nil, caseAlertStartResult{Settled: true, Applied: true, Revision: 9, Fingerprint: caseAlertFingerprint(saved)})
	l.refresh(context.Background())
	if b.asked != 0 {
		t.Fatal("the revision the start applied was applied again")
	}
	l.startFrom(fileConfig(), nil, caseAlertStartResult{})
	l.seen = false
	l.refresh(context.Background())
	if b.committed != 1 {
		t.Fatal("a start that could not read entity-service did not get the settings from the first refresh")
	}
}

// Shadow compares and reports on a refresh, and changes nothing.
func TestCaseAlertLive_ShadowDoesNotChangeWhatRuns(t *testing.T) {
	r, b := &liveReader{saved: savedLadderValues()}, &builds{}
	l := newLive(caseAlertSourceShadow, r, b, time.Minute)
	before := l.Effective()
	l.refresh(context.Background())
	if l.Effective() != before || b.asked != 0 {
		t.Fatal("shadow changed what runs")
	}
	if rep := r.lastReport(); rep.mode != "shadow" || rep.revision != 9 {
		t.Fatalf("report %+v", rep)
	}
}

// Off asks nobody.
func TestCaseAlertLive_OffAsksNobody(t *testing.T) {
	r, b := &liveReader{saved: savedLadderValues()}, &builds{}
	l := newLive(caseAlertSourceOff, r, b, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	l.sleep = func(context.Context, time.Duration) error { cancel(); return ctx.Err() }
	l.run(ctx)
	l.refresh(ctx)
	if r.reads != 0 || b.asked != 0 || len(r.reports) != 0 {
		t.Fatalf("off: reads %d builds %d reports %d", r.reads, b.asked, len(r.reports))
	}
}

func TestCaseAlertRefreshInterval(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": 30 * time.Second, "45s": 45 * time.Second, "0": 0, "off": 0, "1s": 5 * time.Second,
		"soon": 30 * time.Second, "-1m": 30 * time.Second,
	} {
		if got := caseAlertRefreshInterval(in); got != want {
			t.Errorf("caseAlertRefreshInterval(%q) = %v, want %v", in, got, want)
		}
	}
}

// Reads of the effective configuration race with refreshes that replace it.
func TestCaseAlertLive_ConcurrentReadsAreRaceFree(t *testing.T) {
	r, b := &liveReader{}, &builds{}
	l := newLive(caseAlertSourceDB, r, b, time.Minute)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if e := l.Effective(); e == nil || e.Config.SRE.Timing.Interval < 0 {
						t.Error("bad effective configuration")
						return
					}
				}
			}
		}()
	}
	for rev := int64(1); rev <= 200; rev++ {
		s := savedLadderValues()
		s.Revision = rev
		s.Values[2].Value = json.RawMessage(fmt.Sprintf(`"%dm"`, 5+rev%20))
		r.set(s, nil)
		l.refresh(context.Background())
	}
	close(stop)
	wg.Wait()
	if l.Effective().Revision != 200 {
		t.Fatalf("revision %d", l.Effective().Revision)
	}
}

// Integration: a real entity client against an HTTP stub of entity-service,
// and a real engine behind a LiveEngine. Saving a changed ladder changes what
// the next ladder uses, with no new service constructed and no restart.
func TestCaseAlertLive_RefreshSwapsTheEngineAgainstAnEntityServiceStub(t *testing.T) {
	var mu sync.Mutex
	saved := paging.CaseAlertSettings{Revision: 1}
	var reports []map[string]any
	var settingsGets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
		case req.Method == http.MethodGet && req.URL.Path == "/team-schedule/case-alerts/settings/runtime":
			settingsGets.Add(1)
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(saved)
		case req.Method == http.MethodPut && req.URL.Path == "/team-schedule/case-alerts/settings/applied":
			var body map[string]any
			_ = json.NewDecoder(req.Body).Decode(&body)
			mu.Lock()
			reports = append(reports, body)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, req)
		}
	}))
	defer srv.Close()
	entity := paging.NewEntityClient(paging.EntityConfig{BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "test", ClientSecret: "test"})

	build := func(cfg paging.Config, getenv func(string) string) *paging.Engine {
		return paging.NewEngine(paging.DefaultPolicy, nil, nil, nil, paging.PortalLinks{}, nil, nil, "",
			paging.EngineConfig{Ladder: cfg.SRE, Kind: paging.LadderSRE, Channel: paging.ChannelLog, Getenv: getenv})
	}
	file := fileConfig()
	live := paging.NewLiveEngine(build(file, nil))
	service := live // the one service: nothing below constructs another

	l := &caseAlertLive{mode: caseAlertSourceDB, base: file, entity: entity, interval: time.Minute,
		rebuild: func(cfg paging.Config, getenv func(string) string) (func(), error) {
			e := build(cfg, getenv)
			return func() { live.Swap(e) }, nil
		}}
	l.startFrom(file, map[string]string{}, caseAlertStartResult{Settled: true, Revision: 1, Fingerprint: caseAlertFingerprint(saved)})

	if got := time.Duration(service.Current().LadderConfig().Timing.Interval); got != time.Duration(file.SRE.Timing.Interval) {
		t.Fatalf("before: interval %v", got)
	}
	l.refresh(context.Background()) // revision 1, nothing saved: the file stays
	if time.Duration(service.Current().LadderConfig().Timing.Interval) != time.Duration(file.SRE.Timing.Interval) {
		t.Fatal("an empty save changed the ladder")
	}

	mu.Lock()
	saved = savedLadderValues() // timing.interval 10m, saved room for Apollo
	mu.Unlock()
	l.refresh(context.Background())

	if service != live || service.Current().LadderConfig().Timing.Interval == file.SRE.Timing.Interval ||
		time.Duration(service.Current().LadderConfig().Timing.Interval) != 10*time.Minute {
		t.Fatalf("after: the next ladder uses interval %v, want the saved 10m", service.Current().LadderConfig().Timing.Interval)
	}
	if got := l.Effective().Env["CASE_ALERTS_ROOM_SRE_APOLLO"]; got != savedHook {
		t.Fatal("the saved room's URL did not reach the new configuration's lookup")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reports) == 0 || reports[len(reports)-1]["mode"] != "db" || reports[len(reports)-1]["revision"] != float64(9) {
		t.Fatalf("applied reports %v, want the last to be db at revision 9", reports)
	}
	if settingsGets.Load() != 2 {
		t.Fatalf("%d reads of the settings, want one per refresh", settingsGets.Load())
	}
}
