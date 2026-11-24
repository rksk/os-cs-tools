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
	"sync"
	"testing"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/paging"
)

const (
	hookOne = "https://chat.example.test/v1/spaces/ONE/messages?key=k&token=t"
	hookTwo = "https://chat.example.test/v1/spaces/TWO/messages?key=k&token=t"
)

// swapFixture is the engines and the incident card routing a refresh swaps.
type swapFixture struct {
	chat   *notifications.GoogleChatClient
	routes *notifications.IncidentRoutes
	live   map[paging.Ladder]*paging.LiveEngine
	swap   caseAlertSwap
}

func routedConfig() paging.Config {
	cfg := fileConfig()
	cfg.Enabled, cfg.SRE.Enabled = true, true
	cfg.SRE.Teams.ABTs = []string{"apollo"}
	cfg.SRE.Teams.Aliases = map[string]string{"apollo ops": "apollo"}
	cfg.SRE.TeamChats = map[string]paging.Chat{"apollo": {Audience: "Apollo SRE", WebhookURLEnv: "TEST_APOLLO_HOOK"}}
	return cfg
}

func newSwapFixture(t *testing.T, teamSchedule bool) *swapFixture {
	t.Helper()
	f := &swapFixture{
		chat:   notifications.NewGoogleChatClient(notifications.GoogleChatConfig{}),
		routes: &notifications.IncidentRoutes{},
	}
	build := func(cfg paging.Config, kind paging.Ladder, l paging.LadderConfig, _ paging.Resolver, getenv func(string) string) *paging.Engine {
		return paging.NewEngine(paging.DefaultPolicy, nil, nil, nil, paging.PortalLinks{}, nil, nil, "",
			paging.EngineConfig{Ladder: l, Kind: kind, Channel: paging.ChannelLog, Getenv: getenv})
	}
	cfg := routedConfig()
	f.live = map[paging.Ladder]*paging.LiveEngine{
		paging.LadderSRE: paging.NewLiveEngine(build(cfg, paging.LadderSRE, cfg.SRE, nil, nil)),
	}
	f.swap = caseAlertSwap{
		ladders: []caseAlertSwapLadder{{kind: paging.LadderSRE, name: "escalation-sre"}},
		live:    f.live, usingTeamSchedule: teamSchedule, chat: f.chat, routes: f.routes,
		makeResolver: func(_, _ paging.LadderConfig) paging.Resolver { return nil },
		buildEngine:  build,
	}
	return f
}

func (f *swapFixture) apply(t *testing.T, cfg paging.Config, env map[string]string) error {
	t.Helper()
	commit, err := f.swap.prepare(cfg, overlayGetenv(env))
	if err != nil {
		return err
	}
	commit()
	return nil
}

func (f *swapFixture) room() (string, bool) {
	return f.chat.AudienceWebhookURL(notifications.IncidentAudience("apollo ops"))
}

// A saved room or group change reaches incident card routing, in the same step
// as the engine, with no restart.
func TestCaseAlertSwap_SavedRoomReachesIncidentCardRouting(t *testing.T) {
	f := newSwapFixture(t, false)
	if _, ok := f.room(); ok {
		t.Fatal("setup: a room before any refresh")
	}
	before := f.live[paging.LadderSRE].Current()

	if err := f.apply(t, routedConfig(), map[string]string{"TEST_APOLLO_HOOK": hookOne}); err != nil {
		t.Fatal(err)
	}
	if u, ok := f.room(); !ok || u != hookOne {
		t.Fatalf("incident card room = %q, %v; want the saved one", u, ok)
	}
	if f.live[paging.LadderSRE].Current() == before {
		t.Fatal("the engine was not swapped with the routes")
	}

	// A changed room URL, then a changed group route, each without a restart.
	if err := f.apply(t, routedConfig(), map[string]string{"TEST_APOLLO_HOOK": hookTwo}); err != nil {
		t.Fatal(err)
	}
	if u, _ := f.room(); u != hookTwo {
		t.Fatalf("room after a changed URL = %q, want the new one", u)
	}
	cfg := routedConfig()
	cfg.SRE.Teams.Aliases["apollo night"] = "apollo"
	if err := f.apply(t, cfg, map[string]string{"TEST_APOLLO_HOOK": hookTwo}); err != nil {
		t.Fatal(err)
	}
	if u, ok := f.chat.AudienceWebhookURL(notifications.IncidentAudience("apollo night")); !ok || u != hookTwo {
		t.Fatalf("a newly routed group has room %q, %v", u, ok)
	}
}

// A room removed from the settings stops receiving.
func TestCaseAlertSwap_RemovedRoomStopsReceiving(t *testing.T) {
	f := newSwapFixture(t, false)
	if err := f.apply(t, routedConfig(), map[string]string{"TEST_APOLLO_HOOK": hookOne}); err != nil {
		t.Fatal(err)
	}
	cfg := routedConfig()
	cfg.SRE.TeamChats = map[string]paging.Chat{}
	if err := f.apply(t, cfg, nil); err != nil {
		t.Fatal(err)
	}
	if u, ok := f.room(); ok {
		t.Fatalf("a removed room still receives: %q", u)
	}
}

// A refresh that cannot be built keeps the old routes and the old engines.
func TestCaseAlertSwap_FailureKeepsRoutesAndEnginesTogether(t *testing.T) {
	for name, mutate := range map[string]func(*swapFixture, *paging.Config){
		"validation": func(f *swapFixture, c *paging.Config) {
			f.swap.usingTeamSchedule = true
			c.CRE.Teams.ABTs, c.CRE.Teams.ABTType = nil, ""
		},
		"routes cannot be built": func(f *swapFixture, _ *paging.Config) { f.swap.chat = nil },
		"ladder would stop":      func(_ *swapFixture, c *paging.Config) { c.SRE.Enabled = false },
	} {
		t.Run(name, func(t *testing.T) {
			f := newSwapFixture(t, false)
			if err := f.apply(t, routedConfig(), map[string]string{"TEST_APOLLO_HOOK": hookOne}); err != nil {
				t.Fatal(err)
			}
			engine := f.live[paging.LadderSRE].Current()
			cfg := routedConfig()
			cfg.SRE.TeamChats["apollo"] = paging.Chat{Audience: "Apollo SRE", WebhookURLEnv: "TEST_APOLLO_HOOK"}
			mutate(f, &cfg)
			if err := f.apply(t, cfg, map[string]string{"TEST_APOLLO_HOOK": hookTwo}); err == nil {
				t.Fatal("expected a refusal")
			}
			if f.live[paging.LadderSRE].Current() != engine {
				t.Fatal("the engine changed on a refused refresh")
			}
			if u, _ := f.room(); u != hookOne {
				t.Fatalf("routes changed on a refused refresh: %q", u)
			}
		})
	}
}

// Through the refresh loop: routes that cannot be rebuilt refuse the refresh,
// which is reported as refused and never as applied.
func TestCaseAlertLive_RouteFailureIsRefusedAndReported(t *testing.T) {
	f := newSwapFixture(t, false)
	f.swap.chat = nil
	r := &liveReader{saved: savedLadderValues()}
	l := &caseAlertLive{mode: caseAlertSourceDB, base: fileConfig(), entity: r, interval: 1, rebuild: f.swap.prepare}
	l.startFrom(fileConfig(), nil, caseAlertStartResult{})
	before := f.live[paging.LadderSRE].Current()
	l.refresh(context.Background())
	rep := r.lastReport()
	if rep.mode == "db" || rep.summary["error"] == nil || f.live[paging.LadderSRE].Current() != before || l.Effective().Revision != 0 {
		t.Fatalf("report %+v: a stale-routes refresh was applied or reported applied", rep)
	}
}

// Readers of the routing (the card path) race with refreshes that replace it.
func TestCaseAlertSwap_ConcurrentCardReadsAreRaceFree(t *testing.T) {
	f := newSwapFixture(t, false)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	has := func(string) bool { return true }
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					f.room()
					notifications.IncidentAudiences(f.routes, nil, "apollo ops", has)
					notifications.CaseSREAudiences(f.routes, nil, "apollo ops", has)
					f.routes.SRETeams("apollo ops")
					f.live[paging.LadderSRE].Current()
				}
			}
		}()
	}
	for i := 0; i < 300; i++ {
		url := hookOne
		if i%2 == 1 {
			url = hookTwo
		}
		if err := f.apply(t, routedConfig(), map[string]string{"TEST_APOLLO_HOOK": url}); err != nil {
			t.Error(err)
			break
		}
	}
	close(stop)
	wg.Wait()
}
