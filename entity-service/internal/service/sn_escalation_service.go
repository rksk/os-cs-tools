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
	"fmt"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/middleware"
	integrationservice "github.com/wso2-open-operations/cs-tools/entity-service/internal/servicenow-integration-service"
)

// validEscalationAction is the set of accepted CreateEscalationRequest.Action
// values. Mirrors the backing service's own default: an absent action means
// ESCALATE.
var validEscalationAction = map[domain.EscalationAction]bool{
	domain.EscalationActionEscalate:   true,
	domain.EscalationActionDeescalate: true,
}

// snEscalationCaseRef mirrors the case reference embedded in an escalation record.
type snEscalationCaseRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// snEscalationChoiceItem mirrors a ServiceNow choice-list {id, label} pair, used
// for currentLevel/previousLevel. ID is one of validEscalationLevel's keys
// ("0" through "5").
type snEscalationChoiceItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// snEscalationNotifiedUser mirrors one entry of an escalation's
// notificationSentTo list.
type snEscalationNotifiedUser struct {
	ID       *string `json:"id"`
	UserName string  `json:"userName"`
	Name     *string `json:"name"`
	Email    *string `json:"email"`
}

// snEscalation mirrors one record in the backing service's escalation
// search/create responses.
type snEscalation struct {
	ID                 string                     `json:"id"`
	Case               snEscalationCaseRef        `json:"case"`
	CurrentLevel       snEscalationChoiceItem     `json:"currentLevel"`
	PreviousLevel      snEscalationChoiceItem     `json:"previousLevel"`
	CreatedBy          string                     `json:"createdBy"`
	CreatedOn          string                     `json:"createdOn"`
	UpdatedOn          string                     `json:"updatedOn"`
	Reason             *string                    `json:"reason"`
	NotificationSentTo []snEscalationNotifiedUser `json:"notificationSentTo"`
}

// snEscalationSearchFilters mirrors the POST /escalations/search request body's
// filters object.
type snEscalationSearchFilters struct {
	CaseIDs       []string `json:"caseIds,omitempty"`
	CurrentLevels []int    `json:"currentLevels,omitempty"`
}

// snEscalationSort mirrors the POST /escalations/search request body's sortBy object.
type snEscalationSort struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

// snEscalationSearchPayload is the POST /escalations/search request body.
type snEscalationSearchPayload struct {
	Filters    *snEscalationSearchFilters `json:"filters,omitempty"`
	SortBy     *snEscalationSort          `json:"sortBy,omitempty"`
	Pagination snProjectPagination        `json:"pagination"`
}

// snEscalationSearchResponse is the POST /escalations/search response body.
type snEscalationSearchResponse struct {
	Escalations  []snEscalation `json:"escalations"`
	TotalRecords int            `json:"totalRecords"`
	Offset       int            `json:"offset"`
	Limit        int            `json:"limit"`
}

// snCreateEscalationPayload is the POST /escalations request body.
type snCreateEscalationPayload struct {
	CaseID string  `json:"caseId"`
	Reason *string `json:"reason,omitempty"`
	Action *string `json:"action,omitempty"`
}

// snCreateEscalationResponse is the POST /escalations response body.
type snCreateEscalationResponse struct {
	Message    string       `json:"message"`
	Escalation snEscalation `json:"escalation"`
}

// escalationSearchLimit is the fixed page size used when reading a case's
// escalation history. History lists are short by nature (a handful of
// escalate/de-escalate steps per case at most) so a single unpaginated page
// covers every real case; this also keeps the read endpoint's contract simple
// (no caller-supplied pagination params yet).
const escalationSearchLimit = 100

func snEscalationToDomain(ctx context.Context, e snEscalation) (domain.Escalation, error) {
	createdOn, err := parseSNDateTime(ctx, "SearchEscalations", "createdOn", e.CreatedOn)
	if err != nil {
		return domain.Escalation{}, fmt.Errorf("sn escalation: parse createdOn: %w", err)
	}
	updatedOn, err := parseSNDateTime(ctx, "SearchEscalations", "updatedOn", e.UpdatedOn)
	if err != nil {
		return domain.Escalation{}, fmt.Errorf("sn escalation: parse updatedOn: %w", err)
	}

	var notified []domain.EscalationNotifiedUser
	for _, u := range e.NotificationSentTo {
		ref := domain.EscalationNotifiedUser{
			UserName: u.UserName,
			Name:     u.Name,
			Email:    u.Email,
		}
		if u.ID != nil {
			id := sysidToUUID(*u.ID)
			ref.ID = &id
		}
		notified = append(notified, ref)
	}

	return domain.Escalation{
		ID:            sysidToUUID(e.ID),
		CaseID:        sysidToUUID(e.Case.ID),
		CurrentLevel:  e.CurrentLevel.ID,
		PreviousLevel: e.PreviousLevel.ID,
		CreatedBy:     e.CreatedBy,
		CreatedOn:     createdOn,
		UpdatedOn:     updatedOn,
		Reason:        e.Reason,
		NotifiedUsers: notified,
	}, nil
}

type snEscalationService struct {
	client *integrationservice.Client
}

// NewServiceNowEscalationService constructs an EscalationService backed by the
// backing service's shared escalation endpoints (already deployed and unchanged
// by this service — see /escalations and /escalations/search).
func NewServiceNowEscalationService(client *integrationservice.Client) EscalationService {
	return &snEscalationService{client: client}
}

// SearchEscalations implements EscalationService.
func (s *snEscalationService) SearchEscalations(ctx context.Context, caseID string) (domain.SearchEscalationsResponse, error) {
	token := middleware.UserIDTokenFromContext(ctx)

	if err := validateUUIDs("caseId", []string{caseID}); err != nil {
		return domain.SearchEscalationsResponse{}, err
	}

	payload := snEscalationSearchPayload{
		Filters: &snEscalationSearchFilters{
			CaseIDs: []string{uuidToSysid(caseID)},
		},
		Pagination: snProjectPagination{Limit: escalationSearchLimit, Offset: 0},
	}

	raw, err := s.client.Post(ctx, "/escalations/search", token, payload)
	if err != nil {
		return domain.SearchEscalationsResponse{}, err
	}

	var snResp snEscalationSearchResponse
	if err := json.Unmarshal(raw, &snResp); err != nil {
		return domain.SearchEscalationsResponse{}, fmt.Errorf("sn search escalations: parse response: %w", err)
	}

	escalations := make([]domain.Escalation, 0, len(snResp.Escalations))
	for _, e := range snResp.Escalations {
		view, err := snEscalationToDomain(ctx, e)
		if err != nil {
			return domain.SearchEscalationsResponse{}, err
		}
		escalations = append(escalations, view)
	}

	return domain.SearchEscalationsResponse{
		Escalations: escalations,
		Total:       snResp.TotalRecords,
		Limit:       snResp.Limit,
		Offset:      snResp.Offset,
	}, nil
}

// CreateEscalation implements EscalationService.
func (s *snEscalationService) CreateEscalation(ctx context.Context, caseID string, reason *string, action *domain.EscalationAction) (domain.Escalation, error) {
	token := middleware.UserIDTokenFromContext(ctx)

	if err := validateUUIDs("caseId", []string{caseID}); err != nil {
		return domain.Escalation{}, err
	}

	effectiveAction := domain.EscalationActionEscalate
	if action != nil {
		if !validEscalationAction[*action] {
			return domain.Escalation{}, &apierror.ValidationError{Msg: "action contains invalid value: " + string(*action)}
		}
		effectiveAction = *action
	}

	if effectiveAction == domain.EscalationActionEscalate && (reason == nil || *reason == "") {
		return domain.Escalation{}, &apierror.ValidationError{Msg: "reason is required when escalating"}
	}

	payload := snCreateEscalationPayload{
		CaseID: uuidToSysid(caseID),
		Reason: reason,
	}
	if action != nil {
		actionStr := string(effectiveAction)
		payload.Action = &actionStr
	}

	raw, err := s.client.Post(ctx, "/escalations", token, payload)
	if err != nil {
		return domain.Escalation{}, err
	}

	var snResp snCreateEscalationResponse
	if err := json.Unmarshal(raw, &snResp); err != nil {
		return domain.Escalation{}, fmt.Errorf("sn create escalation: parse response: %w", err)
	}

	return snEscalationToDomain(ctx, snResp.Escalation)
}
