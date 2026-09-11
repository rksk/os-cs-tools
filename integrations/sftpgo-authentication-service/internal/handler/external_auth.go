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
	"path/filepath"
	"strings"

	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/models"
)

// externalAuthProtocolHTTP is the protocol value SFTPGo sends on the
// external_auth_hook request for the REST "GET /api/v2/user/token" path (the
// web attachment access path, where the CSM backend forwards its x-jwt-assertion
// as the HTTP Basic-auth password).
const externalAuthProtocolHTTP = "HTTP"

// ExternalAuthHook handles SFTPGo's external_auth_hook for the web attachment
// access path. The CSM backend calls SFTPGo's GET /api/v2/user/token with
// HTTP Basic auth, passing the same x-jwt-assertion it already validated on the
// incoming request as the password. SFTPGo then POSTs this hook's request body
// for every such attempt; this handler independently verifies that JWT's
// signature and claims (it must not trust the caller — SFTPGo is the one
// invoking this endpoint, not the CSM backend directly) and, on success,
// returns an SFTPGo user object so SFTPGo can mint a REST API token for it.
//
// This is a separate, additive hook alongside PreLoginHook/AuthHandler (PR #71's
// SFTP-channel flow via pre-login + keyboard-interactive) — it does not change
// their behavior.
func (h *Handler) ExternalAuthHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.authenticateExternalAuthHook(r, w) {
		return
	}
	defer r.Body.Close()

	var req models.ExternalAuthHookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Invalid payload in external-auth hook: %v", err)
		h.auditLog(r, "unknown", "external-auth-hook", "failure", "invalid payload")
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if req.Protocol != externalAuthProtocolHTTP {
		h.logger.Warn("External-auth hook: rejecting protocol %q for user %q (only %q is supported)", req.Protocol, req.Username, externalAuthProtocolHTTP)
		h.auditLog(r, req.Username, "external-auth-hook", "denied", fmt.Sprintf("unsupported protocol: %s", req.Protocol))
		h.denyExternalAuth(w)
		return
	}

	if req.Password == "" {
		h.logger.Debug("External-auth hook: no credential presented for user %q", req.Username)
		h.auditLog(r, req.Username, "external-auth-hook", "denied", "no credential presented")
		h.denyExternalAuth(w)
		return
	}

	if h.jwtAuth == nil {
		h.logger.Error("External-auth hook invoked but AUTH_JWKS_ENDPOINT/AUTH_ISSUER are not configured on this deployment")
		h.auditLog(r, req.Username, "external-auth-hook", "failure", "jwt validator not configured")
		http.Error(w, "Not configured", http.StatusServiceUnavailable)
		return
	}

	userInfo, err := h.jwtAuth.ValidateAndExtract(req.Password)
	if err != nil {
		h.logger.Warn("External-auth hook: JWT validation failed for user %q: %v", req.Username, err)
		h.auditLog(r, req.Username, "external-auth-hook", "denied", fmt.Sprintf("jwt validation failed: %v", err))
		h.denyExternalAuth(w)
		return
	}

	// The username on the returned SFTPGo user is the JWT's email claim: this PR
	// keeps email as the identity key. Switching to userid is a separate,
	// not-yet-decided item.
	home := filepath.Join(h.cfg.DIRPath, sanitizeUsername(userInfo.Email))
	perms := map[string][]string{"/": {PermissionList}}
	var vfs []models.UserVirtualFolder

	// Without this, every identity minted here got only list-only
	// permission on an isolated per-user home_dir, with no mapping onto
	// the shared attachments tree at all. That makes
	// apps/csm-portal/backend's CreateShare (write-scoped upload shares,
	// read-scoped download shares) fail for every caller, since the
	// storage keys it mints ("/attachments/project-<id>/cases/<id>/<id>",
	// see internal/handler/attachment_storage.go buildStorageKey) never
	// resolved to anything inside this per-user home. Mounting the shared
	// tree as a "/attachments" virtual folder makes that path convention
	// resolve correctly for any authenticated caller. ATTACHMENTS_DIR_PATH
	// is optional and unset by default, so a deployment that hasn't wired
	// it yet keeps the prior (broken-for-this-feature, but unchanged)
	// behavior rather than a new failure mode.
	//
	// The permission set granted here is attachmentShareMountPermissions,
	// not generalFileMgtPermissions — see that var's doc comment for why it
	// is scoped down to only the verbs the attachment-share flow actually
	// exercises (no delete/rename), and for the residual risk that still
	// isn't closeable from this hook alone.
	if h.cfg.AttachmentsPath != "" {
		perms["/attachments"] = attachmentShareMountPermissions
		vfs = append(vfs, models.UserVirtualFolder{
			Name:        "attachments",
			VirtualPath: "/attachments",
			MappedPath:  h.cfg.AttachmentsPath,
		})
	}

	res := models.MinimalSFTPGoUser{
		Username:       userInfo.Email,
		HomeDir:        home,
		Permissions:    perms,
		Status:         1,
		VirtualFolders: vfs,
	}

	h.auditLog(r, userInfo.Email, "external-auth-hook", "success",
		fmt.Sprintf("userid=%s groups=%s", userInfo.UserID, strings.Join(userInfo.Groups, ",")))

	if h.logger.IsDebugEnabled() {
		resBody, _ := json.Marshal(res)
		h.logger.Debug("External-auth hook response for user %q: %s", userInfo.Email, string(resBody))
	}

	writeJSONResponse(w, http.StatusOK, res, h.logger)
}

// authenticateExternalAuthHook checks the request's API key for
// /external-auth-hook specifically. Unlike authenticate() (used by
// PreLoginHook/AuthHandler, which deliberately fail open when HOOK_API_KEY is
// unconfigured — a PR #71 behavior kept unchanged for those two hooks), this
// route fails closed: a successful call here mints a full SFTPGo identity
// from an arbitrary presented JWT, not just SFTP-channel login state, so it
// must never be reachable unauthenticated just because an operator forgot to
// set HOOK_API_KEY.
func (h *Handler) authenticateExternalAuthHook(r *http.Request, w http.ResponseWriter) bool {
	if h.cfg.HookAPIKey == "" {
		h.logger.Error("External-auth hook invoked but HOOK_API_KEY is not configured; refusing to serve this route unauthenticated")
		h.auditLog(r, "unknown", "external-auth-hook", "failure", "HOOK_API_KEY not configured")
		http.Error(w, "Service unavailable: authentication not configured", http.StatusServiceUnavailable)
		return false
	}

	if !apiKeyMatches(r, h.cfg.HookAPIKey) {
		h.logger.Warn("Unauthorized access attempt from %s: invalid or missing API key", r.RemoteAddr)
		h.auditLog(r, "unknown", "external-auth-hook", "unauthorized", "invalid or missing api key")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

// denyExternalAuth signals an authentication failure to SFTPGo's
// external_auth_hook.
//
// This deliberately does NOT return a non-2xx status. Per SFTPGo's actual hook
// contract (see internal/dataprovider/dataprovider.go upstream:
// getExternalAuthResponse/doExternalAuth):
//
//   - A non-200 response is treated as a HOOK EXECUTION ERROR
//     ("wrong external auth http status code: %v, expected 200"), not a clean
//     "invalid credentials" denial. It still results in the login being
//     rejected, but it is logged and handled as an infrastructure fault rather
//     than a normal auth failure.
//   - An EMPTY response body means "no modification requested," which SFTPGo
//     only accepts for a user it already has a record for (it falls back to
//     checking the presented password against that stored record). For a
//     brand-new JWT-derived identity — the normal case here, since SFTPGo has
//     no prior record — an empty body instead resolves to "username does not
//     exist," not an explicit deny.
//   - The actual clean-denial signal is: HTTP 200 with a non-empty JSON body
//     whose "username" field is empty. SFTPGo unmarshals the body into its
//     internal user record and explicitly checks
//     `if user.Username == "" { return ErrInvalidCredentials }`.
//
// So credential failures (bad signature, expired token, missing required
// claims, disallowed protocol) all return HTTP 200 with an empty-username body
// here, matching that contract precisely.
func (h *Handler) denyExternalAuth(w http.ResponseWriter) {
	writeJSONResponse(w, http.StatusOK, models.MinimalSFTPGoUser{}, h.logger)
}
