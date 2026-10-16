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
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
)

// entityCaseAlertSettingsClient is the entity-service calls the settings
// section of the Alert Settings tab needs. Its own interface, so nothing that
// fakes the schedule client changes.
type entityCaseAlertSettingsClient interface {
	GetCaseAlertSettings(ctx context.Context) ([]byte, error)
	PutCaseAlertRoom(ctx context.Context, body []byte) ([]byte, error)
	PutCaseAlertWebhook(ctx context.Context, roomID string, body []byte) ([]byte, error)
	DeleteCaseAlertRoom(ctx context.Context, roomID string) ([]byte, error)
	TestCaseAlertRoom(ctx context.Context, roomID string) ([]byte, error)
	PutCaseAlertRoute(ctx context.Context, body []byte) ([]byte, error)
	DeleteCaseAlertRoute(ctx context.Context, routeID string) ([]byte, error)
	PutCaseAlertValues(ctx context.Context, body []byte) ([]byte, error)
	DeleteCaseAlertValue(ctx context.Context, valueID string) ([]byte, error)
}

// CaseAlertSettingsHandler proxies the settings section of the Alert Settings tab:
// the chat rooms, ServiceNow group routes and ladder values the paging ladders
// use. Who may see or change them is entity-service's decision -- a CSM admin
// or portal admin, or a ladder's rota admin -- so this checks only the shape of
// each request, rebuilds its body from the known fields, and forwards it.
//
// A room's webhook URL passes through on its way to entity-service, which
// seals it; it is never logged here and never comes back.
type CaseAlertSettingsHandler struct {
	entity entityCaseAlertSettingsClient
}

// NewCaseAlertSettingsHandler creates a CaseAlertSettingsHandler.
func NewCaseAlertSettingsHandler(entity entityCaseAlertSettingsClient) *CaseAlertSettingsHandler {
	return &CaseAlertSettingsHandler{entity: entity}
}

// caseAlertRoomChannels are the channels a team's room can have; "" clears
// it back to the ladder's own.
var caseAlertRoomChannels = map[string]bool{"": true, "log": true, "chat": true, "call": true, "both": true}

var (
	caseAlertLadderRe = regexp.MustCompile(`(?i)^(cre|sre)$`)
	// caseAlertRoomLadderRe is the ladders with chat rooms and ladder values:
	// SME's too (0237, 0238), but no routes.
	caseAlertRoomLadderRe = regexp.MustCompile(`(?i)^(cre|sre|sme)$`)
	caseAlertValueKeyRe   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.]*$`)
)

const (
	maxCaseAlertWebhookLen = 2048
	maxCaseAlertTextLen    = 255
	// A room's label is entity-service's VARCHAR(100), checked there too.
	maxCaseAlertLabelLen = 100
	// A ladder value is one key of the paging file: entity-service knows the
	// keys and checks each value. These only keep a request to a sane size.
	maxCaseAlertValueKeys     = 50
	maxCaseAlertValueKeyLen   = 64
	maxCaseAlertValueRawBytes = 8 << 10
)

// GetCaseAlertSettings handles GET /team-schedule/case-alerts/settings.
// Somebody who may see nothing gets an empty answer, which is how the page
// hides the section.
func (h *CaseAlertSettingsHandler) GetCaseAlertSettings(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}
	result, err := h.entity.GetCaseAlertSettings(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetCaseAlertSettings failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to load the Case Alerts settings.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// PutCaseAlertRoom handles PUT /team-schedule/case-alerts/settings/rooms.
func (h *CaseAlertSettingsHandler) PutCaseAlertRoom(w http.ResponseWriter, r *http.Request) {
	body, callerID, ok := readScheduleBody(w, r)
	if !ok {
		return
	}
	var req struct {
		Ladder   string  `json:"ladder"`
		TeamKey  string  `json:"teamKey"`
		Label    string  `json:"label"`
		Enabled  *bool   `json:"enabled"`
		Teamless *bool   `json:"teamless"`
		Channel  *string `json:"channel"`
	}
	if err := json.Unmarshal(body, &req); err != nil || !caseAlertRoomLadderRe.MatchString(req.Ladder) ||
		strings.TrimSpace(req.Label) == "" || len(req.Label) > maxCaseAlertLabelLen ||
		(req.TeamKey != "" && !rotaTeamKeyRe.MatchString(req.TeamKey)) {
		writeError(w, http.StatusBadRequest, "Send a ladder (CRE, SRE or SME), a label, and optionally an SRE or SME team.")
		return
	}
	// A team's own channel: "" goes back to the ladder's. The shared room
	// always follows the ladder, so it never has one.
	if req.Channel != nil && (!caseAlertRoomChannels[*req.Channel] || (*req.Channel != "" && req.TeamKey == "")) {
		writeError(w, http.StatusBadRequest, "A team's channel is log, chat, call, both, or empty for the ladder's; the shared room has none.")
		return
	}
	fwd := map[string]any{"ladder": req.Ladder, "label": req.Label}
	if req.TeamKey != "" {
		fwd["teamKey"] = req.TeamKey
	}
	if req.Enabled != nil {
		fwd["enabled"] = *req.Enabled
	}
	if req.Teamless != nil {
		fwd["teamless"] = *req.Teamless
	}
	if req.Channel != nil {
		fwd["channel"] = *req.Channel
	}
	h.forward(w, r, callerID, "PutCaseAlertRoom", "Failed to save the chat room.", http.StatusOK,
		func(ctx context.Context) ([]byte, error) { return h.entity.PutCaseAlertRoom(ctx, mustJSON(fwd)) })
}

// PutCaseAlertWebhook handles PUT /team-schedule/case-alerts/settings/rooms/{roomId}/webhook.
func (h *CaseAlertSettingsHandler) PutCaseAlertWebhook(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("roomId")
	if !uuidRe.MatchString(roomID) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	body, callerID, ok := readScheduleBody(w, r)
	if !ok {
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.URL) == "" || len(req.URL) > maxCaseAlertWebhookLen {
		writeError(w, http.StatusBadRequest, "Paste the room's Google Chat webhook URL.")
		return
	}
	h.forward(w, r, callerID, "PutCaseAlertWebhook", "Failed to save the webhook URL.", http.StatusOK,
		func(ctx context.Context) ([]byte, error) {
			return h.entity.PutCaseAlertWebhook(ctx, roomID, mustJSON(map[string]string{"url": req.URL}))
		})
}

// DeleteCaseAlertRoom handles DELETE /team-schedule/case-alerts/settings/rooms/{roomId}.
func (h *CaseAlertSettingsHandler) DeleteCaseAlertRoom(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, "roomId", "DeleteCaseAlertRoom", "Failed to remove the chat room.", http.StatusNoContent, h.entity.DeleteCaseAlertRoom)
}

// TestCaseAlertRoom handles POST /team-schedule/case-alerts/settings/rooms/{roomId}/test.
func (h *CaseAlertSettingsHandler) TestCaseAlertRoom(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, "roomId", "TestCaseAlertRoom", "Failed to send the test card.", http.StatusOK, h.entity.TestCaseAlertRoom)
}

// PutCaseAlertRoute handles PUT /team-schedule/case-alerts/settings/routes.
func (h *CaseAlertSettingsHandler) PutCaseAlertRoute(w http.ResponseWriter, r *http.Request) {
	body, callerID, ok := readScheduleBody(w, r)
	if !ok {
		return
	}
	var req struct {
		Ladder    string `json:"ladder"`
		GroupName string `json:"groupName"`
		TeamKey   string `json:"teamKey"`
		RotaCode  string `json:"rotaCode"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(body, &req); err != nil || !caseAlertLadderRe.MatchString(req.Ladder) ||
		strings.TrimSpace(req.GroupName) == "" || len(req.GroupName) > maxCaseAlertTextLen ||
		(req.TeamKey == "") == (req.RotaCode == "") ||
		(req.TeamKey != "" && !rotaTeamKeyRe.MatchString(req.TeamKey)) ||
		(req.RotaCode != "" && !rotaTeamKeyRe.MatchString(req.RotaCode)) {
		writeError(w, http.StatusBadRequest, "Send a ladder, a group name, and either a team or a rota.")
		return
	}
	fwd := map[string]any{"ladder": req.Ladder, "groupName": req.GroupName}
	if req.TeamKey != "" {
		fwd["teamKey"] = req.TeamKey
	} else {
		fwd["rotaCode"] = req.RotaCode
	}
	if req.Enabled != nil {
		fwd["enabled"] = *req.Enabled
	}
	h.forward(w, r, callerID, "PutCaseAlertRoute", "Failed to save the route.", http.StatusOK,
		func(ctx context.Context) ([]byte, error) { return h.entity.PutCaseAlertRoute(ctx, mustJSON(fwd)) })
}

// DeleteCaseAlertRoute handles DELETE /team-schedule/case-alerts/settings/routes/{routeId}.
func (h *CaseAlertSettingsHandler) DeleteCaseAlertRoute(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, "routeId", "DeleteCaseAlertRoute", "Failed to remove the route.", http.StatusNoContent, h.entity.DeleteCaseAlertRoute)
}

// PutCaseAlertValues handles PUT /team-schedule/case-alerts/settings/values:
// one or more ladder values saved together. Each value is any JSON -- a list,
// a number, a head's name and email -- and is passed on as sent; entity-service
// knows what each key takes and answers with the key it refused.
func (h *CaseAlertSettingsHandler) PutCaseAlertValues(w http.ResponseWriter, r *http.Request) {
	body, callerID, ok := readScheduleBody(w, r)
	if !ok {
		return
	}
	var req struct {
		Ladder string                     `json:"ladder"`
		Values map[string]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(body, &req); err != nil || !caseAlertRoomLadderRe.MatchString(req.Ladder) ||
		len(req.Values) == 0 || len(req.Values) > maxCaseAlertValueKeys {
		writeError(w, http.StatusBadRequest, "Send a ladder (CRE, SRE or SME) and 1 to 50 settings to save.")
		return
	}
	for key, value := range req.Values {
		if len(key) > maxCaseAlertValueKeyLen || !caseAlertValueKeyRe.MatchString(key) || len(value) > maxCaseAlertValueRawBytes {
			writeError(w, http.StatusBadRequest, "Each setting needs a known key and a value under 8 KB.")
			return
		}
	}
	fwd := map[string]any{"ladder": req.Ladder, "values": req.Values}
	h.forward(w, r, callerID, "PutCaseAlertValues", "Failed to save the ladder settings.", http.StatusOK,
		func(ctx context.Context) ([]byte, error) { return h.entity.PutCaseAlertValues(ctx, mustJSON(fwd)) })
}

// DeleteCaseAlertValue handles DELETE /team-schedule/case-alerts/settings/values/{valueId}.
func (h *CaseAlertSettingsHandler) DeleteCaseAlertValue(w http.ResponseWriter, r *http.Request) {
	h.byID(w, r, "valueId", "DeleteCaseAlertValue", "Failed to remove the saved setting.", http.StatusNoContent, h.entity.DeleteCaseAlertValue)
}

// byID forwards a call addressed by one id in the path.
func (h *CaseAlertSettingsHandler) byID(w http.ResponseWriter, r *http.Request, param, op, fallbackMsg string, okStatus int,
	call func(ctx context.Context, id string) ([]byte, error)) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}
	id := r.PathValue(param)
	if !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	h.forward(w, r, user.UserID, op, fallbackMsg, okStatus,
		func(ctx context.Context) ([]byte, error) { return call(ctx, id) })
}

// forward makes the call and answers with its result. A refusal keeps
// entity-service's own reason -- "only a CSM admin, a portal admin or the SRE
// rota admin can ...", "a test card was sent less than two minutes ago" -- so
// the tab can say why. Anything else stays generic.
func (h *CaseAlertSettingsHandler) forward(w http.ResponseWriter, r *http.Request, callerID, op, fallbackMsg string, okStatus int,
	call func(ctx context.Context) ([]byte, error)) {
	result, err := call(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "entity "+op+" failed", "userID", callerID, "err", err)
		var apiErr *apierror.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict,
				http.StatusTooManyRequests, http.StatusServiceUnavailable:
				writeError(w, apiErr.StatusCode, upstreamErrorMessageStrict(apiErr.Body, fallbackMsg))
				return
			}
		}
		mapUpstreamErrorGeneric(w, err, fallbackMsg)
		return
	}
	if okStatus == http.StatusNoContent {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, okStatus, result)
}

// mustJSON encodes a request body built here from known fields, which cannot
// fail to encode.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
