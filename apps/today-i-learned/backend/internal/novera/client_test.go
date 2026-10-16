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

package novera

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/today-i-learned/backend/internal/entry"
)

func ptr(s string) *string { return &s }

// newNovera fakes the gateway's token endpoint and Novera's notification
// endpoint, which answers with status and hands back what it received.
func newNovera(t *testing.T, status int) (*httptest.Server, <-chan map[string]any) {
	t.Helper()
	received := make(chan map[string]any, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"gateway-token","token_type":"Bearer","expires_in":3600}`))
	})
	mux.HandleFunc("POST /notify", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gateway-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		received <- body
		w.WriteHeader(status)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, received
}

func newClient(srv *httptest.Server, iconURL string) *Client {
	return New(Config{NotifyURL: srv.URL + "/notify", TokenURL: srv.URL + "/token", ClientID: "til", ClientSecret: "s3cret", IconURL: iconURL})
}

func TestNotifySendsTheEntry(t *testing.T) {
	srv, received := newNovera(t, http.StatusAccepted)
	e := entry.Entry{
		ID:          "e1",
		Title:       `Use <b> & "quotes"`,
		Who:         "Jane <Doe>",
		Where:       "Customer",
		WhereDetail: ptr("Acme & <Co>"),
		What:        "<p>Learned <strong>a lot</strong>.</p><ul><li>One</li><li>Two</li></ul>",
	}
	if err := newClient(srv, "https://one.example/til-logo.png").Notify(context.Background(), e, "https://one.example/knowledge/today-i-learned/e1?view=article"); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	// Plain text throughout; Novera escapes it.
	got := <-received
	want := map[string]any{
		"source":   "til",
		"title":    `Use <b> & "quotes"`,
		"subtitle": "Customer — Acme & <Co>",
		"body":     "Learned a lot. • One • Two",
		"link":     map[string]any{"url": "https://one.example/knowledge/today-i-learned/e1?view=article", "label": "Read more"},
		"footer":   "Authored by Jane <Doe>",
		"iconUrl":  "https://one.example/til-logo.png",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sent %#v\nwant %#v", got, want)
	}
}

func TestNotifyLeavesOutAbsentValues(t *testing.T) {
	srv, received := newNovera(t, http.StatusOK)
	if err := newClient(srv, "").Notify(context.Background(), entry.Entry{ID: "e1", Title: "T", Who: "J", Where: "Internal", What: "<p>x</p>"}, ""); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	got := <-received
	if got["subtitle"] != "Internal" {
		t.Errorf("subtitle = %#v, want Internal", got["subtitle"])
	}
	for _, k := range []string{"link", "iconUrl"} {
		if v, present := got[k]; present {
			t.Errorf("%s = %#v, want it left out", k, v)
		}
	}
}

func TestIconURL(t *testing.T) {
	for base, want := range map[string]string{
		"https://one.example":   "https://one.example/til-logo.png",
		"https://one.example/":  "https://one.example/til-logo.png",
		"http://localhost:3000": "",
		"":                      "",
	} {
		if got := IconURL(base); got != want {
			t.Errorf("IconURL(%q) = %q, want %q", base, got, want)
		}
	}
}

// Anything but Novera accepting the announcement is an error, so the entry
// stays unannounced for a moderator to send again.
func TestNotifyReportsFailures(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusInternalServerError, http.StatusBadRequest} {
		srv, received := newNovera(t, status)
		if err := newClient(srv, "").Notify(context.Background(), entry.Entry{ID: "e1"}, ""); err == nil {
			t.Errorf("status %d: Notify returned nil", status)
		}
		<-received
	}

	// Nothing listening at all.
	srv, _ := newNovera(t, http.StatusAccepted)
	err := New(Config{NotifyURL: "http://127.0.0.1:1/notify", TokenURL: srv.URL + "/token", ClientID: "til", ClientSecret: "s"}).
		Notify(context.Background(), entry.Entry{ID: "e2"}, "")
	if err == nil {
		t.Error("unreachable Novera: Notify returned nil")
	}
}
