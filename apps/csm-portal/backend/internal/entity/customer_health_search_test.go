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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchAccountsWithHealthSendsBodyAndDecodesRows(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("/accounts/search", func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accounts":[{"id":"00000000-0000-0000-0000-000000000000","name":"Example Corp",
			"healthSummary":{"goLiveStatus":{"status":"Go Live","isRisk":false,"state":"success"},
			"hasRecentCases":true,"hasEolProduct":false,"hasAbandonedMigrations":false,"hasMigrationDelays":true,
			"hasRecentEscalations":false,"noSupportCases6mo":false,"healthStatus":"healthy"}}],
			"total":7,"limit":10,"offset":0,"hasMore":false}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewCustomerEntityClient(CustomerEntityConfig{
		BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "c", ClientSecret: "s",
	})

	resp, err := client.SearchAccountsWithHealth(context.Background(), CHAccountSearchRequest{
		Pagination: CHAccountSearchPagination{Limit: 10, Offset: 0},
		Filters: CHAccountSearchFilters{
			LifeCycle: "Customer", HasProject: true, HealthStatus: "healthy", IncludeHealthSummary: true,
			SubRegion: []string{"ASIA"},
		},
	})
	if err != nil {
		t.Fatalf("SearchAccountsWithHealth: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/accounts/search" {
		t.Errorf("request = %s %s, want POST /accounts/search", gotMethod, gotPath)
	}
	const wantBody = `{"pagination":{"limit":10,"offset":0},"filters":{"lifeCycle":"Customer","hasProject":true,"subRegion":["ASIA"],"healthStatus":"healthy","includeHealthSummary":true}}`
	if gotBody != wantBody {
		t.Errorf("body = %s\nwant   %s", gotBody, wantBody)
	}
	if resp.Total != 7 || len(resp.Accounts) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	hs := resp.Accounts[0].HealthSummary
	if hs == nil || hs.HealthStatus != "healthy" || hs.GoLiveStatus.State != "success" || !hs.HasRecentCases || !hs.HasMigrationDelays {
		t.Errorf("health summary = %+v", hs)
	}
}
