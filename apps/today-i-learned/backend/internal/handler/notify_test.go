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
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/apps/today-i-learned/backend/internal/entry"
	"github.com/wso2-open-operations/cs-tools/apps/today-i-learned/backend/internal/middleware"
)

var modViaOtherApp = &middleware.User{Subject: "u-mod", Email: "mod@example.com", DisplayName: "Mod", IsModerator: true, ClientIDs: []string{"another-app"}}

func notifyPath(id string) string { return "/submissions/" + id + "/notify" }

// get reads an entry back through the API.
func (a *testAPI) get(t *testing.T, id string) entry.Entry {
	t.Helper()
	w := a.do(t, jane, http.MethodGet, "/submissions/"+id, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get: status %d: %s", w.Code, w.Body)
	}
	return decodeJSON[entry.Entry](t, w)
}

// createUnsent shares an entry whose announcement Novera doesn't accept.
func (a *testAPI) createUnsent(t *testing.T) entry.Entry {
	t.Helper()
	a.notifier.fail = errors.New("novera: notification rejected with status 503")
	e := a.create(t, jane, validBody)
	a.notifier.fail = nil
	return e
}

func TestCreateMarksTheEntryAnnounced(t *testing.T) {
	api := newTestAPI(t)
	e := api.create(t, jane, validBody)
	if e.NotifiedAt == nil {
		t.Fatal("created entry has no notifiedAt")
	}
	if got := api.get(t, e.ID); got.NotifiedAt == nil || !got.NotifiedAt.Equal(*e.NotifiedAt) {
		t.Fatalf("stored notifiedAt = %v, want %v", got.NotifiedAt, e.NotifiedAt)
	}
}

func TestCreateLeavesTheEntryUnsentWhenNoveraDoesNotAcceptIt(t *testing.T) {
	api := newTestAPI(t)
	e := api.createUnsent(t)
	if e.NotifiedAt != nil {
		t.Fatalf("notifiedAt = %v, want null", e.NotifiedAt)
	}
	if got := api.get(t, e.ID); got.NotifiedAt != nil {
		t.Fatalf("stored notifiedAt = %v, want null: the claim wasn't released", got.NotifiedAt)
	}
}

func TestCreateWithAnnouncementsOffLeavesNotifiedAtNull(t *testing.T) {
	api := newTestAPI(t, func(c *Config) { c.Notifier = nil })
	if e := api.create(t, jane, validBody); e.NotifiedAt != nil {
		t.Fatalf("notifiedAt = %v, want null", e.NotifiedAt)
	}
}

func TestNotifySendsAnUnsentEntry(t *testing.T) {
	api := newTestAPI(t)
	e := api.createUnsent(t)

	w := api.do(t, mod, http.MethodPost, notifyPath(e.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	got := decodeJSON[entry.Entry](t, w)
	if got.ID != e.ID || got.NotifiedAt == nil {
		t.Fatalf("response = %+v, want the entry with notifiedAt", got)
	}
	if stored := api.get(t, e.ID); stored.NotifiedAt == nil {
		t.Fatal("stored notifiedAt is null")
	}
	if len(api.notifier.sent) != 2 {
		t.Fatalf("announcements = %d, want 2 (the failed one, then the resend)", len(api.notifier.sent))
	}
	if last := api.notifier.sent[1]; last.entry.ID != e.ID || last.entryURL != "https://one.example/knowledge/today-i-learned/"+e.ID+"?view=article" {
		t.Fatalf("resent %+v", last)
	}
}

func TestNotifyNeverSendsTwice(t *testing.T) {
	api := newTestAPI(t)
	e := api.create(t, jane, validBody)
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath(e.ID), ""), http.StatusConflict, msgNotified)

	unsent := api.createUnsent(t)
	if w := api.do(t, mod, http.MethodPost, notifyPath(unsent.ID), ""); w.Code != http.StatusOK {
		t.Fatalf("first send: status %d: %s", w.Code, w.Body)
	}
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath(unsent.ID), ""), http.StatusConflict, msgNotified)
	if len(api.notifier.sent) != 3 {
		t.Fatalf("announcements = %d, want 3", len(api.notifier.sent))
	}
}

func TestNotifyLosesARaceToAnotherSenderWithoutSending(t *testing.T) {
	api := newTestAPI(t)
	e := api.createUnsent(t)
	// Another moderator's request claims the entry between this one loading
	// it and claiming it.
	api.store.beforeClaim = func(id string) {
		api.store.beforeClaim = nil
		if _, claimed, _ := api.store.ClaimNotification(t.Context(), id); !claimed {
			t.Fatal("the other request couldn't claim")
		}
	}
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath(e.ID), ""), http.StatusConflict, msgNotified)
	if len(api.notifier.sent) != 1 {
		t.Fatalf("announcements = %d, want only the failed first one", len(api.notifier.sent))
	}
}

func TestNotifyIsOnlyForModeratorsInTheWebapp(t *testing.T) {
	api := newTestAPI(t)
	e := api.createUnsent(t)
	assertError(t, api.do(t, jane, http.MethodPost, notifyPath(e.ID), ""), http.StatusForbidden, msgCannotNotify)
	assertError(t, api.do(t, modViaOtherApp, http.MethodPost, notifyPath(e.ID), ""), http.StatusForbidden, msgWebappOnly)
	assertError(t, api.do(t, nil, http.MethodPost, notifyPath(e.ID), ""), http.StatusUnauthorized, msgNotSignedIn)
	if len(api.notifier.sent) != 1 {
		t.Fatalf("announcements = %d, want only the failed first one", len(api.notifier.sent))
	}
}

func TestNotifyKeepsTheEntryUnsentWhenNoveraDoesNotAcceptItAgain(t *testing.T) {
	api := newTestAPI(t)
	e := api.createUnsent(t)
	api.notifier.fail = errors.New("novera: send notification: timeout")
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath(e.ID), ""), http.StatusBadGateway, msgNotifyFailed)
	if got := api.get(t, e.ID); got.NotifiedAt != nil {
		t.Fatalf("notifiedAt = %v, want null so it can be sent again", got.NotifiedAt)
	}
	api.notifier.fail = nil
	if w := api.do(t, mod, http.MethodPost, notifyPath(e.ID), ""); w.Code != http.StatusOK {
		t.Fatalf("retry: status %d: %s", w.Code, w.Body)
	}
}

func TestNotifyWithAnnouncementsOff(t *testing.T) {
	api := newTestAPI(t, func(c *Config) { c.Notifier = nil })
	e := api.create(t, jane, validBody)
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath(e.ID), ""), http.StatusServiceUnavailable, msgNoNotifier)
}

func TestNotifyAnEntryThatDoesNotExist(t *testing.T) {
	api := newTestAPI(t)
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath("00000000-0000-4000-8000-999999999999"), ""), http.StatusNotFound, msgNotFound)
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath("not-an-id"), ""), http.StatusNotFound, msgNotFound)
}

func TestNotifyReportsAStoreFailure(t *testing.T) {
	api := newTestAPI(t)
	e := api.createUnsent(t)
	api.store.fail = errors.New("database is down")
	assertError(t, api.do(t, mod, http.MethodPost, notifyPath(e.ID), ""), http.StatusInternalServerError, msgInternal)
	if len(api.notifier.sent) != 1 {
		t.Fatalf("announcements = %d, want nothing sent without a claim", len(api.notifier.sent))
	}
}

func TestSettingsSayWhetherNotificationsAreOn(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  func(*Config)
		want bool
	}{
		{"on", func(*Config) {}, true},
		{"off", func(c *Config) { c.Notifier = nil }, false},
	} {
		got := decodeJSON[map[string]any](t, newTestAPI(t, tc.cfg).do(t, jane, http.MethodGet, "/settings", ""))
		if got["notificationsEnabled"] != tc.want {
			t.Errorf("%s: notificationsEnabled = %v, want %v", tc.name, got["notificationsEnabled"], tc.want)
		}
	}
}

// The resend's notifiedAt is the claim's time, not the response's.
func TestNotifyReturnsTheClaimTime(t *testing.T) {
	api := newTestAPI(t)
	e := api.createUnsent(t)
	got := decodeJSON[entry.Entry](t, api.do(t, mod, http.MethodPost, notifyPath(e.ID), ""))
	if want := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC); got.NotifiedAt == nil || !got.NotifiedAt.Equal(want) {
		t.Fatalf("notifiedAt = %v, want %v", got.NotifiedAt, want)
	}
}
