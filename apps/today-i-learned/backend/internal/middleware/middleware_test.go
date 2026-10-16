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

package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if _, err := io.ReadAll(r.Body); err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	w.WriteHeader(http.StatusOK)
})

func TestCORSAllowsOnlyListedOrigins(t *testing.T) {
	h := CORS([]string{"https://one.example.com"})(ok)

	req := httptest.NewRequest(http.MethodGet, "/submissions", nil)
	req.Header.Set("Origin", "https://one.example.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://one.example.com" {
		t.Fatalf("allowed origin: Access-Control-Allow-Origin = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/submissions", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("other origin: Access-Control-Allow-Origin = %q", got)
	}
	if w.Header().Get("Vary") != "Origin" {
		t.Fatal("missing Vary: Origin")
	}
}

func TestCORSAnswersPreflightWithoutReachingTheHandler(t *testing.T) {
	reached := false
	h := CORS([]string{"https://one.example.com"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	req := httptest.NewRequest(http.MethodOptions, "/submissions", nil)
	req.Header.Set("Origin", "https://one.example.com")
	req.Header.Set("Access-Control-Request-Method", "PUT")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || reached {
		t.Fatalf("preflight: status %d, reached handler %v", w.Code, reached)
	}
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatalf("Allow-Methods = %q", w.Header().Get("Access-Control-Allow-Methods"))
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must never be allowed")
	}
}

func TestBodyLimit(t *testing.T) {
	h := BodyLimit(10)(ok)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader("0123456789")))
	if w.Code != http.StatusOK {
		t.Fatalf("at the limit: %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/submissions", strings.NewReader("0123456789x")))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("declared over the limit: %d", w.Code)
	}

	// No Content-Length: caught while reading.
	req := httptest.NewRequest(http.MethodPost, "/submissions", io.NopCloser(strings.NewReader("0123456789x")))
	req.ContentLength = -1
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("undeclared over the limit: %d", w.Code)
	}
}

func TestCorrelationID(t *testing.T) {
	h := CorrelationID(ok)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(correlationIDHeader, "abc-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get(correlationIDHeader); got != "abc-123" {
		t.Fatalf("echoed id = %q", got)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := w.Header().Get(correlationIDHeader); len(got) != 36 {
		t.Fatalf("generated id = %q", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	SecurityHeaders(ok).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Strict-Transport-Security") == "" {
		t.Fatalf("headers = %v", w.Header())
	}
}
