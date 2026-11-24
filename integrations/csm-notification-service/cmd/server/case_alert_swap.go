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
	"errors"
	"fmt"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/paging"
)

type caseAlertSwapLadder struct {
	kind paging.Ladder
	name string
}

// caseAlertSwap is what a refresh needs to put a new configuration in use: the
// running ladders' engines, and the incident card routing that follows the
// same saved rooms and routes.
type caseAlertSwap struct {
	ladders           []caseAlertSwapLadder
	live              map[paging.Ladder]*paging.LiveEngine
	usingTeamSchedule bool
	chat              *notifications.GoogleChatClient
	routes            *notifications.IncidentRoutes
	makeResolver      func(cre, sre paging.LadderConfig) paging.Resolver
	buildEngine       func(cfg paging.Config, kind paging.Ladder, l paging.LadderConfig, r paging.Resolver, getenv func(string) string) *paging.Engine
}

// prepare checks cfg the way a start does and builds everything it needs --
// the engines and the incident route table -- without changing anything. The
// returned commit applies the routes and swaps the engines in one step; an
// error means nothing at all changes.
func (d caseAlertSwap) prepare(cfg paging.Config, getenv func(string) string) (func(), error) {
	creNow, creOn := cfg.For(paging.LadderKeyCRE)
	sreNow, sreOn := cfg.For(paging.LadderKeySRE)
	var running []caseAlertSwapLadder
	for _, l := range d.ladders {
		if _, started := d.live[l.kind]; !started {
			continue
		}
		if (l.kind == paging.LadderCRE && !creOn) || (l.kind == paging.LadderSRE && !sreOn) {
			return nil, fmt.Errorf("it would turn off the %s ladder, which is running; that takes a restart", l.name)
		}
		running = append(running, l)
	}
	// The same check a start makes: a team-schedule resolver with no teams
	// resolves almost nobody.
	if d.usingTeamSchedule && len(creNow.Teams.ABTs) == 0 && creNow.Teams.ABTType == "" {
		return nil, errors.New("teams.abtType or teams.abts is needed with the team-schedule resolver")
	}
	// Incident cards and the replies under them follow the same saved rooms
	// and routes as the engines. If they cannot be built, nothing changes.
	applyRoutes, err := prepareIncidentRoutes(sreNow, d.chat, d.routes, getenv)
	if err != nil {
		return nil, fmt.Errorf("incident card routing: %w", err)
	}
	resolver := d.makeResolver(creNow, sreNow)
	engines := map[paging.Ladder]*paging.Engine{}
	for _, l := range running {
		lc := creNow
		if l.kind == paging.LadderSRE {
			lc = sreNow
		}
		engines[l.kind] = d.buildEngine(cfg, l.kind, lc, resolver, getenv)
	}
	return func() {
		applyRoutes()
		for kind, e := range engines {
			d.live[kind].Swap(e)
		}
	}, nil
}
