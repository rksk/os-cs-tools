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
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/config"
	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/constants"
	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/log"
	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/models"
	"github.com/wso2-open-operations/cs-tools/integrations/sftpgo-authentication-service/internal/service"
)

const (
	testIssuer     = "https://idp.example.com/oauth2/token"
	testAudience   = "test-audience"
	testEmail      = "jane.doe@example.com"
	testUserID     = "00000000-0000-0000-0000-000000000000"
	testHookAPIKey = "test-hook-api-key"
)

// newTestJWKSServer starts an httptest server serving the JWKS for key, and
// returns it alongside the server so the caller can Close it.
func newTestJWKSServer(t *testing.T, key *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()

	n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	eBytes := big.NewInt(int64(key.PublicKey.E)).Bytes()
	e := base64.RawURLEncoding.EncodeToString(eBytes)

	jwks := map[string]any{
		"keys": []map[string]any{
			{
				"kty": "RSA",
				"kid": kid,
				"use": "sig",
				"alg": "RS256",
				"n":   n,
				"e":   e,
			},
		},
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
}

// signTestToken builds and signs a JWT with the given claims overrides.
func signTestToken(t *testing.T, key *rsa.PrivateKey, kid string, mutate func(c *jwt.MapClaims)) string {
	t.Helper()

	claims := jwt.MapClaims{
		"email":  testEmail,
		"userid": testUserID,
		"groups": []string{"cs-engineers"},
		"iss":    testIssuer,
		"aud":    []string{testAudience},
		"exp":    time.Now().Add(time.Hour).Unix(),
		"iat":    time.Now().Unix(),
	}
	if mutate != nil {
		mutate(&claims)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// newTestHandler wires a Handler with a real JWTAuthService pointed at a local
// JWKS server, matching how NewJWTAuthService is constructed in main.go.
func newTestHandler(t *testing.T, jwksURL string) *Handler {
	t.Helper()

	cfg := &config.Config{
		DIRPath:                   "/data",
		AuthJWKSEndpoint:          jwksURL,
		AuthIssuer:                testIssuer,
		AuthAudiences:             []string{testAudience},
		AuthTokenValidatorEnabled: true,
		HookAPIKey:                testHookAPIKey,
	}
	logger := log.NewAppLogger("ERROR")

	jwtAuth, err := service.NewJWTAuthService(cfg, logger)
	if err != nil {
		t.Fatalf("failed to construct JWTAuthService: %v", err)
	}

	return &Handler{cfg: cfg, logger: logger, jwtAuth: jwtAuth}
}

func postExternalAuth(t *testing.T, h *Handler, body models.ExternalAuthHookRequest) *httptest.ResponseRecorder {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/external-auth-hook", bytes.NewReader(raw))
	if h.cfg.HookAPIKey != "" {
		req.Header.Set(constants.HeaderAPIKey, h.cfg.HookAPIKey)
	}
	w := httptest.NewRecorder()
	h.ExternalAuthHook(w, req)
	return w
}

func TestExternalAuthHook_ValidJWT_ReturnsSFTPGoUser(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	token := signTestToken(t, key, "key-1", nil)

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: token,
		Protocol: externalAuthProtocolHTTP,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var got models.MinimalSFTPGoUser
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got.Username != testEmail {
		t.Errorf("expected username %q, got %q", testEmail, got.Username)
	}
	if got.Status != 1 {
		t.Errorf("expected status 1 (active), got %d", got.Status)
	}
	if got.HomeDir == "" {
		t.Error("expected a non-empty home_dir")
	}
	if perms, ok := got.Permissions["/"]; !ok || len(perms) == 0 {
		t.Errorf("expected root permissions to be set, got %v", got.Permissions)
	}
}

// TestExternalAuthHook_AttachmentsPathConfigured_GrantsScopedPermissionsOnly
// proves the fix for the overly-broad "/attachments" grant: the mounted
// virtual folder must carry exactly attachmentShareMountPermissions (the
// verbs the attachment-share flow actually exercises) and must NOT include
// "delete" or "rename" — those let any authenticated caller destructively
// modify any file anywhere under the shared attachments tree via SFTPGo's own
// file-management API, entirely outside of any Share object.
func TestExternalAuthHook_AttachmentsPathConfigured_GrantsScopedPermissionsOnly(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	h.cfg.AttachmentsPath = "/data/attachments"
	token := signTestToken(t, key, "key-1", nil)

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: token,
		Protocol: externalAuthProtocolHTTP,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var got models.MinimalSFTPGoUser
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	perms, ok := got.Permissions["/attachments"]
	if !ok {
		t.Fatalf("expected a permission entry for /attachments, got %v", got.Permissions)
	}

	permSet := make(map[string]bool, len(perms))
	for _, p := range perms {
		permSet[p] = true
	}

	for _, want := range []string{"list", "download", "upload", "create_dirs", "overwrite"} {
		if !permSet[want] {
			t.Errorf("expected /attachments permissions to include %q, got %v", want, perms)
		}
	}
	for _, forbidden := range []string{"delete", "rename"} {
		if permSet[forbidden] {
			t.Errorf("expected /attachments permissions to NOT include %q (over-broad grant), got %v", forbidden, perms)
		}
	}

	found := false
	for _, folder := range got.VirtualFolders {
		if folder.VirtualPath == "/attachments" {
			found = true
			if folder.MappedPath != h.cfg.AttachmentsPath {
				t.Errorf("expected mapped path %q, got %q", h.cfg.AttachmentsPath, folder.MappedPath)
			}
		}
	}
	if !found {
		t.Errorf("expected a virtual folder mapping onto /attachments, got %v", got.VirtualFolders)
	}
}

func TestExternalAuthHook_TamperedSignature_DeniesWithEmptyUsername(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate second RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	// Signed with a key that is NOT the one published at the JWKS endpoint but
	// claims the same "kid" — a tampered/forged signature.
	token := signTestToken(t, otherKey, "key-1", nil)

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: token,
		Protocol: externalAuthProtocolHTTP,
	})

	assertDenied(t, w)
}

func TestExternalAuthHook_ExpiredToken_DeniesWithEmptyUsername(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	token := signTestToken(t, key, "key-1", func(c *jwt.MapClaims) {
		(*c)["exp"] = time.Now().Add(-time.Hour).Unix()
		(*c)["iat"] = time.Now().Add(-2 * time.Hour).Unix()
	})

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: token,
		Protocol: externalAuthProtocolHTTP,
	})

	assertDenied(t, w)
}

func TestExternalAuthHook_MissingRequiredClaim_DeniesWithEmptyUsername(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	token := signTestToken(t, key, "key-1", func(c *jwt.MapClaims) {
		delete(*c, "email")
	})

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: token,
		Protocol: externalAuthProtocolHTTP,
	})

	assertDenied(t, w)
}

func TestExternalAuthHook_WrongAudience_DeniesWithEmptyUsername(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	token := signTestToken(t, key, "key-1", func(c *jwt.MapClaims) {
		(*c)["aud"] = []string{"some-other-audience"}
	})

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: token,
		Protocol: externalAuthProtocolHTTP,
	})

	assertDenied(t, w)
}

func TestExternalAuthHook_WrongProtocol_DeniesWithEmptyUsername(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)
	token := signTestToken(t, key, "key-1", nil)

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: token,
		Protocol: "SSH", // only "HTTP" is accepted on this path
	})

	assertDenied(t, w)
}

func TestExternalAuthHook_NoCredential_DeniesWithEmptyUsername(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	srv := newTestJWKSServer(t, key, "key-1")
	defer srv.Close()

	h := newTestHandler(t, srv.URL)

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: "",
		Protocol: externalAuthProtocolHTTP,
	})

	assertDenied(t, w)
}

// TestExternalAuthHook_ValidatorNotConfigured_ReturnsServiceUnavailable covers
// the JWT-validator-missing path specifically (HookAPIKey IS configured and
// presented correctly here, so the request clears authenticateExternalAuthHook
// and the 503 comes from jwtAuth == nil, not from the API-key check).
func TestExternalAuthHook_ValidatorNotConfigured_ReturnsServiceUnavailable(t *testing.T) {
	cfg := &config.Config{DIRPath: "/data", HookAPIKey: testHookAPIKey}
	h := &Handler{cfg: cfg, logger: log.NewAppLogger("ERROR"), jwtAuth: nil}

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: "irrelevant",
		Protocol: externalAuthProtocolHTTP,
	})

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d: %s", w.Code, w.Body.String())
	}
}

// TestExternalAuthHook_HookAPIKeyNotConfigured_FailsClosed proves fix #2:
// unlike the original two hooks (see TestHandler_Authenticate and
// TestPreLoginHookAndAuthHandler_FailOpenWhenHookAPIKeyUnset below),
// /external-auth-hook must NOT fail open when HOOK_API_KEY is unset.
func TestExternalAuthHook_HookAPIKeyNotConfigured_FailsClosed(t *testing.T) {
	cfg := &config.Config{DIRPath: "/data"} // HookAPIKey deliberately empty
	h := &Handler{cfg: cfg, logger: log.NewAppLogger("ERROR"), jwtAuth: nil}

	w := postExternalAuth(t, h, models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: "irrelevant",
		Protocol: externalAuthProtocolHTTP,
	})

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 (fail closed with HOOK_API_KEY unset), got %d: %s", w.Code, w.Body.String())
	}
}

// TestExternalAuthHook_WrongAPIKey_ReturnsUnauthorized proves the constant-time
// comparison still correctly rejects a mismatched key on this route.
func TestExternalAuthHook_WrongAPIKey_ReturnsUnauthorized(t *testing.T) {
	cfg := &config.Config{DIRPath: "/data", HookAPIKey: testHookAPIKey}
	h := &Handler{cfg: cfg, logger: log.NewAppLogger("ERROR"), jwtAuth: nil}

	raw, err := json.Marshal(models.ExternalAuthHookRequest{
		Username: testEmail,
		Password: "irrelevant",
		Protocol: externalAuthProtocolHTTP,
	})
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/external-auth-hook", bytes.NewReader(raw))
	req.Header.Set(constants.HeaderAPIKey, "wrong-key")
	w := httptest.NewRecorder()
	h.ExternalAuthHook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExternalAuthHook_InvalidPayload_ReturnsBadRequest(t *testing.T) {
	cfg := &config.Config{HookAPIKey: testHookAPIKey}
	h := &Handler{cfg: cfg, logger: log.NewAppLogger("ERROR")}

	req := httptest.NewRequest(http.MethodPost, "/external-auth-hook", bytes.NewReader([]byte("not json")))
	req.Header.Set(constants.HeaderAPIKey, testHookAPIKey)
	w := httptest.NewRecorder()
	h.ExternalAuthHook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestExternalAuthHook_MethodNotAllowed(t *testing.T) {
	h := &Handler{cfg: &config.Config{}, logger: log.NewAppLogger("ERROR")}

	req := httptest.NewRequest(http.MethodGet, "/external-auth-hook", nil)
	w := httptest.NewRecorder()
	h.ExternalAuthHook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", w.Code)
	}
}

// TestPreLoginHookAndAuthHandler_FailOpenWhenHookAPIKeyUnset proves fix #2 did
// not touch the original two hooks: they must still fail open (auth passes)
// when HOOK_API_KEY is unset, unlike /external-auth-hook.
func TestPreLoginHookAndAuthHandler_FailOpenWhenHookAPIKeyUnset(t *testing.T) {
	cfg := &config.Config{} // HookAPIKey deliberately empty
	h := &Handler{cfg: cfg, logger: log.NewAppLogger("ERROR")}

	req := httptest.NewRequest(http.MethodPost, "/prelogin-hook", nil)
	w := httptest.NewRecorder()

	if !h.authenticate(req, w) {
		t.Fatalf("expected authenticate() to fail open (return true) when HOOK_API_KEY is unset")
	}
}

// assertDenied checks that a credential-failure response follows SFTPGo's own
// external_auth_hook denial contract: HTTP 200 with a JSON body whose
// "username" field is empty (SFTPGo treats this as ErrInvalidCredentials; a
// non-200 status would instead be treated as a hook execution error, and an
// empty body would be ambiguous with "no change" for a pre-existing user).
func assertDenied(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 (SFTPGo's clean-denial signal), got %d: %s", w.Code, w.Body.String())
	}

	var got models.MinimalSFTPGoUser
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Username != "" {
		t.Errorf("expected empty username to signal denial, got %q", got.Username)
	}
}
