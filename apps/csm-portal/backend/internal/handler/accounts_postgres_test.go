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
	"testing"
)

type fakeEntityEscalationsClient struct {
	cases       entitySearchCasesResponse
	escalations entitySearchEscalationsResponse
	gotCaseIDs  []string
}

func (f *fakeEntityEscalationsClient) SearchCases(ctx context.Context, body []byte) ([]byte, error) {
	return json.Marshal(f.cases)
}

func (f *fakeEntityEscalationsClient) SearchEscalations(ctx context.Context, body []byte) ([]byte, error) {
	var req entitySearchEscalationsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	f.gotCaseIDs = req.Filters.CaseIDs
	return json.Marshal(f.escalations)
}

func TestPostgresViewerAccountClient_GetEscalationsByAccount(t *testing.T) {
	fake := &fakeEntityEscalationsClient{
		cases: entitySearchCasesResponse{
			Cases: []entitySearchCaseView{{ID: "case-1"}, {ID: "case-2"}},
			Total: 2,
		},
		escalations: entitySearchEscalationsResponse{
			Escalations: []entityEscalation{
				{ID: "esc-1", CurrentLevel: entityChoiceListItem{ID: "3", Label: "3"}, CreatedOn: "2026-01-01T00:00:00Z"},
				{ID: "esc-2", CurrentLevel: entityChoiceListItem{ID: "0", Label: "0"}, CreatedOn: "2026-01-02T00:00:00Z"},
			},
			Total: 2,
		},
	}
	c := NewPostgresViewerAccountClient(fake)

	got, err := c.GetEscalationsByAccount(context.Background(), "account-1", 0, 10)
	if err != nil {
		t.Fatalf("GetEscalationsByAccount: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d escalations, want 2", len(got))
	}
	if got[0].ID != "esc-1" || got[0].Severity != "3" || got[0].State != "Escalated" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].ID != "esc-2" || got[1].Severity != "0" || got[1].State != "De-escalated" {
		t.Errorf("got[1] = %+v", got[1])
	}
	if len(fake.gotCaseIDs) != 2 {
		t.Errorf("escalations search filtered by %d case ids, want 2", len(fake.gotCaseIDs))
	}
}

func TestPostgresViewerAccountClient_GetEscalationsByAccount_NoCases(t *testing.T) {
	fake := &fakeEntityEscalationsClient{
		cases: entitySearchCasesResponse{Cases: nil, Total: 0},
	}
	c := NewPostgresViewerAccountClient(fake)

	got, err := c.GetEscalationsByAccount(context.Background(), "account-1", 0, 10)
	if err != nil {
		t.Fatalf("GetEscalationsByAccount: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d escalations, want 0", len(got))
	}
}

func TestEscalationState(t *testing.T) {
	if got := escalationState("0"); got != "De-escalated" {
		t.Errorf("escalationState(0) = %q, want De-escalated", got)
	}
	for _, level := range []string{"1", "2", "3", "4", "5"} {
		if got := escalationState(level); got != "Escalated" {
			t.Errorf("escalationState(%s) = %q, want Escalated", level, got)
		}
	}
}
