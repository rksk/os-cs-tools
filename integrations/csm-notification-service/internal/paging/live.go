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
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/eventbus"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/events"
)

// LiveEngine is one ladder's engine behind an atomically swappable pointer, so
// the settings saved on the Alert Settings tab can replace the engine's
// configuration while the process keeps running. Consumers and the ticker are
// given the LiveEngine instead of an *Engine; every call loads the current
// engine from memory (one atomic load, no I/O, no allocation) and delegates.
//
// An Engine is immutable once built and keeps nothing a ladder needs in
// process (ladder state is in the store), so swapping is just a pointer store.
type LiveEngine struct {
	cur atomic.Pointer[Engine]
}

// NewLiveEngine returns a LiveEngine serving e.
func NewLiveEngine(e *Engine) *LiveEngine {
	l := &LiveEngine{}
	l.cur.Store(e)
	return l
}

// Current is the engine serving now.
func (l *LiveEngine) Current() *Engine { return l.cur.Load() }

// Swap makes e the engine that serves from the next call on. A call already
// running finishes on the engine it started with.
func (l *LiveEngine) Swap(e *Engine) { l.cur.Store(e) }

// Handle implements eventbus.Handle.
func (l *LiveEngine) Handle(ctx context.Context, record eventbus.Record) error {
	return l.cur.Load().Handle(ctx, record)
}

// Tick places whatever is due on the current engine.
func (l *LiveEngine) Tick(ctx context.Context, now time.Time) error {
	return l.cur.Load().Tick(ctx, now)
}

// HandleSpecialOpsAlert places the Special Ops page on the current engine.
func (l *LiveEngine) HandleSpecialOpsAlert(ctx context.Context, incidentID string, p events.IncidentSpecialOpsAlertPayload) error {
	return l.cur.Load().HandleSpecialOpsAlert(ctx, incidentID, p)
}

// RunTicker is Engine.RunTicker on whichever engine is current at each tick.
func (l *LiveEngine) RunTicker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := l.Tick(ctx, time.Now()); err != nil {
				slog.ErrorContext(ctx, "escalation: tick failed", "err", err)
			}
		}
	}
}

// LadderConfig is the ladder configuration this engine runs with.
func (e *Engine) LadderConfig() LadderConfig { return e.cfg.Ladder }
