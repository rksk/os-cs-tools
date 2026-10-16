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
	"net/http/httptest"
	"testing"
)

func newTestCustomerClient(t *testing.T, tokenSrv, apiSrv *httptest.Server) *CustomerEntityClient {
	t.Helper()
	return NewCustomerEntityClient(CustomerEntityConfig{
		BaseURL:      apiSrv.URL,
		TokenURL:     tokenSrv.URL,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Scopes:       []string{"customer"},
	})
}

func newCustomerTokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
}

// TestSearchUsersByEmail_BatchesRequestsOverLimit verifies that a call with
// more emails than searchUsersByEmailLimit results in multiple upstream
// POST /users/search requests, each capped at searchUsersByEmailLimit
// emails, with the responses concatenated into a single result.
func TestSearchUsersByEmail_BatchesRequestsOverLimit(t *testing.T) {
	emailCount := searchUsersByEmailLimit + 1
	emails := make([]string, emailCount)
	for i := range emails {
		emails[i] = fmt.Sprintf("user%d@example.com", i)
	}

	var requestedBatches [][]string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req searchUsersByEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requestedBatches = append(requestedBatches, req.Filters.Emails)

		users := make([]UserRoleInfo, len(req.Filters.Emails))
		for i, email := range req.Filters.Emails {
			users[i] = UserRoleInfo{Email: email, Roles: []string{"customer"}, UserType: "customer"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"users": users})
	}))
	defer apiSrv.Close()
	tokenSrv := newCustomerTokenServer(t)
	defer tokenSrv.Close()

	c := newTestCustomerClient(t, tokenSrv, apiSrv)

	got, err := c.SearchUsersByEmail(context.Background(), emails)
	if err != nil {
		t.Fatalf("SearchUsersByEmail returned error: %v", err)
	}

	if len(requestedBatches) != 2 {
		t.Fatalf("upstream received %d requests, want 2 (one full batch of %d, one with the remainder)", len(requestedBatches), searchUsersByEmailLimit)
	}
	if len(requestedBatches[0]) != searchUsersByEmailLimit {
		t.Errorf("first batch size = %d, want %d", len(requestedBatches[0]), searchUsersByEmailLimit)
	}
	if len(requestedBatches[1]) != 1 {
		t.Errorf("second batch size = %d, want 1", len(requestedBatches[1]))
	}

	if len(got) != emailCount {
		t.Fatalf("SearchUsersByEmail returned %d users, want %d (results from all batches concatenated)", len(got), emailCount)
	}
	seen := make(map[string]bool, len(got))
	for _, u := range got {
		seen[u.Email] = true
	}
	for _, email := range emails {
		if !seen[email] {
			t.Errorf("result missing user %q — a batch's results were dropped", email)
		}
	}
}

// TestSearchUsersByEmail_SingleBatchUnderLimit verifies the common case (a
// recipient list under searchUsersByEmailLimit) makes exactly one upstream
// request, not one per email.
func TestSearchUsersByEmail_SingleBatchUnderLimit(t *testing.T) {
	emails := []string{"alice@example.com", "bob@example.com"}

	requests := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"users": []UserRoleInfo{
				{Email: "alice@example.com", Roles: []string{"customer"}, UserType: "customer"},
				{Email: "bob@example.com", Roles: []string{"csm-agent"}, UserType: "internal"},
			},
		})
	}))
	defer apiSrv.Close()
	tokenSrv := newCustomerTokenServer(t)
	defer tokenSrv.Close()

	c := newTestCustomerClient(t, tokenSrv, apiSrv)

	got, err := c.SearchUsersByEmail(context.Background(), emails)
	if err != nil {
		t.Fatalf("SearchUsersByEmail returned error: %v", err)
	}
	if requests != 1 {
		t.Errorf("upstream received %d requests, want 1", requests)
	}
	if len(got) != 2 {
		t.Fatalf("got %d users, want 2", len(got))
	}
}

// TestSearchUsersByEmail_EmptyInput verifies no upstream request is made for
// an empty email list.
func TestSearchUsersByEmail_EmptyInput(t *testing.T) {
	called := false
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer apiSrv.Close()
	tokenSrv := newCustomerTokenServer(t)
	defer tokenSrv.Close()

	c := newTestCustomerClient(t, tokenSrv, apiSrv)

	got, err := c.SearchUsersByEmail(context.Background(), nil)
	if err != nil {
		t.Fatalf("SearchUsersByEmail returned error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if called {
		t.Error("upstream should not have been called for an empty email list")
	}
}

// TestGetCaseDescription_SendsBearerToken verifies GetCaseDescription sends
// the same OAuth2 client-credentials bearer token every other method on this
// client already does (see SearchUsersByEmail's own tests) — entity-service
// resolves this caller's identity from it (its own gateway translates the
// token into its internal x-jwt-assertion form), the same path every
// existing call on this client already relies on, so this one needs no
// header handling of its own.
func TestGetCaseDescription_SendsBearerToken(t *testing.T) {
	var gotAuthorization string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"description": "full description"})
	}))
	defer apiSrv.Close()
	tokenSrv := newCustomerTokenServer(t)
	defer tokenSrv.Close()

	c := newTestCustomerClient(t, tokenSrv, apiSrv)

	got, err := c.GetCaseDescription(context.Background(), "case-1")
	if err != nil {
		t.Fatalf("GetCaseDescription returned error: %v", err)
	}
	if got != "full description" {
		t.Errorf("description = %q, want %q", got, "full description")
	}
	if gotAuthorization != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuthorization, "Bearer test-token")
	}
}

// TestGetCommentContent_SendsBearerToken is GetCaseDescription's
// GET /comments/{id} counterpart — see that test's own doc comment.
func TestGetCommentContent_SendsBearerToken(t *testing.T) {
	var gotAuthorization string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"content": "full comment"})
	}))
	defer apiSrv.Close()
	tokenSrv := newCustomerTokenServer(t)
	defer tokenSrv.Close()

	c := newTestCustomerClient(t, tokenSrv, apiSrv)

	got, err := c.GetCommentContent(context.Background(), "comment-1")
	if err != nil {
		t.Fatalf("GetCommentContent returned error: %v", err)
	}
	if got != "full comment" {
		t.Errorf("content = %q, want %q", got, "full comment")
	}
	if gotAuthorization != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuthorization, "Bearer test-token")
	}
}
