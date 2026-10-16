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

// Package entity is a minimal client for this repo's own entity-service.
// Unlike apps/csm-portal/backend's own entity client (a ~60-method
// passthrough surface for that backend's own handlers), this one
// deliberately implements exactly the endpoints a real consumer here needs:
// POST /users/search (customer.go — the read internal/recipientlinks needs
// to decide which portal link a notification recipient gets), GET
// /cases/{id} and GET /comments/{id} (customer.go — internal/dispatch's
// truncated-event re-fetch, see GetCaseDescription/GetCommentContent's own
// doc comments), and PUT /onboarding-steps/{membershipSfId}/{step}
// (onboarding.go — the one write, internal/dispatch's
// project_contact.invited handler recording each onboarding step's
// outcome). It carries no x-user-id-token or correlation-ID forwarding:
// those exist on the csm-portal-backend client to propagate an end-user's
// identity/request tracing through a real HTTP request, neither of which
// exists in a Kafka consumer.
package entity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/oauthhttp"
)

// CustomerEntityConfig holds the configuration for the customer entity
// service client. Unlike EmailConfig/TwilioConfig (each with its own
// independent credentials), cmd/server/main.go populates TokenURL/ClientID/
// ClientSecret here from this service's shared OAUTH2_* app — the same
// entity-service, authenticated the same way, as apps/csm-portal/backend's
// own entity client. Only BaseURL/Scopes come from this client's own
// CUSTOMER_ENTITY_* env vars. The fields themselves stay generic (not
// hardcoded to the shared app) so a future caller could still construct
// this with independent credentials if that ever changes.
type CustomerEntityConfig struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// CustomerEntityClient is an HTTP client for the customer entity service,
// authenticated via the OAuth2 client credentials grant. Tokens are acquired
// and refreshed automatically; callers need not manage them.
//
// NewCustomerEntityClient never fails and never contacts the token endpoint,
// so it is safe to construct with a zero-value CustomerEntityConfig (e.g.
// when this client is not yet configured for a given deployment) — a
// missing or invalid configuration only surfaces as an error the first time
// a request is actually made.
type CustomerEntityClient struct {
	http    *http.Client
	baseURL string
}

// NewCustomerEntityClient constructs a CustomerEntityClient that
// authenticates against the customer entity service using the OAuth2 client
// credentials grant type.
func NewCustomerEntityClient(cfg CustomerEntityConfig) *CustomerEntityClient {
	httpClient := oauthhttp.NewClient(oauthhttp.Config{
		TokenURL:     cfg.TokenURL,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Scopes:       cfg.Scopes,
	})

	return &CustomerEntityClient{
		http:    httpClient,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
	}
}

// do executes an authenticated HTTP request against the entity service and
// returns the raw JSON response body. The caller owns the returned slice.
func (c *CustomerEntityClient) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("entity: build request %s %s: %w", method, path, err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entity: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("entity: read response body: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		// Unlike this repo's other do() implementations (email, googlechat,
		// twilio), the response body is never included here: a /users/search
		// error can echo the offending recipient email back, which would
		// otherwise leak that PII the moment this error gets logged upstream
		// (eventbus.Consumer logs the full error chain, including
		// apierror.Error's Body, via "err") — see this repo's "No recipient
		// emails in logs" security convention. GetCaseDescription/
		// GetCommentContent's own error bodies are just as safe omitted —
		// case/comment content is exactly what this client exists to fetch
		// deliberately, not something an error path should also echo.
		return nil, &apierror.Error{StatusCode: resp.StatusCode, Body: "response omitted (may contain recipient email addresses or case/comment content)"}
	}

	return respBody, nil
}

// UserRoleInfo is the subset of an entity-service user record
// SearchUsersByEmail needs: enough to resolve which portal a notification
// recipient belongs to (their roles, matched against configured
// customer/CSM role lists — see internal/recipientlinks), plus userType as
// a fallback signal. An email not found on the entity service is simply
// absent from SearchUsersByEmail's result.
type UserRoleInfo struct {
	// ID is entity-service's own user id — logged (never the email address
	// itself, which is PII) by dispatch when an email actually sends, so a
	// support engineer can trace a delivery back to a specific recipient
	// without raw addresses ever appearing in logs.
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	UserType string   `json:"userType"`
}

type searchUsersByEmailRequest struct {
	Pagination struct {
		Limit int `json:"limit"`
	} `json:"pagination"`
	Filters struct {
		Emails []string `json:"emails"`
	} `json:"filters"`
}

// searchUsersByEmailLimit caps how many emails a single POST /users/search
// call filters on — set to the entity service's own enforced maximum.
// SearchUsersByEmail batches into calls of at most this many emails each, so
// a case with an unusually large recipient list still gets every match, not
// a silently truncated first page (entity-service's own response is capped
// at this same maximum regardless of how many emails a single request
// filters on).
const searchUsersByEmailLimit = 50

// SearchUsersByEmail calls POST /users/search filtered to emails (batching
// into calls of at most searchUsersByEmailLimit emails each — see that
// constant's doc comment) and returns each matched user's roles/userType.
func (c *CustomerEntityClient) SearchUsersByEmail(ctx context.Context, emails []string) ([]UserRoleInfo, error) {
	var users []UserRoleInfo
	for start := 0; start < len(emails); start += searchUsersByEmailLimit {
		end := min(start+searchUsersByEmailLimit, len(emails))
		batch, err := c.searchUsersByEmailBatch(ctx, emails[start:end])
		if err != nil {
			return nil, err
		}
		users = append(users, batch...)
	}
	return users, nil
}

// searchUsersByEmailBatch is SearchUsersByEmail's single-request worker —
// emails must already be at most searchUsersByEmailLimit long.
func (c *CustomerEntityClient) searchUsersByEmailBatch(ctx context.Context, emails []string) ([]UserRoleInfo, error) {
	req := searchUsersByEmailRequest{}
	req.Pagination.Limit = searchUsersByEmailLimit
	req.Filters.Emails = emails

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("entity: encode SearchUsersByEmail request: %w", err)
	}

	respBody, err := c.do(ctx, http.MethodPost, "/users/search", reqBody)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Users []UserRoleInfo `json:"users"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("entity: decode SearchUsersByEmail response: %w", err)
	}
	return parsed.Users, nil
}

// GetCaseDescription calls GET /cases/{id} and returns the case's full,
// untouched description — used only when a case.created event arrives with
// DescriptionTruncated set (see events.CaseCreatedPayload's own doc
// comment): entity-service strips embedded base64 inline images out of the
// Kafka event payload to stay under Event Hub's per-message size limit, so
// this is how dispatch.handleCaseCreated recovers the original content
// (images included) for the notification EMAIL specifically — an HTTP
// response has no comparable size ceiling. The Google Chat card never reads
// this; it uses the (already safe, stripped) event payload as sent.
func (c *CustomerEntityClient) GetCaseDescription(ctx context.Context, caseID string) (string, error) {
	respBody, err := c.do(ctx, http.MethodGet, "/cases/"+url.PathEscape(caseID), nil)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("entity: decode GetCaseDescription response: %w", err)
	}
	return parsed.Description, nil
}

// GetCommentContent calls GET /comments/{id} (internal-caller-only on
// entity-service) and returns the comment's full, untouched body — the
// case.comment_added counterpart of GetCaseDescription above, used only when
// CaseCommentTruncated is set on the event (see
// events.CommentAddedPayload.CaseCommentTruncated's own doc comment).
func (c *CustomerEntityClient) GetCommentContent(ctx context.Context, commentID string) (string, error) {
	respBody, err := c.do(ctx, http.MethodGet, "/comments/"+url.PathEscape(commentID), nil)
	if err != nil {
		return "", err
	}
	var parsed struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("entity: decode GetCommentContent response: %w", err)
	}
	return parsed.Content, nil
}
