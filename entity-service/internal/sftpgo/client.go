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

// Package sftpgo is entity-service's HTTP client for SFTPGo: it mints a
// short-lived per-user access token (server-side only, never handed to a
// browser) and reads/writes file bytes directly against SFTPGo's plain
// user-file REST API (GET/POST/DELETE /api/v2/user/files). This is
// deliberately much smaller than the equivalent client that used to live in
// apps/csm-portal/backend/internal/sftpgo: there is no Share object and no
// TUS/chunked-upload protocol here, because the browser never talks to
// SFTPGo directly anymore — entity-service relays bytes synchronously in one
// request, mirroring how it already relays ServiceNow attachment bytes (see
// service.snCaseService's CreateCaseAttachment/GetCaseAttachmentContent).
package sftpgo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
)

// maxErrBodyBytes bounds how much of a non-2xx response body is retained on
// an *apierror.Error.
const maxErrBodyBytes = 256

// Config holds the configuration for a SFTPGo Client.
type Config struct {
	// BaseURL is SFTPGo's REST API base, e.g. "https://sftpgo.internal:8080".
	BaseURL string
}

// Client is a minimal SFTPGo REST API client scoped to token-mint and the
// plain (non-share) user-file read/write/delete operations entity-service
// needs.
type Client struct {
	http    *http.Client
	baseURL string
}

// NewClient constructs a SFTPGo Client from cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second, CheckRedirect: refuseInsecureRedirect},
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
	}
}

// refuseInsecureRedirect refuses to follow any redirect whose target is not
// HTTPS or whose origin differs from the original request's origin — see the
// equivalent, more-extensively-documented guard this was ported from in
// apps/csm-portal/backend/internal/sftpgo/client.go.
func refuseInsecureRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("sftpgo: refusing to follow redirect to non-https URL %q", req.URL.Redacted())
	}
	if len(via) > 0 {
		origin := via[0].URL
		if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
			return fmt.Errorf("sftpgo: refusing to follow redirect to foreign origin %q (original origin %s://%s)", req.URL.Redacted(), origin.Scheme, origin.Host)
		}
	}
	return nil
}

// do executes req and returns the full response body plus response headers.
// A transport failure is wrapped with opDesc; any non-2xx status is mapped
// to an *apierror.Error carrying a truncated body excerpt.
func (c *Client) do(req *http.Request, opDesc string) ([]byte, http.Header, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("sftpgo: %s request: %w", opDesc, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		errBody, err := io.ReadAll(io.LimitReader(resp.Body, maxErrBodyBytes+1))
		if err != nil {
			return nil, nil, fmt.Errorf("sftpgo: read %s response: %w", opDesc, err)
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil, &apierror.NotFoundError{Msg: fmt.Sprintf("sftpgo: %s: object not found", opDesc)}
		}
		return nil, nil, &apierror.DownstreamError{Msg: fmt.Sprintf("sftpgo %s failed with status %d: %s", opDesc, resp.StatusCode, truncate(errBody))}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("sftpgo: read %s response: %w", opDesc, err)
	}
	return body, resp.Header, nil
}

// Token is the response body of SFTPGo's GET /api/v2/user/token.
type Token struct {
	AccessToken string          `json:"access_token"`
	ExpiresAt   json.RawMessage `json:"expires_at"`
}

// MintToken calls SFTPGo's GET /api/v2/user/token using HTTP Basic auth:
// username is the caller's email claim, password is the caller's raw
// gateway-issued JWT (the x-jwt-assertion header value). SFTPGo's
// external_auth_hook (integrations/sftpgo-authentication-service's
// ExternalAuthHook) independently re-validates that JWT, so the "password"
// here is never a SFTPGo-native credential. Ported unchanged from
// apps/csm-portal/backend/internal/sftpgo/client.go's MintToken — same
// proven mechanism, now called from entity-service instead of the BFF.
func (c *Client) MintToken(ctx context.Context, email, jwtAssertion string) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/user/token", nil)
	if err != nil {
		return nil, fmt.Errorf("sftpgo: build token request: %w", err)
	}
	req.SetBasicAuth(email, jwtAssertion)

	body, _, err := c.do(req, "token")
	if err != nil {
		return nil, err
	}

	var tok Token
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("sftpgo: decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("sftpgo: token response carried no access_token")
	}
	return &tok, nil
}

// WriteFile uploads data to SFTPGo at storageKey (an absolute path, e.g.
// "/attachments/cases/<caseId>/<dirId>/<filename>"), authenticated as the
// caller via accessToken (minted by MintToken).
//
// Verified against the real SFTPGo OSS source
// (internal/httpd/api_http_user.go's uploadUserFiles/doUploadFiles, routed
// from internal/httpd/server.go's `Post(userFilesPath, uploadUserFiles)`,
// userFilesPath = "/api/v2/user/files" per internal/httpd/httpd.go): the
// upload is a multipart/form-data POST to /api/v2/user/files with the
// destination directory as the "path" query parameter and
// "mkdir_parents=true" so intermediate directories are created on demand,
// carrying the file bytes under the "filenames" form-file field name (the
// exact field name uploadUserFiles reads via r.MultipartForm.File["filenames"]).
func (c *Client) WriteFile(ctx context.Context, accessToken, storageKey string, data []byte) error {
	dir := path.Dir(storageKey)
	fileName := path.Base(storageKey)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("filenames", fileName)
	if err != nil {
		return fmt.Errorf("sftpgo: build multipart file part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("sftpgo: write multipart file part: %w", err)
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("sftpgo: close multipart writer: %w", err)
	}

	q := url.Values{}
	q.Set("path", dir)
	q.Set("mkdir_parents", "true")
	endpoint := c.baseURL + "/api/v2/user/files?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return fmt.Errorf("sftpgo: build file-write request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+accessToken)

	if _, _, err := c.do(req, "file-write"); err != nil {
		return err
	}
	return nil
}

// ReadFile downloads the file stored at storageKey, authenticated as the
// caller via accessToken, and returns its bytes plus the Content-Type
// SFTPGo reports (derived server-side from the file extension — see
// internal/httpd/api_utils.go's downloadFile, `mime.TypeByExtension`).
//
// Verified against the real SFTPGo OSS source
// (internal/httpd/api_http_user.go's getUserFile, routed from
// internal/httpd/server.go's `Get(userFilesPath, getUserFile)`): GET
// /api/v2/user/files?path=<full file path>.
func (c *Client) ReadFile(ctx context.Context, accessToken, storageKey string) ([]byte, string, error) {
	endpoint := c.baseURL + "/api/v2/user/files?path=" + url.QueryEscape(storageKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("sftpgo: build file-read request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	body, header, err := c.do(req, "file-read")
	if err != nil {
		return nil, "", err
	}
	return body, header.Get("Content-Type"), nil
}

// RemoveFile deletes one stored file, addressed by its storage key, via
// SFTPGo's DELETE /api/v2/user/files?path=... endpoint. Used for best-effort
// rollback cleanup when a multi-step flow (see the inline-image extraction
// in service.caseService.CreateCaseComment) fails after some bytes were
// already uploaded. Verified against the real SFTPGo OSS source
// (internal/httpd/api_http_user.go's deleteUserFile, routed from
// internal/httpd/server.go's `Delete(userFilesPath, deleteUserFile)`).
func (c *Client) RemoveFile(ctx context.Context, accessToken, storageKey string) error {
	endpoint := c.baseURL + "/api/v2/user/files?path=" + url.QueryEscape(storageKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("sftpgo: build file-delete request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	if _, _, err := c.do(req, "file-delete"); err != nil {
		return err
	}
	return nil
}

// truncate bounds body to maxErrBodyBytes for inclusion on an error message.
func truncate(body []byte) string {
	if len(body) > maxErrBodyBytes {
		body = body[:maxErrBodyBytes]
	}
	return string(body)
}
