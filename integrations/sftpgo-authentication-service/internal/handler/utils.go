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
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/log"
	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/models"
	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/constants"
	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/util"
)

const (
	// AuthResultFailure indicates a failed authentication attempt.
	AuthResultFailure = -1
	// AuthResultIncomplete indicates that further authentication steps are required.
	AuthResultIncomplete = 0
	// AuthResultSuccess indicates a successful authentication.
	AuthResultSuccess = 1

	// PermissionList is the SFTPGo permission to list directory contents.
	PermissionList = "list"
)

var (
	// generalFileMgtPermissions is the default set of permissions for project folders.
	generalFileMgtPermissions = []string{"upload", "list", "download", "create_dirs", "delete", "overwrite", "rename"}

	// attachmentShareMountPermissions is the standing permission set granted on
	// the shared "/attachments" virtual folder to every externally authenticated
	// caller (see ExternalAuthHook). It is deliberately narrower than
	// generalFileMgtPermissions.
	//
	// Why this can't be scoped to a specific case/project path instead: at the
	// time this hook runs, all that is known about the caller is their identity
	// (email/userid/groups from the validated JWT) — the JWT is the CSM
	// backend's general session assertion, minted once per login and reused for
	// every SFTPGo call for that session (see MintToken in
	// apps/csm-portal/backend/internal/sftpgo/client.go), not a per-case or
	// per-attachment token. The case/attachment path is only known later, when
	// the CSM backend calls SFTPGo's POST /api/v2/user/shares with the specific
	// storage key (buildStorageKey in attachment_storage.go). So the underlying
	// SFTPGo user minted here cannot be scoped to one case's subtree; the actual
	// per-case boundary is enforced entirely by the short-lived, path-scoped
	// Share object created afterward.
	//
	// What *can* be scoped, and is: SFTPGo's own share-serving code path
	// (internal/httpd/handler.go's getFileReader/getFileWriter, confirmed
	// against upstream SFTPGo source) checks the underlying user's standing
	// Permissions map for every share-based download/upload — share creation
	// itself (internal/dataprovider/share.go's Share.validate/validatePaths)
	// performs no permission check at all, so it is exclusively this standing
	// grant that gates whether a minted share can actually be read from or
	// written to. Of generalFileMgtPermissions' seven verbs, only five are ever
	// exercised by the attachment-share flow: "list" and "download" (read
	// shares — SFTPGo stats the shared path before streaming it) and "upload",
	// "create_dirs" (the BFF's tus upload always sets mkdir_parents=true), and
	// "overwrite" (write shares). "delete" and "rename" are never exercised by
	// any attachment code path — there is no delete/rename-via-share feature —
	// so granting them here only handed every authenticated caller the ability
	// to directly delete or rename ANY file anywhere under the shared
	// "/attachments" tree via SFTPGo's own file-management API, entirely
	// outside of any Share object. Dropping them closes that off with zero
	// impact on the real upload/download flow.
	//
	// Residual, accepted risk: because the grant is still rooted at
	// "/attachments" (not a per-case subpath — see above for why that isn't
	// possible from this hook), a caller can still directly list/download
	// arbitrary attachments outside the ones they were actually issued a Share
	// for, by calling SFTPGo's own file API instead of going through a Share.
	// Closing that fully requires either embedding the specific case/project
	// path in the token-mint request (a BFF-side change, out of scope here) or
	// moving to per-case SFTPGo virtual folders, and should be tracked as a
	// follow-up rather than silently left undocumented.
	attachmentShareMountPermissions = []string{"list", "download", "upload", "create_dirs", "overwrite"}

	// usernameRegex validates that a username is non-empty, at most 254 characters (RFC 5321),
	// and contains no carriage return or newline characters.
	usernameRegex = regexp.MustCompile(`^[^\r\n]{1,254}$`)
)

// writeJSONResponse is a helper to standardize writing JSON responses.
func writeJSONResponse(w http.ResponseWriter, status int, data interface{}, logger *log.AppLogger) {
	w.Header().Set(constants.HeaderContentType, constants.MIMEApplicationJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("Error writing JSON response: %v", err)
	}
}

// sanitizeUsername replaces special characters in a username with underscores.
func sanitizeUsername(u string) string {
	return util.SanitizeUsername(u)
}

// validateUsername validates that a username is non-empty, at most 254 characters (RFC 5321),
// and contains no carriage return or newline characters.
func validateUsername(username string) error {
	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("invalid username: must be 1–254 characters with no newline characters")
	}
	return nil
}

// handleAuthStep1 initiates the authentication flow by identifying the user and retrieving the first step from the IdP.
func (h *Handler) handleAuthStep1(resp *models.KeyIntResponse, req *models.KeyIntRequest) {
	if req.Username == "" {
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication failed: Username not provided."
		return
	}

	// Validate username before initiating IdP flow
	if err := validateUsername(req.Username); err != nil {
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication failed: Invalid username."
		h.logger.Error("Invalid username in auth step 1: %v", err)
		return
	}

	initFlowResp, err := h.idp.InitFlow(req.Username)
	if err != nil || initFlowResp.NextStep == nil || len(initFlowResp.NextStep.Authenticators) == 0 {
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication failed: Error initiating flow."
		h.logger.Error("Failed to get initial flow from IdP: %v", err)
		return
	}

	// First step is typically identifier-first
	discoveryPayload := map[string]interface{}{
		"flowId": initFlowResp.FlowID,
		"selectedAuthenticator": map[string]interface{}{
			"authenticatorId": initFlowResp.NextStep.Authenticators[0].AuthenticatorID,
			"params":          map[string]interface{}{"username": req.Username},
		},
	}

	idpResp, err := h.idp.PostToAuthnEndpoint(discoveryPayload, util.IsInternalUser(req.Username, h.cfg.InternalUserSuffix))
	if err != nil || idpResp.FlowStatus == "FAILED" || idpResp.NextStep == nil {
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication failed."
		h.logger.Error("IdP discovery failed: %v", err)
		return
	}

	sessionData := models.SessionData{
		FlowID:   idpResp.FlowID,
		NextStep: idpResp.NextStep,
	}
	if err := h.db.SaveSession(req.RequestID, sessionData); err != nil {
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication failed: Internal error."
		h.logger.Error("Failed to save session (RequestID: %s, FlowID: %s): %v", req.RequestID, idpResp.FlowID, err)
		return
	}

	resp.Instruction, resp.Questions, resp.Echos = generatePromptFromAuthenticators(*idpResp)
	resp.AuthResult = AuthResultIncomplete // Incomplete
}

// handleAuthSubsequentSteps processes subsequent steps of the keyboard-interactive authentication flow.
func (h *Handler) handleAuthSubsequentSteps(resp *models.KeyIntResponse, req *models.KeyIntRequest, session models.SessionData) {
	if session.NextStep == nil || len(session.NextStep.Authenticators) == 0 {
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication session expired or invalid."
		return
	}

	selectedAuth := session.NextStep.Authenticators[0]
	params := make(map[string]interface{})

	if len(session.NextStep.Authenticators) > 1 {
		if len(req.Answers) == 0 {
			resp.AuthResult = AuthResultFailure
			resp.Instruction = "Authentication failed: No selection provided."
			return
		}
		selection, err := strconv.Atoi(req.Answers[0])
		if err != nil || selection < 1 || selection > len(session.NextStep.Authenticators) {
			resp.AuthResult = AuthResultFailure
			resp.Instruction = "Authentication failed: Invalid selection."
			return
		}
		selectedAuth = session.NextStep.Authenticators[selection-1]
	} else {
		// Populate params from user answers
		counter := 0
		for _, param := range selectedAuth.Metadata.Params {
			if param.ParamName != "username" && len(req.Answers) > counter {
				params[param.ParamName] = req.Answers[counter]
				counter++
			}
		}
	}

	// Support BasicAuthenticator where username is mandatory
	if selectedAuth.AuthenticatorID == h.cfg.BasicAuthenticatorID {
		if _, hasPassword := params["password"]; hasPassword && len(params) == 1 {
			params["username"] = req.Username
		}
	}

	payload := map[string]interface{}{
		"flowId": session.FlowID,
		"selectedAuthenticator": map[string]interface{}{
			"authenticatorId": selectedAuth.AuthenticatorID,
			"params":          params,
		},
	}

	idpResp, err := h.idp.PostToAuthnEndpoint(payload, util.IsInternalUser(req.Username, h.cfg.InternalUserSuffix))
	if err != nil {
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication failed."
		h.logger.Error("IdP authn step failed for user %s: %v", req.Username, err)
		return
	}

	switch idpResp.FlowStatus {
	case "SUCCESS_COMPLETED":
		h.handleAuthSuccess(resp, req)
	case "INCOMPLETE", "FAIL_INCOMPLETE":
		if err := h.db.SaveSession(req.RequestID, models.SessionData{FlowID: session.FlowID, NextStep: idpResp.NextStep}); err != nil {
			resp.AuthResult = AuthResultFailure
			resp.Instruction = "Authentication failed: Internal error."
			h.logger.Error("Failed to update session (RequestID: %s, FlowID: %s): %v", req.RequestID, session.FlowID, err)
			return
		}
		resp.Instruction, resp.Questions, resp.Echos = generatePromptFromAuthenticators(*idpResp)
		resp.AuthResult = AuthResultIncomplete
	default:
		resp.AuthResult = AuthResultFailure
		resp.Instruction = "Authentication failed."
		h.db.DeleteSession(req.RequestID)
	}
}

// handleAuthSuccess finalizes a successful authentication flow.
func (h *Handler) handleAuthSuccess(resp *models.KeyIntResponse, req *models.KeyIntRequest) {
	resp.AuthResult = AuthResultSuccess
	h.db.DeleteSession(req.RequestID)
}

// generatePromptFromAuthenticators parses IdP response to generate instruction and questions for SFTPGo.
func generatePromptFromAuthenticators(idpResp models.IdPResponse) (string, []string, []bool) {
	if idpResp.NextStep == nil || len(idpResp.NextStep.Authenticators) == 0 {
		return "No auth methods available.", nil, nil
	}
	if idpResp.NextStep.StepType == "MULTI_OPTIONS_PROMPT" && len(idpResp.NextStep.Authenticators) > 1 {
		// If multiple authenticators, prompt for selection
		instruction := "Select an authentication method:"
		selectionPrompt := ""
		for i, auth := range idpResp.NextStep.Authenticators {
			selectionPrompt += fmt.Sprintf("%d for %s ", i+1, auth.DisplayName)
		}
		selectionPrompt += "Enter selection: "
		questions := []string{selectionPrompt}
		echos := []bool{true}
		return instruction, questions, echos
	}

	auth := idpResp.NextStep.Authenticators[0]
	var questions []string
	var echos []bool
	for _, param := range auth.Metadata.Params {
		if param.ParamName != "username" {
			questions = append(questions, fmt.Sprintf("%s: ", param.DisplayName))
			echos = append(echos, !param.IsConfidential)
		}
	}
	instruction := "Please provide the following information:"
	if len(idpResp.NextStep.Messages) > 0 {
		instruction = idpResp.NextStep.Messages[0].Message
	}
	return instruction, questions, echos
}
