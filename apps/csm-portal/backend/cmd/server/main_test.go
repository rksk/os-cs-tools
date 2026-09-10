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

package main

import (
	"os"
	"os/exec"
	"testing"
)

// TestValidateHTTPSURL covers the pure validation logic behind mustHTTPSURL,
// which gates SFTPGO_BASE_URL and SFTPGO_PUBLIC_BASE_URL: both are used to
// build requests carrying the caller's email and raw gateway JWT (see
// internal/sftpgo.Client.MintToken) or a public download link handed to end
// users (see internal/sftpgo.Client.PublicShareURL), so neither may be
// non-HTTPS or carry embedded userinfo.
func TestValidateHTTPSURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "https ok", value: "https://sftpgo.internal.example.com", wantErr: false},
		{name: "https with trailing slash ok", value: "https://sftpgo.internal.example.com/", wantErr: false},
		{name: "https with path rejected", value: "https://sftpgo.internal.example.com/api", wantErr: true},
		{name: "https with query rejected", value: "https://sftpgo.internal.example.com?x=1", wantErr: true},
		{name: "https with fragment rejected", value: "https://sftpgo.internal.example.com#frag", wantErr: true},
		{name: "http rejected", value: "http://sftpgo.internal.example.com", wantErr: true},
		{name: "scheme-less rejected", value: "sftpgo.internal.example.com", wantErr: true},
		{name: "unparseable rejected", value: "https://%zz", wantErr: true},
		{name: "embedded userinfo rejected", value: "https://user:pass@sftpgo.internal.example.com", wantErr: true},
		{name: "embedded username only rejected", value: "https://user@sftpgo.internal.example.com", wantErr: true},
		{name: "no host rejected", value: "https:///api", wantErr: true},
		{name: "opaque no host rejected", value: "https:opaque", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHTTPSURL(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateHTTPSURL(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

// TestLoadSftpgoConfigRejectsBadURLs exercises the real exit path exercised
// at server startup: loadSftpgoConfig calls os.Exit(1) via mustHTTPSURL when
// SFTPGO_BASE_URL or SFTPGO_PUBLIC_BASE_URL is invalid while the feature flag
// is on, so this re-execs the test binary as a subprocess (the standard Go
// pattern for testing os.Exit call sites) and asserts on its exit status.
func TestLoadSftpgoConfigRejectsBadURLs(t *testing.T) {
	if os.Getenv("BE_LOAD_SFTPGO_CONFIG_SUBPROCESS") == "1" {
		loadSftpgoConfig()
		return
	}

	tests := []struct {
		name            string
		baseURL         string
		publicBaseURL   string
		wantExitNonZero bool
	}{
		{name: "valid https base URL, no public URL", baseURL: "https://sftpgo.internal.example.com", wantExitNonZero: false},
		{name: "valid https for both", baseURL: "https://sftpgo.internal.example.com", publicBaseURL: "https://share.example.com", wantExitNonZero: false},
		{name: "http base URL rejected", baseURL: "http://sftpgo.internal.example.com", wantExitNonZero: true},
		{name: "http public base URL rejected", baseURL: "https://sftpgo.internal.example.com", publicBaseURL: "http://share.example.com", wantExitNonZero: true},
		{name: "userinfo in base URL rejected", baseURL: "https://user:pass@sftpgo.internal.example.com", wantExitNonZero: true},
		{name: "userinfo in public base URL rejected", baseURL: "https://sftpgo.internal.example.com", publicBaseURL: "https://user:pass@share.example.com", wantExitNonZero: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLoadSftpgoConfigRejectsBadURLs$")
			cmd.Env = append(os.Environ(),
				"BE_LOAD_SFTPGO_CONFIG_SUBPROCESS=1",
				"SFTPGO_ATTACHMENT_STORAGE_ENABLED=true",
				"SFTPGO_BASE_URL="+tt.baseURL,
				"SFTPGO_PUBLIC_BASE_URL="+tt.publicBaseURL,
			)
			out, err := cmd.CombinedOutput()

			if tt.wantExitNonZero {
				if err == nil {
					t.Fatalf("expected loadSftpgoConfig to exit non-zero, but it exited cleanly; output:\n%s", out)
				}
				var exitErr *exec.ExitError
				if !isExitError(err, &exitErr) {
					t.Fatalf("expected an *exec.ExitError, got %T: %v; output:\n%s", err, err, out)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected loadSftpgoConfig to exit cleanly, got error %v; output:\n%s", err, out)
			}
		})
	}
}

// isExitError reports whether err is an *exec.ExitError and, if so, assigns
// it to *target.
func isExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}
