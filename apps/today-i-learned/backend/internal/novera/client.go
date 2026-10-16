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

// Package novera tells Novera, the Google Chat assistant, about a new entry.
// Novera sends it as a card to every employee who has a chat open with it.
package novera

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/wso2-open-operations/cs-tools/apps/today-i-learned/backend/internal/entry"
)

// Config is how to reach Novera's notifications endpoint. The API gateway in
// front of it takes a client-credentials token from an application
// subscribed to Novera's API.
type Config struct {
	NotifyURL    string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
	// IconURL is the TIL logo shown on the card; empty for none.
	IconURL string
}

// IconURL is where the One WSO2 webapp at baseURL serves the TIL logo, or ""
// when baseURL isn't https: Chat only loads icons over https.
func IconURL(baseURL string) string {
	if !strings.HasPrefix(baseURL, "https://") {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/til-logo.png"
}

// Client sends new-entry notifications.
type Client struct {
	url     string
	iconURL string
	http    *http.Client
}

// New returns a Client for cfg.
func New(cfg Config) *Client {
	cc := clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		Scopes:       cfg.Scopes,
	}
	tokenCtx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: 10 * time.Second})
	httpClient := cc.Client(tokenCtx)
	httpClient.Timeout = 10 * time.Second
	return &Client{url: cfg.NotifyURL, iconURL: cfg.IconURL, http: httpClient}
}

// notification is the body of Novera's POST /internal/notifications. Every
// value is plain text: Novera builds the card and escapes it.
type notification struct {
	Source   string `json:"source"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body"`
	Link     *link  `json:"link,omitempty"`
	Footer   string `json:"footer,omitempty"`
	IconURL  string `json:"iconUrl,omitempty"`
}

type link struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Notify announces e. entryURL, when not empty, is the link the card opens.
// It returns an error, already logged, when Novera didn't accept the
// announcement, e.g. because Novera or Google Chat can't be reached; the
// entry then stays unannounced for a moderator to send again.
func (c *Client) Notify(ctx context.Context, e entry.Entry, entryURL string) error {
	n := notification{
		Source:   "til",
		Title:    e.Title,
		Subtitle: e.Where,
		Body:     entry.BulletedText(e.What),
		Footer:   "Authored by " + e.Who,
		IconURL:  c.iconURL,
	}
	if e.WhereDetail != nil {
		n.Subtitle += " — " + *e.WhereDetail
	}
	if entryURL != "" {
		n.Link = &link{URL: entryURL, Label: "Read more"}
	}
	body, err := json.Marshal(n)
	if err != nil {
		slog.ErrorContext(ctx, "novera: encode notification", "err", err)
		return fmt.Errorf("novera: encode notification: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		slog.ErrorContext(ctx, "novera: build notification", "err", err)
		return fmt.Errorf("novera: build notification: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "novera: send notification", "entryID", e.ID, "err", err)
		return fmt.Errorf("novera: send notification: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.ErrorContext(ctx, "novera: notification rejected", "entryID", e.ID, "status", resp.StatusCode)
		return fmt.Errorf("novera: notification rejected with status %d", resp.StatusCode)
	}
	return nil
}
