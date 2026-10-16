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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A 1st, 2nd or 3rd responder (Level 0) must have a phone number on their CSM
// Profile: the change is refused before it reaches entity-service otherwise.
func TestUpdatePagingMember_ResponderNeedsProfilePhone(t *testing.T) {
	const id = "22222222-2222-2222-2222-222222222222"

	// chain answers the CRE chain with one membership, id, whose
	// hasProfilePhone is the given raw JSON value ("" leaves it out).
	chain := func(hasProfilePhone string) func(context.Context, string) ([]byte, error) {
		field := ""
		if hasProfilePhone != "" {
			field = `,"hasProfilePhone":` + hasProfilePhone
		}
		return func(_ context.Context, family string) ([]byte, error) {
			if family != "CRE" {
				return []byte(`{"members":[]}`), nil
			}
			return []byte(`{"members":[{"membershipId":"` + id + `","name":"Jane Doe","email":"Jane@Example.com"` + field + `}]}`), nil
		}
	}

	run := func(h *ScheduleHandler, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPatch, "/team-schedule/paging-chain/members/"+id, strings.NewReader(body))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h.UpdatePagingMember(w, withUser(r))
		return w
	}

	// handler builds a handler whose entity client records whether the
	// change was forwarded.
	handler := func(hasProfilePhone string, forwarded *bool) *ScheduleHandler {
		return NewScheduleHandler(&mockEntityScheduleClient{
			pagingChainFn: chain(hasProfilePhone),
			pagingMemberFn: func(_ context.Context, _ string, body []byte) ([]byte, error) {
				*forwarded = true
				return body, nil
			},
		})
	}

	for _, rank := range []string{"1", "2", "3"} {
		t.Run("refuses rank "+rank+" without a profile number", func(t *testing.T) {
			var forwarded bool
			w := run(handler("false", &forwarded), `{"responderRank":`+rank+`}`)
			assertStatus(t, w, http.StatusConflict)
			if forwarded {
				t.Fatal("the change reached entity-service")
			}
			body := w.Body.String()
			if !strings.Contains(body, "Jane Doe can't be added to Level 0: a phone number should be included in their CSM Profile") ||
				!strings.Contains(body, errCodeResponderNeedsProfilePhone) {
				t.Fatalf("refusal = %s", body)
			}
		})
	}

	t.Run("lets a responder with a profile number through", func(t *testing.T) {
		var forwarded bool
		assertStatus(t, run(handler("true", &forwarded), `{"responderRank":1}`), http.StatusOK)
		if !forwarded {
			t.Fatal("the change was not forwarded")
		}
	})

	t.Run("never checks clearing a responder or another change", func(t *testing.T) {
		for _, body := range []string{`{"responderRank":0}`, `{"role":"lead"}`} {
			var forwarded bool
			assertStatus(t, run(handler("false", &forwarded), body), http.StatusOK)
			if !forwarded {
				t.Fatalf("%s was not forwarded", body)
			}
		}
	})

	t.Run("a membership no chain lists is left to entity-service", func(t *testing.T) {
		var forwarded bool
		h := NewScheduleHandler(&mockEntityScheduleClient{
			pagingMemberFn: func(_ context.Context, _ string, body []byte) ([]byte, error) {
				forwarded = true
				return body, nil
			},
		})
		assertStatus(t, run(h, `{"responderRank":1}`), http.StatusOK)
		if !forwarded {
			t.Fatal("the change was not forwarded")
		}
	})

	t.Run("with no answer and no checker, entity-service decides", func(t *testing.T) {
		var forwarded bool
		assertStatus(t, run(handler("", &forwarded), `{"responderRank":1}`), http.StatusOK)
		if !forwarded {
			t.Fatal("the change was not forwarded")
		}
	})

	t.Run("a number found through the SCIM fallback counts", func(t *testing.T) {
		var forwarded bool
		sc := &fakeSCIM{phones: map[string]string{"jane@example.com": "+94771234567"}}
		h := handler("false", &forwarded).WithPagingPhones(NewPagingPhoneChecker(sc))
		assertStatus(t, run(h, `{"responderRank":1}`), http.StatusOK)
		if !forwarded {
			t.Fatal("the change was not forwarded")
		}
	})

	t.Run("no number anywhere is refused with the checker on", func(t *testing.T) {
		var forwarded bool
		sc := &fakeSCIM{phones: map[string]string{"jane@example.com": ""}}
		h := handler("false", &forwarded).WithPagingPhones(NewPagingPhoneChecker(sc))
		assertStatus(t, run(h, `{"responderRank":2}`), http.StatusConflict)
		if forwarded {
			t.Fatal("the change reached entity-service")
		}
	})

	t.Run("a check that cannot be made just now refuses rather than guesses", func(t *testing.T) {
		var forwarded bool
		sc := &fakeSCIM{fail: map[string]bool{"jane@example.com": true}}
		h := handler("false", &forwarded).WithPagingPhones(NewPagingPhoneChecker(sc))
		w := run(h, `{"responderRank":1}`)
		assertStatus(t, w, http.StatusServiceUnavailable)
		if forwarded {
			t.Fatal("the change reached entity-service")
		}
		if !strings.Contains(w.Body.String(), "Try again") {
			t.Fatalf("refusal = %s", w.Body.String())
		}
	})
}
