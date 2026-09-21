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

package middleware

import (
	"context"
	"net/http"
)

// jwtAssertionKey is the context key for the x-jwt-assertion header value.
type jwtAssertionKey struct{}

// JWTAssertion extracts the x-jwt-assertion request header and stores it in
// the request context, mirroring UserIDToken's pattern for x-user-id-token.
//
// This header carries the caller's raw gateway-issued JWT, used only to
// authenticate this service's own outbound SFTPGo token-mint call (see
// sftpgo.Client.MintToken and service.caseService's SFTPGo-backed attachment
// methods) — never re-validated or otherwise trusted by entity-service
// itself, since x-user-id-token (see UserIDToken) is already this service's
// real authentication signal.
//
// As of this change, only the BFF's forwarding of this header on the
// relevant attachment/comment routes is assumed, not yet implemented — see
// this feature's task file for that follow-up. Until the BFF forwards it,
// JWTAssertionFromContext returns "" for every request reaching
// entity-service through the BFF, and the SFTPGo-backed methods fail closed
// (see caseService's own doc comments on that fallback).
func JWTAssertion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertion := r.Header.Get("x-jwt-assertion")
		if assertion != "" {
			r = r.WithContext(context.WithValue(r.Context(), jwtAssertionKey{}, assertion))
		}
		next.ServeHTTP(w, r)
	})
}

// JWTAssertionFromContext retrieves the x-jwt-assertion value stored by the
// JWTAssertion middleware. Returns an empty string if not present.
func JWTAssertionFromContext(ctx context.Context) string {
	v, _ := ctx.Value(jwtAssertionKey{}).(string)
	return v
}
