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
