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
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/entity"
)

// liveEntityClient is a minimal entityCustomerHealthClient/
// entityEscalationsClient/entityProductsClient implementation that calls a
// REAL running entity-service instance directly over HTTP, carrying a
// caller-supplied bearer/x-jwt-assertion header -- it exercises this
// package's actual production code (postgresCustomerHealthClient,
// postgresViewerAccountClient, postgresLookupsClient) end to end against
// real data, the only way a hand-rolled fake test double can't: a fake
// proves this package calls its client correctly, never that the client's
// own request shape (field names, operators) is accepted by the real
// service or that the real data answers the way the code assumes.
//
// Only runs when LIVE_ENTITY_SERVICE_TEST_URL is set (to a locally running
// entity-service instance, e.g. one pointed at a copy of staging) and
// LIVE_ENTITY_SERVICE_TEST_JWT holds a bearer value for its
// x-jwt-assertion header (an M2M client id entity-service's own
// M2M_CLIENT_IDS recognizes). Skipped otherwise -- this is a manual,
// interactive verification tool, not something CI can run unattended.
type liveEntityClient struct {
	baseURL string
	jwt     string
}

func (c *liveEntityClient) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, bytes.NewReader(body))
}

func (c *liveEntityClient) get(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *liveEntityClient) do(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-jwt-assertion", c.jwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &liveEntityError{status: resp.StatusCode, body: string(respBody)}
	}
	return respBody, nil
}

type liveEntityError struct {
	status int
	body   string
}

func (e *liveEntityError) Error() string {
	return e.body
}

func (c *liveEntityClient) GetAccount(ctx context.Context, id string) ([]byte, error) {
	return c.get(ctx, "/accounts/"+id)
}
func (c *liveEntityClient) GetAccountHealthDetail(ctx context.Context, id string) (entity.CHAccountHealthDetail, error) {
	var out entity.CHAccountHealthDetail
	raw, err := c.get(ctx, "/accounts/"+id+"/health-detail")
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}
func (c *liveEntityClient) SearchAccounts(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/accounts/search", body)
}
func (c *liveEntityClient) SearchProjects(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/projects/search", body)
}
func (c *liveEntityClient) SearchCases(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/cases/search", body)
}
func (c *liveEntityClient) SearchDeployments(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/deployments/search", body)
}
func (c *liveEntityClient) SearchDeployedProducts(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/deployed-products/search", body)
}
func (c *liveEntityClient) SearchEscalations(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/escalations/search", body)
}
func (c *liveEntityClient) SearchTeams(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/teams/search", body)
}
func (c *liveEntityClient) SearchProducts(ctx context.Context, body []byte) ([]byte, error) {
	return c.post(ctx, "/products/search", body)
}

// liveTestClient returns a liveEntityClient and true, or nil and false when
// the required env vars aren't set (the common case -- this test is skipped).
func liveTestClient(t *testing.T) (*liveEntityClient, bool) {
	t.Helper()
	url := os.Getenv("LIVE_ENTITY_SERVICE_TEST_URL")
	jwt := os.Getenv("LIVE_ENTITY_SERVICE_TEST_JWT")
	if url == "" || jwt == "" {
		t.Skip("LIVE_ENTITY_SERVICE_TEST_URL/LIVE_ENTITY_SERVICE_TEST_JWT not set")
	}
	return &liveEntityClient{baseURL: url, jwt: jwt}, true
}

// TestLive_GetABTTeamList runs postgresLookupsClient.GetABTTeamList against a
// real running entity-service.
func TestLive_GetABTTeamList(t *testing.T) {
	client, ok := liveTestClient(t)
	if !ok {
		return
	}
	c := NewPostgresLookupsClient(client)
	teams, err := c.GetABTTeamList(context.Background())
	if err != nil {
		t.Fatalf("GetABTTeamList: %v", err)
	}
	t.Logf("ABT teams: %v", teams)
	if len(teams) == 0 {
		t.Error("expected at least one ABT team from real data")
	}
}

// TestLive_GetEscalationsByAccount runs postgresViewerAccountClient.GetEscalationsByAccount
// against a real running entity-service, for an account id supplied via
// LIVE_ENTITY_SERVICE_TEST_ACCOUNT_ID (a real account UUID known to have
// escalations).
func TestLive_GetEscalationsByAccount(t *testing.T) {
	client, ok := liveTestClient(t)
	if !ok {
		return
	}
	accountID := os.Getenv("LIVE_ENTITY_SERVICE_TEST_ACCOUNT_ID")
	if accountID == "" {
		t.Skip("LIVE_ENTITY_SERVICE_TEST_ACCOUNT_ID not set")
	}
	c := NewPostgresViewerAccountClient(client)
	escalations, err := c.GetEscalationsByAccount(context.Background(), accountID, 0, 10)
	if err != nil {
		t.Fatalf("GetEscalationsByAccount: %v", err)
	}
	raw, _ := json.MarshalIndent(escalations, "", "  ")
	t.Logf("escalations for %s:\n%s", accountID, raw)
	if len(escalations) == 0 {
		t.Error("expected at least one escalation for this account")
	}
}

// TestLive_GetCustomerHealthDetail runs postgresCustomerHealthClient.GetCustomerHealthDetail
// against a real running entity-service, for an account id supplied via
// LIVE_ENTITY_SERVICE_TEST_ACCOUNT_ID.
func TestLive_GetCustomerHealthDetail(t *testing.T) {
	client, ok := liveTestClient(t)
	if !ok {
		return
	}
	accountID := os.Getenv("LIVE_ENTITY_SERVICE_TEST_ACCOUNT_ID")
	if accountID == "" {
		t.Skip("LIVE_ENTITY_SERVICE_TEST_ACCOUNT_ID not set")
	}
	c := NewPostgresCustomerHealthClient(client)
	detail, err := c.GetCustomerHealthDetail(context.Background(), accountID)
	if err != nil {
		t.Fatalf("GetCustomerHealthDetail: %v", err)
	}
	raw, _ := json.MarshalIndent(detail, "", "  ")
	t.Logf("detail for %s:\n%s", accountID, raw)
}
