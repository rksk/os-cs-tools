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

// Package entity looks up customers in entity-service, for the form's
// "Customer name" suggestions.
package entity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// searchLimit is how many suggestions one search returns.
const searchLimit = 20

// Config is how to reach entity-service. The token comes from a
// client-credentials grant; the client has to be one entity-service accepts.
type Config struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// Customer is one suggestion.
type Customer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Client searches entity-service's accounts.
type Client struct {
	baseURL string
	tokens  oauth2.TokenSource
	http    *http.Client
}

// New returns a Client for cfg. Tokens are fetched when first needed and
// reused until shortly before they expire.
func New(cfg Config) *Client {
	cc := clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		Scopes:       cfg.Scopes,
	}
	tokenCtx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: 10 * time.Second})
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		tokens:  cc.TokenSource(tokenCtx),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// SearchCustomers returns the accounts whose name contains query, as
// entity-service matches them.
func (c *Client) SearchCustomers(ctx context.Context, query string) ([]Customer, error) {
	tok, err := c.tokens.Token()
	if err != nil {
		return nil, fmt.Errorf("entity: get token: %w", err)
	}
	body, err := json.Marshal(map[string]any{
		"pagination": map[string]int{"limit": searchLimit, "offset": 0},
		"filters":    map[string]string{"searchQuery": query},
	})
	if err != nil {
		return nil, fmt.Errorf("entity: encode search: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/accounts/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("entity: build search: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// entity-service reads the caller's token from x-jwt-assertion, as it
	// would arrive through the gateway.
	req.Header.Set("x-jwt-assertion", tok.AccessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entity: search accounts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("entity: search accounts: status %d", resp.StatusCode)
	}

	var result struct {
		Accounts []Customer `json:"accounts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("entity: decode search: %w", err)
	}
	if result.Accounts == nil {
		return []Customer{}, nil
	}
	return result.Accounts, nil
}
