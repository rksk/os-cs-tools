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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/paging"
)

// The saved Case Alerts settings are re-read while the service runs
// (PAGING_CONFIG_REFRESH), so a save on the Alert Settings tab is live without
// a restart. entity-service offers no revision-only call, so each refresh is
// one GET of the settings; the cost is one small request per interval, and
// nothing on the paging path ever waits for it. A change is applied only when
// the same checks a start uses pass; otherwise the last good configuration
// keeps running and the reason is logged at error level.
const (
	defaultCaseAlertRefresh = 30 * time.Second
	minCaseAlertRefresh     = 5 * time.Second
)

// caseAlertRefreshInterval reads PAGING_CONFIG_REFRESH: unset is 30s, "0" (or
// "off") turns the refresh off, anything shorter than 5s is 5s.
func caseAlertRefreshInterval(raw string) time.Duration {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "":
		return defaultCaseAlertRefresh
	case "0", "off":
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		slog.Warn("PAGING_CONFIG_REFRESH is not a duration; using the default", "value", raw, "default", defaultCaseAlertRefresh)
		return defaultCaseAlertRefresh
	}
	if d == 0 {
		return 0
	}
	if d < minCaseAlertRefresh {
		slog.Warn("PAGING_CONFIG_REFRESH is below the minimum; using the minimum", "value", raw, "minimum", minCaseAlertRefresh)
		return minCaseAlertRefresh
	}
	return d
}

// caseAlertRooms holds the saved rooms' webhook URLs a configuration needs,
// looked up before the process environment. Used only while starting; a
// refresh gives each new configuration its own map.
type caseAlertRooms struct {
	mu sync.RWMutex
	m  map[string]string
}

func newCaseAlertRooms() *caseAlertRooms { return &caseAlertRooms{m: map[string]string{}} }

func (r *caseAlertRooms) set(name, url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name] = url
	return nil
}

func (r *caseAlertRooms) snapshot() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.m))
	for k, v := range r.m {
		out[k] = v
	}
	return out
}

func (r *caseAlertRooms) get(name string) string {
	r.mu.RLock()
	v, ok := r.m[name]
	r.mu.RUnlock()
	if ok {
		return v
	}
	return os.Getenv(name)
}

// caseAlertEffective is the configuration the ladders run with, as one
// immutable value.
type caseAlertEffective struct {
	Config paging.Config
	// Env holds the saved rooms' webhook URLs; never logged.
	Env map[string]string
	// Revision is the saved settings' revision this runs with; 0 when it is the file's.
	Revision int64
}

// caseAlertLive keeps the case ladders' configuration current. The ladders
// themselves are swapped by rebuild (paging.LiveEngine); this holds the
// effective configuration and drives the refresh.
type caseAlertLive struct {
	mode      string
	base      paging.Config // what the file and env give, before any saved setting
	entity    caseAlertSettingsReader
	snapshots caseAlertSnapshots
	interval  time.Duration

	// rebuild checks cfg the way a start does and prepares the ladders for it;
	// the returned commit swaps them in. An error leaves everything as it was.
	rebuild func(cfg paging.Config, getenv func(string) string) (commit func(), err error)

	// sleep waits between refreshes; jitter returns a fraction in [0,1).
	// Both are replaced in tests.
	sleep  func(context.Context, time.Duration) error
	jitter func() float64

	effective atomic.Pointer[caseAlertEffective]

	// Owned by the refresh goroutine alone.
	seen      bool
	seenPrint string
}

// Effective is the configuration in use now. An atomic load: no I/O.
func (l *caseAlertLive) Effective() *caseAlertEffective { return l.effective.Load() }

// startFrom records what the start settled on. A start that read the saved
// settings from entity-service itself is not repeated by the first refresh.
func (l *caseAlertLive) startFrom(cfg paging.Config, env map[string]string, res caseAlertStartResult) {
	e := &caseAlertEffective{Config: cfg, Env: env}
	if res.Applied {
		e.Revision = res.Revision
	}
	l.effective.Store(e)
	if res.Settled {
		l.seen, l.seenPrint = true, res.Fingerprint
	}
}

// delay is one wait between refreshes: the interval plus up to a fifth, so
// replicas do not read in step.
func (l *caseAlertLive) delay() time.Duration {
	j := rand.Float64
	if l.jitter != nil {
		j = l.jitter
	}
	return l.interval + time.Duration(float64(l.interval)*0.2*j())
}

// run refreshes until ctx is done.
func (l *caseAlertLive) run(ctx context.Context) {
	if l.mode == caseAlertSourceOff {
		return
	}
	sleep := l.sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	slog.Info("case alerts: saved settings are re-read while running", "mode", l.mode, "every", l.interval)
	for {
		if err := sleep(ctx, l.delay()); err != nil {
			return
		}
		l.refresh(ctx)
	}
}

// caseAlertFingerprint identifies saved settings by content, not revision
// alone, so a change a revision bump missed is still seen. Never logged.
func caseAlertFingerprint(s paging.CaseAlertSettings) string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// refresh reads the saved settings once and, when they differ from the last
// ones seen, applies them. It never panics the process and never changes what
// is paged on any failure.
func (l *caseAlertLive) refresh(ctx context.Context) {
	if l.mode == caseAlertSourceOff {
		return // off asks nobody
	}
	running := l.effective.Load()
	readCtx, cancel := context.WithTimeout(ctx, caseAlertReadTimeout)
	saved, err := l.entity.CaseAlertSettings(readCtx)
	cancel()
	if err != nil {
		slog.Error("case alerts refresh: could not read the saved settings; the last good ones keep running",
			"mode", l.mode, "runningRevision", running.Revision, "err", err)
		return
	}
	fp := caseAlertFingerprint(saved)
	if l.seen && fp == l.seenPrint {
		return
	}
	// Whatever happens next, this content is dealt with once: a refused one
	// is not logged again every interval.
	l.seen, l.seenPrint = true, fp

	merged, env, changes, mergeErr := paging.MergeCaseAlertSettings(l.base, saved)
	summary := map[string]any{"rooms": len(saved.Rooms), "routes": len(saved.Routes), "values": len(saved.Values), "changes": changes}
	getenv := overlayGetenv(env)

	// refused keeps the last good configuration and says why, to the log and
	// to the tab.
	refused := func(reason string, err error) {
		summary["error"] = err.Error()
		slog.Error("case alerts refresh: "+reason+"; the last good settings keep running",
			"mode", l.mode, "revision", saved.Revision, "runningRevision", running.Revision, "err", err)
		if l.mode == caseAlertSourceDB && running.Revision != 0 {
			summary["rejectedRevision"] = saved.Revision
			l.report(ctx, running.Revision, caseAlertSourceDB, summary)
			return
		}
		l.report(ctx, saved.Revision, caseAlertSourceOff, summary)
	}
	if mergeErr != nil {
		refused("the saved settings would not pass the file's checks", mergeErr)
		return
	}

	if l.mode == caseAlertSourceShadow {
		for _, c := range changes {
			slog.Info("case alerts shadow: the saved settings would change", "change", c, "revision", saved.Revision)
		}
		slog.Info("case alerts shadow: compared the saved settings with the file; the file's are used",
			"revision", saved.Revision, "differences", len(changes))
		l.report(ctx, saved.Revision, caseAlertSourceShadow, summary)
		return
	}

	commit, err := l.rebuild(merged, getenv)
	if err != nil {
		refused("the saved settings cannot be put to use", err)
		return
	}
	commit()
	l.effective.Store(&caseAlertEffective{Config: merged, Env: env, Revision: saved.Revision})
	for _, c := range changes {
		slog.Info("case alerts: using a saved setting", "change", c, "revision", saved.Revision)
	}
	slog.Info("case alerts: now using the saved settings over the file's, without a restart",
		"revision", saved.Revision, "previousRevision", running.Revision, "values", len(saved.Values), "changes", len(changes))
	summary["running"] = paging.RunningCaseAlertValuesEnv(merged, paging.SavedCaseAlertKeys(saved), getenv)
	l.report(ctx, saved.Revision, caseAlertSourceDB, summary)
	if l.snapshots != nil {
		saveCaseAlertSnapshot(ctx, l.snapshots, saved)
	}
}

func (l *caseAlertLive) report(ctx context.Context, revision int64, mode string, summary map[string]any) {
	reportCtx, cancel := context.WithTimeout(ctx, caseAlertReadTimeout)
	defer cancel()
	if err := l.entity.ReportCaseAlertSettingsApplied(reportCtx, revision, mode, summary); err != nil {
		slog.Warn("could not report the Case Alerts settings this process loaded", "err", err)
	}
}
