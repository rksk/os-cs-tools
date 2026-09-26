// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

package store

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v2"
	"github.com/scylladb/gocqlx/v2/qb"

	"alert-core-service/internal/model"
)

var incidentColumns = []string{
	"fingerprint", "incident_id", "incident_number", "status", "severity", "impact", "urgency", "service",
	"metric_name", "description", "category", "environment", "source", "alert_ids", "alert_count", "work_notes",
	"pending_notes", "first_seen", "last_seen", "state_checked_at", "notified", "csm_confirmed", "csm_attempts",
	"csm_permanently_failed",
}

// Bounds unbounded lists so a flapping alert can't blow past Cosmos's row-size limit; AlertCount keeps growing regardless.
const maxWorkNotes = 200

// IncidentRepo owns the incidents table; dedup uniqueness is a lightweight CAS transaction on fingerprint.
type IncidentRepo struct {
	session gocqlx.Session
	// maxAlertIDs bounds the AlertIDs list used for replay idempotency. It must be at least
	// poll.max_window: a poll window can replay any of its ids after a later Retry stalls the cycle,
	// and a shorter cap would let an id already tail-trimmed out of AlertIDs be treated as new again,
	// duplicating its work note.
	maxAlertIDs int
	// dedupWindow bounds how long an incident keeps absorbing duplicates before IsOpen treats it as
	// closed and Upsert starts a fresh generation; see model.Incident.IsOpen.
	dedupWindow time.Duration
}

// NewIncidentRepo ties the AlertIDs idempotency cap to the poller's own max_window so the two can't drift apart.
func NewIncidentRepo(session *gocql.Session, maxWindow int, dedupWindow time.Duration) (*IncidentRepo, error) {
	return &IncidentRepo{session: gocqlx.NewSession(session), maxAlertIDs: maxWindow, dedupWindow: dedupWindow}, nil
}

// pendingIncidentNumber is a placeholder until CSM assigns the real one, derived from fingerprint so it's deterministic.
func pendingIncidentNumber(fingerprint string) string {
	return "PENDING-" + fingerprint[:12]
}

// capTail keeps only the last max entries of list, so a long-lived, flapping incident's stored history stays bounded.
func capTail[T any](list []T, max int) []T {
	if len(list) <= max {
		return list
	}
	return append([]T{}, list[len(list)-max:]...)
}

// isPending mirrors the filter RetrySweep needs: an incident still owes CSM/Chat delivery either
// because CSM hasn't been confirmed yet and hasn't permanently failed (so it must keep being
// retried), or because CSM is confirmed but a work note is still queued to be pushed.
func isPending(csmConfirmed, csmPermanentlyFailed bool, pendingNotesLen int) bool {
	owesCSMOrChat := !csmConfirmed && !csmPermanentlyFailed
	owesNotes := csmConfirmed && pendingNotesLen > 0
	return owesCSMOrChat || owesNotes
}

// setPendingIndex keeps incidents_pending (RetrySweep's lookup index) in sync with pending, without
// re-reading incidents_processed -- callers that already know the up-to-date field values (Upsert,
// AppendWorkNote) use this directly to avoid an extra round trip.
func (r *IncidentRepo) setPendingIndex(ctx context.Context, fp string, pending bool) error {
	if pending {
		stmt, names := qb.Insert("incidents_pending").Columns("fingerprint").ToCql()
		if err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{"fingerprint": fp}).ExecRelease(); err != nil {
			return fmt.Errorf("mark incident %s pending: %w", fp, err)
		}
		return nil
	}
	stmt, names := qb.Delete("incidents_pending").Where(qb.Eq("fingerprint")).ToCql()
	if err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{"fingerprint": fp}).ExecRelease(); err != nil {
		return fmt.Errorf("clear incident %s from pending index: %w", fp, err)
	}
	return nil
}

// syncPendingIndex re-derives pending status from the ground-truth row and applies it via
// setPendingIndex -- used by callers (RecordCSMIncident, RecordCSMAttemptFailure, ClearPendingNotes)
// that don't already have every relevant field in hand. Still a single point read on the primary key,
// nothing like the full-table scan this index replaces.
func (r *IncidentRepo) syncPendingIndex(ctx context.Context, fp string) error {
	inc, found, err := r.get(ctx, fp)
	if err != nil {
		return fmt.Errorf("sync pending index for %s: %w", fp, err)
	}
	if !found {
		return nil
	}
	return r.setPendingIndex(ctx, fp, isPending(inc.CSMConfirmed, inc.CSMPermanentlyFailed, len(inc.PendingNotes)))
}

// Upsert maps the alert onto an incident by fingerprint, creating it via IF NOT EXISTS or updating it.
func (r *IncidentRepo) Upsert(ctx context.Context, alertID string, a model.Alert, severityNum int) (model.Incident, bool, error) {
	fp := model.Fingerprint(a.Source, a.Service, a.MetricName, a.Environment, a.UniqueIdentifier)

	existing, found, err := r.get(ctx, fp)
	if err != nil {
		return model.Incident{}, false, err
	}

	if !found {
		now := time.Now().UTC()
		impact, urgency := model.ImpactUrgency(severityNum)
		inc := model.Incident{
			Fingerprint:    fp,
			IncidentNumber: pendingIncidentNumber(fp),
			Status:         "new",
			Severity:       severityNum,
			Impact:         impact,
			Urgency:        urgency,
			Service:        a.Service,
			MetricName:     a.MetricName,
			Description:    model.BuildCreationNote(alertID, a.MetricName, a.Source),
			Category:       a.Category,
			Environment:    a.Environment,
			Source:         a.Source,
			AlertIDs:       []string{alertID},
			AlertCount:     1,
			FirstSeen:      now,
			LastSeen:       now,
		}
		stmt, names := qb.Insert("incidents_processed").Columns(incidentColumns...).Unique().ToCql()
		applied, err := r.session.Query(stmt, names).WithContext(ctx).BindStruct(inc).ExecCASRelease()
		if err != nil {
			return model.Incident{}, false, fmt.Errorf("create incident %s: %w", fp, err)
		}
		if applied {
			// Best-effort: the incident row is already durably created, and Handle's own idempotency
			// check (matching this alertID in AlertIDs) would skip calling Upsert again on retry, so
			// failing this call over a missed index write would strand the incident with no delivery
			// ever attempted. A missed insert here just delays RetrySweep noticing it, it doesn't lose it.
			_ = r.setPendingIndex(ctx, fp, true)
			return inc, true, nil
		}
		// Lost the race to another core; fall through and treat this alert as an update.
		existing, found, err = r.get(ctx, fp)
		if err != nil {
			return model.Incident{}, false, err
		}
		if !found {
			ins, insNames := qb.Insert("incidents_processed").Columns(incidentColumns...).ToCql()
			if err := r.session.Query(ins, insNames).WithContext(ctx).BindStruct(inc).ExecRelease(); err != nil {
				return model.Incident{}, false, fmt.Errorf("create incident %s (unconditional after stale CAS): %w", fp, err)
			}
			// Best-effort, same reasoning as the CAS-applied branch above.
			_ = r.setPendingIndex(ctx, fp, true)
			return inc, true, nil
		}
	}

	// Skip if alertID already folded in (retry after ambiguous timeout); engine.Handle's idempotency check relies on this too.
	for _, seen := range existing.AlertIDs {
		if seen == alertID {
			return existing, false, nil
		}
	}

	updated := existing
	updated.AlertIDs = capTail(append(append([]string{}, existing.AlertIDs...), alertID), r.maxAlertIDs)
	updated.AlertCount = existing.AlertCount + 1
	if severityNum < existing.Severity { // lower number = more severe
		updated.Severity = severityNum
		// Impact/Urgency must be recomputed on escalation -- otherwise a Minor->Critical incident keeps its original, now-stale pair.
		updated.Impact, updated.Urgency = model.ImpactUrgency(severityNum)
	}
	updated.LastSeen = time.Now().UTC()
	if updated.Category == "" && a.Category != "" {
		// Self-heal: an incident with no category yet picks one up from a later alert instead of staying blank.
		updated.Category = a.Category
	}
	if updated.Description == "" {
		// Self-heal: same as Category, so an incident created before its first descriptive alert still fills in.
		updated.Description = model.BuildCreationNote(alertID, a.MetricName, a.Source)
	}

	setCols := []string{"alert_ids", "alert_count", "severity", "impact", "urgency", "category", "description", "last_seen"}
	// Handle only reaches here for a closed (or permanently-failed) incident: reset delivery fields or
	// the recurrence is silently swallowed. FirstSeen reset also gives DedupTag a fresh value for
	// NotifyCSM. PendingNotes/StateCheckedAt reset too: they belonged to the old CSM incident this
	// generation is leaving behind.
	if !existing.IsOpen(time.Now(), r.dedupWindow) {
		// New generation: the old Description named the previous generation's alert id, so it must
		// be rebuilt from this alert or NotifyCSM would push a stale creation note to the new CSM incident.
		updated.Description = model.BuildCreationNote(alertID, a.MetricName, a.Source)
		updated.Status = "new"
		updated.IncidentID = ""
		updated.IncidentNumber = pendingIncidentNumber(fp)
		updated.Notified = false
		updated.CSMConfirmed = false
		updated.CSMAttempts = 0
		updated.CSMPermanentlyFailed = false
		updated.FirstSeen = updated.LastSeen
		updated.PendingNotes = nil
		updated.StateCheckedAt = time.Time{}
		setCols = append(setCols, "status", "incident_id", "incident_number", "notified", "csm_confirmed", "csm_attempts",
			"csm_permanently_failed", "first_seen", "pending_notes", "state_checked_at")
	}

	stmt, names := qb.Update("incidents_processed").
		Set(setCols...).
		Where(qb.Eq("fingerprint")).
		ToCql()
	if err := r.session.Query(stmt, names).WithContext(ctx).BindStruct(updated).ExecRelease(); err != nil {
		return model.Incident{}, false, fmt.Errorf("update incident %s: %w", fp, err)
	}
	// Best-effort, same reasoning as the create branches above: the row is already durably updated
	// (including this alertID), so Handle's idempotency check would skip retrying this call.
	_ = r.setPendingIndex(ctx, fp, isPending(updated.CSMConfirmed, updated.CSMPermanentlyFailed, len(updated.PendingNotes)))
	return updated, false, nil
}

// RecordAlertID appends alertID for idempotent annotate-only paths, matching Upsert's dedup check; no-op if already present.
func (r *IncidentRepo) RecordAlertID(ctx context.Context, existing model.Incident, alertID string) error {
	for _, seen := range existing.AlertIDs {
		if seen == alertID {
			return nil
		}
	}
	updated := capTail(append(append([]string{}, existing.AlertIDs...), alertID), r.maxAlertIDs)
	stmt, names := qb.Update("incidents_processed").
		Set("alert_ids").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint": existing.Fingerprint,
		"alert_ids":   updated,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("record alert id on incident %s: %w", existing.Fingerprint, err)
	}
	return nil
}

// RecordCSMIncident writes id, number, and csm_confirmed together so confirmed is never observed with a placeholder id.
func (r *IncidentRepo) RecordCSMIncident(ctx context.Context, fingerprint, incidentID, incidentNumber string) error {
	stmt, names := qb.Update("incidents_processed").
		Set("incident_id", "incident_number", "csm_confirmed").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint":     fingerprint,
		"incident_id":     incidentID,
		"incident_number": incidentNumber,
		"csm_confirmed":   true,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("record csm incident for %s: %w", fingerprint, err)
	}
	// Best-effort: csm_confirmed is already durably set, so failing this call would make the engine
	// wrongly believe CSM confirmation itself failed and retry NotifyCSM -- relying on its dedup-by-tag
	// search to avoid a duplicate incident, when the index sync failing has nothing to do with that.
	_ = r.syncPendingIndex(ctx, fingerprint)
	return nil
}

// RecordCSMAttemptStarted persists the bumped attempt count before NotifyCSM is called, not after an
// observed failure: a lost success response, or a failed write here or in RecordCSMAttemptFailure,
// must never leave csm_attempts understating how many attempts may have already reached CSM, since
// NotifyCSM's own dedup-search fail-open decision depends on that count being at least as large as
// the number of CreateIncident calls actually made.
func (r *IncidentRepo) RecordCSMAttemptStarted(ctx context.Context, fingerprint string, attempts int) error {
	stmt, names := qb.Update("incidents_processed").
		Set("csm_attempts").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint":  fingerprint,
		"csm_attempts": attempts,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("record csm attempt started for %s: %w", fingerprint, err)
	}
	return nil
}

// RecordCSMAttemptFailure sets csm_permanently_failed once attempts are exhausted or CSM rejects non-retryably, stopping RetrySweep from retrying forever.
func (r *IncidentRepo) RecordCSMAttemptFailure(ctx context.Context, fingerprint string, attempts, maxAttempts int, permanent bool) error {
	failed := permanent || attempts >= maxAttempts
	stmt, names := qb.Update("incidents_processed").
		Set("csm_attempts", "csm_permanently_failed").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint":            fingerprint,
		"csm_attempts":           attempts,
		"csm_permanently_failed": failed,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("record csm attempt failure for %s: %w", fingerprint, err)
	}
	// Best-effort, same reasoning as RecordCSMIncident: the attempt/failure state is already durably
	// persisted, so a missed index sync must not be reported as this call having failed.
	_ = r.syncPendingIndex(ctx, fingerprint)
	return nil
}

// SyncStatus persists CSM's status so IsOpen reflects CSM's lifecycle, not a value only this service wrote.
// checkedAt is stamped alongside so the next syncIncidentState call can throttle off it.
func (r *IncidentRepo) SyncStatus(ctx context.Context, fingerprint, status string, checkedAt time.Time) error {
	stmt, names := qb.Update("incidents_processed").
		Set("status", "state_checked_at").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint":      fingerprint,
		"status":           status,
		"state_checked_at": checkedAt,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("sync status for %s: %w", fingerprint, err)
	}
	return nil
}

// RecordStateChecked stamps state_checked_at alone, for the common case where CSM's status hasn't
// changed since the last check but the throttle window must still advance.
func (r *IncidentRepo) RecordStateChecked(ctx context.Context, fingerprint string, checkedAt time.Time) error {
	stmt, names := qb.Update("incidents_processed").
		Set("state_checked_at").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint":      fingerprint,
		"state_checked_at": checkedAt,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("record state checked for %s: %w", fingerprint, err)
	}
	return nil
}

// FindByFingerprint reads without mutating, so the engine can decide annotate vs Upsert before touching any row.
func (r *IncidentRepo) FindByFingerprint(ctx context.Context, fp string) (model.Incident, bool, error) {
	return r.get(ctx, fp)
}

// MarkNotified flips notified to true once Chat has delivered to every configured target.
func (r *IncidentRepo) MarkNotified(ctx context.Context, fingerprint string) error {
	stmt, names := qb.Update("incidents_processed").
		Set("notified").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint": fingerprint,
		"notified":    true,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("mark incident %s notified: %w", fingerprint, err)
	}
	return nil
}

// ListPending reads the maintained incidents_pending index, bounded by outstanding work, instead of
// scanning the whole (unbounded, ever-growing) incidents_processed table.
func (r *IncidentRepo) ListPending(ctx context.Context) ([]model.Incident, error) {
	stmt, names := qb.Select("incidents_pending").Columns("fingerprint").ToCql()
	var rows []struct {
		Fingerprint string `db:"fingerprint"`
	}
	if err := r.session.Query(stmt, names).WithContext(ctx).SelectRelease(&rows); err != nil {
		return nil, fmt.Errorf("list pending fingerprints: %w", err)
	}
	pending := make([]model.Incident, 0, len(rows))
	for _, row := range rows {
		inc, found, err := r.get(ctx, row.Fingerprint)
		if err != nil {
			return nil, fmt.Errorf("list pending: read incident %s: %w", row.Fingerprint, err)
		}
		if !found {
			continue // index entry outlived its row; skip, nothing to retry.
		}
		if !isPending(inc.CSMConfirmed, inc.CSMPermanentlyFailed, len(inc.PendingNotes)) {
			// Index entry is stale (e.g. a partial failure between the primary write and its index
			// sync elsewhere) -- self-heal so future sweeps don't keep re-reading a delivered incident.
			_ = r.setPendingIndex(ctx, row.Fingerprint, false)
			continue
		}
		pending = append(pending, inc)
	}
	return pending, nil
}

// BackfillPendingIndex populates incidents_pending for any row in incidents_processed that already
// owes delivery, so upgrading to the indexed ListPending above doesn't silently lose track of
// incidents created before this index existed. Intended to run once at startup: it's a full scan of
// incidents_processed, but a one-time cost per process start rather than a recurring one every sweep
// interval, and idempotent (inserting an already-indexed fingerprint is a harmless no-op) so it's safe
// to run on every restart.
func (r *IncidentRepo) BackfillPendingIndex(ctx context.Context) error {
	stmt, names := qb.Select("incidents_processed").
		Columns("fingerprint", "csm_confirmed", "csm_permanently_failed", "pending_notes").
		ToCql()
	var rows []struct {
		Fingerprint          string   `db:"fingerprint"`
		CSMConfirmed         bool     `db:"csm_confirmed"`
		CSMPermanentlyFailed bool     `db:"csm_permanently_failed"`
		PendingNotes         []string `db:"pending_notes"`
	}
	if err := r.session.Query(stmt, names).WithContext(ctx).SelectRelease(&rows); err != nil {
		return fmt.Errorf("backfill pending index: list incidents: %w", err)
	}
	for _, row := range rows {
		if !isPending(row.CSMConfirmed, row.CSMPermanentlyFailed, len(row.PendingNotes)) {
			continue
		}
		if err := r.setPendingIndex(ctx, row.Fingerprint, true); err != nil {
			return fmt.Errorf("backfill pending index: mark %s pending: %w", row.Fingerprint, err)
		}
	}
	return nil
}

// AppendWorkNote appends the note to both the full audit log (work_notes) and the not-yet-pushed
// queue (pending_notes), since Cosmos's Cassandra API lacks native list append.
func (r *IncidentRepo) AppendWorkNote(ctx context.Context, existing model.Incident, note string) error {
	updatedNotes := capTail(append(append([]string{}, existing.WorkNotes...), note), maxWorkNotes)
	updatedPending := capTail(append(append([]string{}, existing.PendingNotes...), note), maxWorkNotes)
	stmt, names := qb.Update("incidents_processed").
		Set("work_notes", "pending_notes").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint":   existing.Fingerprint,
		"work_notes":    updatedNotes,
		"pending_notes": updatedPending,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("append work note to incident %s: %w", existing.Fingerprint, err)
	}
	// Best-effort, same reasoning as Upsert: the note is already durably appended, so a missed index
	// sync must not be reported as this call having failed.
	_ = r.setPendingIndex(ctx, existing.Fingerprint, isPending(existing.CSMConfirmed, existing.CSMPermanentlyFailed, len(updatedPending)))
	return nil
}

// ClearPendingNotes persists the notes still owed to CSM after a (possibly partial) push attempt;
// remaining is empty on full success, or the unpushed suffix on a failure partway through.
func (r *IncidentRepo) ClearPendingNotes(ctx context.Context, fingerprint string, remaining []string) error {
	stmt, names := qb.Update("incidents_processed").
		Set("pending_notes").
		Where(qb.Eq("fingerprint")).
		ToCql()
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{
		"fingerprint":   fingerprint,
		"pending_notes": remaining,
	}).ExecRelease()
	if err != nil {
		return fmt.Errorf("clear pending notes for %s: %w", fingerprint, err)
	}
	// Best-effort, same reasoning as the other index-sync calls above.
	_ = r.syncPendingIndex(ctx, fingerprint)
	return nil
}

// get reads the incident for a fingerprint, reporting absence as false rather than an error.
func (r *IncidentRepo) get(ctx context.Context, fp string) (model.Incident, bool, error) {
	stmt, names := qb.Select("incidents_processed").Columns(incidentColumns...).Where(qb.Eq("fingerprint")).ToCql()
	var inc model.Incident
	err := r.session.Query(stmt, names).WithContext(ctx).BindMap(qb.M{"fingerprint": fp}).GetRelease(&inc)
	if err == gocql.ErrNotFound {
		return model.Incident{}, false, nil
	}
	if err != nil {
		return model.Incident{}, false, fmt.Errorf("read incident %s: %w", fp, err)
	}
	return inc, true, nil
}
