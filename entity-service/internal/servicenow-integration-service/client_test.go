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

package integrationservice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
)

// newTestClient builds a Client pointed at srv with a pre-seeded bearer token
// so tests exercise do()'s status-code switch without standing up a token
// endpoint.
func newTestClient(srv *httptest.Server) *Client {
	c := New(srv.URL, ClientCredentialsConfig{})
	c.cachedToken = "test-token"
	c.tokenExpiry = time.Now().Add(time.Hour)
	return c
}

func TestClient_Do_StatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		check      func(t *testing.T, err error)
	}{
		{
			name:       "409 conflict maps to typed ConflictError with upstream message",
			statusCode: http.StatusConflict,
			body:       `{"message":"Call is already closed."}`,
			check: func(t *testing.T, err error) {
				var ce *apierror.ConflictError
				if !errors.As(err, &ce) {
					t.Fatalf("expected *apierror.ConflictError, got %T: %v", err, err)
				}
				if ce.Msg != "Call is already closed." {
					t.Errorf("Msg = %q, want upstream message preserved", ce.Msg)
				}
			},
		},
		{
			name:       "422 unprocessable entity maps to typed UnprocessableEntityError with upstream message",
			statusCode: http.StatusUnprocessableEntity,
			body:       `{"message":"Call can only be rejected from Pending on WSO2 state."}`,
			check: func(t *testing.T, err error) {
				var uee *apierror.UnprocessableEntityError
				if !errors.As(err, &uee) {
					t.Fatalf("expected *apierror.UnprocessableEntityError, got %T: %v", err, err)
				}
				if uee.Msg != "Call can only be rejected from Pending on WSO2 state." {
					t.Errorf("Msg = %q, want upstream message preserved", uee.Msg)
				}
			},
		},
		{
			name:       "422 with non-JSON body falls back to default message",
			statusCode: http.StatusUnprocessableEntity,
			body:       "not json",
			check: func(t *testing.T, err error) {
				var uee *apierror.UnprocessableEntityError
				if !errors.As(err, &uee) {
					t.Fatalf("expected *apierror.UnprocessableEntityError, got %T: %v", err, err)
				}
				if uee.Msg == "" {
					t.Errorf("expected a non-empty fallback message")
				}
			},
		},
		{
			name:       "500 falls through to the untyped default branch",
			statusCode: http.StatusInternalServerError,
			body:       `{"message":"boom"}`,
			check: func(t *testing.T, err error) {
				var uee *apierror.UnprocessableEntityError
				var ce *apierror.ConflictError
				if errors.As(err, &uee) || errors.As(err, &ce) {
					t.Fatalf("500 should not be mapped to a 4xx typed error, got %T: %v", err, err)
				}
				if err == nil {
					t.Fatalf("expected an error for status 500")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			c := newTestClient(srv)

			_, err := c.Get(context.Background(), "/some/path", "")
			tt.check(t, err)
		})
	}
}
