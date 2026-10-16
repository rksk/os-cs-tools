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

package domain

import (
	"encoding/json"
	"time"
)

// CaseAlertSettings is the settings section of the Team Schedule's Case Alerts
// tab: the chat rooms, the ServiceNow group routing and the other ladder
// settings the paging ladders use (migration 0233), and what the caller may
// change.
//
// csm-notification-service uses these only when it runs with
// PAGING_CONFIG_SOURCE set; until then its file mount and env vars decide, and
// the tab says so (Revision.AppliedMode).
type CaseAlertSettings struct {
	CanManage CaseAlertSettingsRights `json:"canManage"`
	// Environment is CASE_ALERTS_ENVIRONMENT ("" when unset).
	Environment string `json:"environment"`
	// WebhooksEnabled is false when this deployment has no
	// CASE_ALERTS_SECRET_KEY: rooms can be listed and changed, but no URL can
	// be saved.
	WebhooksEnabled bool                   `json:"webhooksEnabled"`
	Revision        CaseAlertRevision      `json:"revision"`
	Rooms           []CaseAlertChatRoom    `json:"rooms"`
	Routes          []CaseAlertGroupRoute  `json:"routes"`
	Values          []CaseAlertLadderValue `json:"values"`
	// Running is what csm-notification-service reported it runs with, at its
	// last start in shadow or db mode; nil when it never reported one.
	Running *CaseAlertRunning `json:"running,omitempty"`
	// RouteOptions are what a group route can be made of, so the tab offers
	// lists instead of free text; nil for somebody who may see nothing.
	RouteOptions *CaseAlertRouteOptions `json:"routeOptions,omitempty"`
}

// CaseAlertRouteOptions are the choices for a group route: the CSM Portal
// teams a case or incident can be assigned to (CRE, SRE and SME teams; a
// route matches the assignment group by its name), the Team Schedule teams a
// route can send it to, and the active SRE rotas.
type CaseAlertRouteOptions struct {
	Groups []CaseAlertTeamOption `json:"groups"`
	Teams  []CaseAlertTeamOption `json:"teams"`
	Rotas  []CaseAlertRotaOption `json:"rotas"`
}

// CaseAlertTeamOption is a team by key, name and family (CRE, SRE, SME).
type CaseAlertTeamOption struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Family string `json:"family"`
}

// CaseAlertRotaOption is an active SRE rota.
type CaseAlertRotaOption struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// CaseAlertSettingsRights is what the caller may see and change. View is false
// for anybody who may see none of it: the page asks this to decide whether to
// show the section at all.
type CaseAlertSettingsRights struct {
	View bool `json:"view"`
	// RoomLadders and RouteLadders are the ladders (CRE, SRE) whose rooms and
	// routes the caller may change; RoomLadders is also whose ladder values.
	RoomLadders  []string `json:"roomLadders"`
	RouteLadders []string `json:"routeLadders"`
}

// CaseAlertRevision tells what is saved from what is running: every change to a
// room, its URL or a route moves Saved on; Applied is what
// csm-notification-service last reported it loaded at start-up.
type CaseAlertRevision struct {
	Saved       int64      `json:"saved"`
	ChangedAt   *time.Time `json:"changedAt,omitempty"`
	Applied     *int64     `json:"applied,omitempty"`
	AppliedAt   *time.Time `json:"appliedAt,omitempty"`
	AppliedMode string     `json:"appliedMode,omitempty"`
}

// CaseAlertChatRoom is one chat room. TeamKey is empty for the ladder's shared
// room. The webhook URL itself is never sent to a browser: WebhookHint is the
// space id and the last characters of the URL's token.
type CaseAlertChatRoom struct {
	ID       string `json:"id"`
	Ladder   string `json:"ladder"`
	TeamKey  string `json:"teamKey,omitempty"`
	Label    string `json:"label"`
	Enabled  bool   `json:"enabled"`
	Teamless bool   `json:"teamless"`
	// Channel is how a team room's alerts go out (chat, call, both, log);
	// empty is the ladder's channel. Never more than the ladder's.
	Channel        string     `json:"channel,omitempty"`
	HasWebhook     bool       `json:"hasWebhook"`
	WebhookHint    string     `json:"webhookHint,omitempty"`
	WebhookSetAt   *time.Time `json:"webhookSetAt,omitempty"`
	WebhookSetBy   string     `json:"webhookSetBy,omitempty"`
	LastTestAt     *time.Time `json:"lastTestAt,omitempty"`
	LastTestStatus string     `json:"lastTestStatus,omitempty"`
	UpdatedBy      string     `json:"updatedBy"`
	UpdatedOn      time.Time  `json:"updatedOn"`
}

// CaseAlertGroupRoute sends the alerts of a CSM Portal team (a case's or
// incident's assignment group, by name) to a Team Schedule team, or (SRE) to a
// default rota. Exactly one of TeamKey and RotaCode is set.
type CaseAlertGroupRoute struct {
	ID        string    `json:"id"`
	Ladder    string    `json:"ladder"`
	GroupName string    `json:"groupName"`
	TeamKey   string    `json:"teamKey,omitempty"`
	RotaCode  string    `json:"rotaCode,omitempty"`
	Enabled   bool      `json:"enabled"`
	UpdatedBy string    `json:"updatedBy"`
	UpdatedOn time.Time `json:"updatedOn"`
}

// SaveCaseAlertRoomRequest creates or changes the room of one scope: a
// ladder's shared room (no teamKey) or an SRE team's room.
type SaveCaseAlertRoomRequest struct {
	Ladder   string `json:"ladder"`
	TeamKey  string `json:"teamKey,omitempty"`
	Label    string `json:"label"`
	Enabled  *bool  `json:"enabled,omitempty"`
	Teamless *bool  `json:"teamless,omitempty"`
	// Channel is chat, call, both or log for a team room; "" or absent is
	// the ladder's.
	Channel *string `json:"channel,omitempty"`
}

// SetCaseAlertWebhookRequest sets a room's webhook URL. Write-only: it is
// never read back.
type SetCaseAlertWebhookRequest struct {
	URL string `json:"url"`
}

// SaveCaseAlertRouteRequest creates or changes the route of one group.
type SaveCaseAlertRouteRequest struct {
	Ladder    string `json:"ladder"`
	GroupName string `json:"groupName"`
	TeamKey   string `json:"teamKey,omitempty"`
	RotaCode  string `json:"rotaCode,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

// CaseAlertTestCardResponse is a test card's outcome: delivered or failed.
type CaseAlertTestCardResponse struct {
	LastTestStatus string    `json:"lastTestStatus"`
	LastTestAt     time.Time `json:"lastTestAt"`
}

// CaseAlertLadderValue is one other ladder setting saved here: a key of the
// file's cre: or sre: section (teams.abts, timing.interval, heads.cre, ...)
// and its value, in the file's shape, as JSON. It replaces that one key of the
// file; a key not saved keeps the file's value.
type CaseAlertLadderValue struct {
	ID        string          `json:"id"`
	Ladder    string          `json:"ladder"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedBy string          `json:"updatedBy"`
	UpdatedOn time.Time       `json:"updatedOn"`
}

// SaveCaseAlertValuesRequest saves one or more values of one ladder at once:
// every one is checked first, then all are saved together.
type SaveCaseAlertValuesRequest struct {
	Ladder string                     `json:"ladder"`
	Values map[string]json.RawMessage `json:"values"`
}

// CaseAlertValuesResponse is a ladder's saved values after a change.
type CaseAlertValuesResponse struct {
	Values []CaseAlertLadderValue `json:"values"`
}

// CaseAlertRunning is what csm-notification-service last reported it runs
// with: each ladder setting's value and where it came from.
type CaseAlertRunning struct {
	ReportedAt *time.Time                                  `json:"reportedAt,omitempty"`
	Mode       string                                      `json:"mode,omitempty"`
	Ladders    map[string]map[string]CaseAlertRunningValue `json:"ladders"`
	// FileOnly is what only the file sets, by ladder: whether it runs, its
	// channel, safety caps (allowed numbers counted, never listed) and where
	// phone numbers come from. Shown read-only.
	FileOnly map[string]map[string]json.RawMessage `json:"fileOnly,omitempty"`
}

// CaseAlertRunningValue is one setting as the alert service runs it. Source
// is "file" or "saved".
type CaseAlertRunningValue struct {
	Value  json.RawMessage `json:"value"`
	Source string          `json:"source"`
}

// CaseAlertRuntimeSettings is what csm-notification-service reads at start-up:
// the enabled rooms whose URL could be opened, the enabled routes and the
// saved ladder values. For a service credential only; never for a person.
type CaseAlertRuntimeSettings struct {
	Revision int64                   `json:"revision"`
	Rooms    []CaseAlertRuntimeRoom  `json:"rooms"`
	Routes   []CaseAlertGroupRoute   `json:"routes"`
	Values   []CaseAlertRuntimeValue `json:"values"`
}

// CaseAlertRuntimeValue is a saved ladder value as the alert service reads it.
type CaseAlertRuntimeValue struct {
	Ladder string          `json:"ladder"`
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value"`
}

// CaseAlertRuntimeRoom is an enabled room with its webhook URL opened.
type CaseAlertRuntimeRoom struct {
	Ladder   string `json:"ladder"`
	TeamKey  string `json:"teamKey,omitempty"`
	Label    string `json:"label"`
	Teamless bool   `json:"teamless"`
	// WebhookURL is empty for a team room saved only for its channel.
	WebhookURL string `json:"webhookUrl"`
	Channel    string `json:"channel,omitempty"`
}

// CaseAlertAppliedReport is what csm-notification-service reports after
// start-up: the revision it loaded, the mode it runs in (off, shadow, db),
// and a summary of what it used (names and sources, never a URL). The
// summary's "running" entry, when present, is shown on the tab as what is in
// use (CaseAlertRunning's ladders).
type CaseAlertAppliedReport struct {
	Revision int64          `json:"revision"`
	Mode     string         `json:"mode"`
	Summary  map[string]any `json:"summary,omitempty"`
}
