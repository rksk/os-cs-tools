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
	"sort"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/viewer"
)

// entityEscalationsClient is the subset of internal/entity.CustomerEntityClient
// needed to list an account's escalations from Postgres.
type entityEscalationsClient interface {
	SearchCases(ctx context.Context, body []byte) ([]byte, error)
	SearchEscalations(ctx context.Context, body []byte) ([]byte, error)
}

type entityChoiceListItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type entitySearchEscalationsFilters struct {
	CaseIDs []string `json:"caseIds,omitempty"`
}

type entityEscalationSort struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

type entitySearchEscalationsRequest struct {
	Filters    entitySearchEscalationsFilters `json:"filters"`
	SortBy     entityEscalationSort           `json:"sortBy"`
	Pagination entityPagination               `json:"pagination"`
}

type entityEscalation struct {
	ID            string               `json:"id"`
	Case          entityRef            `json:"case"`
	CurrentLevel  entityChoiceListItem `json:"currentLevel"`
	PreviousLevel entityChoiceListItem `json:"previousLevel"`
	CreatedOn     string               `json:"createdOn"`
}

type entitySearchEscalationsResponse struct {
	Escalations []entityEscalation `json:"escalations"`
	Total       int                `json:"total"`
}

// accountEscalationCaseLookupLimit bounds how many of the account's own
// cases are read to build the "which cases belong to this account" set
// SearchEscalations is then filtered by -- entity-service's own per-request
// cap (SearchCases rejects a limit above 50, same as every other search in
// that service), so this pages through like GetProductList/GetABTTeamList
// do, up to a safety bound rather than assuming one page covers it. Sized at
// 2000 cases (not the original 1000): a real account checked directly against
// staging has 1,585 cases, which the original 1000-case cap would have
// silently truncated, missing that account's own escalations on any case
// past the cutoff.
const (
	accountEscalationCasePageLimit = 50
	accountEscalationCasePageCap   = 40 // 40 * 50 = 2000 cases
)

// postgresViewerAccountClient implements viewerAccountClient.
// GetEscalationsByAccount reads from entity-service (Postgres): an
// account's escalations are its cases' own case_escalation history
// (work_item.account_id is a direct column, confirmed against real data --
// 859 escalation rows, 349 with an account linkage through their case). This
// is a materially different data model from ServiceNow's
// sn_customerservice_escalation (one escalation record can group several
// cases under a single "source_record" link to the account) -- entity-
// service has no such account-level grouping concept, only a per-case
// current_level/previous_level history. There is no 1:1 mapping, so each
// case_escalation row is surfaced here as its own row, severity is the raw
// current-level label (e.g. "EL3") rather than ServiceNow's "High Severity"/
// "Medium Severity" picklist (no equivalent column exists), and state is
// derived (EL0 = de-escalated back to baseline, anything else = escalated)
// since case_escalation has no open/closed column of its own.
//
// The actual "escalate a case" action is not part of this type:
// CsmAccountDetailPage.tsx's EscalationsSection is read-only by design (see
// its own doc comment), and the real escalate UI (EscalateCaseDialog.tsx)
// goes through the entity-service-backed POST /cases/{id}/escalations.
type postgresViewerAccountClient struct {
	entity entityEscalationsClient
}

// NewPostgresViewerAccountClient builds a postgresViewerAccountClient.
// entity is the same *entity.CustomerEntityClient every other CS Portal
// handler uses.
func NewPostgresViewerAccountClient(entity entityEscalationsClient) *postgresViewerAccountClient {
	return &postgresViewerAccountClient{entity: entity}
}

// entityEscalationsSearchMaxLimit is entity-service's own hard cap on
// POST /escalations/search's pagination.limit (confirmed directly: a value
// above it is a 400 "limit cannot exceed 50") -- GetEscalationsByAccount's
// own caller-facing limit can be up to maxPaginationLimit (100, accounts.go),
// so a request for more than 50 rows has to be satisfied by more than one
// entity-service page.
const entityEscalationsSearchMaxLimit = 50

// GetEscalationsByAccount implements viewerAccountClient.
func (c *postgresViewerAccountClient) GetEscalationsByAccount(ctx context.Context, accountID string, offset, limit int) ([]viewer.EscalationDetail, error) {
	caseIDs, err := c.accountCaseIDs(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if len(caseIDs) == 0 {
		return []viewer.EscalationDetail{}, nil
	}

	results := make([]viewer.EscalationDetail, 0, limit)
	for fetched, pageOffset := 0, offset; fetched < limit; {
		pageLimit := limit - fetched
		if pageLimit > entityEscalationsSearchMaxLimit {
			pageLimit = entityEscalationsSearchMaxLimit
		}
		body, err := json.Marshal(entitySearchEscalationsRequest{
			Filters:    entitySearchEscalationsFilters{CaseIDs: caseIDs},
			SortBy:     entityEscalationSort{Field: "createdOn", Order: "desc"},
			Pagination: entityPagination{Limit: pageLimit, Offset: pageOffset},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal entity-service escalations request: %w", err)
		}
		raw, err := c.entity.SearchEscalations(ctx, body)
		if err != nil {
			return nil, err
		}
		var resp entitySearchEscalationsResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal entity-service escalations response: %w", err)
		}
		for _, e := range resp.Escalations {
			results = append(results, viewer.EscalationDetail{
				ID:          e.ID,
				EscalatedOn: e.CreatedOn,
				State:       escalationState(e.CurrentLevel.ID),
				Severity:    e.CurrentLevel.Label,
			})
		}
		fetched += len(resp.Escalations)
		pageOffset += len(resp.Escalations)
		if len(resp.Escalations) < pageLimit || pageOffset >= resp.Total {
			break
		}
	}
	return results, nil
}

// escalationState derives a state label from the current escalation level --
// case_escalation has no open/closed column of its own (see this file's own
// doc comment). entity-service's own wire shape for CurrentLevel.ID is the
// bare level number ("0".."5"), NOT the raw case_escalation_level_enum label
// ("EL0".."EL5") the database column itself stores -- confirmed directly
// against a real escalation (currentLevel: {id: "1", label: "1"}), the same
// "EL" prefix stripped" convention GetCaseByID's own EscalationLevel field
// already uses. "0" is the baseline a case starts at and returns to once
// de-escalated, anything else means it is still actively escalated.
func escalationState(currentLevelID string) string {
	if currentLevelID == "0" {
		return "De-escalated"
	}
	return "Escalated"
}

// accountCaseIDs pages through the account's own cases (work_item.account_id,
// a real, direct column) to build the case id set GetEscalationsByAccount
// filters escalations by.
func (c *postgresViewerAccountClient) accountCaseIDs(ctx context.Context, accountID string) ([]string, error) {
	var caseIDs []string
	for page := 0; page < accountEscalationCasePageCap; page++ {
		body, err := json.Marshal(entitySearchCasesRequest{
			Filters: entitySearchCasesFilters{
				Filters: []entityCaseFieldFilter{{Field: "accountId", Op: "in", Values: []string{accountID}}},
			},
			Pagination: entityPagination{Limit: accountEscalationCasePageLimit, Offset: page * accountEscalationCasePageLimit},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal entity-service cases request: %w", err)
		}
		raw, err := c.entity.SearchCases(ctx, body)
		if err != nil {
			return nil, err
		}
		var resp entitySearchCasesResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal entity-service cases response: %w", err)
		}
		for _, cv := range resp.Cases {
			caseIDs = append(caseIDs, cv.ID)
		}
		if len(resp.Cases) < accountEscalationCasePageLimit || (page+1)*accountEscalationCasePageLimit >= resp.Total {
			break
		}
	}
	sort.Strings(caseIDs)
	return caseIDs, nil
}
