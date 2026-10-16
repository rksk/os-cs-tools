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

package handler

import (
	"context"
	"encoding/json"
	"log/slog"
)

// withPortalRoles adds a `csmPlatformRoles` field to GET /users/{id}'s
// response, carrying the same portal-role vocabulary GET /users/me reports
// (viewer, escalator, cs_engineer, admin, ...) -- a second, independent
// vocabulary from the profile's own `roles` field, which stays entity-
// service's own role vocabulary (admin, commenter, customer, ...), the one
// the Customer Portal's access is actually modeled on. The two describe
// different things for the same person and neither should hide the other:
// this used to overwrite `roles` outright for an internal target, which
// read as though the Customer-Portal-relevant roles had disappeared, and,
// for a target holding no configured Asgardeo role at all, left the whole
// section looking empty instead of showing what entity-service already had.
//
// GET /users/me computes this from the CALLER's own JWT "roles" claim, which
// only ever describes the caller -- there is no way to read another user's
// token from here. SCIM reaches the same portal-role vocabulary for the user
// being VIEWED instead, in two steps: SearchUser (by email, the only lookup
// key available here) to resolve the target's own Asgardeo user ID, then
// GetUserByID (by that ID) for their full, authoritative role assignment
// list -- deliberately not SearchUser's own Roles field, even though that
// search response does carry one. A search response only returns whichever
// attributes the caller asked for, while GetUserByID's GET-by-ID is the
// canonical resource read and always returns the complete representation.
//
// More importantly, RolesForIDs matches by each role's own Asgardeo ID, not
// its display name: two distinct roles can share the same display name,
// which a name-based match (the plain RolesFor GetMe uses on the JWT's own
// role-name claim) cannot tell apart. ASGARDEO_ROLE_IDS is what supplies the
// name -> ID mapping AccessGuard.WithRoleIDs resolves this against (see
// cmd/server/main.go) -- a role with no configured ID there simply never
// matches, same "no entry means not wired up" posture as every other
// consumer of that config.
//
// SCIM's own role list spans every Asgardeo application the person holds a
// role in, not just this portal, but the full, unfiltered list is handed
// straight to RolesForIDs: a role belonging to another application has no
// configured ID here either, so it never matches anything, the same "doesn't
// match, so it's silently ignored" shape RolesFor already has for names. An
// earlier version first narrowed SCIM's roles to a hardcoded prefix (guessed
// at, not derived from this deployment's actual Asgardeo role-naming
// convention) before calling RolesFor -- confirmed live, via temporary
// diagnostic logging, that the guess was wrong, so the filter silently
// dropped every role and no CSM Platform role was ever resolved for anyone,
// regardless of what they actually held in Asgardeo. Matching against
// configuration (by name there, by ID here) rather than a guessed shape is
// what makes that whole class of failure impossible.
//
// Gated on the target's email domain (isWso2Email), not `userType`: a
// wso2.com address is reserved for WSO2 staff regardless of what userType
// the backing data source happens to have recorded for that row (the same
// edge case withExternalAccountStatus already special-cases for the SCIM
// "external" org lookup) -- a wso2.com account mistakenly tagged `external`
// still has a real Asgardeo CSM role assignment worth showing.
//
// Best-effort like every other enrichment on this profile: any failure
// (decode, lookup, re-encode) is logged and the response returned unchanged.
func (h *UsersHandler) withPortalRoles(ctx context.Context, raw []byte, callerID string) []byte {
	if h.access == nil {
		return raw
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		slog.WarnContext(ctx, "withPortalRoles: decode user profile failed", "userID", callerID, "err", err)
		return raw
	}

	var identity entityUserIdentity
	if rawEmail, ok := envelope["email"]; ok {
		_ = json.Unmarshal(rawEmail, &identity.Email)
	}

	if identity.Email == "" || !isWso2Email(identity.Email) {
		return raw
	}

	info, err := h.scim.SearchUser(ctx, identity.Email)
	if err != nil {
		slog.WarnContext(ctx, "withPortalRoles: scim SearchUser failed", "userID", callerID, "err", err)
		return raw
	}
	if info == nil || info.ID == "" {
		slog.WarnContext(ctx, "withPortalRoles: scim SearchUser: no result", "userID", callerID)
		return raw
	}

	assignments, err := h.scim.GetUserByID(ctx, info.ID)
	if err != nil {
		slog.WarnContext(ctx, "withPortalRoles: scim GetUserByID failed", "userID", callerID, "err", err)
		return raw
	}

	roleIDs := make([]string, 0, len(assignments))
	for _, a := range assignments {
		if a.ID != "" {
			roleIDs = append(roleIDs, a.ID)
		}
	}

	encoded, err := json.Marshal(h.access.RolesForIDs(roleIDs))
	if err != nil {
		slog.WarnContext(ctx, "withPortalRoles: encode roles failed", "userID", callerID, "err", err)
		return raw
	}
	envelope["csmPlatformRoles"] = encoded

	out, err := json.Marshal(envelope)
	if err != nil {
		slog.WarnContext(ctx, "withPortalRoles: encode user profile failed", "userID", callerID, "err", err)
		return raw
	}
	return out
}
