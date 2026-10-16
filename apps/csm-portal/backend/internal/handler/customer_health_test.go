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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/risk"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/viewer"
)

// fakeRiskClient is a hand-rolled riskClient test double: only the
// methods a given test needs are set; every other call panics via a nil
// func, which surfaces immediately as a test failure rather than a silent
// zero value. riskID/actionItemID are UUID strings -- entity-service's own
// primary keys -- not the auto-increment ints the old MySQL-backed client
// used, and there is no longer an explicit email parameter: entity-service
// resolves the actor itself from the forwarded x-user-id-token.
type fakeRiskClient struct {
	openProjectRisk         func(ctx context.Context, projectSysID, comment string) (*risk.ProjectRisk, error)
	closeProjectRisk        func(ctx context.Context, riskID, comment string) (*risk.ProjectRisk, error)
	markProjectHealthy      func(ctx context.Context, projectSysID string, comment *string) (*risk.HealthStatusRecord, error)
	revertProjectHealth     func(ctx context.Context, projectSysID string) (*risk.HealthStatusRecord, error)
	getAccountHealthStatus  func(ctx context.Context, accountSysID string) ([]risk.ProjectHealthStatus, error)
	getAccountHealthSummary func(ctx context.Context, accountSysID string) (*risk.HealthSummary, error)
	getProjectRiskHistory   func(ctx context.Context, projectSysID string) ([]risk.ProjectRisk, error)
	initProjectHealthRows   func(ctx context.Context, projectSysIDs []string, accountSysID string) error
	createActionItem        func(ctx context.Context, riskID string, payload risk.CreateActionItemRequest) (*risk.RiskActionItem, error)
	updateActionItemStatus  func(ctx context.Context, actionItemID, newStatus string, resolutionComment *string) (*risk.RiskActionItem, error)
	updateActionItem        func(ctx context.Context, actionItemID string, payload risk.UpdateActionItemRequest) (*risk.RiskActionItem, error)
	getActionItemsByRisk    func(ctx context.Context, riskID string, statusFilter *string) ([]risk.RiskActionItem, error)
	getActionItemsByAccount func(ctx context.Context, accountSysID string, projectSysID, statusFilter *string) ([]risk.RiskActionItem, error)
	createActionItemComment func(ctx context.Context, actionItemID, comment string) (*risk.ActionItemComment, error)
	getActionItemComments   func(ctx context.Context, actionItemID string) ([]risk.ActionItemComment, error)
}

func (f *fakeRiskClient) OpenProjectRisk(ctx context.Context, projectSysID, comment string) (*risk.ProjectRisk, error) {
	return f.openProjectRisk(ctx, projectSysID, comment)
}
func (f *fakeRiskClient) CloseProjectRisk(ctx context.Context, riskID, comment string) (*risk.ProjectRisk, error) {
	return f.closeProjectRisk(ctx, riskID, comment)
}
func (f *fakeRiskClient) MarkProjectHealthy(ctx context.Context, projectSysID string, comment *string) (*risk.HealthStatusRecord, error) {
	return f.markProjectHealthy(ctx, projectSysID, comment)
}
func (f *fakeRiskClient) RevertProjectHealth(ctx context.Context, projectSysID string) (*risk.HealthStatusRecord, error) {
	return f.revertProjectHealth(ctx, projectSysID)
}
func (f *fakeRiskClient) GetAccountHealthStatus(ctx context.Context, accountSysID string) ([]risk.ProjectHealthStatus, error) {
	return f.getAccountHealthStatus(ctx, accountSysID)
}
func (f *fakeRiskClient) GetAccountHealthSummary(ctx context.Context, accountSysID string) (*risk.HealthSummary, error) {
	return f.getAccountHealthSummary(ctx, accountSysID)
}
func (f *fakeRiskClient) GetProjectRiskHistory(ctx context.Context, projectSysID string) ([]risk.ProjectRisk, error) {
	return f.getProjectRiskHistory(ctx, projectSysID)
}
func (f *fakeRiskClient) InitProjectHealthRows(ctx context.Context, projectSysIDs []string, accountSysID string) error {
	return f.initProjectHealthRows(ctx, projectSysIDs, accountSysID)
}
func (f *fakeRiskClient) CreateActionItem(ctx context.Context, riskID string, payload risk.CreateActionItemRequest) (*risk.RiskActionItem, error) {
	return f.createActionItem(ctx, riskID, payload)
}
func (f *fakeRiskClient) UpdateActionItemStatus(ctx context.Context, actionItemID, newStatus string, resolutionComment *string) (*risk.RiskActionItem, error) {
	return f.updateActionItemStatus(ctx, actionItemID, newStatus, resolutionComment)
}
func (f *fakeRiskClient) UpdateActionItem(ctx context.Context, actionItemID string, payload risk.UpdateActionItemRequest) (*risk.RiskActionItem, error) {
	return f.updateActionItem(ctx, actionItemID, payload)
}
func (f *fakeRiskClient) GetActionItemsByRisk(ctx context.Context, riskID string, statusFilter *string) ([]risk.RiskActionItem, error) {
	return f.getActionItemsByRisk(ctx, riskID, statusFilter)
}
func (f *fakeRiskClient) GetActionItemsByAccount(ctx context.Context, accountSysID string, projectSysID, statusFilter *string) ([]risk.RiskActionItem, error) {
	return f.getActionItemsByAccount(ctx, accountSysID, projectSysID, statusFilter)
}
func (f *fakeRiskClient) CreateActionItemComment(ctx context.Context, actionItemID, comment string) (*risk.ActionItemComment, error) {
	return f.createActionItemComment(ctx, actionItemID, comment)
}
func (f *fakeRiskClient) GetActionItemComments(ctx context.Context, actionItemID string) ([]risk.ActionItemComment, error) {
	return f.getActionItemComments(ctx, actionItemID)
}

type fakeCustomerHealthDetailClient struct {
	getCustomerHealthDetail func(ctx context.Context, accountID string) (*viewer.AccountDetail, error)
}

func (f *fakeCustomerHealthDetailClient) GetCustomerHealthDetail(ctx context.Context, accountID string) (*viewer.AccountDetail, error) {
	return f.getCustomerHealthDetail(ctx, accountID)
}

// upstreamErr builds the *apierror.Error a risk.Client method would return
// for an entity-service refusal: a JSON {"message": "..."} body (the same
// envelope entity-service's apierror.WriteJSON writes) at the given status.
func upstreamErr(status int, message string) error {
	return &apierror.Error{StatusCode: status, Body: `{"message":"` + message + `"}`}
}

// testRiskID/testActionItemID are well-formed UUIDs for path params that
// now require uuidRe to match -- entity-service's own primary key shape.
const (
	testRiskID       = "11111111-1111-1111-1111-111111111111"
	testActionItemID = "22222222-2222-2222-2222-222222222222"
)

// wireBody is the exact JSON body entity-service receives.
func wireBody(t *testing.T, req entity.CHAccountSearchRequest) string {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestCustomerHealthHandler_GetSummary_UnfilteredIsOneEntityCall pins that a
// request with no filters at all is also exactly one entity-service search
// (no per-account fan-out, no separate health-status lookup: the fakes panic
// on any other call), with only the base scope and includeHealthSummary set.
func TestCustomerHealthHandler_GetSummary_UnfilteredIsOneEntityCall(t *testing.T) {
	search := &fakeHealthAccountSearch{}
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)

	assertStatus(t, postSummary(h, `{}`), http.StatusOK)
	if len(search.calls) != 1 {
		t.Fatalf("entity search called %d times, want exactly 1", len(search.calls))
	}
	const want = `{"pagination":{"limit":10,"offset":0},"filters":{"lifeCycle":"Customer","hasProject":true,"includeHealthSummary":true}}`
	if got := wireBody(t, search.calls[0]); got != want {
		t.Errorf("wire body = %s\nwant        %s", got, want)
	}
}

// TestCustomerHealthHandler_GetSummary_ClientSideFiltersWithoutStatusAreOneCall
// pins that region/risks/product/abtTeam without a health-status filter (the
// requests that used to scan up to 10,000 accounts) are one call carrying
// those filters, with no healthStatus on the wire.
func TestCustomerHealthHandler_GetSummary_ClientSideFiltersWithoutStatusAreOneCall(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{
			"region and risks",
			`{"region":["ASIA","NA - WEST"],"risks":"hasEolProduct, hasNoGoLive,","offset":10,"limit":25}`,
			`{"pagination":{"limit":25,"offset":10},"filters":{"lifeCycle":"Customer","hasProject":true,"risks":["hasEolProduct","hasNoGoLive"],"subRegion":["ASIA","NA - WEST"],"includeHealthSummary":true}}`,
		},
		{
			"product only",
			`{"product":" wso2is "}`,
			`{"pagination":{"limit":10,"offset":0},"filters":{"lifeCycle":"Customer","hasProject":true,"product":"wso2is","includeHealthSummary":true}}`,
		},
		{
			"abtTeam and phrase",
			`{"abtTeam":" Team A ","phrase":"acme"}`,
			`{"pagination":{"limit":10,"offset":0},"filters":{"searchQuery":"acme","lifeCycle":"Customer","hasProject":true,"abtTeam":"Team A","includeHealthSummary":true}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			search := &fakeHealthAccountSearch{}
			h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)
			assertStatus(t, postSummary(h, tc.body), http.StatusOK)
			if len(search.calls) != 1 {
				t.Fatalf("entity search called %d times, want exactly 1", len(search.calls))
			}
			if got := wireBody(t, search.calls[0]); got != tc.want {
				t.Errorf("wire body = %s\nwant        %s", got, tc.want)
			}
		})
	}
}

// TestCustomerHealthHandler_GetSummary_UnfilteredMapsRows pins the response
// mapping without a status filter: dashed id as returned, lowercase
// healthStatus read from each row's summary (an account with no recorded
// health rows is to_be_reviewed, never null or empty), the go-live object,
// the flags and the server-side total.
func TestCustomerHealthHandler_GetSummary_UnfilteredMapsRows(t *testing.T) {
	const idA, idB, idC = "441e160e-1bbd-3610-0bb3-da47b04bcb4f", "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"
	search := &fakeHealthAccountSearch{resp: entity.CHAccountSearchResponse{
		Total: 87,
		Accounts: []entity.CHAccountSearchRow{
			{ID: idA, Name: "Example Corp", HealthSummary: &entity.CHAccountSearchHealthSummary{
				GoLiveStatus:   entity.CHAccountGoLiveStatus{Status: "Has Gone Live", State: "success"},
				HasRecentCases: true, NoSupportCases6mo: false, HealthStatus: "at_risk",
			}},
			{ID: idB, Name: "Jane Doe Ltd", HealthSummary: &entity.CHAccountSearchHealthSummary{
				GoLiveStatus:      entity.CHAccountGoLiveStatus{Status: "No Go Live", IsRisk: true, State: "failure"},
				NoSupportCases6mo: true, HealthStatus: "to_be_reviewed",
			}},
			// Defensive: a row without a summary reads as to_be_reviewed.
			{ID: idC, Name: "Bare Inc"},
		},
	}}
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)

	w := postSummary(h, `{}`)

	assertStatus(t, w, http.StatusOK)
	rawBody := w.Body.String()
	resp := decodeJSON[accountSummaryResponse](t, w)
	if resp.TotalCount != 87 || len(resp.Data) != 3 {
		t.Fatalf("resp = %+v, want totalCount 87 (filtered total) and three rows", resp)
	}
	wantStatus := []string{"at_risk", "to_be_reviewed", "to_be_reviewed"}
	for i, row := range resp.Data {
		if row.HealthStatus == nil || *row.HealthStatus != wantStatus[i] {
			t.Errorf("Data[%d].HealthStatus = %v, want %s", i, row.HealthStatus, wantStatus[i])
		}
	}
	if resp.Data[0].AccountSysID != idA || !resp.Data[0].HasRecentCases || resp.Data[0].HasNoGoLive.State != "success" {
		t.Errorf("Data[0] = %+v", resp.Data[0])
	}
	if !resp.Data[1].NoSupportCases6mo || !resp.Data[1].HasNoGoLive.IsRisk {
		t.Errorf("Data[1] = %+v", resp.Data[1])
	}
	// The wire shape: healthStatus is a lowercase string for every row.
	if !bytes.Contains([]byte(rawBody), []byte(`"healthStatus":"to_be_reviewed"`)) {
		t.Errorf("body = %s", rawBody)
	}
}

// TestCustomerHealthHandler_GetSummary_IgnoresCallerSuppliedEmail verifies a
// real gap this closes: the "My Accounts" filter sends the signed-in
// caller's own email as the request's "email" field, with nothing on the
// backend ever checking it actually belongs to the caller -- any caller
// could substitute a colleague's email and see that colleague's own "my
// accounts" results instead of their own. GetSummary must always use the
// authenticated caller's own token email (middleware.UserInfo.Email) for
// this field, never the value the client sent, whenever the client asks to
// filter by one at all -- with or without a health-status filter.
func TestCustomerHealthHandler_GetSummary_IgnoresCallerSuppliedEmail(t *testing.T) {
	for _, body := range []string{
		`{"email":"someone-else@example.com"}`,
		`{"email":"someone-else@example.com","healthStatus":"healthy"}`,
	} {
		search := &fakeHealthAccountSearch{}
		h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)
		assertStatus(t, postSummary(h, body), http.StatusOK)
		if len(search.calls) != 1 {
			t.Fatalf("%s: calls = %d, want 1", body, len(search.calls))
		}
		if got := search.calls[0].Filters.OwnerEmail; got != testUser.Email {
			t.Errorf("%s: ownerEmail sent upstream = %q, want the caller's own token email %q", body, got, testUser.Email)
		}
	}
}

// TestCustomerHealthHandler_GetSummary_NoEmailMeansNoOwnerFilter pins that an
// absent or blank email is not turned into a "my accounts" filter.
func TestCustomerHealthHandler_GetSummary_NoEmailMeansNoOwnerFilter(t *testing.T) {
	for _, body := range []string{`{}`, `{"email":""}`} {
		search := &fakeHealthAccountSearch{}
		h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)
		assertStatus(t, postSummary(h, body), http.StatusOK)
		if got := search.calls[0].Filters.OwnerEmail; got != "" {
			t.Errorf("%s: ownerEmail = %q, want none", body, got)
		}
	}
}

// fakeHealthAccountSearch records every SearchAccountsWithHealth call.
type fakeHealthAccountSearch struct {
	calls []entity.CHAccountSearchRequest
	resp  entity.CHAccountSearchResponse
	err   error
}

func (f *fakeHealthAccountSearch) SearchAccountsWithHealth(ctx context.Context, req entity.CHAccountSearchRequest) (entity.CHAccountSearchResponse, error) {
	f.calls = append(f.calls, req)
	return f.resp, f.err
}

func postSummary(h *CustomerHealthHandler, body string) *httptest.ResponseRecorder {
	req := withUser(httptest.NewRequest(http.MethodPost, "/spl/customer-health/summary", bytes.NewReader([]byte(body))))
	w := httptest.NewRecorder()
	h.GetSummary(w, req)
	return w
}

// TestCustomerHealthHandler_GetSummary_HealthStatusFilterIsOneEntityCall pins
// the whole point of the single-call branch: exactly one entity-service
// search, the exact request body, and no ServiceNow-style summary or
// risk lookups (the fakes below panic on a nil func if called).
func TestCustomerHealthHandler_GetSummary_HealthStatusFilterIsOneEntityCall(t *testing.T) {
	search := &fakeHealthAccountSearch{resp: entity.CHAccountSearchResponse{Total: 0}}
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)

	w := postSummary(h, `{"healthStatus":"at_risk","email":"someone-else@example.com","phrase":"acme","risks":"hasEolProduct, hasRecentCases,","region":["ASIA","NA - WEST"],"product":" wso2is ","abtTeam":" Team A ","offset":20,"limit":10}`)

	assertStatus(t, w, http.StatusOK)
	if len(search.calls) != 1 {
		t.Fatalf("entity search called %d times, want exactly 1", len(search.calls))
	}
	want := entity.CHAccountSearchRequest{
		Pagination: entity.CHAccountSearchPagination{Limit: 10, Offset: 20},
		Filters: entity.CHAccountSearchFilters{
			SearchQuery:          "acme",
			OwnerEmail:           testUser.Email, // the caller's token email, never the supplied one
			LifeCycle:            "Customer",
			HasProject:           true,
			Risks:                []string{"hasEolProduct", "hasRecentCases"},
			SubRegion:            []string{"ASIA", "NA - WEST"},
			Product:              "wso2is",
			AbtTeam:              "Team A",
			HealthStatus:         "at_risk",
			IncludeHealthSummary: true,
		},
	}
	if !reflect.DeepEqual(search.calls[0], want) {
		t.Errorf("request = %+v\nwant      %+v", search.calls[0], want)
	}
}

// TestCustomerHealthHandler_GetSummary_HealthStatusFilterOmitsUnsetFilters
// pins that no email filter is sent unless the caller asked for "my
// accounts", and that unset filters stay off the wire.
func TestCustomerHealthHandler_GetSummary_HealthStatusFilterOmitsUnsetFilters(t *testing.T) {
	search := &fakeHealthAccountSearch{}
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)

	assertStatus(t, postSummary(h, `{"healthStatus":"healthy"}`), http.StatusOK)
	raw, err := json.Marshal(search.calls[0])
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"pagination":{"limit":10,"offset":0},"filters":{"lifeCycle":"Customer","hasProject":true,"healthStatus":"healthy","includeHealthSummary":true}}`
	if string(raw) != want {
		t.Errorf("wire body = %s\nwant        %s", raw, want)
	}
}

// TestCustomerHealthHandler_GetSummary_HealthStatusFilterMapsRows pins the
// response mapping: the dashed account id exactly as entity-service
// returns it (the unfiltered branch returns the same representation),
// lowercase healthStatus, the go-live object, every flag, and the
// server-side filtered total (not the page size).
func TestCustomerHealthHandler_GetSummary_HealthStatusFilterMapsRows(t *testing.T) {
	const dashedID = "441e160e-1bbd-3610-0bb3-da47b04bcb4f"
	search := &fakeHealthAccountSearch{resp: entity.CHAccountSearchResponse{
		Total: 137,
		Accounts: []entity.CHAccountSearchRow{{
			ID: dashedID, Name: "Example Corp",
			HealthSummary: &entity.CHAccountSearchHealthSummary{
				GoLiveStatus:           entity.CHAccountGoLiveStatus{Status: "No Go Live", IsRisk: true, State: "failure"},
				HasRecentCases:         true,
				HasEolProduct:          true,
				HasAbandonedMigrations: true,
				HasMigrationDelays:     true,
				HasRecentEscalations:   true,
				NoSupportCases6mo:      true,
				HealthStatus:           "at_risk",
			},
		}},
	}}
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)

	w := postSummary(h, `{"healthStatus":"at_risk","offset":30,"limit":1}`)

	assertStatus(t, w, http.StatusOK)
	resp := decodeJSON[accountSummaryResponse](t, w)
	if resp.TotalCount != 137 || len(resp.Data) != 1 {
		t.Fatalf("resp = %+v, want totalCount 137 (filtered total) and one row", resp)
	}
	got := resp.Data[0]
	if got.AccountSysID != dashedID {
		t.Errorf("AccountSysID = %q, want %q", got.AccountSysID, dashedID)
	}
	if got.AccountName == nil || *got.AccountName != "Example Corp" {
		t.Errorf("AccountName = %v", got.AccountName)
	}
	if got.HealthStatus == nil || *got.HealthStatus != "at_risk" {
		t.Errorf("HealthStatus = %v, want at_risk", got.HealthStatus)
	}
	if got.HasNoGoLive != (viewer.GoLiveStatus{Status: "No Go Live", IsRisk: true, State: "failure"}) {
		t.Errorf("HasNoGoLive = %+v", got.HasNoGoLive)
	}
	if !got.HasRecentCases || !got.HasEolProduct || !got.HasAbandonedMigrations || !got.HasMigrationDelays || !got.HasRecentEscalations || !got.NoSupportCases6mo {
		t.Errorf("flags not all mapped: %+v", got)
	}
}

// TestCustomerHealthHandler_GetSummary_HealthStatusFilterEmptyPageIsEmptyArray
// pins that an empty result serialises "data" as [] not null.
func TestCustomerHealthHandler_GetSummary_HealthStatusFilterEmptyPageIsEmptyArray(t *testing.T) {
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, &fakeHealthAccountSearch{}, viewerAccessGuard)
	w := postSummary(h, `{"healthStatus":"to_be_reviewed"}`)
	assertStatus(t, w, http.StatusOK)
	if got := w.Body.String(); !bytes.Contains([]byte(got), []byte(`"data":[]`)) || !bytes.Contains([]byte(got), []byte(`"totalCount":0`)) {
		t.Errorf("body = %s, want empty data array and totalCount 0", got)
	}
}

// TestCustomerHealthHandler_GetSummary_HealthStatusFilterPagination covers
// offset/limit passthrough, the clamps, the default limit and the cut to
// entity-service's cap, with and without a health-status filter.
func TestCustomerHealthHandler_GetSummary_HealthStatusFilterPagination(t *testing.T) {
	cases := []struct {
		name, body          string
		wantLimit, wantOffs int
	}{
		{"passthrough", `{"healthStatus":"healthy","offset":25,"limit":25}`, 25, 25},
		{"default limit", `{"healthStatus":"healthy"}`, 10, 0},
		{"negative offset clamps to 0", `{"healthStatus":"healthy","offset":-5,"limit":5}`, 5, 0},
		{"limit above entity cap is cut to it", `{"healthStatus":"healthy","limit":100}`, 50, 0},
		{"limit above route cap is cut to it, then to entity cap", `{"healthStatus":"healthy","limit":5000}`, 50, 0},
		{"no status: passthrough", `{"offset":50,"limit":25}`, 25, 50},
		{"no status: default limit", `{}`, 10, 0},
		{"no status: negative offset clamps to 0", `{"offset":-5,"limit":5}`, 5, 0},
		{"no status: limit above entity cap is cut to it", `{"limit":100}`, 50, 0},
		{"no status: limit far above route cap is cut to entity cap", `{"region":["ASIA"],"limit":5000}`, 50, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			search := &fakeHealthAccountSearch{}
			h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)
			assertStatus(t, postSummary(h, tc.body), http.StatusOK)
			if len(search.calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(search.calls))
			}
			p := search.calls[0].Pagination
			if p.Limit != tc.wantLimit || p.Offset != tc.wantOffs {
				t.Errorf("pagination = %+v, want limit %d offset %d", p, tc.wantLimit, tc.wantOffs)
			}
		})
	}
}

// TestCustomerHealthHandler_GetSummary_HealthStatusFilterUnknownValueShortCircuits
// pins that an unrecognised status answers empty without any upstream call,
// as it did before (entity-service would reject it with an error).
func TestCustomerHealthHandler_GetSummary_HealthStatusFilterUnknownValueShortCircuits(t *testing.T) {
	search := &fakeHealthAccountSearch{}
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)
	w := postSummary(h, `{"healthStatus":"bogus"}`)
	assertStatus(t, w, http.StatusOK)
	if len(search.calls) != 0 {
		t.Errorf("calls = %d, want 0", len(search.calls))
	}
}

// TestCustomerHealthHandler_GetSummary_HealthStatusFilterUpstreamErrorMapsGenerically
// pins that an upstream failure (including the gateway's 429 throttle) goes
// through mapUpstreamErrorGeneric with the summary's fallback message.
func TestCustomerHealthHandler_GetSummary_HealthStatusFilterUpstreamErrorMapsGenerically(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"throttled 429 takes the generic fallback", upstreamErr(http.StatusTooManyRequests, "900800 Message throttled out"), http.StatusInternalServerError},
		{"400 keeps the generic message", upstreamErr(http.StatusBadRequest, "limit cannot exceed 50"), http.StatusBadRequest},
		{"503 stays 503", upstreamErr(http.StatusServiceUnavailable, "down"), http.StatusServiceUnavailable},
		{"non-API error is a 500", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		// With a health-status filter, with none, and with only a client-side
		// style filter: every request is the same single call.
		for _, body := range []string{`{"healthStatus":"at_risk"}`, `{}`, `{"region":["ASIA"],"risks":"hasEolProduct"}`} {
			t.Run(tc.name+" "+body, func(t *testing.T) {
				search := &fakeHealthAccountSearch{err: tc.err}
				h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, search, viewerAccessGuard)
				w := postSummary(h, body)
				assertStatus(t, w, tc.wantStatus)
				if !bytes.Contains(w.Body.Bytes(), []byte("Failed to retrieve customer health summary.")) {
					t.Errorf("body = %s, want the generic fallback message", w.Body.String())
				}
				if len(search.calls) != 1 {
					t.Errorf("calls = %d, want 1 (no retry, no fallback path)", len(search.calls))
				}
			})
		}
	}
}

func TestCustomerHealthHandler_OpenRisk_RejectsMissingUser(t *testing.T) {
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, &fakeHealthAccountSearch{}, viewerAccessGuard)
	req := httptest.NewRequest(http.MethodPost, "/spl/customer-health/projects/proj-1/risk", bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("projectSysId", "proj-1")
	w := httptest.NewRecorder()
	h.OpenRisk(w, req)
	assertStatus(t, w, http.StatusUnauthorized)
}

func TestCustomerHealthHandler_OpenRisk_RejectsMissingSPLAccess(t *testing.T) {
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, &fakeHealthAccountSearch{}, viewerAccessGuard)
	req := httptest.NewRequest(http.MethodPost, "/spl/customer-health/projects/proj-1/risk", bytes.NewReader([]byte(`{}`)))
	// Authenticated but holds no role granting PermViewerAccess.
	req = req.WithContext(middleware.WithUserInfo(req.Context(), &middleware.UserInfo{Email: "nobody@example.com", UserID: "u-nobody"}))
	req.SetPathValue("projectSysId", "proj-1")
	w := httptest.NewRecorder()
	h.OpenRisk(w, req)
	assertStatus(t, w, http.StatusForbidden)
}

func TestCustomerHealthHandler_CloseRisk_UpstreamConflictMapsTo409(t *testing.T) {
	rc := &fakeRiskClient{
		closeProjectRisk: func(ctx context.Context, riskID, comment string) (*risk.ProjectRisk, error) {
			return nil, upstreamErr(http.StatusConflict, "Cannot close risk: 1 action item(s) are still open.")
		},
	}
	h := NewCustomerHealthHandler(rc, &fakeCustomerHealthDetailClient{}, &fakeHealthAccountSearch{}, viewerAccessGuard)

	req := withUser(httptest.NewRequest(http.MethodPut, "/spl/customer-health/risks/"+testRiskID+"/close", bytes.NewReader([]byte(`{"comment":"done"}`))))
	req.SetPathValue("riskId", testRiskID)
	w := httptest.NewRecorder()
	h.CloseRisk(w, req)

	assertStatus(t, w, http.StatusConflict)
	assertErrorMessage(t, w, "Cannot close risk: 1 action item(s) are still open.")
}

func TestCustomerHealthHandler_CloseRisk_InvalidRiskIDIs400(t *testing.T) {
	h := NewCustomerHealthHandler(&fakeRiskClient{}, &fakeCustomerHealthDetailClient{}, &fakeHealthAccountSearch{}, viewerAccessGuard)
	req := withUser(httptest.NewRequest(http.MethodPut, "/spl/customer-health/risks/not-a-uuid/close", bytes.NewReader([]byte(`{}`))))
	req.SetPathValue("riskId", "not-a-uuid")
	w := httptest.NewRecorder()
	h.CloseRisk(w, req)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestCustomerHealthHandler_GetAccountDetail_NotFoundMapsTo404(t *testing.T) {
	detail := &fakeCustomerHealthDetailClient{
		getCustomerHealthDetail: func(ctx context.Context, accountID string) (*viewer.AccountDetail, error) {
			return nil, viewer.ErrAccountNotFound
		},
	}
	h := NewCustomerHealthHandler(&fakeRiskClient{}, detail, &fakeHealthAccountSearch{}, viewerAccessGuard)

	req := withUser(httptest.NewRequest(http.MethodGet, "/spl/customer-health/accounts/acct-1", nil))
	req.SetPathValue("accountId", "acct-1")
	w := httptest.NewRecorder()
	h.GetAccountDetail(w, req)

	assertStatus(t, w, http.StatusNotFound)
}

func TestCustomerHealthHandler_UpdateActionItemStatus_RequiresResolutionComment(t *testing.T) {
	rc := &fakeRiskClient{
		updateActionItemStatus: func(ctx context.Context, actionItemID, newStatus string, resolutionComment *string) (*risk.RiskActionItem, error) {
			return nil, upstreamErr(http.StatusBadRequest, "resolutionComment is required when status is RESOLVED or CANCELLED")
		},
	}
	h := NewCustomerHealthHandler(rc, &fakeCustomerHealthDetailClient{}, &fakeHealthAccountSearch{}, viewerAccessGuard)

	req := withUser(httptest.NewRequest(http.MethodPut, "/spl/customer-health/action-items/"+testActionItemID+"/status", bytes.NewReader([]byte(`{"status":"resolved"}`))))
	req.SetPathValue("actionItemId", testActionItemID)
	w := httptest.NewRecorder()
	h.UpdateActionItemStatus(w, req)

	assertStatus(t, w, http.StatusBadRequest)
	assertErrorMessage(t, w, "resolutionComment is required when status is RESOLVED or CANCELLED")
}

func TestCustomerHealthHandler_InitHealthTracking_Returns202(t *testing.T) {
	rc := &fakeRiskClient{
		initProjectHealthRows: func(ctx context.Context, projectSysIDs []string, accountSysID string) error {
			if accountSysID != "acct-1" || len(projectSysIDs) != 2 {
				t.Errorf("unexpected args: accountSysID=%q projectSysIDs=%v", accountSysID, projectSysIDs)
			}
			return nil
		},
	}
	h := NewCustomerHealthHandler(rc, &fakeCustomerHealthDetailClient{}, &fakeHealthAccountSearch{}, viewerAccessGuard)

	body := []byte(`{"projectSysIds":["proj-1","proj-2"]}`)
	req := withUser(httptest.NewRequest(http.MethodPost, "/spl/customer-health/accounts/acct-1/init-health-tracking", bytes.NewReader(body)))
	req.SetPathValue("accountSysId", "acct-1")
	w := httptest.NewRecorder()
	h.InitHealthTracking(w, req)

	assertStatus(t, w, http.StatusAccepted)
}
