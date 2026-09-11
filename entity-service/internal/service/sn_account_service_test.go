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
	"net/http"
	"testing"
)

// snAccountJSON is a minimal ServiceNow account payload, with placeholders for
// the id and sfId under test.
func snAccountJSON(id, sfID string) string {
	body := `{
		"id": "` + id + `",
		"name": "Acme",
		"classification": "enterprise",
		"pod": "US - WEST",
		"activationDate": "2026-01-01 00:00:00",
		"hasAgent": false,
		"hasKbReferences": false,
		"createdOn": "2026-01-01 00:00:00",
		"updatedOn": "2026-01-01 00:00:00"`
	if sfID != "" {
		body += `, "sfId": "` + sfID + `"`
	}
	body += `}`
	return body
}

// TestSNAccountService_GetAccountByID_SfIDRoundTrips verifies that the SF ID
// ServiceNow's AccountUtils now emits (u_account_id, surfaced as sfId) survives
// the SN JSON -> domain.AccountDetail conversion in GetAccountByID.
func TestSNAccountService_GetAccountByID_SfIDRoundTrips(t *testing.T) {
	const wantSfID = "001E2000014mQ2hIAE"

	mux := http.NewServeMux()
	mux.HandleFunc("/accounts/"+testAccountSysid, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snAccountJSON(testAccountSysid, wantSfID)))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowAccountService(client)

	got, err := svc.GetAccountByID(contextWithUserIDToken("token"), sysidToUUID(testAccountSysid))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.SfID == nil || *got.SfID != wantSfID {
		t.Fatalf("SfID = %v, want %q", got.SfID, wantSfID)
	}
}

// TestSNAccountService_GetAccountByID_SfIDAbsent verifies that an account with
// no linked Salesforce ID surfaces a nil SfID rather than an empty string, so
// FE consumers can distinguish "not linked" from "linked, empty" the same way
// they already do for pod/region.
func TestSNAccountService_GetAccountByID_SfIDAbsent(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/accounts/"+testAccountSysid, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(snAccountJSON(testAccountSysid, "")))
	})

	client := newTestSNClient(t, mux)
	svc := NewServiceNowAccountService(client)

	got, err := svc.GetAccountByID(contextWithUserIDToken("token"), sysidToUUID(testAccountSysid))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.SfID != nil {
		t.Fatalf("SfID = %v, want nil", *got.SfID)
	}
}
