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
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/viewer"
)

// fakeEntityCustomerHealthClient scripts the one health-detail request and
// counts how often it is made.
type fakeEntityCustomerHealthClient struct {
	mu     sync.Mutex
	calls  []string
	detail entity.CHAccountHealthDetail
	err    error
}

func (f *fakeEntityCustomerHealthClient) GetAccountHealthDetail(_ context.Context, accountID string) (entity.CHAccountHealthDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, accountID)
	return f.detail, f.err
}

const (
	healthTestAccountID = "00000000-0000-0000-0000-000000000000"
	healthDashedProject = "12f3e20a-1bfd-3610-0bb3-da47b04bcb44"
	healthFlatProject   = "12f3e20a1bfd36100bb3da47b04bcb44"
)

// healthDetailFixture is a realistic entity-service response (dummy data) for
// one account with a busy project and an empty one.
const healthDetailFixture = `{
  "accountName": "Example Corp",
  "customerProjects": [
    {
      "id": "12f3e20a-1bfd-3610-0bb3-da47b04bcb44",
      "name": "Project One",
      "hasRecentCases": true,
      "detailedRecentCases": [
        {"priority": "high", "count": 2, "cases": [
          {"id": "aaaaaaaa-0000-0000-0000-000000000001", "number": "CS0000001"},
          {"id": "aaaaaaaa-0000-0000-0000-000000000002", "number": "CS0000002"}]},
        {"priority": "Unspecified", "count": 1, "cases": [
          {"id": "aaaaaaaa-0000-0000-0000-000000000003", "number": "CS0000003"}]}
      ],
      "totalRecentCases": 3,
      "hasAbandonedCases": true,
      "detailedAbandonedCases": [{"id": "bbbbbbbb-0000-0000-0000-000000000001", "number": "CS0000010"}],
      "isUsingEolProduct": true,
      "softwareModel": [
        {"name": "Example Gateway 1.2.0", "eolDate": "2025-01-01T00:00:00Z",
         "deployments": [{"id": "cccccccc-0000-0000-0000-000000000001", "name": "Production"}]}
      ],
      "deployments": [
        {"id": "cccccccc-0000-0000-0000-000000000001", "name": "Production"},
        {"id": "cccccccc-0000-0000-0000-000000000002", "name": "Staging"}
      ],
      "hasMigrationDelays": true,
      "detailedMigrationDelays": [
        {"id": "bbbbbbbb-0000-0000-0000-000000000001", "number": "CS0000010"},
        {"id": "bbbbbbbb-0000-0000-0000-000000000002", "number": "CS0000011"}],
      "hasEscalatedCases": true,
      "detailedEscalatedCases": [
        {"number": "CS0000001", "escalationId": "dddddddd-0000-0000-0000-000000000001", "escalationNumber": "CS0000001"}],
      "goLiveStatus": {"status": "Hasn't Gone Live in 6 months", "isRisk": true, "state": "failure"}
    },
    {
      "id": "00000000-0000-0000-0000-0000000000a1",
      "name": "Project Two",
      "hasRecentCases": false,
      "detailedRecentCases": [],
      "totalRecentCases": 0,
      "hasAbandonedCases": false,
      "detailedAbandonedCases": [],
      "isUsingEolProduct": false,
      "softwareModel": [],
      "deployments": [],
      "hasMigrationDelays": false,
      "detailedMigrationDelays": [],
      "hasEscalatedCases": false,
      "detailedEscalatedCases": [],
      "goLiveStatus": {"status": "Has Gone Live", "isRisk": false, "state": "success"}
    }
  ]
}`

// wantHealthDetailJSON is the portal's wire format for healthDetailFixture:
// flat 32-character ids everywhere an id appears, the escalation link carrying
// escalationSysId (and no sysId), and every empty list as [].
const wantHealthDetailJSON = `{
  "accountName": "Example Corp",
  "customerProjects": [
    {
      "sysId": "12f3e20a1bfd36100bb3da47b04bcb44",
      "name": "Project One",
      "hasRecentCases": true,
      "detailedRecentCases": [
        {"priority": "high", "count": 2, "cases": [
          {"sysId": "aaaaaaaa000000000000000000000001", "number": "CS0000001"},
          {"sysId": "aaaaaaaa000000000000000000000002", "number": "CS0000002"}]},
        {"priority": "Unspecified", "count": 1, "cases": [
          {"sysId": "aaaaaaaa000000000000000000000003", "number": "CS0000003"}]}
      ],
      "totalRecentCases": 3,
      "hasAbandonedCases": true,
      "detailedAbandonedCases": [{"sysId": "bbbbbbbb000000000000000000000001", "number": "CS0000010"}],
      "isUsingEolProduct": true,
      "softwareModel": [
        {"name": "Example Gateway 1.2.0", "eolDate": "2025-01-01T00:00:00Z",
         "deployments": [{"sysId": "cccccccc000000000000000000000001", "name": "Production"}]}
      ],
      "deployments": [
        {"sysId": "cccccccc000000000000000000000001", "name": "Production"},
        {"sysId": "cccccccc000000000000000000000002", "name": "Staging"}
      ],
      "hasMigrationDelays": true,
      "detailedMigrationDelays": [
        {"sysId": "bbbbbbbb000000000000000000000001", "number": "CS0000010"},
        {"sysId": "bbbbbbbb000000000000000000000002", "number": "CS0000011"}],
      "hasEscalatedCases": true,
      "detailedEscalatedCases": [
        {"number": "CS0000001", "escalationSysId": "dddddddd000000000000000000000001", "escalationNumber": "CS0000001"}],
      "goLiveStatus": {"status": "Hasn't Gone Live in 6 months", "isRisk": true, "state": "failure"}
    },
    {
      "sysId": "000000000000000000000000000000a1",
      "name": "Project Two",
      "hasRecentCases": false,
      "detailedRecentCases": [],
      "totalRecentCases": 0,
      "hasAbandonedCases": false,
      "detailedAbandonedCases": [],
      "isUsingEolProduct": false,
      "softwareModel": [],
      "deployments": [],
      "hasMigrationDelays": false,
      "detailedMigrationDelays": [],
      "hasEscalatedCases": false,
      "detailedEscalatedCases": [],
      "goLiveStatus": {"status": "Has Gone Live", "isRisk": false, "state": "success"}
    }
  ]
}`

func decodeHealthFixture(t *testing.T, raw string) entity.CHAccountHealthDetail {
	t.Helper()
	var d entity.CHAccountHealthDetail
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return d
}

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %v", err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", gb, wb)
	}
}

// TestGetCustomerHealthDetail_MapsEveryFieldToThePortalShape pins the whole
// mapping: ids flat (dashes stripped), escalation link fields, go-live status,
// and that empty lists are [] not null.
func TestGetCustomerHealthDetail_MapsEveryFieldToThePortalShape(t *testing.T) {
	fake := &fakeEntityCustomerHealthClient{detail: decodeHealthFixture(t, healthDetailFixture)}
	c := NewPostgresCustomerHealthClient(fake)

	detail, err := c.GetCustomerHealthDetail(context.Background(), healthTestAccountID)
	if err != nil {
		t.Fatalf("GetCustomerHealthDetail: %v", err)
	}
	got, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, got, wantHealthDetailJSON)

	if len(fake.calls) != 1 || fake.calls[0] != healthTestAccountID {
		t.Errorf("entity calls = %v, want exactly one for %s", fake.calls, healthTestAccountID)
	}
	if detail.CustomerProjects[0].SysID != healthFlatProject {
		t.Errorf("project sysId = %q, want flat %q", detail.CustomerProjects[0].SysID, healthFlatProject)
	}
	if strings.Contains(string(got), "null") {
		t.Errorf("response contains null: %s", got)
	}
}

func TestGetCustomerHealthDetail_NoProjectsSerializesEmptyArray(t *testing.T) {
	fake := &fakeEntityCustomerHealthClient{detail: entity.CHAccountHealthDetail{AccountName: "Example Corp"}}
	c := NewPostgresCustomerHealthClient(fake)

	detail, err := c.GetCustomerHealthDetail(context.Background(), healthTestAccountID)
	if err != nil {
		t.Fatalf("GetCustomerHealthDetail: %v", err)
	}
	got, _ := json.Marshal(detail)
	assertJSONEqual(t, got, `{"accountName":"Example Corp","customerProjects":[]}`)
}

func TestGetCustomerHealthDetail_AccountNotFound(t *testing.T) {
	fake := &fakeEntityCustomerHealthClient{err: &apierror.Error{StatusCode: http.StatusNotFound, Body: `{"message":"account not found"}`}}
	c := NewPostgresCustomerHealthClient(fake)

	_, err := c.GetCustomerHealthDetail(context.Background(), healthTestAccountID)
	if !errors.Is(err, viewer.ErrAccountNotFound) {
		t.Fatalf("got err=%v, want viewer.ErrAccountNotFound", err)
	}
}

func TestGetCustomerHealthDetail_OtherErrorsPassThroughUnchanged(t *testing.T) {
	upstream := &apierror.Error{StatusCode: http.StatusTooManyRequests, Body: "slow down"}
	fake := &fakeEntityCustomerHealthClient{err: upstream}
	c := NewPostgresCustomerHealthClient(fake)

	_, err := c.GetCustomerHealthDetail(context.Background(), healthTestAccountID)
	if err != error(upstream) {
		t.Fatalf("got err=%v, want the upstream error unchanged", err)
	}
	if errors.Is(err, viewer.ErrAccountNotFound) {
		t.Fatal("a 429 must not map to not-found")
	}
}

// healthEntityServer is a fake entity-service recording every request except
// the token fetch, answering the health-detail path with status/body.
type healthEntityServer struct {
	mu       sync.Mutex
	requests []string // "METHOD path"
	status   int
	body     string
}

func (s *healthEntityServer) start(t *testing.T) *entity.CustomerEntityClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return entity.NewCustomerEntityClient(entity.CustomerEntityConfig{
		BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "c", ClientSecret: "s",
	})
}

func serveHealthDetail(t *testing.T, srv *healthEntityServer) *httptest.ResponseRecorder {
	t.Helper()
	client := srv.start(t)
	h := NewCustomerHealthHandler(&fakeRiskClient{}, NewPostgresCustomerHealthClient(client), &fakeHealthAccountSearch{}, viewerAccessGuard)
	req := withUser(httptest.NewRequest(http.MethodGet, "/customer-health/accounts/"+healthTestAccountID, nil))
	req.SetPathValue("accountId", healthTestAccountID)
	w := httptest.NewRecorder()
	h.GetAccountDetail(w, req)
	return w
}

// TestGetAccountDetail_OneEntityRequest drives the real handler, the real
// client and the real typed entity client against a fake server: the request
// must make exactly one upstream call, GET /accounts/{id}/health-detail, and
// answer with the portal's JSON.
func TestGetAccountDetail_OneEntityRequest(t *testing.T) {
	srv := &healthEntityServer{status: http.StatusOK, body: healthDetailFixture}
	w := serveHealthDetail(t, srv)

	assertStatus(t, w, http.StatusOK)
	assertJSONEqual(t, w.Body.Bytes(), wantHealthDetailJSON)
	want := []string{"GET /accounts/" + healthTestAccountID + "/health-detail"}
	if strings.Join(srv.requests, "|") != strings.Join(want, "|") {
		t.Errorf("entity requests = %v, want exactly %v", srv.requests, want)
	}
}

func TestGetAccountDetail_EntityErrorsMapWithOneRequest(t *testing.T) {
	cases := []struct {
		name       string
		upstream   int
		wantStatus int
	}{
		{"not found", http.StatusNotFound, http.StatusNotFound},
		{"throttled", http.StatusTooManyRequests, http.StatusInternalServerError},
		{"unavailable", http.StatusServiceUnavailable, http.StatusServiceUnavailable},
		{"bad gateway", http.StatusBadGateway, http.StatusServiceUnavailable},
		{"server error", http.StatusInternalServerError, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := &healthEntityServer{status: tc.upstream, body: `{"message":"upstream said no"}`}
			w := serveHealthDetail(t, srv)
			assertStatus(t, w, tc.wantStatus)
			if len(srv.requests) != 1 {
				t.Errorf("entity requests = %v, want exactly one (no retry, no fan-out)", srv.requests)
			}
		})
	}
}
