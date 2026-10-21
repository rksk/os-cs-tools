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

// Package recipientlinks resolves, for a list of notification recipients,
// which portal's case link each one should get in their email: a customer
// contact should never be handed a CSM-portal link they can't access (and
// vice versa isn't the point, but keeping the audiences separate is). This
// is a per-recipient decision, not a per-event one — the same
// case.comment_added notification can go to both a customer watcher and an
// internal CSM watcher at once, each needing a different link — so
// internal/dispatch calls this to resolve links, then groups recipients by
// the link they resolved to, before rendering/sending anything (see
// Dispatcher.groupByLink). This does not resolve *who* to notify — every
// case.* payload still carries its own caller-supplied Recipients list, see
// internal/events' package doc — only which link a given recipient gets.
//
// Note the customer portal has no comment-permalink fragment handling today
// (only the CSM portal's frontend reads location.hash to scroll to a
// comment) — a comment-specific fragment on a customer-portal link is
// simply inert there, not an error.
package recipientlinks

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/entity"
)

// entityClient abstracts entity.CustomerEntityClient's user-role lookup for
// testability.
type entityClient interface {
	SearchUsersByEmail(ctx context.Context, emails []string) ([]entity.UserRoleInfo, error)
}

// wso2EmailDomain is WSO2's own corporate domain — mirrors
// apps/csm-portal/backend's own wso2EmailDomain constant (see that
// package's user_external_account.go), used the same way here: linkFor's
// fallback classifier when neither a recipient's user type nor its roles
// (both from the same SearchUsersByEmail response ResolveLinks already calls)
// resolve a portal.
const wso2EmailDomain = "wso2.com"

// Config holds the role classification and portal base URLs Resolver needs.
// CustomerRoles and CSMRoles are a secondary signal, consulted only when the
// user type does not decide, and need not be exhaustive of every role in the
// system — see ResolveLinks' doc comment for the full order.
type Config struct {
	CustomerRoles   []string
	CSMRoles        []string
	CustomerBaseURL string
	CSMBaseURL      string
}

// Resolver resolves recipient emails to portal-appropriate case links.
type Resolver struct {
	entity        entityClient
	customerRoles map[string]bool
	csmRoles      map[string]bool
	customerBase  string
	csmBase       string
}

// New constructs a Resolver.
func New(entity entityClient, cfg Config) *Resolver {
	return &Resolver{
		entity:        entity,
		customerRoles: toSet(cfg.CustomerRoles),
		csmRoles:      toSet(cfg.CSMRoles),
		customerBase:  cfg.CustomerBaseURL,
		csmBase:       cfg.CSMBaseURL,
	}
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

// RecipientLink pairs a recipient's email with the case link resolved for
// their role.
type RecipientLink struct {
	Email    string
	CaseLink string
	// UserID is the recipient's entity-service user id, "" when entity-service
	// has no record for this email (linkFor's "not found" fallback path) —
	// dispatch logs this instead of the email address itself when a
	// notification actually sends, so a delivery is traceable without a raw
	// address ever appearing in logs.
	UserID string
}

// ResolveLinks looks up each of emails via entity-service and returns the
// case link appropriate to each: a customer gets the customer portal's link
// (<CustomerBaseURL>/projects/{projectID}/support/cases/{caseID}), a CSM
// recipient gets the CSM portal's link (<CSMBaseURL>/cases/{caseID}).
//
// Classification order (see classify, shared with IsCustomer): the user type
// entity-service reports first, then the operator-curated role lists as a
// secondary signal, then the email domain. A recipient neither of those
// resolves — entity-service has no record, or its user type is empty/unknown
// and its roles match neither list — falls back to the email domain
// (wso2EmailDomain gets the CSM portal link, anything else the customer
// portal link) and that is logged at warn: with user types in play it is a
// genuine data gap, not an expected path.
//
// The returned links are the bare case link only — appending a
// comment-specific anchor/fragment is internal/dispatch's job (see
// commentLinkFor there), since that varies by event type and this package
// only knows about portal audiences.
func (r *Resolver) ResolveLinks(ctx context.Context, emails []string, projectID, caseID string) ([]RecipientLink, error) {
	if len(emails) == 0 {
		return nil, nil
	}

	users, err := r.entity.SearchUsersByEmail(ctx, emails)
	if err != nil {
		return nil, fmt.Errorf("recipientlinks: search users: %w", err)
	}

	// Keyed lowercase: entity-service and the caller's own recipient list
	// don't necessarily agree on email casing, and an email address is the
	// same address regardless of case.
	byEmail := make(map[string]entity.UserRoleInfo, len(users))
	for _, u := range users {
		byEmail[strings.ToLower(u.Email)] = u
	}

	links := make([]RecipientLink, 0, len(emails))
	for _, email := range emails {
		user, found := byEmail[strings.ToLower(email)]
		links = append(links, RecipientLink{
			Email:    email,
			CaseLink: r.linkFor(ctx, user, found, email, projectID, caseID),
			UserID:   user.ID,
		})
	}
	return links, nil
}

// linkFor takes the recipient's email only for its own domain comparison
// against wso2EmailDomain — never logged, same PII reasoning as everywhere
// else in this package (an email address in the logs is what this repo's
// own convention disallows). caseID identifies which notification a
// log record belongs to without identifying who it's about.
//
// The classification itself is classify's; see its doc comment for the order.
// When it could not resolve the recipient and the email domain decided, that
// is logged at warn with the raw userType (an enum, not PII).
func (r *Resolver) linkFor(ctx context.Context, user entity.UserRoleInfo, found bool, email, projectID, caseID string) string {
	isCustomer, resolved := r.classify(user, found, email)
	if !resolved {
		slog.WarnContext(ctx, "recipientlinks: recipient's roles did not resolve a portal; used email domain as a fallback",
			"caseID", caseID, "found", found, "userType", user.UserType, "roleCount", len(user.Roles), "isCustomer", isCustomer)
	}

	if isCustomer {
		return fmt.Sprintf("%s/projects/%s/support/cases/%s", r.customerBase, url.PathEscape(projectID), url.PathEscape(caseID))
	}
	return fmt.Sprintf("%s/cases/%s", r.csmBase, url.PathEscape(caseID))
}

// classify decides whether a recipient is a customer. It is the single
// classifier behind both linkFor and IsCustomer so the two cannot drift.
// resolved is false only when the email domain had to decide.
//
// Order:
//  1. found, userType "internal" (case-insensitive): CSM.
//  2. found, userType "customer" or "external" (case-insensitive): customer.
//     "external" is another data source's label for the same customer users.
//  3. found, a role in CustomerRoles: customer; else a role in CSMRoles: CSM.
//     The role lists are a secondary, operator-curated signal.
//  4. Otherwise (not found, or userType empty/"system"/unknown with roles
//     unmatched): email domain, wso2EmailDomain is CSM, anything else is a
//     customer.
//
// The user type wins over roles: an internal user carrying a customer role
// entry is still CSM.
func (r *Resolver) classify(user entity.UserRoleInfo, found bool, email string) (isCustomer, resolved bool) {
	if found {
		switch strings.ToLower(strings.TrimSpace(user.UserType)) {
		case "internal":
			return false, true
		case "customer", "external":
			return true, true
		}
		switch {
		case r.matchesAny(user.Roles, r.customerRoles):
			return true, true
		case r.matchesAny(user.Roles, r.csmRoles):
			return false, true
		}
	}
	return !strings.EqualFold(emailDomain(email), wso2EmailDomain), false
}

// IsCustomer classifies a single email exactly as linkFor does (user type,
// then roles, then the email's own domain; see classify). Unlike
// ResolveLinks, this doesn't build a case link at all: it exists for a caller
// (dispatch's frustration-detection check) that only needs the yes/no
// classification itself, not a portal link. It does not log the fallback:
// it has no case to attribute a record to.
func (r *Resolver) IsCustomer(ctx context.Context, email string) (bool, error) {
	users, err := r.entity.SearchUsersByEmail(ctx, []string{email})
	if err != nil {
		return false, fmt.Errorf("recipientlinks: search users: %w", err)
	}

	var user entity.UserRoleInfo
	found := false
	for _, u := range users {
		if strings.EqualFold(u.Email, email) {
			user, found = u, true
			break
		}
	}

	isCustomer, _ := r.classify(user, found, email)
	return isCustomer, nil
}

// CSMLink builds the CSM portal's case link directly, without any recipient
// or role lookup — for a notification with no per-recipient audience to
// resolve against (e.g. dispatch's case.created Google Chat alert, which
// posts once to a shared internal Chat space rather than per-recipient), the
// CSM portal is always the right audience; there's no customer-facing
// equivalent of that Chat space to route to instead.
func (r *Resolver) CSMLink(caseID string) string {
	return fmt.Sprintf("%s/cases/%s", r.csmBase, url.PathEscape(caseID))
}

// ChangeRequestLink builds the link in a change-request approval notice.
//
// Not CSMLink: that returns "<csmBase>/cases/<id>", and a change request is not
// a case — the CSM portal serves it at /operations/change-requests/<id>, and
// the customer portal nests its own copy under the project. The audience on the
// notice picks between them, which is also why it must: an internal approver
// linked into the customer portal lands somewhere they have no reason to be,
// and a customer linked into the CSM portal lands somewhere they cannot go at
// all.
//
// A customer notice with no project falls back to the CSM link rather than
// building "/projects//operations/..." — a wrong link a recipient can recognise
// beats a malformed one.
func (r *Resolver) ChangeRequestLink(audience, changeRequestID, projectID string) string {
	if audience == "customer" && projectID != "" {
		return fmt.Sprintf("%s/projects/%s/operations/change-requests/%s",
			r.customerBase, url.PathEscape(projectID), url.PathEscape(changeRequestID))
	}
	return fmt.Sprintf("%s/operations/change-requests/%s", r.csmBase, url.PathEscape(changeRequestID))
}

// OutageLink is an outage's page in the CSM portal, the route the portal's own
// outage list navigates to.
func (r *Resolver) OutageLink(outageID string) string {
	return fmt.Sprintf("%s/operations/outages/%s", r.csmBase, url.PathEscape(outageID))
}

// ServiceRequestLink is a service request's page in the CSM portal. Not
// CSMLink: the portal serves an SR at /operations/service-requests/<id>, and
// /cases/<id> would open it as a generic case.
func (r *Resolver) ServiceRequestLink(caseID string) string {
	return fmt.Sprintf("%s/operations/service-requests/%s", r.csmBase, url.PathEscape(caseID))
}

// IncidentLink is an incident's page in the CSM portal.
func (r *Resolver) IncidentLink(incidentID string) string {
	return fmt.Sprintf("%s/operations/incidents/%s", r.csmBase, url.PathEscape(incidentID))
}

// emailDomain returns the part of email after its last "@", lowercased —
// "" for an address with no "@" at all, which wso2EmailDomain will simply
// never match (see linkFor).
func emailDomain(email string) string {
	i := strings.LastIndex(email, "@")
	if i < 0 {
		return ""
	}
	return strings.ToLower(email[i+1:])
}

func (r *Resolver) matchesAny(roles []string, set map[string]bool) bool {
	for _, role := range roles {
		if set[role] {
			return true
		}
	}
	return false
}
