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

import (
	"context"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// This file serves the customer-health account list: it lets the account
// search return, in one statement, the page of accounts matching the risk and
// health-status filters together with each account's seven summary flags and
// overall health status. The CSM portal backend used to compute the same
// answer by scanning every account and issuing several searches per account.
//
// The flag definitions below are a port of that backend's own Go logic and
// must stay equivalent to it:
//
//   - go-live: a project is "live" when it has an onboarding go-live date; an
//     account activated before 2024-01-01 counts as live regardless. Every
//     project that is not live yields the same result, because that result
//     depends only on the account's activation date, so the account-level
//     aggregate is simply "success" unless some project is not live, in
//     which case it is the activation-date result: pending with no
//     activation date or one in the future, failure once 6 whole months
//     (floor(days / 30.4375)) have passed, pending otherwise.
//   - recent cases: a case-like work item (case, service request,
//     engagement, security report analysis; never an announcement) created
//     within the last 180 days. noSupportCases6mo is its negation.
//   - recent escalations: an escalation created within the last 90 days on a
//     work item of the account, any of the five case-like types (announcement
//     included, as the case search it replaces did not exclude it).
//   - migration delayed / abandoned: an engagement of type MIGRATION that is
//     not closed (or has no state) and was created 60 / 180 days ago or
//     earlier.
//   - EOL product: a deployed product (not deactivated) on any deployment of
//     any project of the account, active or not, whose product version's
//     support end date has passed.
//   - health status: AT_RISK if any project is, else HEALTHY if every project
//     row is, else TO_BE_REVIEWED. An account with no project_health_status
//     row is reported as to_be_reviewed but matches no healthStatus filter.

// Window lengths of the flags above, as SQL interval literals. Constants, never
// built from request data.
const (
	accountHealthRecentCasesWindow       = "180 days"
	accountHealthRecentEscalationsWindow = "90 days"
	accountHealthMigrationDelayedWindow  = "60 days"
	accountHealthMigrationAbandonedWin   = "180 days"
)

// accountHasProductPredicate matches accounts with a deployed product named
// like the bound parameter (%d placeholder, case-insensitive) under any
// project's deployment, active or deactivated. A deactivated deployed product
// is excluded, as in deployed-product search. Refers to the outer alias a.
const accountHasProductPredicate = `EXISTS (
		SELECT 1
		FROM project hp
		JOIN deployment hd ON hd.project_id = hp.id
		JOIN deployed_product hdp ON hdp.deployment_id = hd.id
		JOIN product hpr ON hpr.id = hdp.product_id
		WHERE hp.account_id = a.id
		  AND (hdp.active IS NULL OR hdp.active = TRUE)
		  AND lower(hpr.name) = lower($%d))`

// AccountHealthRow is the raw per-account health summary read by a search that
// asked for it. GoLiveState is success, pending or failure; GoLiveMonths is
// the whole months since activation, nil when there is no activation date or
// it lies in the future. HealthStatus is at_risk, healthy or to_be_reviewed.
type AccountHealthRow struct {
	GoLiveState            string
	GoLiveMonths           *int
	HasRecentCases         bool
	HasEolProduct          bool
	HasAbandonedMigrations bool
	HasMigrationDelays     bool
	HasRecentEscalations   bool
	HealthStatus           string
}

// cleanAccountRisks trims the requested risk keys and drops blank ones.
func cleanAccountRisks(risks []string) []string {
	var out []string
	for _, r := range risks {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// accountSearchNeedsHealth reports whether the filters need the health-summary
// query: a risk or health-status filter, or a request for the summary itself.
func accountSearchNeedsHealth(f domain.SearchAccountsFilters) bool {
	return f.IncludeHealthSummary || f.HealthStatus != "" || len(cleanAccountRisks(f.Risks)) > 0
}

// accountRiskPredicates maps a risk key to its predicate over the fl relation.
var accountRiskPredicates = map[string]string{
	domain.AccountRiskNoGoLive:            "fl.go_live_state = 'failure'",
	domain.AccountRiskGoneLive:            "fl.go_live_state = 'success'",
	domain.AccountRiskRecentCases:         "fl.has_recent_cases",
	domain.AccountRiskNoSupportCases6mo:   "NOT fl.has_recent_cases",
	domain.AccountRiskEolProduct:          "fl.has_eol_product",
	domain.AccountRiskAbandonedMigrations: "fl.has_abandoned_migrations",
	domain.AccountRiskMigrationDelays:     "fl.has_migration_delays",
	domain.AccountRiskRecentEscalations:   "fl.has_recent_escalations",
}

// accountHealthStatusPredicates maps a healthStatus filter value to its
// predicate over fl. to_be_reviewed requires at least one health row.
var accountHealthStatusPredicates = map[string]string{
	domain.AccountHealthStatusAtRisk:       "fl.hs_at_risk",
	domain.AccountHealthStatusHealthy:      "fl.hs_all_healthy",
	domain.AccountHealthStatusToBeReviewed: "(fl.hs_has_rows AND NOT fl.hs_at_risk AND NOT fl.hs_all_healthy)",
}

// accountHealthFilterPredicate renders the AND-ed risk/health-status filter
// over fl, or "" when neither filter is set. Risks are OR-ed; an unrecognized
// risk key matches nothing, and so does an unrecognized health status.
func accountHealthFilterPredicate(f domain.SearchAccountsFilters) string {
	var parts []string
	if risks := cleanAccountRisks(f.Risks); len(risks) > 0 {
		var ors []string
		for _, r := range risks {
			if p, ok := accountRiskPredicates[r]; ok {
				ors = append(ors, p)
			}
		}
		if len(ors) == 0 {
			parts = append(parts, "FALSE")
		} else {
			parts = append(parts, "("+strings.Join(ors, " OR ")+")")
		}
	}
	if f.HealthStatus != "" {
		if p, ok := accountHealthStatusPredicates[f.HealthStatus]; ok {
			parts = append(parts, p)
		} else {
			parts = append(parts, "FALSE")
		}
	}
	return strings.Join(parts, " AND ")
}

// accountHealthFlagsCTE renders the CTE pair fl0/fl that computes every flag for
// the accounts in src (a relation exposing id and activation_date). fl exposes
// the columns the filters and the final SELECT read.
func accountHealthFlagsCTE(src string) string {
	return fmt.Sprintf(`fl0 AS (
	SELECT s.id, s.activation_date,
		EXISTS (SELECT 1 FROM project p WHERE p.account_id = s.id AND p.onboarding_go_live_date IS NULL) AS any_project_not_live,
		CASE
			WHEN s.activation_date IS NULL OR (s.activation_date::timestamp AT TIME ZONE 'UTC') > now() THEN NULL
			ELSE floor(extract(epoch FROM now() - (s.activation_date::timestamp AT TIME ZONE 'UTC')) / 86400 / 30.4375)::int
		END AS go_live_months,
		EXISTS (SELECT 1 FROM work_item wi
		         WHERE wi.account_id = s.id
		           AND wi.type = ANY('{CASE,SERVICE_REQUEST,ENGAGEMENT,SECURITY_REPORT_ANALYSIS}'::work_item_type_enum[])
		           AND wi.created_on >= now() - interval '%[2]s') AS has_recent_cases,
		EXISTS (SELECT 1 FROM project p
		          JOIN deployment d ON d.project_id = p.id
		          JOIN deployed_product dp ON dp.deployment_id = d.id
		          JOIN product pr ON pr.id = dp.product_id
		          JOIN product_version pv ON pv.id = dp.version_id
		         WHERE p.account_id = s.id
		           AND (dp.active IS NULL OR dp.active = TRUE)
		           AND (pv.support_eol_date::timestamp AT TIME ZONE 'UTC') < now()) AS has_eol_product,
		EXISTS (SELECT 1 FROM work_item wi
		          JOIN engagement eng ON eng.id = wi.id
		         WHERE wi.account_id = s.id AND wi.type = 'ENGAGEMENT' AND eng.type = 'MIGRATION'
		           AND eng.state::text IS DISTINCT FROM 'CLOSED'
		           AND wi.created_on <= now() - interval '%[5]s') AS has_abandoned_migrations,
		EXISTS (SELECT 1 FROM work_item wi
		          JOIN engagement eng ON eng.id = wi.id
		         WHERE wi.account_id = s.id AND wi.type = 'ENGAGEMENT' AND eng.type = 'MIGRATION'
		           AND eng.state::text IS DISTINCT FROM 'CLOSED'
		           AND wi.created_on <= now() - interval '%[4]s') AS has_migration_delays,
		EXISTS (SELECT 1 FROM case_escalation ce
		          JOIN work_item wi ON wi.id = ce.work_item_id
		         WHERE wi.account_id = s.id
		           AND wi.type = ANY(%[6]s)
		           AND ce.created_on > now() - interval '%[3]s') AS has_recent_escalations,
		hs.account_id IS NOT NULL AS hs_has_rows,
		COALESCE(hs.at_risk, FALSE) AS hs_at_risk,
		COALESCE(hs.all_healthy, FALSE) AS hs_all_healthy
	FROM %[1]s s
	LEFT JOIN (
		SELECT account_id, bool_or(status = 'AT_RISK') AS at_risk, bool_and(status = 'HEALTHY') AS all_healthy
		FROM project_health_status
		WHERE account_id IN (SELECT id FROM %[1]s)
		GROUP BY account_id
	) hs ON hs.account_id = s.id
),
fl AS (
	SELECT f.*,
		CASE
			WHEN f.activation_date < DATE '2024-01-01' THEN 'success'
			WHEN NOT f.any_project_not_live THEN 'success'
			WHEN f.go_live_months IS NULL THEN 'pending'
			WHEN f.go_live_months >= 6 THEN 'failure'
			ELSE 'pending'
		END AS go_live_state,
		CASE WHEN f.hs_at_risk THEN 'at_risk' WHEN f.hs_all_healthy THEN 'healthy' ELSE 'to_be_reviewed' END AS health_status
	FROM fl0 f
)`, src,
		accountHealthRecentCasesWindow, accountHealthRecentEscalationsWindow,
		accountHealthMigrationDelayedWindow, accountHealthMigrationAbandonedWin,
		caseLikeWorkItemTypes)
}

// buildAccountHealthSearchSQL renders the health-summary statements. where and
// argIdx come from buildAccountSearchWhere; limitIdx/offsetIdx are the
// placeholders for pagination.
//
// Shape: base (accounts passing the plain filters) -> [fl over base, when a
// flag filter needs every candidate] -> matched -> page (window COUNT before
// LIMIT/OFFSET) -> [fl over the page only, when no flag filter] -> final join
// back to the account columns. The count statement stops at matched and is
// only run when the page came back empty at a non-zero offset, the one case
// the window count cannot answer.
func buildAccountHealthSearchSQL(f domain.SearchAccountsFilters, where string, argIdx int) (dataSQL, countSQL string) {
	filter := accountHealthFilterPredicate(f)

	var ctes []string
	ctes = append(ctes, fmt.Sprintf(`base AS (
	SELECT a.id, a.created_on, a.activation_date
	%s
	%s
)`, accountFromJoins, where))

	matchedFrom := "base b"
	if filter != "" {
		ctes = append(ctes, accountHealthFlagsCTE("base"))
		matchedFrom = "base b JOIN fl ON fl.id = b.id WHERE " + filter
	}
	ctes = append(ctes, fmt.Sprintf("matched AS (SELECT b.id, b.created_on FROM %s)", matchedFrom))
	prefix := "WITH " + strings.Join(ctes, ",\n")

	countSQL = prefix + "\nSELECT COUNT(*) FROM matched"

	pageCTEs := []string{fmt.Sprintf(`page AS (
	SELECT m.id, m.created_on, COUNT(*) OVER () AS total_count
	FROM matched m
	ORDER BY m.created_on DESC, m.id
	LIMIT $%d OFFSET $%d
)`, argIdx, argIdx+1)}
	if filter == "" {
		pageCTEs = append(pageCTEs,
			"psrc AS (SELECT b.id, b.activation_date FROM page pg JOIN base b ON b.id = pg.id)",
			accountHealthFlagsCTE("psrc"))
	}

	dataSQL = prefix + ",\n" + strings.Join(pageCTEs, ",\n") + fmt.Sprintf(`
SELECT %s,
	pg.total_count,
	fl.go_live_state, fl.go_live_months, fl.has_recent_cases, fl.has_eol_product,
	fl.has_abandoned_migrations, fl.has_migration_delays, fl.has_recent_escalations, fl.health_status
FROM page pg
JOIN account a ON a.id = pg.id
%s
JOIN fl ON fl.id = a.id
ORDER BY pg.created_on DESC, a.id`, accountSelectColumns, accountLeftJoins)
	return dataSQL, countSQL
}

// searchAccountsWithHealth runs the health-summary search. See
// buildAccountHealthSearchSQL for the statement's shape.
func (r *accountRepo) searchAccountsWithHealth(ctx context.Context, req domain.SearchAccountsRequest, where string, filterArgs []any, argIdx int) ([]AccountRow, int, error) {
	dataSQL, countSQL := buildAccountHealthSearchSQL(req.Filters, where, argIdx)
	dataArgs := append(append([]any{}, filterArgs...), req.Pagination.Limit, req.Pagination.Offset)

	rows, err := r.db.Query(ctx, dataSQL, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query accounts with health: %w", err)
	}
	defer rows.Close()

	accounts := make([]AccountRow, 0, req.Pagination.Limit)
	total := 0
	for rows.Next() {
		var a AccountRow
		var h AccountHealthRow
		dests := append(accountRowScanDests(&a), &total,
			&h.GoLiveState, &h.GoLiveMonths, &h.HasRecentCases, &h.HasEolProduct,
			&h.HasAbandonedMigrations, &h.HasMigrationDelays, &h.HasRecentEscalations, &h.HealthStatus)
		if err := rows.Scan(dests...); err != nil {
			return nil, 0, fmt.Errorf("scan account with health: %w", err)
		}
		a.Health = &h
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate accounts with health: %w", err)
	}
	rows.Close()

	if len(accounts) == 0 && req.Pagination.Offset > 0 {
		if err := r.db.QueryRow(ctx, countSQL, filterArgs...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("count accounts with health: %w", err)
		}
	}
	return accounts, total, nil
}
