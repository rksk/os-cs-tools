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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockEntityProductVulnerabilityClient struct {
	searchFn func(ctx context.Context, body []byte) ([]byte, error)
	getFn    func(ctx context.Context, id string) ([]byte, error)
	metaFn   func(ctx context.Context) ([]byte, error)
}

func (m *mockEntityProductVulnerabilityClient) SearchProductVulnerabilities(ctx context.Context, body []byte) ([]byte, error) {
	return m.searchFn(ctx, body)
}

func (m *mockEntityProductVulnerabilityClient) GetProductVulnerability(ctx context.Context, id string) ([]byte, error) {
	return m.getFn(ctx, id)
}

func (m *mockEntityProductVulnerabilityClient) GetVulnerabilityMeta(ctx context.Context) ([]byte, error) {
	return m.metaFn(ctx)
}

func TestGetVulnerabilityMeta(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewProductVulnerabilityHandler(&mockEntityProductVulnerabilityClient{})
		r := httptest.NewRequest(http.MethodGet, "/products/vulnerabilities/meta", nil)
		w := httptest.NewRecorder()
		h.GetVulnerabilityMeta(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
	})

	t.Run("forwards the entity service response verbatim", func(t *testing.T) {
		const wantBody = `{"severities":[{"id":"4","label":"Critical"},{"id":"3","label":"High"}]}`
		entity := &mockEntityProductVulnerabilityClient{
			metaFn: func(ctx context.Context) ([]byte, error) {
				return []byte(wantBody), nil
			},
		}
		h := NewProductVulnerabilityHandler(entity)
		r := withUser(httptest.NewRequest(http.MethodGet, "/products/vulnerabilities/meta", nil))
		w := httptest.NewRecorder()
		h.GetVulnerabilityMeta(w, r)
		assertStatus(t, w, http.StatusOK)
		if got := w.Body.String(); got != wantBody {
			t.Fatalf("body = %q, want %q", got, wantBody)
		}
	})

	t.Run("maps an upstream failure to a generic error", func(t *testing.T) {
		entity := &mockEntityProductVulnerabilityClient{
			metaFn: func(ctx context.Context) ([]byte, error) {
				return nil, errors.New("boom")
			},
		}
		h := NewProductVulnerabilityHandler(entity)
		r := withUser(httptest.NewRequest(http.MethodGet, "/products/vulnerabilities/meta", nil))
		w := httptest.NewRecorder()
		h.GetVulnerabilityMeta(w, r)
		assertStatus(t, w, http.StatusInternalServerError)
	})
}
