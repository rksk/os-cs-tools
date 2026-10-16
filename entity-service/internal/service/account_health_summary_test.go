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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

func TestAccountGoLiveStatus(t *testing.T) {
	activation := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	months := func(n int) *int { return &n }
	cases := []struct {
		name       string
		state      string
		months     *int
		activation *time.Time
		want       domain.AccountGoLiveStatus
	}{
		{"success", "success", nil, nil, domain.AccountGoLiveStatus{Status: "Has Gone Live", State: "success"}},
		{"failure", "failure", months(7), &activation, domain.AccountGoLiveStatus{Status: "Hasn't Gone Live in 6 months", IsRisk: true, State: "failure"}},
		{"no start date", "pending", nil, nil, domain.AccountGoLiveStatus{Status: "N/A (No Start Date)", State: "pending"}},
		{"future activation", "pending", nil, &activation, domain.AccountGoLiveStatus{Status: "N/A (Activation Date in Future)", State: "pending"}},
		{"under a month", "pending", months(0), &activation, domain.AccountGoLiveStatus{Status: "Recently Activated (Less than a month)", State: "pending"}},
		{"one month", "pending", months(1), &activation, domain.AccountGoLiveStatus{Status: "Hasn't Gone Live Since 1 month", State: "pending"}},
		{"several months", "pending", months(5), &activation, domain.AccountGoLiveStatus{Status: "Hasn't Gone Live Since 5 months", State: "pending"}},
	}
	for _, tc := range cases {
		if got := accountGoLiveStatus(tc.state, tc.months, tc.activation); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestAccountRowToView_HealthSummaryOnlyWhenRead(t *testing.T) {
	if v := accountRowToView(repository.AccountRow{ID: "a", Name: "n"}); v.HealthSummary != nil {
		t.Fatalf("HealthSummary = %+v for a row without health, want nil", v.HealthSummary)
	}
	row := repository.AccountRow{ID: "a", Name: "n", Health: &repository.AccountHealthRow{
		GoLiveState: "success", HasRecentCases: true, HasEolProduct: true, HealthStatus: "at_risk",
	}}
	got := accountRowToView(row).HealthSummary
	if got == nil {
		t.Fatal("HealthSummary nil, want set")
	}
	if got.GoLiveStatus.State != "success" || !got.HasRecentCases || !got.HasEolProduct || got.NoSupportCases6mo || got.HealthStatus != "at_risk" {
		t.Errorf("unexpected summary %+v", got)
	}
}

func TestSNAccountService_SearchAccounts_RejectsHealthFilters(t *testing.T) {
	svc := NewServiceNowAccountService(nil)
	for name, f := range map[string]domain.SearchAccountsFilters{
		"risks":        {Risks: []string{"hasEolProduct"}},
		"subRegion":    {SubRegion: []string{"ASIA"}},
		"product":      {Product: "Gateway"},
		"abtTeam":      {AbtTeam: "Team Nova"},
		"healthStatus": {HealthStatus: "healthy"},
		"summary":      {IncludeHealthSummary: true},
	} {
		_, err := svc.SearchAccounts(context.Background(), domain.SearchAccountsRequest{
			Pagination: domain.Pagination{Limit: 10},
			Filters:    f,
		})
		var verr *apierror.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
}

func TestSNAccountService_SearchAccounts_UnchangedWithoutHealthFilters(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/accounts/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accounts": [], "totalRecords": 0}`))
	})
	svc := NewServiceNowAccountService(newTestSNClient(t, mux))
	resp, err := svc.SearchAccounts(contextWithUserIDToken("token"), domain.SearchAccountsRequest{
		Pagination: domain.Pagination{Limit: 10},
		Filters:    domain.SearchAccountsFilters{LifeCycle: "Customer", HasProject: true},
	})
	if err != nil || resp.Total != 0 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestAccountViewJSON_HealthSummaryOmittedByDefault(t *testing.T) {
	b, err := json.Marshal(accountRowToView(repository.AccountRow{ID: "a", Name: "n"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "healthSummary") {
		t.Errorf("response must not mention healthSummary by default: %s", b)
	}
}
