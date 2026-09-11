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

package csmclient

import (
	"fmt"
	"net/http"
	"net/url"
)

// httpsOnlyTransport refuses to send a request whose URL isn't HTTPS, unless
// the host is loopback — this client always carries either the OAuth2
// client-secret (token endpoint) or the bearer token it returns (every other
// request), and neither should ever go out in cleartext. Loopback is
// exempted so this package's httptest-backed tests (which bind to
// 127.0.0.1/localhost) keep working without a separate test-only client
// constructor.
type httpsOnlyTransport struct {
	base http.RoundTripper
}

func (t *httpsOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isHTTPSOrLoopback(req.URL) {
		return nil, fmt.Errorf("csmclient: refusing non-HTTPS endpoint %s", req.URL.Redacted())
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

func isHTTPSOrLoopback(u *url.URL) bool {
	if u == nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}
