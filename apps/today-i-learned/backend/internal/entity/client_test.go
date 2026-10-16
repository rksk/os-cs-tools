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
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// newEntityService fakes the token endpoint and entity-service's account
// search, counting token requests.
func newEntityService(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var tokenCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		if id, secret, ok := r.BasicAuth(); !ok || id != "til" || secret != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"service-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("POST /accounts/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-jwt-assertion") != "service-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct {
			Pagination struct{ Limit, Offset int }
			Filters    struct{ SearchQuery string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Pagination.Limit != searchLimit || body.Filters.SearchQuery != "acme" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"accounts":[{"id":"a1","name":"Acme Corp","region":"EU"},{"id":"a2","name":"Acme Bank"}],"pagination":{}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &tokenCalls
}

func newClient(srv *httptest.Server) *Client {
	return New(Config{BaseURL: srv.URL + "/", TokenURL: srv.URL + "/token", ClientID: "til", ClientSecret: "s3cret"})
}

func TestSearchCustomers(t *testing.T) {
	srv, tokenCalls := newEntityService(t, http.StatusOK)
	c := newClient(srv)
	for range 2 {
		got, err := c.SearchCustomers(context.Background(), "acme")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0] != (Customer{ID: "a1", Name: "Acme Corp"}) || got[1].Name != "Acme Bank" {
			t.Fatalf("SearchCustomers = %+v", got)
		}
	}
	if n := tokenCalls.Load(); n != 1 {
		t.Fatalf("token fetched %d times, want once (reused)", n)
	}
}

func TestSearchCustomersFailsOnAnUpstreamError(t *testing.T) {
	srv, _ := newEntityService(t, http.StatusInternalServerError)
	if _, err := newClient(srv).SearchCustomers(context.Background(), "acme"); err == nil {
		t.Fatal("want an error for a 500")
	}
}

func TestSearchCustomersFailsWithoutAToken(t *testing.T) {
	srv, _ := newEntityService(t, http.StatusOK)
	c := New(Config{BaseURL: srv.URL, TokenURL: srv.URL + "/token", ClientID: "til", ClientSecret: "wrong"})
	if _, err := c.SearchCustomers(context.Background(), "acme"); err == nil {
		t.Fatal("want an error when the token is refused")
	}
}
