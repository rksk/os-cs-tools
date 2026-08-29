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

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/csmclient"
	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/severity"
)

// alertStore is the subset of internal/store.Store AlertHandler depends on.
type alertStore interface {
	Enqueue(ctx context.Context, payload []byte) (id string, err error)
}

// AlertRequest is the inbound, vendor-agnostic normalized alert payload
// third-party monitoring/alerting tools POST to this service. It is
// intentionally decoupled from CreateIncidentRequest's shape — a vendor
// speaks "alerts", CSM speaks "incidents"; MapToIncident bridges the two.
type AlertRequest struct {
	Source           string `json:"source"`
	Severity         string `json:"severity"`
	Service          string `json:"service"`
	MetricName       string `json:"metricName"`
	Category         string `json:"category,omitempty"`
	Environment      string `json:"environment,omitempty"`
	UniqueIdentifier string `json:"uniqueIdentifier,omitempty"`
	Description      string `json:"description"`
}

// validate reports the first missing required field, or "" if req is
// well-formed. Category/Environment/UniqueIdentifier are genuinely optional
// (see severity.MapCategory's safe default and the WorkNotes builder below).
func (req AlertRequest) validate() string {
	switch {
	case strings.TrimSpace(req.Source) == "":
		return "source is required"
	case strings.TrimSpace(req.Severity) == "":
		return "severity is required"
	case strings.TrimSpace(req.Service) == "":
		return "service is required"
	case strings.TrimSpace(req.MetricName) == "":
		return "metricName is required"
	case strings.TrimSpace(req.Description) == "":
		return "description is required"
	}
	return ""
}

// MapToIncident builds the CreateIncidentRequest this service will
// eventually send to csm-integration-service, from req and the configured
// callerID (see AlertHandler.callerID's doc comment — CSM has no "system"
// user concept for machine-created incidents today, so this must be a real,
// operator-provisioned CSM user id passed in via config, never guessed or
// hardcoded here).
func MapToIncident(req AlertRequest, callerID string) csmclient.CreateIncidentRequest {
	iu := severity.MapImpactUrgency(req.Severity)
	category := severity.MapCategory(req.Category)

	out := csmclient.CreateIncidentRequest{
		CallerID:  callerID,
		Category:  category,
		ServiceID: req.Service,
		Impact:    iu.Impact,
		Urgency:   iu.Urgency,
		Subject:   buildSubject(req),
	}

	if ct, ok := severity.MapContactType(req.Source); ok {
		out.ContactType = &ct
	}

	// Description is the customer/incident-facing narrative -> AdditionalComments.
	comments := req.Description
	out.AdditionalComments = &comments

	// WorkNotes carries the alert's own metadata for the engineer who picks
	// this up — the fields that don't have a home anywhere else on
	// CreateIncidentRequest (source, environment, unique identifier, the
	// raw severity string as reported).
	if notes := buildWorkNotes(req); notes != "" {
		out.WorkNotes = &notes
	}

	return out
}

// buildSubject composes CreateIncidentRequest.Subject from the alert's
// metric name and source, e.g. "[azure] high_error_rate alert: svc-checkout".
// Kept short and scannable — full detail belongs in AdditionalComments/WorkNotes.
func buildSubject(req AlertRequest) string {
	return fmt.Sprintf("[%s] %s alert: %s", req.Source, req.MetricName, req.Service)
}

// buildWorkNotes composes an internal-engineer-facing note from the alert
// fields that have no other home on CreateIncidentRequest. Returns "" only
// if req somehow carries none of these (validate() already requires
// Source/MetricName, so this is effectively always non-empty in practice).
func buildWorkNotes(req AlertRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Alert source: %s\n", req.Source)
	fmt.Fprintf(&b, "Reported severity: %s\n", req.Severity)
	fmt.Fprintf(&b, "Metric: %s\n", req.MetricName)
	if req.Environment != "" {
		fmt.Fprintf(&b, "Environment: %s\n", req.Environment)
	}
	if req.UniqueIdentifier != "" {
		fmt.Fprintf(&b, "Alert identifier: %s\n", req.UniqueIdentifier)
	}
	return strings.TrimRight(b.String(), "\n")
}

// AlertHandler handles POST /alerts.
type AlertHandler struct {
	store alertStore
	// callerID is CreateIncidentRequest.CallerID for every incident this
	// service creates: a real CSM user id the operator must provision ahead
	// of time (SRE_ALERT_CALLER_ID). CSM has no "system"/machine-caller
	// concept today. This is required, non-empty config, never a guessed or
	// hardcoded value — see this service's README/CLAUDE.md.
	callerID string
}

// NewAlertHandler creates an AlertHandler. callerID must be non-empty — the
// caller (cmd/server/main.go) is expected to fail startup via mustEnv if
// SRE_ALERT_CALLER_ID is unset, rather than this constructor silently
// accepting an empty string.
func NewAlertHandler(store alertStore, callerID string) *AlertHandler {
	return &AlertHandler{store: store, callerID: callerID}
}

// CreateAlert handles POST /alerts: validates the inbound alert, maps it to
// a CreateIncidentRequest, and persists it to the buffer — never attempting
// delivery inline. This ordering (persist, then respond; delivery happens
// later, on the worker's own schedule) is a correctness requirement, not
// just a latency optimization: if this service crashed between a delivery
// attempt and persisting, an alert could be lost with no record it was ever
// received. Persisting first, unconditionally, before any attempt is made,
// is what makes "never lose a buffered alert, even across this service's
// own restart" true regardless of when in the flow a crash happens.
func (h *AlertHandler) CreateAlert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	var req AlertRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	if msg := req.validate(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	incidentReq := MapToIncident(req, h.callerID)
	payload, err := json.Marshal(incidentReq)
	if err != nil {
		slog.ErrorContext(r.Context(), "handler: failed to marshal mapped incident request", "err", err)
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return
	}

	id, err := h.store.Enqueue(r.Context(), payload)
	if err != nil {
		slog.ErrorContext(r.Context(), "handler: failed to persist buffered alert", "err", err)
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return
	}

	slog.InfoContext(r.Context(), "alert buffered", "id", id, "source", req.Source, "severity", req.Severity, "service", req.Service)
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
}
