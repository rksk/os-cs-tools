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

// snFeedbackEmojiChip mirrors the Choreo FeedbackEmojiChip shape.
type snFeedbackEmojiChip struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// snFeedbackEmoji mirrors the Choreo FeedbackEmoji shape.
type snFeedbackEmoji struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Value           string                `json:"value"`
	UnselectedImage string                `json:"unselectedImage"`
	SelectedImage   string                `json:"selectedImage"`
	Chips           []snFeedbackEmojiChip `json:"chips"`
}

func (e snFeedbackEmoji) toDomain() domain.FeedbackEmoji {
	chips := make([]domain.FeedbackEmojiChip, 0, len(e.Chips))
	for _, c := range e.Chips {
		chips = append(chips, domain.FeedbackEmojiChip{ID: sysidToUUID(c.ID), Name: c.Name, Value: c.Value})
	}
	return domain.FeedbackEmoji{
		ID:              sysidToUUID(e.ID),
		Name:            e.Name,
		Value:           e.Value,
		UnselectedImage: e.UnselectedImage,
		SelectedImage:   e.SelectedImage,
		Chips:           chips,
	}
}

// snSystemMetadataResponse mirrors the Choreo GET /metadata response.
type snSystemMetadataResponse struct {
	TimeZones      []snChoiceOption       `json:"timeZones"`
	ProjectTypes   []snReferenceTableItem `json:"projectTypes"`
	FeedbackEmojis []snFeedbackEmoji      `json:"feedbackEmojies"`
}

// snGlobalSearchFilters mirrors the Choreo GlobalSearchPayload.filters shape.
type snGlobalSearchFilters struct {
	SearchQuery string   `json:"searchQuery,omitempty"`
	Tables      []string `json:"tables,omitempty"`
}

// snGlobalSearchSort mirrors the Choreo GlobalSearchPayload.sortBy shape.
type snGlobalSearchSort struct {
	Field string `json:"field,omitempty"`
	Order string `json:"order,omitempty"`
}

// snGlobalSearchPayload mirrors the Choreo POST /search request body.
type snGlobalSearchPayload struct {
	Filters            *snGlobalSearchFilters `json:"filters,omitempty"`
	SortBy             *snGlobalSearchSort    `json:"sortBy,omitempty"`
	ProjectsPagination *snProjectPagination   `json:"projectsPagination,omitempty"`
	CasesPagination    *snProjectPagination   `json:"casesPagination,omitempty"`
}

// snGlobalSearchProject mirrors a project row in the Choreo POST /search response.
type snGlobalSearchProject struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Description         *string              `json:"description"`
	Key                 string               `json:"key"`
	Type                snReferenceTableItem `json:"type"`
	CreatedOn           string               `json:"createdOn"`
	StartDate           *string              `json:"startDate"`
	EndDate             *string              `json:"endDate"`
	HasPdpSubscription  bool                 `json:"hasPdpSubscription"`
	ClosureState        *string              `json:"closureState"`
	Account             snReferenceTableItem `json:"account"`
	ActiveChatsCount    int                  `json:"activeChatsCount"`
	ActionRequiredCount int                  `json:"actionRequiredCount"`
	OutstandingCount    int                  `json:"outstandingCount"`
}

func (p snGlobalSearchProject) toDomain() domain.GlobalSearchProject {
	return domain.GlobalSearchProject{
		ID:                  sysidToUUID(p.ID),
		Name:                p.Name,
		Description:         p.Description,
		Key:                 p.Key,
		Type:                p.Type.toDomain(),
		CreatedOn:           p.CreatedOn,
		StartDate:           p.StartDate,
		EndDate:             p.EndDate,
		HasPdpSubscription:  p.HasPdpSubscription,
		ClosureState:        p.ClosureState,
		Account:             p.Account.toDomain(),
		ActiveChatsCount:    p.ActiveChatsCount,
		ActionRequiredCount: p.ActionRequiredCount,
		OutstandingCount:    p.OutstandingCount,
	}
}

// snGlobalSearchCase mirrors a case row in the Choreo POST /search response.
// InternalID is the WSO2-internal case identifier (a separate concept from
// the sysid), passed through unchanged like everywhere else it appears.
type snGlobalSearchCase struct {
	ID               string                `json:"id"`
	InternalID       string                `json:"internalId"`
	Number           string                `json:"number"`
	Title            *string               `json:"title"`
	Description      *string               `json:"description"`
	CreatedOn        string                `json:"createdOn"`
	CreatedBy        string                `json:"createdBy"`
	UpdatedOn        string                `json:"updatedOn"`
	Project          *snReferenceTableItem `json:"project"`
	CaseType         *snReferenceTableItem `json:"caseType"`
	State            *snChoiceOption       `json:"state"`
	Severity         *snChoiceOption       `json:"severity"`
	AssignedEngineer *snReferenceTableItem `json:"assignedEngineer"`
	Account          snReferenceTableItem  `json:"account"`
}

func (c snGlobalSearchCase) toDomain() domain.GlobalSearchCase {
	out := domain.GlobalSearchCase{
		ID:          sysidToUUID(c.ID),
		InternalID:  c.InternalID,
		Number:      c.Number,
		Title:       c.Title,
		Description: c.Description,
		CreatedOn:   c.CreatedOn,
		CreatedBy:   c.CreatedBy,
		UpdatedOn:   c.UpdatedOn,
		Account:     c.Account.toDomain(),
	}
	if c.Project != nil {
		v := c.Project.toDomain()
		out.Project = &v
	}
	if c.CaseType != nil {
		v := c.CaseType.toDomain()
		out.CaseType = &v
	}
	if c.State != nil {
		v := c.State.toDomain()
		out.State = &v
	}
	if c.Severity != nil {
		v := c.Severity.toDomain()
		out.Severity = &v
	}
	if c.AssignedEngineer != nil {
		v := c.AssignedEngineer.toDomain()
		out.AssignedEngineer = &v
	}
	return out
}

// snGlobalSearchResponse mirrors the Choreo POST /search response.
type snGlobalSearchResponse struct {
	Query         string                  `json:"query"`
	ProjectsTotal int                     `json:"projectsTotal"`
	CasesTotal    int                     `json:"casesTotal"`
	Projects      []snGlobalSearchProject `json:"projects"`
	Cases         []snGlobalSearchCase    `json:"cases"`
}

var validGlobalSearchTables = map[string]bool{"projects": true, "cases": true}
var validGlobalSearchSortOrder = map[string]bool{"asc": true, "desc": true}

type snGlobalService struct {
	client *integrationservice.Client
}

// NewServiceNowGlobalService constructs a GlobalService backed by the Choreo API.
func NewServiceNowGlobalService(client *integrationservice.Client) GlobalService {
	return &snGlobalService{client: client}
}

func (s *snGlobalService) GetSystemMetadata(ctx context.Context) (domain.SystemMetadataResponse, error) {
	token := middleware.UserIDTokenFromContext(ctx)

	raw, err := s.client.Get(ctx, "/metadata", token)
	if err != nil {
		return domain.SystemMetadataResponse{}, err
	}

	var snResp snSystemMetadataResponse
	if err := json.Unmarshal(raw, &snResp); err != nil {
		return domain.SystemMetadataResponse{}, fmt.Errorf("sn system metadata: parse response: %w", err)
	}

	emojis := make([]domain.FeedbackEmoji, 0, len(snResp.FeedbackEmojis))
	for _, e := range snResp.FeedbackEmojis {
		emojis = append(emojis, e.toDomain())
	}

	return domain.SystemMetadataResponse{
		TimeZones:      toDomainChoiceListItems(snResp.TimeZones),
		ProjectTypes:   toDomainReferenceTableItems(snResp.ProjectTypes),
		FeedbackEmojis: emojis,
	}, nil
}

func (s *snGlobalService) GlobalSearch(ctx context.Context, req domain.GlobalSearchRequest) (domain.GlobalSearchResponse, error) {
	var query string
	if req.Filters != nil {
		query = normalizeGlobalSearchQuery(req.Filters.SearchQuery)
		if err := validateSearchQuery(query); err != nil {
			return domain.GlobalSearchResponse{}, err
		}
		for _, t := range req.Filters.Tables {
			if !validGlobalSearchTables[t] {
				return domain.GlobalSearchResponse{}, &apierror.ValidationError{Msg: "filters.tables contains invalid value: " + t}
			}
		}
	}
	if req.SortBy != nil && req.SortBy.Order != "" && !validGlobalSearchSortOrder[req.SortBy.Order] {
		return domain.GlobalSearchResponse{}, &apierror.ValidationError{Msg: "sortBy.order contains invalid value: " + req.SortBy.Order}
	}
	if req.ProjectsPagination != nil {
		if err := normalizePagination(req.ProjectsPagination); err != nil {
			return domain.GlobalSearchResponse{}, err
		}
	}
	if req.CasesPagination != nil {
		if err := normalizePagination(req.CasesPagination); err != nil {
			return domain.GlobalSearchResponse{}, err
		}
	}

	token := middleware.UserIDTokenFromContext(ctx)

	payload := snGlobalSearchPayload{}
	if req.Filters != nil {
		payload.Filters = &snGlobalSearchFilters{SearchQuery: query, Tables: req.Filters.Tables}
	}
	if req.SortBy != nil {
		payload.SortBy = &snGlobalSearchSort{Field: req.SortBy.Field, Order: req.SortBy.Order}
	}
	if req.ProjectsPagination != nil {
		payload.ProjectsPagination = &snProjectPagination{Limit: req.ProjectsPagination.Limit, Offset: req.ProjectsPagination.Offset}
	}
	if req.CasesPagination != nil {
		payload.CasesPagination = &snProjectPagination{Limit: req.CasesPagination.Limit, Offset: req.CasesPagination.Offset}
	}

	raw, err := s.client.Post(ctx, "/search", token, payload)
	if err != nil {
		return domain.GlobalSearchResponse{}, err
	}

	var snResp snGlobalSearchResponse
	if err := json.Unmarshal(raw, &snResp); err != nil {
		return domain.GlobalSearchResponse{}, fmt.Errorf("sn global search: parse response: %w", err)
	}

	projects := make([]domain.GlobalSearchProject, 0, len(snResp.Projects))
	for _, p := range snResp.Projects {
		projects = append(projects, p.toDomain())
	}
	cases := make([]domain.GlobalSearchCase, 0, len(snResp.Cases))
	for _, c := range snResp.Cases {
		cases = append(cases, c.toDomain())
	}

	respQuery := snResp.Query
	if req.Filters != nil {
		respQuery = query
	}

	return domain.GlobalSearchResponse{
		Query:         respQuery,
		ProjectsTotal: snResp.ProjectsTotal,
		CasesTotal:    snResp.CasesTotal,
		Projects:      projects,
		Cases:         cases,
	}, nil
}
