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
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
)

type fakeCaseAlertClient struct {
	op    string
	id    string
	body  map[string]any
	err   error
	reply []byte
}

func (f *fakeCaseAlertClient) record(op, id string, body []byte) ([]byte, error) {
	f.op, f.id = op, id
	if body != nil {
		f.body = map[string]any{}
		_ = json.Unmarshal(body, &f.body)
	}
	if f.reply != nil {
		return f.reply, f.err
	}
	return []byte(`{"ok":true}`), f.err
}

func (f *fakeCaseAlertClient) GetCaseAlertSettings(context.Context) ([]byte, error) {
	return f.record("get", "", nil)
}
func (f *fakeCaseAlertClient) PutCaseAlertRoom(_ context.Context, b []byte) ([]byte, error) {
	return f.record("put-room", "", b)
}
func (f *fakeCaseAlertClient) PutCaseAlertWebhook(_ context.Context, id string, b []byte) ([]byte, error) {
	return f.record("put-webhook", id, b)
}
func (f *fakeCaseAlertClient) DeleteCaseAlertRoom(_ context.Context, id string) ([]byte, error) {
	return f.record("delete-room", id, nil)
}
func (f *fakeCaseAlertClient) TestCaseAlertRoom(_ context.Context, id string) ([]byte, error) {
	return f.record("test-room", id, nil)
}
func (f *fakeCaseAlertClient) PutCaseAlertRoute(_ context.Context, b []byte) ([]byte, error) {
	return f.record("put-route", "", b)
}
func (f *fakeCaseAlertClient) DeleteCaseAlertRoute(_ context.Context, id string) ([]byte, error) {
	return f.record("delete-route", id, nil)
}
func (f *fakeCaseAlertClient) PutCaseAlertValues(_ context.Context, b []byte) ([]byte, error) {
	return f.record("put-values", "", b)
}
func (f *fakeCaseAlertClient) DeleteCaseAlertValue(_ context.Context, id string) ([]byte, error) {
	return f.record("delete-value", id, nil)
}

const caseAlertRoomID = "22222222-2222-2222-2222-222222222222"

func caseAlertRequest(method, path, body string, pathValues ...string) *http.Request {
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for i := 0; i+1 < len(pathValues); i += 2 {
		r.SetPathValue(pathValues[i], pathValues[i+1])
	}
	return withUser(r)
}

// Each write is forwarded with only the fields it knows; anything else in the
// request stays behind.
func TestCaseAlertSettings_ForwardsKnownFieldsOnly(t *testing.T) {
	cases := []struct {
		name   string
		call   func(*CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request)
		req    *http.Request
		status int
		op     string
		id     string
		body   map[string]any
	}{
		{"room", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","teamKey":"apollo","label":"Apollo SRE","teamless":true,"createdBy":"someone else"}`),
			http.StatusOK, "put-room", "", map[string]any{"ladder": "SRE", "teamKey": "apollo", "label": "Apollo SRE", "teamless": true}},
		{"SME team room", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SME","teamKey":"asgardeo","label":"Asgardeo SME"}`),
			http.StatusOK, "put-room", "", map[string]any{"ladder": "SME", "teamKey": "asgardeo", "label": "Asgardeo SME"}},
		{"team room channel", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","teamKey":"iaas","label":"IaaS SRE","enabled":true,"channel":"chat","phone":"x"}`),
			http.StatusOK, "put-room", "", map[string]any{"ladder": "SRE", "teamKey": "iaas", "label": "IaaS SRE", "enabled": true, "channel": "chat"}},
		{"team room back to the ladder's channel", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertRoom
		},
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","teamKey":"atlas","label":"Atlas","channel":""}`),
			http.StatusOK, "put-room", "", map[string]any{"ladder": "CRE", "teamKey": "atlas", "label": "Atlas", "channel": ""}},
		{"webhook", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertWebhook
		},
			caseAlertRequest(http.MethodPut, "/x", `{"url":"https://chat.googleapis.com/v1/spaces/A/messages?key=k&token=t","extra":1}`, "roomId", caseAlertRoomID),
			http.StatusOK, "put-webhook", caseAlertRoomID, map[string]any{"url": "https://chat.googleapis.com/v1/spaces/A/messages?key=k&token=t"}},
		{"route to a rota", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoute },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"sre","groupName":"default sre group","rotaCode":"SRE_SAAS","enabled":false}`),
			http.StatusOK, "put-route", "", map[string]any{"ladder": "sre", "groupName": "default sre group", "rotaCode": "SRE_SAAS", "enabled": false}},
		{"test card", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.TestCaseAlertRoom },
			caseAlertRequest(http.MethodPost, "/x", "", "roomId", caseAlertRoomID), http.StatusOK, "test-room", caseAlertRoomID, nil},
		{"remove room", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.DeleteCaseAlertRoom
		},
			caseAlertRequest(http.MethodDelete, "/x", "", "roomId", caseAlertRoomID), http.StatusNoContent, "delete-room", caseAlertRoomID, nil},
		{"remove route", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.DeleteCaseAlertRoute
		},
			caseAlertRequest(http.MethodDelete, "/x", "", "routeId", caseAlertRoomID), http.StatusNoContent, "delete-route", caseAlertRoomID, nil},
		{"ladder values", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertValues
		},
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":{"teams.abts":["atlas","castor"],"heads.cre":{"name":"N","email":"e@x"},`+
				`"teams.teamLeadsToCall":3,"acknowledgement.requireBoth":true},"updatedBy":"someone else"}`),
			http.StatusOK, "put-values", "", map[string]any{"ladder": "CRE", "values": map[string]any{
				"teams.abts": []any{"atlas", "castor"}, "heads.cre": map[string]any{"name": "N", "email": "e@x"},
				"teams.teamLeadsToCall": float64(3), "acknowledgement.requireBoth": true}}},
		{"remove value", func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.DeleteCaseAlertValue
		},
			caseAlertRequest(http.MethodDelete, "/x", "", "valueId", caseAlertRoomID), http.StatusNoContent, "delete-value", caseAlertRoomID, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeCaseAlertClient{}
			w := httptest.NewRecorder()
			c.call(NewCaseAlertSettingsHandler(fake))(w, c.req)
			if w.Code != c.status || fake.op != c.op || fake.id != c.id {
				t.Fatalf("status %d, forwarded %q %q", w.Code, fake.op, fake.id)
			}
			if c.body != nil && !reflect.DeepEqual(fake.body, c.body) {
				t.Fatalf("forwarded body %v, want %v", fake.body, c.body)
			}
		})
	}
}

func TestCaseAlertSettings_RefusesABadRequestWithoutCallingOut(t *testing.T) {
	cases := map[string]struct {
		call func(*CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request)
		req  *http.Request
	}{
		"room on another ladder": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"PAAS","label":"x"}`)},
		"values on another ladder": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertValues
		},
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"PAAS","values":{"onLeave":"call"}}`)},
		"route on SME": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoute },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SME","groupName":"g","teamKey":"asgardeo"}`)},
		"room without label": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","label":" "}`)},
		"room label over 100": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","label":"`+strings.Repeat("l", 101)+`"}`)},
		"room with a bad team key": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","label":"x","teamKey":"../rooms"}`)},
		"webhook without url": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertWebhook
		},
			caseAlertRequest(http.MethodPut, "/x", `{"url":""}`, "roomId", caseAlertRoomID)},
		"webhook far too long": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertWebhook
		},
			caseAlertRequest(http.MethodPut, "/x", `{"url":"https://chat.googleapis.com/`+strings.Repeat("a", 2100)+`"}`, "roomId", caseAlertRoomID)},
		"webhook on a bad room id": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertWebhook
		},
			caseAlertRequest(http.MethodPut, "/x", `{"url":"https://chat.googleapis.com/x"}`, "roomId", "room-1")},
		"route to a team and a rota": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoute },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","groupName":"g","teamKey":"apollo","rotaCode":"SRE_SAAS"}`)},
		"route to neither": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoute },
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","groupName":"g"}`)},
		"test a bad room id": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.TestCaseAlertRoom },
			caseAlertRequest(http.MethodPost, "/x", "", "roomId", "nope")},
		"room with an unknown channel": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertRoom
		},
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","teamKey":"iaas","label":"x","channel":"sms"}`)},
		"room with an upper-case channel": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertRoom
		},
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","teamKey":"iaas","label":"x","channel":"CHAT"}`)},
		"room with a channel that is not text": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertRoom
		},
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","teamKey":"iaas","label":"x","channel":1}`)},
		"shared room with a channel": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.PutCaseAlertRoom
		},
			caseAlertRequest(http.MethodPut, "/x", `{"ladder":"SRE","label":"SRE Case Alerts","channel":"chat"}`)},
		"not JSON": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) { return h.PutCaseAlertRoom },
			caseAlertRequest(http.MethodPut, "/x", `ladder=SRE`)},
		"values not an object":      {putValues, caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":["onLeave","skip"]}`)},
		"no values":                 {putValues, caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":{}}`)},
		"values missing":            {putValues, caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE"}`)},
		"too many values":           {putValues, caseAlertRequest(http.MethodPut, "/x", caseAlertValuesBody(51, "x"))},
		"a key with a slash":        {putValues, caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":{"teams/abts":["atlas"]}}`)},
		"a key with a number first": {putValues, caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":{"1teams":["atlas"]}}`)},
		"a key far too long":        {putValues, caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":{"t`+strings.Repeat("a", 64)+`":1}}`)},
		"a value far too big":       {putValues, caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":{"teams.abts":["`+strings.Repeat("a", 8200)+`"]}}`)},
		"remove a bad value id": {func(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
			return h.DeleteCaseAlertValue
		},
			caseAlertRequest(http.MethodDelete, "/x", "", "valueId", "../rooms")},
	}
	for name, c := range cases {
		fake := &fakeCaseAlertClient{}
		w := httptest.NewRecorder()
		c.call(NewCaseAlertSettingsHandler(fake))(w, c.req)
		if w.Code != http.StatusBadRequest || fake.op != "" {
			t.Errorf("%s: status %d, forwarded %q", name, w.Code, fake.op)
		}
	}
}

func putValues(h *CaseAlertSettingsHandler) func(http.ResponseWriter, *http.Request) {
	return h.PutCaseAlertValues
}

// caseAlertValuesBody is a values request with n keys, each set to a string.
func caseAlertValuesBody(n int, value string) string {
	values := map[string]string{}
	for i := 0; i < n; i++ {
		values["key"+strconv.Itoa(i)] = value
	}
	return string(mustJSON(map[string]any{"ladder": "CRE", "values": values}))
}

// Fifty keys and a value just under 8 KB are still a fair request.
func TestCaseAlertSettings_TakesValuesAtTheLimits(t *testing.T) {
	for name, body := range map[string]string{
		"fifty keys":           caseAlertValuesBody(50, "x"),
		"a big value":          `{"ladder":"SRE","values":{"trigger.teams":["` + strings.Repeat("a", 8000) + `"]}}`,
		"a 64-char key":        `{"ladder":"SRE","values":{"t` + strings.Repeat("a", 63) + `":true}}`,
		"a value that is null": `{"ladder":"SRE","values":{"timing.interval":null}}`,
	} {
		fake := &fakeCaseAlertClient{}
		w := httptest.NewRecorder()
		NewCaseAlertSettingsHandler(fake).PutCaseAlertValues(w, caseAlertRequest(http.MethodPut, "/x", body))
		if w.Code != http.StatusOK || fake.op != "put-values" {
			t.Errorf("%s: status %d, forwarded %q", name, w.Code, fake.op)
		}
	}
}

// The settings read is passed through as it comes: the saved ladder values
// and what the alert service reported running, the file-only settings
// included, reach the tab untouched.
func TestCaseAlertSettings_GetPassesValuesAndRunningThrough(t *testing.T) {
	reply := `{"canManage":{"view":true,"roomLadders":["CRE"]},"rooms":[],"routes":[],` +
		`"values":[{"id":"` + caseAlertRoomID + `","ladder":"CRE","key":"teams.abts","value":["atlas","castor"],"updatedBy":"a@b","updatedOn":"2026-10-13T10:00:00Z"}],` +
		`"running":{"reportedAt":"2026-10-13T10:00:00Z","mode":"shadow","ladders":{"CRE":{"teams.abts":{"value":["atlas"],"source":"file"}},` +
		`"SRE":{"timing.interval":{"value":"5m","source":"saved"}}},` +
		`"fileOnly":{"CRE":{"enabled":true,"channel":"both","safety.maxCallsPerLadder":32,"safety.maxLevel":3,"safety.allowedNumbers":0,` +
		`"safety.callWithoutVerifiedLeads":false,"safety.callHeadsWithoutLowerTiers":false,"phones.source":"profile"}}}}`
	fake := &fakeCaseAlertClient{reply: []byte(reply)}
	w := httptest.NewRecorder()
	NewCaseAlertSettingsHandler(fake).GetCaseAlertSettings(w, caseAlertRequest(http.MethodGet, "/x", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got, want map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal([]byte(reply), &want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A refused value keeps entity-service's words, which name the key, so the
// tab can show which setting was wrong.
func TestCaseAlertSettings_KeepsTheValueRefusal(t *testing.T) {
	fake := &fakeCaseAlertClient{err: &apierror.Error{StatusCode: http.StatusBadRequest,
		Body: `{"code":1,"message":"teams.teamLeadsToCall must be between 0 and 20"}`}}
	w := httptest.NewRecorder()
	NewCaseAlertSettingsHandler(fake).PutCaseAlertValues(w,
		caseAlertRequest(http.MethodPut, "/x", `{"ladder":"CRE","values":{"teams.teamLeadsToCall":99}}`))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "teams.teamLeadsToCall") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestCaseAlertSettings_NeedsAUser(t *testing.T) {
	w := httptest.NewRecorder()
	NewCaseAlertSettingsHandler(&fakeCaseAlertClient{}).GetCaseAlertSettings(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

// A refusal keeps entity-service's words, so the tab can say why; anything
// else stays generic and never shows a server's internals.
func TestCaseAlertSettings_KeepsTheRefusalReason(t *testing.T) {
	for _, c := range []struct {
		status int
		msg    string
		want   string
	}{
		{http.StatusForbidden, "only a CSM admin, a portal admin or the SRE rota admin can change the SRE alert settings", "SRE rota admin"},
		{http.StatusTooManyRequests, "a test card was sent less than two minutes ago", "two minutes"},
		{http.StatusConflict, "this room has no webhook URL saved here to test", "no webhook URL"},
		{http.StatusServiceUnavailable, "test cards are not available on this deployment", "not available"},
		{http.StatusInternalServerError, "pq: relation does not exist", "Failed to send the test card."},
	} {
		fake := &fakeCaseAlertClient{err: &apierror.Error{StatusCode: c.status, Body: `{"code":1,"message":"` + c.msg + `"}`}}
		w := httptest.NewRecorder()
		NewCaseAlertSettingsHandler(fake).TestCaseAlertRoom(w, caseAlertRequest(http.MethodPost, "/x", "", "roomId", caseAlertRoomID))
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.want) {
			t.Fatalf("%d: got %d %s", c.status, w.Code, w.Body.String())
		}
	}
}
