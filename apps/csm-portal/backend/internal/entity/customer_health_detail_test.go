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

package entity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
)

func newHealthDetailTestClient(t *testing.T, handler http.HandlerFunc) *CustomerEntityClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewCustomerEntityClient(CustomerEntityConfig{
		BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "c", ClientSecret: "s",
	})
}

func TestGetAccountHealthDetailRequestsPathAndDecodes(t *testing.T) {
	var gotMethod, gotPath, gotRawPath string
	client := newHealthDetailTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotRawPath = r.Method, r.URL.Path, r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountName":"Example Corp","customerProjects":[{
			"id":"00000000-0000-0000-0000-000000000001","name":"Project One",
			"hasRecentCases":true,
			"detailedRecentCases":[{"priority":"high","count":1,"cases":[{"id":"00000000-0000-0000-0000-000000000002","number":"CS0000001"}]}],
			"totalRecentCases":1,"hasAbandonedCases":false,"detailedAbandonedCases":[],
			"isUsingEolProduct":true,
			"softwareModel":[{"name":"Example Gateway 1.2.0","eolDate":"2025-01-01T00:00:00Z","deployments":[{"id":"00000000-0000-0000-0000-000000000003","name":"Production"}]}],
			"deployments":[{"id":"00000000-0000-0000-0000-000000000003","name":"Production"}],
			"hasMigrationDelays":false,"detailedMigrationDelays":[],
			"hasEscalatedCases":true,
			"detailedEscalatedCases":[{"number":"CS0000001","escalationId":"00000000-0000-0000-0000-000000000004","escalationNumber":"CS0000001"}],
			"goLiveStatus":{"status":"Has Gone Live","isRisk":false,"state":"success"}}]}`))
	})

	resp, err := client.GetAccountHealthDetail(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetAccountHealthDetail: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/accounts/00000000-0000-0000-0000-000000000000/health-detail" {
		t.Errorf("request = %s %s, want GET /accounts/<id>/health-detail", gotMethod, gotPath)
	}
	if gotRawPath != gotPath {
		t.Errorf("escaped path %q differs from path %q", gotRawPath, gotPath)
	}
	if resp.AccountName != "Example Corp" || len(resp.CustomerProjects) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	p := resp.CustomerProjects[0]
	if p.ID != "00000000-0000-0000-0000-000000000001" || !p.HasRecentCases || p.TotalRecentCases != 1 ||
		len(p.DetailedRecentCases) != 1 || p.DetailedRecentCases[0].Cases[0].Number != "CS0000001" ||
		!p.IsUsingEolProduct || p.SoftwareModel[0].Deployments[0].Name != "Production" ||
		len(p.Deployments) != 1 || !p.HasEscalatedCases ||
		p.DetailedEscalatedCases[0].EscalationID != "00000000-0000-0000-0000-000000000004" ||
		p.DetailedEscalatedCases[0].EscalationNumber != "CS0000001" ||
		p.GoLiveStatus.Status != "Has Gone Live" || p.GoLiveStatus.State != "success" {
		t.Errorf("project decoded wrong: %+v", p)
	}
}

func TestGetAccountHealthDetailEscapesTheID(t *testing.T) {
	var gotRawPath string
	client := newHealthDetailTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"accountName":"","customerProjects":[]}`))
	})
	if _, err := client.GetAccountHealthDetail(context.Background(), "a/b?c"); err != nil {
		t.Fatalf("GetAccountHealthDetail: %v", err)
	}
	if gotRawPath != "/accounts/a%2Fb%3Fc/health-detail" {
		t.Errorf("escaped path = %q, want the id percent-encoded", gotRawPath)
	}
}

func TestGetAccountHealthDetailSurfacesUpstreamStatus(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		client := newHealthDetailTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
		})
		_, err := client.GetAccountHealthDetail(context.Background(), "00000000-0000-0000-0000-000000000000")
		var apiErr *apierror.Error
		if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
			t.Errorf("status %d: err = %v, want *apierror.Error with that status", status, err)
		}
	}
}
