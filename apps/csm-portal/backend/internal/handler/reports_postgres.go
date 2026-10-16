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
	"fmt"
	"net/http"
	"strconv"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/viewer"
)

// entityReportsClient is the subset of internal/entity.CustomerEntityClient
// this file needs.
type entityReportsClient interface {
	SearchProjects(ctx context.Context, body []byte) ([]byte, error)
	GetProject(ctx context.Context, id string) ([]byte, error)
	SearchCases(ctx context.Context, body []byte) ([]byte, error)
	SearchTimeCards(ctx context.Context, body []byte) ([]byte, error)
}

type entityTimeCardRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type entityTimeCardView struct {
	ID          string             `json:"id"`
	TotalTime   float64            `json:"totalTime"`
	WorkDate    string             `json:"workDate"`
	HasBillable bool               `json:"hasBillable"`
	State       *string            `json:"state"`
	User        *entityTimeCardRef `json:"user"`
}

type entitySearchTimeCardsRequest struct {
	Pagination entityPagination            `json:"pagination"`
	Filters    entitySearchTimeCardsFilter `json:"filters"`
}

type entitySearchTimeCardsFilter struct {
	CaseID string `json:"caseId,omitempty"`
}

type entitySearchTimeCardsResponse struct {
	TimeCards []entityTimeCardView `json:"timeCards"`
	Total     int                  `json:"total"`
}

// postgresReportsClient implements reportsClient by calling entity-service
// (Postgres). The former SLA and CS reports are retired rather than ported:
// their statistics (SLA percentile breakdowns, monthly case counts,
// subscription/SLA summaries) only ever existed inside ServiceNow scoped-app
// scripts, with no equivalent data or business rules on the Postgres side.
type postgresReportsClient struct {
	entity entityReportsClient
}

// NewPostgresReportsClient builds a postgresReportsClient. entity is
// typically the same *entity.CustomerEntityClient every other CS Portal
// handler already uses.
func NewPostgresReportsClient(entity entityReportsClient) *postgresReportsClient {
	return &postgresReportsClient{entity: entity}
}

// formatHoursMinutes converts a fractional-hours value to the report's
// "1h 2m"/"2m" display format, the same one the original SupportPortalLite
// report used.
func formatHoursMinutes(hours float64) string {
	totalMinutes := int(hours*60 + 0.5) // round to nearest minute
	h := totalMinutes / 60
	m := totalMinutes % 60
	if h == 0 {
		return strconv.Itoa(m) + "m"
	}
	return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
}

// resolveProject resolves the report's projectId query parameter to
// entity-service's project detail. The project detail page links to this
// report with entity-service's own project id (a UUID), while the original
// callers passed the project key, so both are accepted: a UUID is looked up
// directly, anything else goes through resolveProjectByNumber. An unknown id
// of either shape is viewer.ErrProjectNotFound so the route 404s.
func (c *postgresReportsClient) resolveProject(ctx context.Context, projectID string) (entityProjectDetailsView, error) {
	if !uuidRe.MatchString(projectID) {
		return c.resolveProjectByNumber(ctx, projectID)
	}
	detail, err := c.projectDetail(ctx, projectID)
	if err != nil {
		var apiErr *apierror.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return entityProjectDetailsView{}, viewer.ErrProjectNotFound
		}
		return entityProjectDetailsView{}, err
	}
	return detail, nil
}

// projectDetail fetches and decodes GET /projects/{id}.
func (c *postgresReportsClient) projectDetail(ctx context.Context, projectID string) (entityProjectDetailsView, error) {
	detailRaw, err := c.entity.GetProject(ctx, projectID)
	if err != nil {
		return entityProjectDetailsView{}, err
	}
	var detail entityProjectDetailsView
	if err := json.Unmarshal(detailRaw, &detail); err != nil {
		return entityProjectDetailsView{}, fmt.Errorf("unmarshal entity-service project detail response: %w", err)
	}
	return detail, nil
}

// resolveProjectByNumber resolves a project key (the "number" the original
// report callers passed -- see postgresSplProjectClient's own doc comment on
// why Number==Key here) to entity-service's internal project detail.
// Search-then-exact-match, the same pattern used throughout this migration
// for every number-keyed lookup.
func (c *postgresReportsClient) resolveProjectByNumber(ctx context.Context, projectNumber string) (entityProjectDetailsView, error) {
	// Limit is entity-service's own maxLimit (see its SearchProjects
	// validation) -- SearchQuery is a substring match against name/key/
	// subscription type, and the exact-Key match below only looks inside
	// this one page, so a low limit risked missing the target project
	// whenever more than that many projects contained projectNumber as a
	// substring.
	body, err := json.Marshal(entitySearchProjectsRequest{
		Pagination:  entityPagination{Limit: 50, Offset: 0},
		SearchQuery: projectNumber,
	})
	if err != nil {
		return entityProjectDetailsView{}, fmt.Errorf("marshal entity-service projects request: %w", err)
	}
	raw, err := c.entity.SearchProjects(ctx, body)
	if err != nil {
		return entityProjectDetailsView{}, err
	}
	var resp entitySearchProjectsResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return entityProjectDetailsView{}, fmt.Errorf("unmarshal entity-service projects response: %w", err)
	}
	var projectID string
	for _, p := range resp.Projects {
		if p.Key == projectNumber {
			projectID = p.ID
			break
		}
	}
	if projectID == "" {
		return entityProjectDetailsView{}, viewer.ErrProjectNotFound
	}
	return c.projectDetail(ctx, projectID)
}

// entitySearchPageLimit is entity-service's own hard cap on Pagination.Limit
// per request ("limit cannot exceed 50", confirmed against a real running
// instance) -- searchAllCases/searchAllTimeCards page through Total using
// this as the page size, rather than reading a single page and stopping.
const entitySearchPageLimit = 50

// searchAllCases pages through entity-service's SearchCases using body as
// the template request (its Pagination field is overwritten each page),
// returning every case rather than just the first entitySearchPageLimit.
func (c *postgresReportsClient) searchAllCases(ctx context.Context, filters entitySearchCasesFilters, sortBy entityCaseSort) ([]entitySearchCaseView, error) {
	var all []entitySearchCaseView
	for offset := 0; ; offset += entitySearchPageLimit {
		body, err := json.Marshal(entitySearchCasesRequest{
			Filters:    filters,
			SortBy:     sortBy,
			Pagination: entityPagination{Limit: entitySearchPageLimit, Offset: offset},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal entity-service cases-by-project request: %w", err)
		}
		raw, err := c.entity.SearchCases(ctx, body)
		if err != nil {
			return nil, err
		}
		var resp entitySearchCasesResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal entity-service cases-by-project response: %w", err)
		}
		all = append(all, resp.Cases...)
		if len(all) >= resp.Total || len(resp.Cases) == 0 {
			return all, nil
		}
	}
}

// searchAllTimeCards pages through entity-service's SearchTimeCards for a
// single case, returning every time card rather than just the first
// entitySearchPageLimit.
func (c *postgresReportsClient) searchAllTimeCards(ctx context.Context, caseID string) ([]entityTimeCardView, error) {
	var all []entityTimeCardView
	for offset := 0; ; offset += entitySearchPageLimit {
		body, err := json.Marshal(entitySearchTimeCardsRequest{
			Pagination: entityPagination{Limit: entitySearchPageLimit, Offset: offset},
			Filters:    entitySearchTimeCardsFilter{CaseID: caseID},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal entity-service time-cards request: %w", err)
		}
		raw, err := c.entity.SearchTimeCards(ctx, body)
		if err != nil {
			return nil, err
		}
		var resp entitySearchTimeCardsResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal entity-service time-cards response: %w", err)
		}
		all = append(all, resp.TimeCards...)
		if len(all) >= resp.Total || len(resp.TimeCards) == 0 {
			return all, nil
		}
	}
}

// GetTimeLogBreakdown implements reportsClient, paging through every
// case in the project and every time card per case (see
// searchAllCases/searchAllTimeCards) rather than reading a single
// entitySearchPageLimit-sized page and truncating the rest, which ServiceNow's
// own version of this report never did either.
func (c *postgresReportsClient) GetTimeLogBreakdown(ctx context.Context, projectID string) (viewer.TimeLogBreakdownDetails, error) {
	project, err := c.resolveProject(ctx, projectID)
	if err != nil {
		return viewer.TimeLogBreakdownDetails{}, err
	}

	cases, err := c.searchAllCases(ctx,
		entitySearchCasesFilters{Filters: []entityCaseFieldFilter{{Field: "projectId", Op: "in", Values: []string{project.ID}}}},
		entityCaseSort{Field: "createdOn", Order: "desc"})
	if err != nil {
		return viewer.TimeLogBreakdownDetails{}, err
	}

	result := viewer.TimeLogBreakdownDetails{
		ProjectName:         project.Name,
		ProjectKey:          project.Key,
		ProjectType:         project.SubscriptionType,
		RemainingQueryHours: hoursOrEmpty(project.RemainingQueryHours),
		TotalQueryHours:     hoursOrEmpty(project.TotalQueryHours),
		Cases:               make([]viewer.TimeLogBreakdownCase, 0, len(cases)),
	}

	for _, cv := range cases {
		state := derefStr(cv.State)
		displayState, ok := caseStateToDisplay[state]
		if !ok {
			displayState = state
		}

		timeCards, err := c.searchAllTimeCards(ctx, cv.ID)
		if err != nil {
			return viewer.TimeLogBreakdownDetails{}, err
		}

		caseResult := viewer.TimeLogBreakdownCase{
			CaseNumber:       cv.Number,
			CaseID:           cv.InternalID,
			ShortDescription: derefStr(cv.Subject),
			State:            displayState,
			TimeCards:        make([]viewer.TimeCardDetails, 0, len(timeCards)),
		}

		var totalHours, consumedHours float64
		for _, tc := range timeCards {
			totalHours += tc.TotalTime
			tcState := derefStr(tc.State)
			if tc.HasBillable && tcState == "approved" {
				consumedHours += tc.TotalTime
			}
			isBillable := "false"
			if tc.HasBillable {
				isBillable = "true"
			}
			caseResult.TimeCards = append(caseResult.TimeCards, viewer.TimeCardDetails{
				Total:      formatHoursMinutes(tc.TotalTime),
				CreatedOn:  tc.WorkDate,
				IsBillable: isBillable,
				State:      tcState,
			})
		}
		caseResult.TotalHours = formatHoursMinutes(totalHours)
		caseResult.ConsumedQueryHours = formatHoursMinutes(consumedHours)

		result.Cases = append(result.Cases, caseResult)
	}

	return result, nil
}
