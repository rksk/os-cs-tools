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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Typed client for entity-service's customer-health risk-tracking feature
// (migration 0219), which replaced apps/csm-portal/backend's own standalone
// MySQL internal/risk package. Like onboarding.go, these methods are typed
// rather than raw passthrough, since internal/risk reshapes ids (UUID <->
// upstream sys_id) and field names before handing a response to the
// customer-health handlers.

// CHProjectHealthStatus is entity-service's project_health_status row.
type CHProjectHealthStatus struct {
	ID              string  `json:"id"`
	ProjectID       string  `json:"projectId"`
	AccountID       string  `json:"accountId"`
	Status          string  `json:"status"`
	ReviewedByEmail *string `json:"reviewedByEmail"`
	ReviewedOn      *string `json:"reviewedOn"`
}

// CHRiskActionItem is entity-service's risk_action_item row.
type CHRiskActionItem struct {
	ID                string  `json:"id"`
	RiskID            string  `json:"riskId"`
	ProjectID         string  `json:"projectId"`
	AccountID         string  `json:"accountId"`
	Title             string  `json:"title"`
	Description       *string `json:"description"`
	Priority          string  `json:"priority"`
	Status            string  `json:"status"`
	AssignedToEmail   *string `json:"assignedToEmail"`
	DueDate           *string `json:"dueDate"`
	ResolutionComment *string `json:"resolutionComment"`
	ResolvedByEmail   *string `json:"resolvedByEmail"`
	ResolvedOn        *string `json:"resolvedOn"`
	CreatedByEmail    string  `json:"createdByEmail"`
	CreatedOn         string  `json:"createdOn"`
	UpdatedOn         string  `json:"updatedOn"`
	CommentCount      int     `json:"commentCount"`
}

// CHProjectRisk is entity-service's project_risk row, with its action items.
type CHProjectRisk struct {
	ID            string             `json:"id"`
	ProjectID     string             `json:"projectId"`
	AccountID     string             `json:"accountId"`
	Status        string             `json:"status"`
	OpenedComment string             `json:"openedComment"`
	OpenedByEmail string             `json:"openedByEmail"`
	OpenedOn      string             `json:"openedOn"`
	ClosedComment *string            `json:"closedComment"`
	ClosedByEmail *string            `json:"closedByEmail"`
	ClosedOn      *string            `json:"closedOn"`
	ActionItems   []CHRiskActionItem `json:"actionItems"`
}

// CHProjectHealthWithOpenRisk pairs a project's health status with its
// currently open risk, if any.
type CHProjectHealthWithOpenRisk struct {
	ProjectID    string                `json:"projectId"`
	HealthStatus CHProjectHealthStatus `json:"healthStatus"`
	OpenRisk     *CHProjectRisk        `json:"openRisk"`
}

// CHAccountHealthSummary is an account's aggregated overall health status.
type CHAccountHealthSummary struct {
	AccountID     string `json:"accountId"`
	OverallStatus string `json:"overallStatus"`
}

// CHActionItemComment is entity-service's action_item_comment row.
type CHActionItemComment struct {
	ID             string `json:"id"`
	ActionItemID   string `json:"actionItemId"`
	Comment        string `json:"comment"`
	CreatedByEmail string `json:"createdByEmail"`
	CreatedOn      string `json:"createdOn"`
}

type chOpenProjectRiskRequest struct {
	Comment string `json:"comment"`
}

type chCloseProjectRiskRequest struct {
	Comment string `json:"comment"`
}

type chMarkProjectHealthyRequest struct {
	Comment *string `json:"comment,omitempty"`
}

type chCreateRiskActionItemRequest struct {
	Title           string  `json:"title"`
	Description     *string `json:"description,omitempty"`
	Priority        string  `json:"priority"`
	AssignedToEmail *string `json:"assignedToEmail,omitempty"`
	DueDate         string  `json:"dueDate"`
}

type chUpdateRiskActionItemStatusRequest struct {
	Status            string  `json:"status"`
	ResolutionComment *string `json:"resolutionComment,omitempty"`
}

type chUpdateRiskActionItemRequest struct {
	Title           string  `json:"title"`
	Description     *string `json:"description,omitempty"`
	Priority        string  `json:"priority"`
	AssignedToEmail *string `json:"assignedToEmail,omitempty"`
	DueDate         *string `json:"dueDate,omitempty"`
}

type chCreateActionItemCommentRequest struct {
	Comment string `json:"comment"`
}

type chInitProjectHealthTrackingRequest struct {
	ProjectIDs []string `json:"projectIds"`
}

type chBatchAccountHealthSummariesRequest struct {
	AccountIDs []string `json:"accountIds"`
}

// CHBatchAccountHealthSummariesResponse is POST
// /accounts/health-summaries/search's response.
type CHBatchAccountHealthSummariesResponse struct {
	Summaries map[string]string `json:"summaries"`
}

type chAccountsByHealthStatusRequest struct {
	Status string `json:"status"`
}

type chAccountsByHealthStatusResponse struct {
	AccountIDs []string `json:"accountIds"`
}

type chGetAccountProjectHealthStatusesResponse struct {
	Projects []CHProjectHealthWithOpenRisk `json:"projects"`
}

type chGetProjectRiskHistoryResponse struct {
	Risks []CHProjectRisk `json:"risks"`
}

type chGetRiskActionItemsResponse struct {
	ActionItems []CHRiskActionItem `json:"actionItems"`
}

type chGetActionItemCommentsResponse struct {
	Comments []CHActionItemComment `json:"comments"`
}

func (c *CustomerEntityClient) chDo(ctx context.Context, method, path string, reqBody any, respBody any) error {
	var encoded []byte
	if reqBody != nil {
		var err error
		encoded, err = json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("entity: encode customer health request: %w", err)
		}
	}
	raw, err := c.do(ctx, method, path, encoded)
	if err != nil {
		return err
	}
	if respBody == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, respBody); err != nil {
		return fmt.Errorf("entity: decode customer health response: %w", err)
	}
	return nil
}

// OpenProjectRisk calls POST /projects/{id}/risk.
func (c *CustomerEntityClient) OpenProjectRisk(ctx context.Context, projectID, comment string) (CHProjectRisk, error) {
	var resp CHProjectRisk
	path := fmt.Sprintf("/projects/%s/risk", url.PathEscape(projectID))
	err := c.chDo(ctx, http.MethodPost, path, chOpenProjectRiskRequest{Comment: comment}, &resp)
	return resp, err
}

// CloseProjectRisk calls PUT /risks/{id}/close.
func (c *CustomerEntityClient) CloseProjectRisk(ctx context.Context, riskID, comment string) (CHProjectRisk, error) {
	var resp CHProjectRisk
	path := fmt.Sprintf("/risks/%s/close", url.PathEscape(riskID))
	err := c.chDo(ctx, http.MethodPut, path, chCloseProjectRiskRequest{Comment: comment}, &resp)
	return resp, err
}

// MarkProjectHealthy calls POST /projects/{id}/mark-healthy.
func (c *CustomerEntityClient) MarkProjectHealthy(ctx context.Context, projectID string, comment *string) (CHProjectHealthStatus, error) {
	var resp CHProjectHealthStatus
	path := fmt.Sprintf("/projects/%s/mark-healthy", url.PathEscape(projectID))
	err := c.chDo(ctx, http.MethodPost, path, chMarkProjectHealthyRequest{Comment: comment}, &resp)
	return resp, err
}

// RevertProjectHealth calls POST /projects/{id}/revert-health.
func (c *CustomerEntityClient) RevertProjectHealth(ctx context.Context, projectID string) (CHProjectHealthStatus, error) {
	var resp CHProjectHealthStatus
	path := fmt.Sprintf("/projects/%s/revert-health", url.PathEscape(projectID))
	err := c.chDo(ctx, http.MethodPost, path, nil, &resp)
	return resp, err
}

// GetAccountProjectHealthStatuses calls GET /accounts/{id}/project-health-statuses.
func (c *CustomerEntityClient) GetAccountProjectHealthStatuses(ctx context.Context, accountID string) ([]CHProjectHealthWithOpenRisk, error) {
	var resp chGetAccountProjectHealthStatusesResponse
	path := fmt.Sprintf("/accounts/%s/project-health-statuses", url.PathEscape(accountID))
	err := c.chDo(ctx, http.MethodGet, path, nil, &resp)
	return resp.Projects, err
}

// GetAccountHealthSummary calls GET /accounts/{id}/health-summary.
func (c *CustomerEntityClient) GetAccountHealthSummary(ctx context.Context, accountID string) (CHAccountHealthSummary, error) {
	var resp CHAccountHealthSummary
	path := fmt.Sprintf("/accounts/%s/health-summary", url.PathEscape(accountID))
	err := c.chDo(ctx, http.MethodGet, path, nil, &resp)
	return resp, err
}

// GetBatchAccountHealthSummaries calls POST /accounts/health-summaries/search.
func (c *CustomerEntityClient) GetBatchAccountHealthSummaries(ctx context.Context, accountIDs []string) (map[string]string, error) {
	var resp CHBatchAccountHealthSummariesResponse
	err := c.chDo(ctx, http.MethodPost, "/accounts/health-summaries/search", chBatchAccountHealthSummariesRequest{AccountIDs: accountIDs}, &resp)
	return resp.Summaries, err
}

// GetAccountsByHealthStatus calls POST /accounts/by-health-status/search.
func (c *CustomerEntityClient) GetAccountsByHealthStatus(ctx context.Context, status string) ([]string, error) {
	var resp chAccountsByHealthStatusResponse
	err := c.chDo(ctx, http.MethodPost, "/accounts/by-health-status/search", chAccountsByHealthStatusRequest{Status: status}, &resp)
	return resp.AccountIDs, err
}

// GetProjectRiskHistory calls GET /projects/{id}/risk-history.
func (c *CustomerEntityClient) GetProjectRiskHistory(ctx context.Context, projectID string) ([]CHProjectRisk, error) {
	var resp chGetProjectRiskHistoryResponse
	path := fmt.Sprintf("/projects/%s/risk-history", url.PathEscape(projectID))
	err := c.chDo(ctx, http.MethodGet, path, nil, &resp)
	return resp.Risks, err
}

// InitProjectHealthTracking calls POST /accounts/{id}/init-health-tracking.
func (c *CustomerEntityClient) InitProjectHealthTracking(ctx context.Context, accountID string, projectIDs []string) error {
	path := fmt.Sprintf("/accounts/%s/init-health-tracking", url.PathEscape(accountID))
	return c.chDo(ctx, http.MethodPost, path, chInitProjectHealthTrackingRequest{ProjectIDs: projectIDs}, nil)
}

// CreateRiskActionItem calls POST /risks/{id}/action-items.
func (c *CustomerEntityClient) CreateRiskActionItem(ctx context.Context, riskID, title string, description *string, priority string, assignedToEmail *string, dueDate string) (CHRiskActionItem, error) {
	var resp CHRiskActionItem
	path := fmt.Sprintf("/risks/%s/action-items", url.PathEscape(riskID))
	req := chCreateRiskActionItemRequest{
		Title: title, Description: description, Priority: priority,
		AssignedToEmail: assignedToEmail, DueDate: dueDate,
	}
	err := c.chDo(ctx, http.MethodPost, path, req, &resp)
	return resp, err
}

// UpdateRiskActionItemStatus calls PUT /action-items/{id}/status.
func (c *CustomerEntityClient) UpdateRiskActionItemStatus(ctx context.Context, actionItemID, status string, resolutionComment *string) (CHRiskActionItem, error) {
	var resp CHRiskActionItem
	path := fmt.Sprintf("/action-items/%s/status", url.PathEscape(actionItemID))
	req := chUpdateRiskActionItemStatusRequest{Status: status, ResolutionComment: resolutionComment}
	err := c.chDo(ctx, http.MethodPut, path, req, &resp)
	return resp, err
}

// UpdateRiskActionItem calls PUT /action-items/{id}.
func (c *CustomerEntityClient) UpdateRiskActionItem(ctx context.Context, actionItemID, title string, description *string, priority string, assignedToEmail, dueDate *string) (CHRiskActionItem, error) {
	var resp CHRiskActionItem
	path := fmt.Sprintf("/action-items/%s", url.PathEscape(actionItemID))
	req := chUpdateRiskActionItemRequest{
		Title: title, Description: description, Priority: priority,
		AssignedToEmail: assignedToEmail, DueDate: dueDate,
	}
	err := c.chDo(ctx, http.MethodPut, path, req, &resp)
	return resp, err
}

// GetActionItemsByRisk calls GET /risks/{id}/action-items.
func (c *CustomerEntityClient) GetActionItemsByRisk(ctx context.Context, riskID string, status *string) ([]CHRiskActionItem, error) {
	var resp chGetRiskActionItemsResponse
	path := fmt.Sprintf("/risks/%s/action-items", url.PathEscape(riskID))
	if status != nil {
		q := url.Values{}
		q.Set("status", *status)
		path += "?" + q.Encode()
	}
	err := c.chDo(ctx, http.MethodGet, path, nil, &resp)
	return resp.ActionItems, err
}

// GetActionItemsByAccount calls GET /accounts/{id}/action-items.
func (c *CustomerEntityClient) GetActionItemsByAccount(ctx context.Context, accountID string, projectID, status *string) ([]CHRiskActionItem, error) {
	var resp chGetRiskActionItemsResponse
	path := fmt.Sprintf("/accounts/%s/action-items", url.PathEscape(accountID))
	q := url.Values{}
	if projectID != nil {
		q.Set("projectId", *projectID)
	}
	if status != nil {
		q.Set("status", *status)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	err := c.chDo(ctx, http.MethodGet, path, nil, &resp)
	return resp.ActionItems, err
}

// CreateActionItemComment calls POST /action-items/{id}/comments.
func (c *CustomerEntityClient) CreateActionItemComment(ctx context.Context, actionItemID, comment string) (CHActionItemComment, error) {
	var resp CHActionItemComment
	path := fmt.Sprintf("/action-items/%s/comments", url.PathEscape(actionItemID))
	err := c.chDo(ctx, http.MethodPost, path, chCreateActionItemCommentRequest{Comment: comment}, &resp)
	return resp, err
}

// GetActionItemComments calls GET /action-items/{id}/comments.
func (c *CustomerEntityClient) GetActionItemComments(ctx context.Context, actionItemID string) ([]CHActionItemComment, error) {
	var resp chGetActionItemCommentsResponse
	path := fmt.Sprintf("/action-items/%s/comments", url.PathEscape(actionItemID))
	err := c.chDo(ctx, http.MethodGet, path, nil, &resp)
	return resp.Comments, err
}

// CHAccountSearchPagination is the pagination block of POST /accounts/search.
type CHAccountSearchPagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// CHAccountSearchFilters is the subset of POST /accounts/search's filters
// the customer-health account list uses. Every field except IncludeHealthSummary
// is omitted from the wire when empty, so an unset filter is no filter.
type CHAccountSearchFilters struct {
	SearchQuery          string   `json:"searchQuery,omitempty"`
	OwnerEmail           string   `json:"ownerEmail,omitempty"`
	LifeCycle            string   `json:"lifeCycle,omitempty"`
	HasProject           bool     `json:"hasProject,omitempty"`
	Risks                []string `json:"risks,omitempty"`
	SubRegion            []string `json:"subRegion,omitempty"`
	Product              string   `json:"product,omitempty"`
	AbtTeam              string   `json:"abtTeam,omitempty"`
	HealthStatus         string   `json:"healthStatus,omitempty"`
	IncludeHealthSummary bool     `json:"includeHealthSummary,omitempty"`
}

// CHAccountSearchRequest is POST /accounts/search's request body.
type CHAccountSearchRequest struct {
	Pagination CHAccountSearchPagination `json:"pagination"`
	Filters    CHAccountSearchFilters    `json:"filters"`
}

// CHAccountGoLiveStatus is an account's aggregated go-live state.
type CHAccountGoLiveStatus struct {
	Status string `json:"status"`
	IsRisk bool   `json:"isRisk"`
	State  string `json:"state"`
}

// CHAccountSearchHealthSummary is the per-account customer-health flags and
// overall status a search row carries when IncludeHealthSummary was set.
// HealthStatus is lowercase: at_risk, healthy or to_be_reviewed.
type CHAccountSearchHealthSummary struct {
	GoLiveStatus           CHAccountGoLiveStatus `json:"goLiveStatus"`
	HasRecentCases         bool                  `json:"hasRecentCases"`
	HasEolProduct          bool                  `json:"hasEolProduct"`
	HasAbandonedMigrations bool                  `json:"hasAbandonedMigrations"`
	HasMigrationDelays     bool                  `json:"hasMigrationDelays"`
	HasRecentEscalations   bool                  `json:"hasRecentEscalations"`
	NoSupportCases6mo      bool                  `json:"noSupportCases6mo"`
	HealthStatus           string                `json:"healthStatus"`
}

// CHAccountSearchRow is one account of a search response; only the fields the
// customer-health list reads are declared.
type CHAccountSearchRow struct {
	ID            string                        `json:"id"`
	Name          string                        `json:"name"`
	HealthSummary *CHAccountSearchHealthSummary `json:"healthSummary"`
}

// CHAccountSearchResponse is POST /accounts/search's response. Total is the
// count after every filter, not the page size.
type CHAccountSearchResponse struct {
	Accounts []CHAccountSearchRow `json:"accounts"`
	Total    int                  `json:"total"`
	Limit    int                  `json:"limit"`
	Offset   int                  `json:"offset"`
	HasMore  bool                 `json:"hasMore"`
}

// SearchAccountsWithHealth calls POST /accounts/search with the
// customer-health filters, filtering, paginating and computing each row's
// health summary server-side in one call.
func (c *CustomerEntityClient) SearchAccountsWithHealth(ctx context.Context, req CHAccountSearchRequest) (CHAccountSearchResponse, error) {
	var resp CHAccountSearchResponse
	err := c.chDo(ctx, http.MethodPost, "/accounts/search", req, &resp)
	return resp, err
}

// CHAccountHealthDetail is GET /accounts/{id}/health-detail's response: every
// project of the account with its risk flags and the items behind them. Ids
// are entity-service's dashed UUIDs, and every list is always an array.
type CHAccountHealthDetail struct {
	AccountName      string                  `json:"accountName"`
	CustomerProjects []CHProjectHealthDetail `json:"customerProjects"`
}

// CHProjectHealthDetail is one project of a CHAccountHealthDetail.
type CHProjectHealthDetail struct {
	ID                      string                      `json:"id"`
	Name                    string                      `json:"name"`
	HasRecentCases          bool                        `json:"hasRecentCases"`
	DetailedRecentCases     []CHHealthCasePriorityGroup `json:"detailedRecentCases"`
	TotalRecentCases        int                         `json:"totalRecentCases"`
	HasAbandonedCases       bool                        `json:"hasAbandonedCases"`
	DetailedAbandonedCases  []CHHealthCaseLink          `json:"detailedAbandonedCases"`
	IsUsingEolProduct       bool                        `json:"isUsingEolProduct"`
	SoftwareModel           []CHHealthEolProduct        `json:"softwareModel"`
	Deployments             []CHHealthDeploymentRef     `json:"deployments"`
	HasMigrationDelays      bool                        `json:"hasMigrationDelays"`
	DetailedMigrationDelays []CHHealthCaseLink          `json:"detailedMigrationDelays"`
	HasEscalatedCases       bool                        `json:"hasEscalatedCases"`
	DetailedEscalatedCases  []CHHealthCaseLink          `json:"detailedEscalatedCases"`
	GoLiveStatus            CHAccountGoLiveStatus       `json:"goLiveStatus"`
}

// CHHealthCasePriorityGroup is the recent cases of one priority.
type CHHealthCasePriorityGroup struct {
	Priority string             `json:"priority"`
	Count    int                `json:"count"`
	Cases    []CHHealthCaseInfo `json:"cases"`
}

// CHHealthCaseInfo is a minimal case reference.
type CHHealthCaseInfo struct {
	ID     string `json:"id"`
	Number string `json:"number"`
}

// CHHealthCaseLink is a case, or an escalation and the case it is on; a field
// that does not apply is empty.
type CHHealthCaseLink struct {
	ID               string `json:"id"`
	Number           string `json:"number"`
	EscalationID     string `json:"escalationId"`
	EscalationNumber string `json:"escalationNumber"`
}

// CHHealthEolProduct is one end-of-life product version and the deployments
// running it.
type CHHealthEolProduct struct {
	Name        string                  `json:"name"`
	EolDate     string                  `json:"eolDate"`
	Deployments []CHHealthDeploymentRef `json:"deployments"`
}

// CHHealthDeploymentRef is a minimal deployment reference.
type CHHealthDeploymentRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetAccountHealthDetail calls GET /accounts/{id}/health-detail: the whole
// per-project health breakdown of an account in one request. A missing
// account is a 404 *apierror.Error.
func (c *CustomerEntityClient) GetAccountHealthDetail(ctx context.Context, accountID string) (CHAccountHealthDetail, error) {
	var resp CHAccountHealthDetail
	err := c.chDo(ctx, http.MethodGet, "/accounts/"+url.PathEscape(accountID)+"/health-detail", nil, &resp)
	return resp, err
}
