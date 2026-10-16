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

package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// A Case Alerts chat room's webhook URL is a credential: whoever holds it can
// post into the room. So it is checked for shape before it is stored, sealed
// with AES-256-GCM under a per-environment key, opened only to post a test
// card or for csm-notification-service, and never written to a log or an
// error. What people see is a hint: the space id and the last characters of
// the URL's token.

// chatWebhookPath is a Google Chat incoming webhook's path; the space id is
// not secret.
var chatWebhookPath = regexp.MustCompile(`^/v1/spaces/([A-Za-z0-9_-]{1,64})/messages$`)

const maxChatWebhookLen = 2048

// parseChatWebhook checks raw is a Google Chat incoming webhook URL: https,
// chat.googleapis.com, /v1/spaces/{space}/messages, with a key and a token
// and nothing else that could carry it elsewhere. It returns the URL as given
// (trimmed) and the hint shown in its place. The error never quotes the URL.
func parseChatWebhook(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	bad := func(why string) (string, string, error) {
		return "", "", fmt.Errorf("the webhook URL %s; copy it again from the space's Apps & integrations > Webhooks", why)
	}
	if raw == "" {
		return bad("is empty")
	}
	if len(raw) > maxChatWebhookLen {
		return bad("is too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return bad("is not a URL")
	}
	if u.Scheme != "https" || u.Host != "chat.googleapis.com" || u.User != nil || u.Fragment != "" {
		return bad("must start with https://chat.googleapis.com/")
	}
	m := chatWebhookPath.FindStringSubmatch(u.Path)
	if m == nil {
		return bad("must be a space's webhook: /v1/spaces/<space>/messages")
	}
	q := u.Query()
	key, token := q.Get("key"), q.Get("token")
	if key == "" || token == "" {
		return bad("is missing its key or token")
	}
	return raw, chatWebhookHint(m[1], token), nil
}

// chatWebhookHint is "spaces/<space> · …<last 4 of the token>".
func chatWebhookHint(space, token string) string {
	tail := token
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return "spaces/" + space + " · …" + tail
}

// caseAlertSealer seals and opens webhook URLs. The room id is bound in as
// additional data, so a sealed URL moved onto another room does not open.
type caseAlertSealer struct {
	keyID string
	aead  cipher.AEAD
}

// newCaseAlertSealer builds the sealer from CASE_ALERTS_SECRET_KEY (base64 of
// 32 bytes) and its id. An empty key returns nil, nil: saving URLs is off.
func newCaseAlertSealer(keyB64, keyID string) (*caseAlertSealer, error) {
	keyB64 = strings.TrimSpace(keyB64)
	if keyB64 == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, errors.New("CASE_ALERTS_SECRET_KEY is not base64")
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("CASE_ALERTS_SECRET_KEY must decode to 32 bytes, not %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("CASE_ALERTS_SECRET_KEY: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("CASE_ALERTS_SECRET_KEY: %w", err)
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		keyID = "1"
	}
	return &caseAlertSealer{keyID: keyID, aead: aead}, nil
}

func (s *caseAlertSealer) seal(roomID, webhook string) (repository.CaseAlertSecret, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return repository.CaseAlertSecret{}, fmt.Errorf("make a nonce: %w", err)
	}
	return repository.CaseAlertSecret{
		KeyID:      s.keyID,
		Nonce:      nonce,
		Ciphertext: s.aead.Seal(nil, nonce, []byte(webhook), []byte(roomID)),
	}, nil
}

// errSealedElsewhere is a URL sealed under another key id: a database from
// another environment, or a key that has been replaced.
var errSealedElsewhere = errors.New("the webhook URL was saved under another key; set it again")

func (s *caseAlertSealer) open(roomID string, sec repository.CaseAlertSecret) (string, error) {
	if sec.KeyID != s.keyID {
		return "", errSealedElsewhere
	}
	plain, err := s.aead.Open(nil, sec.Nonce, sec.Ciphertext, []byte(roomID))
	if err != nil {
		return "", errSealedElsewhere
	}
	return string(plain), nil
}

// chatPoster posts a message to a Google Chat webhook.
type chatPoster interface {
	Post(ctx context.Context, webhook, text string) error
}

type httpChatPoster struct{ client *http.Client }

func newHTTPChatPoster() chatPoster {
	return &httpChatPoster{client: &http.Client{
		Timeout: 10 * time.Second,
		// The URL was checked to be chat.googleapis.com; a redirect elsewhere
		// is not followed.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Post sends text. The error says what failed without the URL, which the
// standard client's errors would quote.
func (p *httpChatPoster) Post(ctx context.Context, webhook, text string) error {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return errors.New("could not build the request")
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := p.client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) && uerr.Timeout() {
			return errors.New("no answer from Google Chat in time")
		}
		return errors.New("could not reach Google Chat")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("the webhook answered %d", resp.StatusCode)
	}
	return nil
}
