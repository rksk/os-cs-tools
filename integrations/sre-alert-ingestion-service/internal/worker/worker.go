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

// Package worker is the background retry/escalation loop: it periodically
// scans internal/store's alert_buffer for rows due for another delivery
// attempt against csm-integration-service, and escalates via Twilio once a
// row's retry budget is exhausted. This is the piece that makes buffering
// meaningful — without it, a persisted-but-undelivered alert would sit in
// Postgres forever.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/backoff"
	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/csmclient"
	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/middleware"
	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/store"
)

// Store is the subset of internal/store.Store the worker depends on.
type Store interface {
	PendingBatch(ctx context.Context, limit int) ([]store.AlertRecord, error)
	MarkDelivered(ctx context.Context, id, incidentID string) error
	MarkAttemptFailed(ctx context.Context, id, lastError string) error
	MarkEscalated(ctx context.Context, id, lastError string) error
	MarkFailed(ctx context.Context, id, lastError string) error
}

// IncidentCreator is the subset of internal/csmclient.Client the worker
// depends on.
type IncidentCreator interface {
	CreateIncident(ctx context.Context, req csmclient.CreateIncidentRequest) (*csmclient.CreateIncidentResult, error)
}

// Escalator is the subset of internal/notifications.TwilioClient the worker
// depends on.
type Escalator interface {
	Escalate(ctx context.Context, message string) error
}

// Config tunes the worker's polling and retry behavior.
type Config struct {
	// MaxRetries is the number of failed, retryable attempts a buffered
	// alert gets before the Twilio escalation call fires and the row is
	// marked escalated. Defaults to 5 if <= 0.
	MaxRetries int
	// BatchSize caps how many pending rows a single scan loads from the
	// store. Defaults to 50 if <= 0.
	BatchSize int
	// PollInterval is how often RunOnce is invoked by Run. Defaults to 15s
	// if <= 0. This is independent of internal/backoff's per-row delay —
	// PollInterval controls how often the worker *looks*, backoff controls
	// which rows it's willing to *act on* once it looks.
	PollInterval time.Duration
}

func (c Config) withDefaults() Config {
	if c.MaxRetries <= 0 {
		c.MaxRetries = 5
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 50
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 15 * time.Second
	}
	return c
}

// Worker runs the periodic buffer scan.
type Worker struct {
	store  Store
	csm    IncidentCreator
	twilio Escalator
	cfg    Config
	// now is overridden in tests for deterministic backoff-due checks.
	now func() time.Time
}

// New constructs a Worker. store, csm, and twilio must be non-nil.
func New(s Store, csm IncidentCreator, twilio Escalator, cfg Config) *Worker {
	return &Worker{
		store:  s,
		csm:    csm,
		twilio: twilio,
		cfg:    cfg.withDefaults(),
		now:    time.Now,
	}
}

// Run blocks, invoking RunOnce every cfg.PollInterval until ctx is
// cancelled. Intended to be started in its own goroutine from main.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.RunOnce(ctx)
		}
	}
}

// RunOnce loads one batch of pending rows and attempts delivery for every
// one that's due per internal/backoff, given its RetryCount/LastAttemptAt.
// Exported (not just reachable via Run) specifically so tests can invoke a
// single scan synchronously without a ticker.
func (w *Worker) RunOnce(ctx context.Context) {
	rows, err := w.store.PendingBatch(ctx, w.cfg.BatchSize)
	if err != nil {
		slog.ErrorContext(ctx, "worker: failed to load pending batch", "err", err)
		return
	}

	now := w.now()
	for _, row := range rows {
		if !backoff.Due(now, row.LastAttemptAt, row.RetryCount) {
			continue
		}
		w.attempt(ctx, row)
	}
}

// attempt makes one delivery attempt for row and applies the resulting
// store transition — deliver, retry, escalate, or terminal-fail — per the
// classification in isRetryable.
func (w *Worker) attempt(ctx context.Context, row store.AlertRecord) {
	// Each attempt gets its own fresh correlation ID rather than reusing the
	// one (if any) from the original POST /alerts request: that request
	// completed and returned to its caller long ago (this service persists
	// before attempting delivery — see internal/handler), and an attempt
	// happening minutes or hours later, on the worker's own schedule, is a
	// distinct traceable event.
	attemptCtx := middleware.WithCorrelationID(ctx, middleware.NewCorrelationID())

	var req csmclient.CreateIncidentRequest
	if err := json.Unmarshal(row.Payload, &req); err != nil {
		// The buffered payload itself is corrupt. No retry can ever fix
		// this — it isn't a CSM-availability problem — so this is terminal,
		// and specifically does not reach Twilio escalation (that channel
		// exists for "CSM won't accept this", not "we can't even ask").
		slog.ErrorContext(attemptCtx, "worker: buffered payload is not valid JSON, marking failed", "id", row.ID, "err", err)
		if merr := w.store.MarkFailed(ctx, row.ID, "corrupt buffered payload: "+err.Error()); merr != nil {
			slog.ErrorContext(attemptCtx, "worker: MarkFailed failed", "id", row.ID, "err", merr)
		}
		return
	}

	result, err := w.csm.CreateIncident(attemptCtx, req)
	if err == nil {
		if merr := w.store.MarkDelivered(ctx, row.ID, result.IncidentID); merr != nil {
			slog.ErrorContext(attemptCtx, "worker: MarkDelivered failed", "id", row.ID, "err", merr)
			return
		}
		slog.InfoContext(attemptCtx, "worker: alert delivered", "id", row.ID, "incidentID", result.IncidentID, "incidentNumber", result.IncidentNumber)
		return
	}

	if !isRetryable(err) {
		slog.ErrorContext(attemptCtx, "worker: non-retryable error, marking failed", "id", row.ID, "err", err)
		if merr := w.store.MarkFailed(ctx, row.ID, err.Error()); merr != nil {
			slog.ErrorContext(attemptCtx, "worker: MarkFailed failed", "id", row.ID, "err", merr)
		}
		return
	}

	nextRetryCount := row.RetryCount + 1
	if nextRetryCount >= w.cfg.MaxRetries {
		slog.WarnContext(attemptCtx, "worker: retry budget exhausted, escalating", "id", row.ID, "retryCount", nextRetryCount, "maxRetries", w.cfg.MaxRetries, "err", err)
		if merr := w.store.MarkEscalated(ctx, row.ID, err.Error()); merr != nil {
			slog.ErrorContext(attemptCtx, "worker: MarkEscalated (store) failed", "id", row.ID, "err", merr)
		}
		message := fmt.Sprintf(
			"SRE alert ingestion service: alert %s could not be delivered to CSM after %d attempts. Last error: %s",
			row.ID, nextRetryCount, truncate(err.Error(), 200),
		)
		if terr := w.twilio.Escalate(ctx, message); terr != nil {
			// The escalation call itself failing is the worst case this
			// service can be in — CSM is unreachable *and* the
			// CSM-independent notification channel just failed too. There
			// is no further fallback by design (see this service's
			// README/CLAUDE.md); log loudly and move on rather than retry
			// the call in a tight loop against Twilio.
			slog.ErrorContext(attemptCtx, "worker: twilio escalation call failed", "id", row.ID, "err", terr)
		}
		return
	}

	slog.WarnContext(attemptCtx, "worker: delivery attempt failed, will retry", "id", row.ID, "retryCount", nextRetryCount, "nextDelay", backoff.Delay(nextRetryCount-1).String(), "err", err)
	if merr := w.store.MarkAttemptFailed(ctx, row.ID, err.Error()); merr != nil {
		slog.ErrorContext(attemptCtx, "worker: MarkAttemptFailed failed", "id", row.ID, "err", merr)
	}
}

// isRetryable classifies an error returned by IncidentCreator.CreateIncident.
//
// A 400 means CSM rejected the request payload itself as invalid — retrying
// the exact same payload can never succeed, so this is treated as a
// terminal, non-retryable failure (Store.MarkFailed), distinct from every
// other case.
//
// Every other error is retryable — critically including a 401, which this
// service treats as retryable *by design*, not by oversight. A 401 would
// normally signal "the caller isn't authorized, stop retrying" for a
// typical client. Here it means something different: csm-integration-service
// is M2M-only and the upstream entity-service incident-creation operation
// is ServiceNow-backed, requiring a forwarded end-user identity token this
// stack cannot currently supply (see internal/csmclient/incidents.go's
// CreateIncident doc comment, and this service's own CLAUDE.md). That is a
// known, currently-permanent state of CSM's own capability, not a
// per-request auth failure — and it's exactly the kind of
// "CSM-side-unavailability" condition this whole service exists to buffer
// and retry through, all the way to Twilio escalation if it doesn't
// resolve. Treating it as non-retryable would silently drop every alert
// this service ever ingests today.
//
// Network/transport-level errors (timeout, connection refused, DNS
// failure, TLS error — anything that never got an HTTP response to
// classify) are also always retryable: they are unambiguously
// CSM-side-unavailability signals.
func isRetryable(err error) bool {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode != http.StatusBadRequest
	}
	return true
}

// truncate bounds s to at most n runes-as-bytes for inclusion in a
// human-read-aloud Twilio message and in logs, appending "..." when
// truncated so it's visibly incomplete rather than silently cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
