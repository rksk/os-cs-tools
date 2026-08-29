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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/integrations/sre-alert-ingestion-service/internal/csmclient"
)

func validAlertJSON() []byte {
	return []byte(`{
		"source": "azure",
		"severity": "critical",
		"service": "svc-checkout",
		"metricName": "error_rate",
		"environment": "production",
		"uniqueIdentifier": "alert-abc-123",
		"description": "Error rate exceeded 5% for 10 minutes"
	}`)
}

func TestCreateAlert_Success(t *testing.T) {
	store := &mockStore{enqueueFn: func(ctx context.Context, payload []byte) (string, error) {
		return "buffered-id-1", nil
	}}
	h := NewAlertHandler(store, "caller-1")

	r := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validAlertJSON()))
	w := httptest.NewRecorder()
	h.CreateAlert(w, r)

	assertStatus(t, w, http.StatusAccepted)
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.ID != "buffered-id-1" {
		t.Errorf("id = %q, want %q", body.ID, "buffered-id-1")
	}

	if len(store.enqueuedPayloads) != 1 {
		t.Fatalf("Enqueue called %d times, want 1", len(store.enqueuedPayloads))
	}
	var incidentReq csmclient.CreateIncidentRequest
	if err := json.Unmarshal(store.enqueuedPayloads[0], &incidentReq); err != nil {
		t.Fatalf("buffered payload is not a valid CreateIncidentRequest: %v", err)
	}
	if incidentReq.CallerID != "caller-1" {
		t.Errorf("CallerID = %q, want %q", incidentReq.CallerID, "caller-1")
	}
	if incidentReq.ServiceID != "svc-checkout" {
		t.Errorf("ServiceID = %q, want %q", incidentReq.ServiceID, "svc-checkout")
	}
	if incidentReq.Impact != "HIGH" || incidentReq.Urgency != "HIGH" {
		t.Errorf("Impact/Urgency = %s/%s, want HIGH/HIGH for critical severity", incidentReq.Impact, incidentReq.Urgency)
	}
	if incidentReq.ContactType == nil || *incidentReq.ContactType != "AZURE" {
		t.Errorf("ContactType = %v, want AZURE", incidentReq.ContactType)
	}
	if incidentReq.Category != "SERVICE_INTERRUPTION" {
		t.Errorf("Category = %q, want SERVICE_INTERRUPTION (default)", incidentReq.Category)
	}
	if incidentReq.AdditionalComments == nil || *incidentReq.AdditionalComments != "Error rate exceeded 5% for 10 minutes" {
		t.Errorf("AdditionalComments = %v, want the alert description", incidentReq.AdditionalComments)
	}
	if incidentReq.WorkNotes == nil || !strings.Contains(*incidentReq.WorkNotes, "alert-abc-123") {
		t.Errorf("WorkNotes = %v, want it to contain the unique identifier", incidentReq.WorkNotes)
	}
}

// This is the persistence-before-delivery guarantee, exercised directly:
// CreateAlert has no delivery-attempt dependency at all (no csmclient in
// sight) — a successful response is only ever contingent on Enqueue
// succeeding.
func TestCreateAlert_NeverAttemptsDeliveryInline(t *testing.T) {
	store := &mockStore{}
	h := NewAlertHandler(store, "caller-1")

	r := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validAlertJSON()))
	w := httptest.NewRecorder()
	h.CreateAlert(w, r)

	assertStatus(t, w, http.StatusAccepted)
}

func TestCreateAlert_RejectsInvalidJSON(t *testing.T) {
	store := &mockStore{}
	h := NewAlertHandler(store, "caller-1")

	r := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader([]byte(`not json`)))
	w := httptest.NewRecorder()
	h.CreateAlert(w, r)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorMessage(t, w, ErrMsgBadRequest)
	if len(store.enqueuedPayloads) != 0 {
		t.Error("Enqueue should not be called for invalid JSON")
	}
}

func TestCreateAlert_RejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing source", `{"severity":"critical","service":"svc","metricName":"m","description":"d"}`},
		{"missing severity", `{"source":"azure","service":"svc","metricName":"m","description":"d"}`},
		{"missing service", `{"source":"azure","severity":"critical","metricName":"m","description":"d"}`},
		{"missing metricName", `{"source":"azure","severity":"critical","service":"svc","description":"d"}`},
		{"missing description", `{"source":"azure","severity":"critical","service":"svc","metricName":"m"}`},
		{"empty body", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockStore{}
			h := NewAlertHandler(store, "caller-1")

			r := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader([]byte(tc.body)))
			w := httptest.NewRecorder()
			h.CreateAlert(w, r)

			assertStatus(t, w, http.StatusBadRequest)
			if len(store.enqueuedPayloads) != 0 {
				t.Error("Enqueue should not be called when validation fails")
			}
		})
	}
}

func TestCreateAlert_RejectsOversizedBody(t *testing.T) {
	store := &mockStore{}
	h := NewAlertHandler(store, "caller-1")

	huge := bytes.Repeat([]byte("a"), maxRequestBodyBytes+1)
	r := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(huge))
	w := httptest.NewRecorder()
	h.CreateAlert(w, r)

	assertStatus(t, w, http.StatusRequestEntityTooLarge)
	assertErrorMessage(t, w, ErrMsgTooLarge)
}

func TestCreateAlert_StoreFailureReturns500(t *testing.T) {
	store := &mockStore{enqueueFn: func(ctx context.Context, payload []byte) (string, error) {
		return "", errors.New("connection refused")
	}}
	h := NewAlertHandler(store, "caller-1")

	r := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewReader(validAlertJSON()))
	w := httptest.NewRecorder()
	h.CreateAlert(w, r)

	assertStatus(t, w, http.StatusInternalServerError)
	assertErrorMessage(t, w, ErrMsgInternal)
}

func TestMapToIncident_UnmappedSourceOmitsContactType(t *testing.T) {
	req := AlertRequest{Source: "datadog", Severity: "minor", Service: "svc", MetricName: "m", Description: "d"}
	out := MapToIncident(req, "caller-1")
	if out.ContactType != nil {
		t.Errorf("ContactType = %v, want nil for an unmapped source", out.ContactType)
	}
}

func TestMapToIncident_CategoryPassthroughWhenValid(t *testing.T) {
	req := AlertRequest{Source: "azure", Severity: "minor", Service: "svc", MetricName: "m", Description: "d", Category: "security"}
	out := MapToIncident(req, "caller-1")
	if out.Category != "SECURITY" {
		t.Errorf("Category = %q, want SECURITY", out.Category)
	}
}
