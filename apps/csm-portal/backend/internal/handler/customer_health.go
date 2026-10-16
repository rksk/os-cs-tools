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
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/risk"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/viewer"
)

// riskClient abstracts the entity-service-backed customer-health risk
// operations used by CustomerHealthHandler. riskID/actionItemID are UUID
// strings (entity-service's own primary keys), not the auto-increment ints
// the old MySQL-backed tables used -- see internal/risk/types.go's own doc
// comment. The actor is no longer passed explicitly (email parameters
// removed): entity-service resolves it server-side from the
// x-user-id-token this backend's own Auth middleware already attached to
// the request context, the same way every other entity-service write in
// this backend works.
type riskClient interface {
	OpenProjectRisk(ctx context.Context, projectSysID, comment string) (*risk.ProjectRisk, error)
	CloseProjectRisk(ctx context.Context, riskID, comment string) (*risk.ProjectRisk, error)
	MarkProjectHealthy(ctx context.Context, projectSysID string, comment *string) (*risk.HealthStatusRecord, error)
	RevertProjectHealth(ctx context.Context, projectSysID string) (*risk.HealthStatusRecord, error)
	GetAccountHealthStatus(ctx context.Context, accountSysID string) ([]risk.ProjectHealthStatus, error)
	GetAccountHealthSummary(ctx context.Context, accountSysID string) (*risk.HealthSummary, error)
	GetProjectRiskHistory(ctx context.Context, projectSysID string) ([]risk.ProjectRisk, error)
	InitProjectHealthRows(ctx context.Context, projectSysIDs []string, accountSysID string) error
	CreateActionItem(ctx context.Context, riskID string, payload risk.CreateActionItemRequest) (*risk.RiskActionItem, error)
	UpdateActionItemStatus(ctx context.Context, actionItemID, newStatus string, resolutionComment *string) (*risk.RiskActionItem, error)
	UpdateActionItem(ctx context.Context, actionItemID string, payload risk.UpdateActionItemRequest) (*risk.RiskActionItem, error)
	GetActionItemsByRisk(ctx context.Context, riskID string, statusFilter *string) ([]risk.RiskActionItem, error)
	GetActionItemsByAccount(ctx context.Context, accountSysID string, projectSysID, statusFilter *string) ([]risk.RiskActionItem, error)
	CreateActionItemComment(ctx context.Context, actionItemID, comment string) (*risk.ActionItemComment, error)
	GetActionItemComments(ctx context.Context, actionItemID string) ([]risk.ActionItemComment, error)
}

// customerHealthDetailClient abstracts the account-detail lookup used by
// CustomerHealthHandler (the summary list goes through
// customerHealthAccountSearchClient, the rest of this domain through
// riskClient). It is entity-service-backed.
type customerHealthDetailClient interface {
	GetCustomerHealthDetail(ctx context.Context, accountID string) (*viewer.AccountDetail, error)
}

// customerHealthAccountSearchClient is the one entity-service call behind the
// summary list: POST /accounts/search with the customer-health filters and
// includeHealthSummary, which filters, paginates and computes each row's
// health summary server-side.
type customerHealthAccountSearchClient interface {
	SearchAccountsWithHealth(ctx context.Context, req entity.CHAccountSearchRequest) (entity.CHAccountSearchResponse, error)
}

// CustomerHealthHandler handles HTTP requests for the viewer audience's
// customer-health/risk-tracking endpoints (/customer-health/*). The
// risk-tracking state, the summary list and the account detail all come from
// entity-service (Postgres).
type CustomerHealthHandler struct {
	risk        riskClient
	detail      customerHealthDetailClient
	search      customerHealthAccountSearchClient
	accessGuard *AccessGuard
}

// NewCustomerHealthHandler creates a CustomerHealthHandler. accessGuard
// enforces PermViewerAccess, the viewer audience's blanket gate — no
// endpoint in this domain has an additional fine-grained permission check
// beyond it (confirmed by reading every customer-health resource function in
// service.bal: none call a group list beyond the global request
// interceptor).
func NewCustomerHealthHandler(riskClient riskClient, detail customerHealthDetailClient, search customerHealthAccountSearchClient, accessGuard *AccessGuard) *CustomerHealthHandler {
	return &CustomerHealthHandler{risk: riskClient, detail: detail, search: search, accessGuard: accessGuard}
}

// customerHealthSummaryRequest is the payload for POST
// /customer-health/summary — mirrors Ballerina
// types:CustomerHealthSummaryRequest.
type customerHealthSummaryRequest struct {
	Email        *string  `json:"email"`
	Phrase       *string  `json:"phrase"`
	Risks        *string  `json:"risks"`
	Region       []string `json:"region"`
	Product      *string  `json:"product"`
	AbtTeam      *string  `json:"abtTeam"`
	HealthStatus *string  `json:"healthStatus"`
	Offset       int      `json:"offset"`
	Limit        int      `json:"limit"`
}

// accountSummaryResponse is the enriched, portal-shaped response for POST
// /customer-health/summary — mirrors Ballerina types:AccountSummaryResponse
// after the resource function's health-status enrichment pass.
type accountSummaryResponse struct {
	Data       []accountSummary `json:"data"`
	TotalCount int              `json:"totalCount"`
}

type accountSummary struct {
	AccountSysID           string              `json:"accountSysId"`
	AccountName            *string             `json:"accountName"`
	HasNoGoLive            viewer.GoLiveStatus `json:"hasNoGoLive"`
	HasRecentCases         bool                `json:"hasRecentCases"`
	HasEolProduct          bool                `json:"hasEolProduct"`
	HasAbandonedMigrations bool                `json:"hasAbandonedMigrations"`
	HasMigrationDelays     bool                `json:"hasMigrationDelays"`
	HasRecentEscalations   bool                `json:"hasRecentEscalations"`
	NoSupportCases6mo      bool                `json:"noSupportCases6mo"`
	HealthStatus           *string             `json:"healthStatus"`
}

// GetSummary handles POST /customer-health/summary.
//
// Every request, with or without a health-status filter, is exactly one
// entity-service call (POST /accounts/search with includeHealthSummary): the
// email/phrase/region/risks/product/abtTeam/healthStatus filters, pagination,
// the filtered total and each row's health summary are all computed
// server-side, so the cost does not grow with the number of accounts.
func (h *CustomerHealthHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}
	var payload customerHealthSummaryRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	// payload.Email is only ever meant to express "my accounts" -- the
	// webapp's own "My Accounts" scope toggle sends the signed-in user's own
	// email here, never an arbitrary one (there is no UI for picking someone
	// else's). Trusting it as sent would let any caller pass a colleague's
	// email and get that colleague's own "my accounts" results instead of
	// their own -- a real gap, since nothing here re-derives it from the
	// caller's own identity. A non-empty value is therefore only ever a
	// signal that the caller wants their OWN accounts; the value used is
	// always the authenticated caller's own token email, never the
	// caller-supplied string.
	if payload.Email != nil && strings.TrimSpace(*payload.Email) != "" {
		callerEmail := user.Email
		payload.Email = &callerEmail
	}
	if payload.Limit <= 0 {
		payload.Limit = 10
	}
	// maxPaginationLimit (accounts.go) is this backend's own bound on every
	// SPL-paginated route; summaryViaSearch further cuts the page to
	// entity-service's own cap (healthSummarySearchMaxLimit).
	if payload.Limit > maxPaginationLimit {
		payload.Limit = maxPaginationLimit
	}

	ctx := r.Context()
	resp, err := h.summaryViaSearch(ctx, payload)
	if err != nil {
		slog.ErrorContext(ctx, "customer health summary failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve customer health summary.")
		return
	}
	writeJSONValue(w, http.StatusOK, resp)
}

// summaryViaSearch implements GetSummary -- see its doc comment. It makes
// exactly one upstream call (none for an unrecognised healthStatus).
//
// Account ids are entity-service's own dashed UUIDs. healthStatus is the
// lowercase value (at_risk, healthy, to_be_reviewed); an account with no
// recorded health rows is reported as to_be_reviewed, as the batch
// health-summary lookup this replaces reported it.
func (h *CustomerHealthHandler) summaryViaSearch(ctx context.Context, payload customerHealthSummaryRequest) (accountSummaryResponse, error) {
	healthStatus := ""
	if payload.HealthStatus != nil {
		healthStatus = *payload.HealthStatus
	}
	// Any non-empty value outside the three statuses matches no account.
	// entity-service answers such a value with an error instead of an empty
	// page, so it is answered here without a round trip.
	switch healthStatus {
	case "", "at_risk", "healthy", "to_be_reviewed":
	default:
		return accountSummaryResponse{Data: []accountSummary{}, TotalCount: 0}, nil
	}

	filters := entity.CHAccountSearchFilters{
		LifeCycle:            customerHealthAccountLifeCycle,
		HasProject:           customerHealthAccountRequiresProject,
		Risks:                parseCustomerHealthRisks(payload.Risks),
		SubRegion:            payload.Region,
		HealthStatus:         healthStatus,
		IncludeHealthSummary: true,
	}
	if payload.Phrase != nil {
		filters.SearchQuery = *payload.Phrase
	}
	if payload.Email != nil {
		filters.OwnerEmail = *payload.Email
	}
	if payload.Product != nil {
		filters.Product = strings.TrimSpace(*payload.Product)
	}
	if payload.AbtTeam != nil {
		filters.AbtTeam = strings.TrimSpace(*payload.AbtTeam)
	}

	resp, err := h.search.SearchAccountsWithHealth(ctx, entity.CHAccountSearchRequest{
		Pagination: entity.CHAccountSearchPagination{
			// A negative offset is clamped to 0, as it always was.
			Offset: max(payload.Offset, 0),
			// entity-service rejects a limit above its own cap
			// (healthSummarySearchMaxLimit), so a larger page is cut to it.
			Limit: min(payload.Limit, healthSummarySearchMaxLimit),
		},
		Filters: filters,
	})
	if err != nil {
		return accountSummaryResponse{}, err
	}

	data := make([]accountSummary, len(resp.Accounts))
	for i, a := range resp.Accounts {
		name := a.Name
		// A row without a summary cannot happen when includeHealthSummary is
		// set. Were it to, the requested status is the only value it could
		// carry (every returned account matched it); with no status
		// requested, an account with no recorded health rows reads as
		// to_be_reviewed.
		hs := healthStatus
		if hs == "" {
			hs = "to_be_reviewed"
		}
		row := accountSummary{AccountSysID: a.ID, AccountName: &name}
		if s := a.HealthSummary; s != nil {
			hs = s.HealthStatus
			row.HasNoGoLive = viewer.GoLiveStatus{Status: s.GoLiveStatus.Status, IsRisk: s.GoLiveStatus.IsRisk, State: s.GoLiveStatus.State}
			row.HasRecentCases = s.HasRecentCases
			row.HasEolProduct = s.HasEolProduct
			row.HasAbandonedMigrations = s.HasAbandonedMigrations
			row.HasMigrationDelays = s.HasMigrationDelays
			row.HasRecentEscalations = s.HasRecentEscalations
			row.NoSupportCases6mo = s.NoSupportCases6mo
		}
		row.HealthStatus = &hs
		data[i] = row
	}
	return accountSummaryResponse{Data: data, TotalCount: resp.Total}, nil
}

// healthSummarySearchMaxLimit is entity-service's pagination cap for POST
// /accounts/search ("limit cannot exceed 50"). maxPaginationLimit (100) is
// this backend's own, wider bound.
const healthSummarySearchMaxLimit = 50

// initHealthTrackingRequest is the payload for POST
// /customer-health/accounts/{accountSysId}/init-health-tracking.
type initHealthTrackingRequest struct {
	ProjectSysIDs []string `json:"projectSysIds"`
}

// InitHealthTracking handles POST
// /customer-health/accounts/{accountSysId}/init-health-tracking.
func (h *CustomerHealthHandler) InitHealthTracking(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	accountSysID := r.PathValue("accountSysId")
	if accountSysID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	var payload initHealthTrackingRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}

	if err := h.risk.InitProjectHealthRows(r.Context(), payload.ProjectSysIDs, accountSysID); err != nil {
		slog.ErrorContext(r.Context(), "risk InitProjectHealthRows failed", "userID", user.UserID, "accountSysId", accountSysID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to initialise health tracking.")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// GetAccountDetail handles GET /customer-health/accounts/{accountId}.
func (h *CustomerHealthHandler) GetAccountDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	accountID := r.PathValue("accountId")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	detail, err := h.detail.GetCustomerHealthDetail(r.Context(), accountID)
	if err != nil {
		if errors.Is(err, viewer.ErrAccountNotFound) {
			writeError(w, http.StatusNotFound, ErrMsgNotFound)
			return
		}
		slog.ErrorContext(r.Context(), "GetCustomerHealthDetail failed", "userID", user.UserID, "accountId", accountID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve account health detail.")
		return
	}
	writeJSONValue(w, http.StatusOK, detail)
}

// OpenRisk handles POST /customer-health/projects/{projectSysId}/risk.
func (h *CustomerHealthHandler) OpenRisk(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	projectSysID := r.PathValue("projectSysId")
	if projectSysID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	var payload risk.OpenRiskRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}

	result, err := h.risk.OpenProjectRisk(r.Context(), projectSysID, payload.Comment)
	if err != nil {
		slog.ErrorContext(r.Context(), "risk OpenProjectRisk failed", "userID", user.UserID, "projectSysId", projectSysID, "err", err)
		mapUpstreamError(w, err, "Failed to open project risk.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}

// CloseRisk handles PUT /customer-health/risks/{riskId}/close.
func (h *CustomerHealthHandler) CloseRisk(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	riskID, ok := requireUUIDPathValue(w, r, "riskId")
	if !ok {
		return
	}

	var payload risk.CloseRiskRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}

	result, err := h.risk.CloseProjectRisk(r.Context(), riskID, payload.Comment)
	if err != nil {
		slog.ErrorContext(r.Context(), "risk CloseProjectRisk failed", "userID", user.UserID, "riskId", riskID, "err", err)
		mapUpstreamError(w, err, "Failed to close project risk.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}

// MarkHealthy handles POST
// /customer-health/projects/{projectSysId}/mark-healthy.
func (h *CustomerHealthHandler) MarkHealthy(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	projectSysID := r.PathValue("projectSysId")
	if projectSysID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	var payload risk.MarkHealthyRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}

	result, err := h.risk.MarkProjectHealthy(r.Context(), projectSysID, payload.Comment)
	if err != nil {
		slog.ErrorContext(r.Context(), "risk MarkProjectHealthy failed", "userID", user.UserID, "projectSysId", projectSysID, "err", err)
		mapUpstreamError(w, err, "Failed to mark project healthy.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}

// RevertReview handles POST
// /customer-health/projects/{projectSysId}/revert-review.
func (h *CustomerHealthHandler) RevertReview(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	projectSysID := r.PathValue("projectSysId")
	if projectSysID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	var payload risk.RevertHealthRequest
	if !decodeJSONBody(w, r, &payload) {
		return
	}

	result, err := h.risk.RevertProjectHealth(r.Context(), projectSysID)
	if err != nil {
		slog.ErrorContext(r.Context(), "risk RevertProjectHealth failed", "userID", user.UserID, "projectSysId", projectSysID, "err", err)
		mapUpstreamError(w, err, "Failed to revert project health review.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}

// GetAccountHealthStatus handles GET
// /customer-health/accounts/{accountSysId}/health-status.
func (h *CustomerHealthHandler) GetAccountHealthStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	accountSysID := r.PathValue("accountSysId")
	if accountSysID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.risk.GetAccountHealthStatus(r.Context(), accountSysID)
	if err != nil {
		slog.ErrorContext(r.Context(), "risk GetAccountHealthStatus failed", "userID", user.UserID, "accountSysId", accountSysID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve account health status.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}

// GetAccountHealthSummary handles GET
// /customer-health/accounts/{accountSysId}/health-summary.
func (h *CustomerHealthHandler) GetAccountHealthSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	accountSysID := r.PathValue("accountSysId")
	if accountSysID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.risk.GetAccountHealthSummary(r.Context(), accountSysID)
	if err != nil {
		slog.ErrorContext(r.Context(), "risk GetAccountHealthSummary failed", "userID", user.UserID, "accountSysId", accountSysID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve account health summary.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}

// GetProjectRiskHistory handles GET
// /customer-health/projects/{projectSysId}/risk-history.
func (h *CustomerHealthHandler) GetProjectRiskHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := requireViewerAccess(w, r, h.accessGuard)
	if !ok {
		return
	}
	projectSysID := r.PathValue("projectSysId")
	if projectSysID == "" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.risk.GetProjectRiskHistory(r.Context(), projectSysID)
	if err != nil {
		slog.ErrorContext(r.Context(), "risk GetProjectRiskHistory failed", "userID", user.UserID, "projectSysId", projectSysID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve project risk history.")
		return
	}
	writeJSONValue(w, http.StatusOK, result)
}

// requireUUIDPathValue reads r.PathValue(name), writing a 400 response and
// returning ok=false unless it's a well-formed UUID -- entity-service's own
// primary key shape for risks/action items (see internal/risk/types.go's
// own doc comment on why these are UUIDs now, not auto-increment ints).
func requireUUIDPathValue(w http.ResponseWriter, r *http.Request, name string) (value string, ok bool) {
	v := r.PathValue(name)
	if v == "" || !uuidRe.MatchString(v) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return "", false
	}
	return v, true
}

// decodeJSONBody reads r.Body (capped at maxRequestBodyBytes) and decodes it
// as JSON into dst, writing an appropriate error response and returning
// false on any failure.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return false
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return false
	}
	if err := json.Unmarshal(body, dst); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return false
	}
	return true
}
