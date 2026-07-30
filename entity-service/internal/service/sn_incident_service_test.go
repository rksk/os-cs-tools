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

package service

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

const (
	testIncidentWatcherUUID1  = "44444444-4444-4444-4444-444444444444"
	testIncidentWatcherUUID2  = "55555555-5555-5555-5555-555555555555"
	testIncidentWatcherSysid1 = "44444444444444444444444444444444"
	testIncidentWatcherSysid2 = "55555555555555555555555555555555"
	testIncidentUUID          = "66666666-6666-6666-6666-666666666666"
	testIncidentSysid         = "66666666666666666666666666666666"
)

// validCreateIncidentRequest returns a minimally valid CreateIncidentRequest so
// that only the field under test needs to be overridden per case.
func validCreateIncidentRequest() domain.CreateIncidentRequest {
	return domain.CreateIncidentRequest{
		CallerID:  testCaseUUID,
		Category:  domain.IncidentCategoryInquiry,
		ServiceID: testCaseUUID,
		Impact:    domain.IncidentImpactLow,
		Urgency:   domain.IncidentUrgencyLow,
		Subject:   "subject",
	}
}

// TestSNIncidentService_CreateIncident_WatchListConvertedToSysids verifies that
// every watchList UUID is converted to a ServiceNow sysid before it reaches the
// outgoing payload, so SN resolves sys_user records instead of 404ing on a raw
// platform UUID.
func TestSNIncidentService_CreateIncident_WatchListConvertedToSysids(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/incidents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message": "Incident created successfully.",
			"incident": {"id": "` + testIncidentSysid + `", "number": "INC0001", "createdOn": "2026-01-01 00:00:00", "createdBy": "engineer@example.com"}
		}`))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowIncidentService(client)

	req := validCreateIncidentRequest()
	req.WatchList = []string{testIncidentWatcherUUID1, testIncidentWatcherUUID2}

	if _, err := svc.CreateIncident(contextWithUserIDToken("token"), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotWatchList, ok := gotBody["watchList"].([]any)
	if !ok {
		t.Fatalf("expected watchList array in payload, got %+v", gotBody["watchList"])
	}
	want := []string{testIncidentWatcherSysid1, testIncidentWatcherSysid2}
	if len(gotWatchList) != len(want) {
		t.Fatalf("watchList length: got %d, want %d", len(gotWatchList), len(want))
	}
	for i, w := range want {
		if gotWatchList[i] != w {
			t.Fatalf("watchList[%d]: got %v, want %q (raw UUID must not be sent to SN)", i, gotWatchList[i], w)
		}
	}
}

// TestSNIncidentService_CreateIncident_WatchList_InvalidUUID verifies a malformed
// watchList entry is rejected with a clean validation error before any SN call.
func TestSNIncidentService_CreateIncident_WatchList_InvalidUUID(t *testing.T) {
	// client is intentionally nil: validation must fail before touching it.
	svc := NewServiceNowIncidentService(nil)

	req := validCreateIncidentRequest()
	req.WatchList = []string{"not-a-uuid"}

	_, err := svc.CreateIncident(contextWithUserIDToken("token"), req)
	if _, ok := err.(*apierror.ValidationError); !ok {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

// TestSNIncidentService_UpdateIncident_WatchListConvertedToSysids mirrors the
// create-path coverage above for the PATCH /incidents/{id} path.
func TestSNIncidentService_UpdateIncident_WatchListConvertedToSysids(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/incidents/"+testIncidentSysid, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message": "Incident updated successfully.",
			"incident": {"id": "` + testIncidentSysid + `", "number": "INC0001", "createdOn": "2026-01-01 00:00:00", "createdBy": "engineer@example.com"}
		}`))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowIncidentService(client)

	watchList := []string{testIncidentWatcherUUID1, testIncidentWatcherUUID2}
	_, err := svc.UpdateIncident(contextWithUserIDToken("token"), domain.UpdateIncidentRequest{
		ID:        testIncidentUUID,
		WatchList: &watchList,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotWatchList, ok := gotBody["watchList"].([]any)
	if !ok {
		t.Fatalf("expected watchList array in payload, got %+v", gotBody["watchList"])
	}
	want := []string{testIncidentWatcherSysid1, testIncidentWatcherSysid2}
	if len(gotWatchList) != len(want) {
		t.Fatalf("watchList length: got %d, want %d", len(gotWatchList), len(want))
	}
	for i, w := range want {
		if gotWatchList[i] != w {
			t.Fatalf("watchList[%d]: got %v, want %q (raw UUID must not be sent to SN)", i, gotWatchList[i], w)
		}
	}
}

// TestSNIncidentService_UpdateIncident_WatchList_InvalidUUID verifies a malformed
// watchList entry is rejected with a clean validation error before any SN call.
func TestSNIncidentService_UpdateIncident_WatchList_InvalidUUID(t *testing.T) {
	// client is intentionally nil: validation must fail before touching it.
	svc := NewServiceNowIncidentService(nil)

	watchList := []string{"not-a-uuid"}
	_, err := svc.UpdateIncident(contextWithUserIDToken("token"), domain.UpdateIncidentRequest{
		ID:        testIncidentUUID,
		WatchList: &watchList,
	})
	if _, ok := err.(*apierror.ValidationError); !ok {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

// TestSNIncidentService_SearchIncidents_FiltersForwarded verifies that the
// SLA-violated, created-date-range and product filters reach the outgoing search
// payload in the wire shape the integration service expects. The date bounds in
// particular must be YYYY-MM-DDTHH:MM:SSZ and both inclusive; a wrongly formatted
// bound is silently accepted upstream, so only this assertion catches it.
func TestSNIncidentService_SearchIncidents_FiltersForwarded(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"incidents": [], "totalRecords": 0, "offset": 0, "limit": 25}`))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowIncidentService(client)

	slaViolated := true
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 31, 23, 59, 59, 0, time.UTC)
	_, err := svc.SearchIncidents(contextWithUserIDToken("token"), domain.SearchIncidentsRequest{
		Filters: domain.SearchIncidentsFilters{
			SLAViolated:      &slaViolated,
			StartCreatedDate: &start,
			EndCreatedDate:   &end,
			ProductNames:     []string{"Choreo", "Asgardeo"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filters, ok := gotBody["filters"].(map[string]any)
	if !ok {
		t.Fatalf("expected filters object in payload, got %+v", gotBody["filters"])
	}
	if filters["slaViolated"] != true {
		t.Fatalf("slaViolated: got %v, want true", filters["slaViolated"])
	}
	if filters["startCreatedDate"] != "2026-05-01T00:00:00Z" {
		t.Fatalf("startCreatedDate: got %v, want 2026-05-01T00:00:00Z", filters["startCreatedDate"])
	}
	if filters["endCreatedDate"] != "2026-05-31T23:59:59Z" {
		t.Fatalf("endCreatedDate: got %v, want 2026-05-31T23:59:59Z", filters["endCreatedDate"])
	}
	products, ok := filters["productNames"].([]any)
	if !ok || len(products) != 2 || products[0] != "Choreo" || products[1] != "Asgardeo" {
		t.Fatalf("productNames: got %+v, want [Choreo Asgardeo]", filters["productNames"])
	}
}

// TestSNIncidentService_SearchIncidents_UnsetFiltersOmitted verifies that unset
// filters are omitted from the payload rather than sent as zero values, so an
// untouched filter bar cannot accidentally constrain the result set.
func TestSNIncidentService_SearchIncidents_UnsetFiltersOmitted(t *testing.T) {
	var gotBody map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"incidents": [], "totalRecords": 0, "offset": 0, "limit": 25}`))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowIncidentService(client)

	if _, err := svc.SearchIncidents(contextWithUserIDToken("token"), domain.SearchIncidentsRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filters, _ := gotBody["filters"].(map[string]any)
	for _, key := range []string{"slaViolated", "startCreatedDate", "endCreatedDate", "productNames"} {
		if _, present := filters[key]; present {
			t.Fatalf("%s must be omitted when unset, got %v", key, filters[key])
		}
	}
}

// TestSNIncidentService_SearchIncidents_InvertedDateRange verifies an inverted
// created-date range is rejected before any upstream call, matching case search.
func TestSNIncidentService_SearchIncidents_InvertedDateRange(t *testing.T) {
	// client is intentionally nil: validation must fail before touching it.
	svc := NewServiceNowIncidentService(nil)

	start := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.SearchIncidents(contextWithUserIDToken("token"), domain.SearchIncidentsRequest{
		Filters: domain.SearchIncidentsFilters{StartCreatedDate: &start, EndCreatedDate: &end},
	})
	if _, ok := err.(*apierror.ValidationError); !ok {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}
