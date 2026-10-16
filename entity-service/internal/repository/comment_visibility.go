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

package repository

import "context"

// hidesDeletedComments reports whether the caller on ctx is a customer-portal
// caller, who must never be handed a soft-deleted comment (deleted_at IS NOT
// NULL): any restricted (non-Unrestricted) caller, a caller whose request
// came through the customer portal's backend even when the signed-in user is
// WSO2 staff (the portal shows staff the customer's view), and a context with
// no identity at all, which fails closed (Scoped refuses such a ctx anyway).
// Unrestricted callers that did not come through the customer portal
// (internal portals, machine-to-machine clients, background jobs stamped
// WithSystemIdentity) keep seeing deleted rows exactly as before.
func hidesDeletedComments(ctx context.Context) bool {
	scope, ok := CallerIdentityFromContext(ctx)
	if !ok {
		return true
	}
	return !scope.Unrestricted || scope.ViaCustomerPortal
}

// deletedCommentFilter returns " AND <prefix>deleted_at IS NULL" for a caller
// hidesDeletedComments says must not see deleted comments, and "" otherwise.
// prefix is the comment table's alias including its trailing dot ("cc."), or
// "" for an unaliased reference. The fragment is built from code constants
// only, never request input.
func deletedCommentFilter(ctx context.Context, prefix string) string {
	if !hidesDeletedComments(ctx) {
		return ""
	}
	return " AND " + prefix + "deleted_at IS NULL"
}
