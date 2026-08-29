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
	"net/http/httptest"
	"testing"
)

// assertStatus fails if the recorded status code differs from want.
func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d; body: %s", w.Code, want, w.Body.String())
	}
}

// assertErrorMessage decodes {"message":"..."} and checks the message field.
func assertErrorMessage(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v; raw: %s", err, w.Body.String())
	}
	if body.Message != want {
		t.Errorf("message = %q, want %q", body.Message, want)
	}
}

// mockStore is a hand-rolled double for alertStore + healthPinger, matching
// this repo's no-mocking-library convention.
type mockStore struct {
	enqueueFn func(ctx context.Context, payload []byte) (string, error)
	pingFn    func(ctx context.Context) error

	enqueuedPayloads [][]byte
}

func (m *mockStore) Enqueue(ctx context.Context, payload []byte) (string, error) {
	m.enqueuedPayloads = append(m.enqueuedPayloads, payload)
	if m.enqueueFn != nil {
		return m.enqueueFn(ctx, payload)
	}
	return "generated-id", nil
}

func (m *mockStore) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return nil
}
