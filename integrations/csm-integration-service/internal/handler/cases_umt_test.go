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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testCaseNumber = "CS0012345"
const testCaseID = "11111111-1111-1111-1111-111111111111"

// searchFoundCase returns a searchCasesFn that resolves testCaseNumber to
// testCaseID, mirroring a real entity-service POST /cases/search response.
func searchFoundCase() func(context.Context, []byte) ([]byte, error) {
	return func(_ context.Context, _ []byte) ([]byte, error) {
		return []byte(`{"cases":[{"id":"` + testCaseID + `","number":"` + testCaseNumber + `"}],"total":1,"limit":0,"offset":0}`), nil
	}
}

func TestUpdates_RequestValidation(t *testing.T) {
	t.Run("rejects missing caseNumber", func(t *testing.T) {
		h := NewCaseHandler(&mockEntityCaseClient{}, "")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`{"label":"urgent"}`))
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgCaseNumberRequired)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects blank caseNumber", func(t *testing.T) {
		h := NewCaseHandler(&mockEntityCaseClient{}, "")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`{"caseNumber":"   ","label":"urgent"}`))
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgCaseNumberRequired)
	})

	t.Run("rejects body exceeding 1 MiB", func(t *testing.T) {
		h := NewCaseHandler(&mockEntityCaseClient{}, "")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1)))
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusRequestEntityTooLarge)
		assertErrorMessage(t, w, ErrMsgTooLarge)
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		h := NewCaseHandler(&mockEntityCaseClient{}, "")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`not-json`))
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
	})

	t.Run("rejects empty body", func(t *testing.T) {
		h := NewCaseHandler(&mockEntityCaseClient{}, "")
		r := httptest.NewRequest(http.MethodPost, "/updates", nil)
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
	})

	t.Run("rejects caseNumber with no action field", func(t *testing.T) {
		h := NewCaseHandler(&mockEntityCaseClient{}, "")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`{"caseNumber":"`+testCaseNumber+`"}`))
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgUpdatesActionRequired)
	})

	t.Run("rejects markFixIssued: false", func(t *testing.T) {
		h := NewCaseHandler(&mockEntityCaseClient{}, "")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`{"caseNumber":"`+testCaseNumber+`","markFixIssued":false}`))
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgMarkFixIssuedMustBeTrue)
	})
}

func TestUpdates_CaseResolution(t *testing.T) {
	t.Run("case not found: 404, no leg calls attempted", func(t *testing.T) {
		legCalled := false
		client := &mockEntityCaseClient{
			searchCasesFn: func(_ context.Context, _ []byte) ([]byte, error) {
				return []byte(`{"cases":[],"total":0,"limit":0,"offset":0}`), nil
			},
			addCaseTagFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				legCalled = true
				return []byte(`{}`), nil
			},
		}
		h := NewCaseHandler(client, "svc@example.com")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`{"caseNumber":"CS9999999","label":"urgent"}`))
		w := httptest.NewRecorder()
		h.Updates(w, r)
		assertStatus(t, w, http.StatusNotFound)
		if legCalled {
			t.Error("a leg was attempted after case resolution failed with not-found")
		}
	})

	t.Run("search upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrors("Failed to look up case.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityCaseClient{
					searchCasesFn: func(_ context.Context, _ []byte) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewCaseHandler(client, "")
				r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`{"caseNumber":"`+testCaseNumber+`","label":"urgent"}`))
				w := httptest.NewRecorder()
				h.Updates(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
			})
		}
	})

	t.Run("builds an exact-match filter on caseNumber", func(t *testing.T) {
		var capturedBody []byte
		client := &mockEntityCaseClient{
			searchCasesFn: func(_ context.Context, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"cases":[{"id":"` + testCaseID + `","number":"` + testCaseNumber + `"}],"total":1,"limit":0,"offset":0}`), nil
			},
			addCaseTagFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				return []byte(`{"id":"tag-1","label":"urgent"}`), nil
			},
		}
		h := NewCaseHandler(client, "svc@example.com")
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(`{"caseNumber":"`+testCaseNumber+`","label":"urgent"}`))
		w := httptest.NewRecorder()
		h.Updates(w, r)

		var sent struct {
			Filters struct {
				Filters []struct {
					Field  string   `json:"field"`
					Op     string   `json:"op"`
					Values []string `json:"values"`
				} `json:"filters"`
			} `json:"filters"`
		}
		if err := json.Unmarshal(capturedBody, &sent); err != nil {
			t.Fatalf("decode captured body: %v; raw: %s", err, capturedBody)
		}
		if len(sent.Filters.Filters) != 1 {
			t.Fatalf("filters = %v, want exactly 1", sent.Filters.Filters)
		}
		f := sent.Filters.Filters[0]
		if f.Field != "number" || f.Op != "eq" || len(f.Values) != 1 || f.Values[0] != testCaseNumber {
			t.Errorf("filter = %+v, want field=number op=eq values=[%s]", f, testCaseNumber)
		}
	})
}

func TestUpdates_LabelLeg(t *testing.T) {
	t.Run("injects the configured actorEmail and ignores a caller-supplied one; only label key present", func(t *testing.T) {
		var capturedCaseID string
		var capturedBody []byte
		client := &mockEntityCaseClient{
			searchCasesFn: searchFoundCase(),
			addCaseTagFn: func(_ context.Context, id string, body []byte) ([]byte, error) {
				capturedCaseID = id
				capturedBody = body
				return []byte(`{"id":"tag-1","label":"urgent"}`), nil
			},
		}
		h := NewCaseHandler(client, "svc@example.com")
		reqBody := `{"caseNumber":"` + testCaseNumber + `","label":"urgent","actorEmail":"attacker@example.com"}`
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		h.Updates(w, r)

		assertStatus(t, w, http.StatusOK)
		if capturedCaseID != testCaseID {
			t.Errorf("caseID = %q, want %q", capturedCaseID, testCaseID)
		}

		var sent struct {
			Label      string `json:"label"`
			ActorEmail string `json:"actorEmail"`
		}
		if err := json.Unmarshal(capturedBody, &sent); err != nil {
			t.Fatalf("decode captured body: %v; raw: %s", err, capturedBody)
		}
		if sent.Label != "urgent" {
			t.Errorf("label = %q, want %q", sent.Label, "urgent")
		}
		if sent.ActorEmail != "svc@example.com" {
			t.Errorf("actorEmail = %q, want the configured service actor email, never the caller-supplied one", sent.ActorEmail)
		}

		resp := decodeJSON[map[string]any](t, w)
		if resp["caseId"] != testCaseID {
			t.Errorf("caseId = %v, want %q", resp["caseId"], testCaseID)
		}
		if _, ok := resp["label"]; !ok {
			t.Error("label key missing from response")
		}
		for _, key := range []string{"fixEta", "markFixIssued", "comment"} {
			if _, ok := resp[key]; ok {
				t.Errorf("%s key present in response, want absent (not requested)", key)
			}
		}
	})
}

func TestUpdates_FixEtaLeg(t *testing.T) {
	t.Run("sends only the fix-ETA fields the caller supplied", func(t *testing.T) {
		var capturedBody []byte
		client := &mockEntityCaseClient{
			searchCasesFn: searchFoundCase(),
			patchCaseFn: func(_ context.Context, _ string, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"message":"Case updated successfully","case":{"id":"` + testCaseID + `"}}`), nil
			},
		}
		h := NewCaseHandler(client, "")
		reqBody := `{"caseNumber":"` + testCaseNumber + `","bestCaseFixEta":"2026-10-15","worstCaseFixEta":"2026-11-05"}`
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		h.Updates(w, r)

		assertStatus(t, w, http.StatusOK)
		var sent map[string]any
		if err := json.Unmarshal(capturedBody, &sent); err != nil {
			t.Fatalf("decode captured body: %v; raw: %s", err, capturedBody)
		}
		if sent["bestCaseFixEta"] != "2026-10-15" {
			t.Errorf("bestCaseFixEta = %v, want 2026-10-15", sent["bestCaseFixEta"])
		}
		if sent["worstCaseFixEta"] != "2026-11-05" {
			t.Errorf("worstCaseFixEta = %v, want 2026-11-05", sent["worstCaseFixEta"])
		}
		if _, present := sent["mostLikelyFixEta"]; present {
			t.Errorf("mostLikelyFixEta present in upstream body, want omitted (not supplied by caller)")
		}

		resp := decodeJSON[map[string]any](t, w)
		if _, ok := resp["fixEta"]; !ok {
			t.Error("fixEta key missing from response")
		}
	})
}

func TestUpdates_MarkFixIssuedLeg(t *testing.T) {
	t.Run("sends its own PATCH call, separate from fix-ETA", func(t *testing.T) {
		var patchCalls []map[string]any
		client := &mockEntityCaseClient{
			searchCasesFn: searchFoundCase(),
			patchCaseFn: func(_ context.Context, _ string, body []byte) ([]byte, error) {
				var m map[string]any
				_ = json.Unmarshal(body, &m)
				patchCalls = append(patchCalls, m)
				return []byte(`{"message":"Case updated successfully","case":{"id":"` + testCaseID + `"}}`), nil
			},
		}
		h := NewCaseHandler(client, "")
		reqBody := `{"caseNumber":"` + testCaseNumber + `","bestCaseFixEta":"2026-10-15","markFixIssued":true}`
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		h.Updates(w, r)

		assertStatus(t, w, http.StatusOK)
		if len(patchCalls) != 2 {
			t.Fatalf("PatchCase called %d times, want 2 (one for fix-ETA, one for markFixIssued)", len(patchCalls))
		}
		var sawFixEta, sawMarkFixIssued bool
		for _, call := range patchCalls {
			if _, ok := call["bestCaseFixEta"]; ok {
				sawFixEta = true
				if _, ok := call["markFixIssued"]; ok {
					t.Error("a single PatchCase call carried both bestCaseFixEta and markFixIssued, want them in separate calls")
				}
			}
			if v, ok := call["markFixIssued"]; ok {
				sawMarkFixIssued = true
				if v != true {
					t.Errorf("markFixIssued = %v, want true", v)
				}
			}
		}
		if !sawFixEta || !sawMarkFixIssued {
			t.Errorf("expected one call carrying bestCaseFixEta and one carrying markFixIssued, got %+v", patchCalls)
		}

		resp := decodeJSON[map[string]any](t, w)
		if _, ok := resp["markFixIssued"]; !ok {
			t.Error("markFixIssued key missing from response")
		}
	})
}

func TestUpdates_CommentLeg(t *testing.T) {
	t.Run("injects actorEmail and folds updateLevel into content", func(t *testing.T) {
		var capturedBody []byte
		client := &mockEntityCaseClient{
			searchCasesFn: searchFoundCase(),
			createCaseCommentFn: func(_ context.Context, _ string, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"message":"Comment created successfully","comment":{"id":"c-1"}}`), nil
			},
		}
		h := NewCaseHandler(client, "svc@example.com")
		reqBody := `{"caseNumber":"` + testCaseNumber + `","comment":"Fix issued.","updateLevel":"1.2.3"}`
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		h.Updates(w, r)

		assertStatus(t, w, http.StatusOK)
		var sent struct {
			Type       string `json:"type"`
			Content    string `json:"content"`
			ActorEmail string `json:"actorEmail"`
		}
		if err := json.Unmarshal(capturedBody, &sent); err != nil {
			t.Fatalf("decode captured body: %v; raw: %s", err, capturedBody)
		}
		if sent.Type != "comment" {
			t.Errorf("type = %q, want %q", sent.Type, "comment")
		}
		if !strings.Contains(sent.Content, "1.2.3") || !strings.Contains(sent.Content, "Fix issued.") {
			t.Errorf("content = %q, want it to fold in updateLevel and the original comment", sent.Content)
		}
		if sent.ActorEmail != "svc@example.com" {
			t.Errorf("actorEmail = %q, want the configured service actor email", sent.ActorEmail)
		}
	})
}

func TestUpdates_AllLegsTogether(t *testing.T) {
	t.Run("all four legs requested: all attempted, all reported independently", func(t *testing.T) {
		var patchCalls, addTagCalls, commentCalls int
		client := &mockEntityCaseClient{
			searchCasesFn: searchFoundCase(),
			addCaseTagFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				addTagCalls++
				return []byte(`{"id":"tag-1","label":"urgent"}`), nil
			},
			patchCaseFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				patchCalls++
				return []byte(`{"message":"Case updated successfully","case":{"id":"` + testCaseID + `"}}`), nil
			},
			createCaseCommentFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				commentCalls++
				return []byte(`{"message":"Comment created successfully","comment":{"id":"c-1"}}`), nil
			},
		}
		h := NewCaseHandler(client, "svc@example.com")
		reqBody := `{"caseNumber":"` + testCaseNumber + `","label":"urgent","bestCaseFixEta":"2026-10-15","markFixIssued":true,"comment":"Fix issued."}`
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		h.Updates(w, r)

		assertStatus(t, w, http.StatusOK)
		if addTagCalls != 1 {
			t.Errorf("AddCaseTag called %d times, want 1", addTagCalls)
		}
		if patchCalls != 2 {
			t.Errorf("PatchCase called %d times, want 2 (fix-ETA + markFixIssued)", patchCalls)
		}
		if commentCalls != 1 {
			t.Errorf("CreateCaseComment called %d times, want 1", commentCalls)
		}

		resp := decodeJSON[map[string]any](t, w)
		for _, key := range []string{"label", "fixEta", "markFixIssued", "comment"} {
			leg, ok := resp[key].(map[string]any)
			if !ok {
				t.Fatalf("%s missing or not an object in response: %v", key, resp[key])
			}
			if leg["success"] != true {
				t.Errorf("%s.success = %v, want true", key, leg["success"])
			}
		}
	})

	t.Run("partial failure: each leg's outcome is independent", func(t *testing.T) {
		client := &mockEntityCaseClient{
			searchCasesFn: searchFoundCase(),
			addCaseTagFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				return []byte(`{"id":"tag-1","label":"urgent"}`), nil
			},
			patchCaseFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				return nil, upstreamErrors("")[0].err
			},
			createCaseCommentFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				return []byte(`{"message":"Comment created successfully","comment":{"id":"c-1"}}`), nil
			},
		}
		h := NewCaseHandler(client, "svc@example.com")
		reqBody := `{"caseNumber":"` + testCaseNumber + `","label":"urgent","markFixIssued":true,"comment":"Fix issued."}`
		r := httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(reqBody))
		w := httptest.NewRecorder()
		h.Updates(w, r)

		assertStatus(t, w, http.StatusOK)
		resp := decodeJSON[map[string]any](t, w)
		label, _ := resp["label"].(map[string]any)
		markFixIssued, _ := resp["markFixIssued"].(map[string]any)
		comment, _ := resp["comment"].(map[string]any)
		if label["success"] != true {
			t.Errorf("label.success = %v, want true", label["success"])
		}
		if markFixIssued["success"] != false {
			t.Errorf("markFixIssued.success = %v, want false (PatchCase errored)", markFixIssued["success"])
		}
		if comment["success"] != true {
			t.Errorf("comment.success = %v, want true", comment["success"])
		}
	})
}
