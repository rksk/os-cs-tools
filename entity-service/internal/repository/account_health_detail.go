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
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// This file serves GET /accounts/{id}/health-detail: the per-project
// customer-health breakdown of one account, with the items behind every flag.
// The CSM portal backend used to assemble it from every case of the account
// (50 per request), a search for the escalations of those cases, and a
// deployment and deployed-product search per project. Here it is six
// set-based statements, however many projects and cases the account has.
//
// It is a port of that backend's own logic and must stay equivalent to it
// (and, flag for flag, to the summary in account_health_search.go):
//
//   - projects: every project of the account, newest first.
//   - recent cases: a case-like work item (case, service request, engagement,
//     security report analysis; never an announcement) created in the last 180
//     days, grouped by priority. The priority is the severity of the "case"
//     row, "Unspecified" when there is none; groups are in the order each
//     priority first appears, and so are the cases in a group, newest first.
//   - abandoned / delayed: an engagement of type MIGRATION whose state, read
//     as caseLikeStateColumn reads it, is not CLOSED (or is missing), created
//     180 / 60 days ago or earlier. Every abandoned one is also delayed.
//   - escalations: every escalation on a work item of the project, any of the
//     five case-like types (announcement included), newest first.
//   - deployments: the active deployments of the project, newest first.
//   - EOL software: an active (or NULL-active) deployed product, on any
//     deployment of the project, active or not, whose product version's
//     support end date has passed; one entry per product version, in the
//     order the newest deployed product of each first appears, each listing
//     the distinct deployments running it.
//   - go-live status: as in the summary, per project.
//
// Work items count toward a project through their own project_id, and only
// work items of the account itself are read.

// accountHealthDetailCaseTypes is the case-like work item types that count as a
// case for the recent-cases list: all of caseLikeWorkItemTypes but ANNOUNCEMENT.
const accountHealthDetailCaseTypes = `'{CASE,SERVICE_REQUEST,ENGAGEMENT,SECURITY_REPORT_ANALYSIS}'::work_item_type_enum[]`

// accountHealthDetailProjects selects the projects of the account bound to $1.
const accountHealthDetailProjects = `(SELECT hp.id FROM project hp WHERE hp.account_id = $1)`

// AccountHealthDetailRow is the raw health detail of one account. GoLiveMonths
// is the whole months since activation, nil when there is no activation date or
// it lies in the future.
type AccountHealthDetailRow struct {
	AccountName    string
	ActivationDate *time.Time
	GoLiveMonths   *int
	Projects       []ProjectHealthDetailRow
}

// ProjectHealthDetailRow is one project of an AccountHealthDetailRow. Detail
// carries everything except GoLiveStatus, which the service builds from
// GoLiveState (success, pending or failure) and the account's activation.
type ProjectHealthDetailRow struct {
	GoLiveState string
	Detail      domain.ProjectHealthDetail
}

// accountHealthDetailAccountSQL reads the account and its projects in one
// statement: one row per project, or a single row with a NULL project when
// there is none; no row at all when the account does not exist.
const accountHealthDetailAccountSQL = `
WITH acc AS (
	SELECT a.id, a.name, a.activation_date,
		CASE
			WHEN a.activation_date IS NULL OR (a.activation_date::timestamp AT TIME ZONE 'UTC') > now() THEN NULL
			ELSE floor(extract(epoch FROM now() - (a.activation_date::timestamp AT TIME ZONE 'UTC')) / 86400 / 30.4375)::int
		END AS go_live_months
	FROM account a
	WHERE a.id = $1
)
SELECT acc.name, acc.activation_date, acc.go_live_months, p.id, p.name,
	CASE
		WHEN p.onboarding_go_live_date IS NOT NULL THEN 'success'
		WHEN acc.activation_date < DATE '2024-01-01' THEN 'success'
		WHEN acc.go_live_months IS NULL THEN 'pending'
		WHEN acc.go_live_months >= 6 THEN 'failure'
		ELSE 'pending'
	END
FROM acc
LEFT JOIN project p ON p.account_id = acc.id
ORDER BY p.created_on DESC NULLS LAST, p.id`

// accountHealthDetailRecentCasesSQL lists the recent cases, newest first, in the
// order the case search it replaces returned them.
var accountHealthDetailRecentCasesSQL = fmt.Sprintf(`
SELECT wi.project_id, wi.id, wi.number, c.severity::TEXT
FROM work_item wi
LEFT JOIN "case" c ON c.id = wi.id
WHERE wi.account_id = $1
  AND wi.project_id IN %s
  AND wi.type = ANY(%s)
  AND wi.created_on >= now() - interval '%s'
ORDER BY wi.created_on DESC NULLS LAST, wi.id`,
	accountHealthDetailProjects, accountHealthDetailCaseTypes, accountHealthRecentCasesWindow)

// accountHealthDetailMigrationsSQL lists the open migration engagements old
// enough to be delayed, newest first, flagging those old enough to be
// abandoned. The state is read through caseLikeStateColumn, as a case search
// reads it.
var accountHealthDetailMigrationsSQL = fmt.Sprintf(`
SELECT wi.project_id, wi.id, wi.number,
	wi.created_on <= now() - interval '%s' AS abandoned
FROM work_item wi
LEFT JOIN "case" c ON c.id = wi.id`+caseLikeJoins+`
WHERE wi.account_id = $1
  AND wi.project_id IN %s
  AND wi.type = 'ENGAGEMENT'
  AND eng.type = 'MIGRATION'
  AND `+caseLikeStateColumn+` IS DISTINCT FROM 'CLOSED'
  AND wi.created_on <= now() - interval '%s'
ORDER BY wi.created_on DESC NULLS LAST, wi.id`,
	accountHealthMigrationAbandonedWin, accountHealthDetailProjects, accountHealthMigrationDelayedWindow)

// accountHealthDetailEscalationsSQL lists every escalation on a work item of a
// project of the account, newest first.
var accountHealthDetailEscalationsSQL = fmt.Sprintf(`
SELECT wi.project_id, ce.id, wi.number
FROM case_escalation ce
JOIN work_item wi ON wi.id = ce.work_item_id
WHERE wi.account_id = $1
  AND wi.project_id IN %s
  AND wi.type = ANY(%s)
ORDER BY ce.created_on DESC, ce.id`,
	accountHealthDetailProjects, caseLikeWorkItemTypes)

// accountHealthDetailDeploymentsSQL lists the deployments of the projects of the
// account, active or not, newest first.
var accountHealthDetailDeploymentsSQL = fmt.Sprintf(`
SELECT d.project_id, d.id, d.name, d.is_active IS TRUE
FROM deployment d
WHERE d.project_id IN %s
ORDER BY d.created_on DESC, d.id`, accountHealthDetailProjects)

// accountHealthDetailEolSQL lists the deployed products past their support end
// date on any deployment of the projects of the account, newest first.
var accountHealthDetailEolSQL = fmt.Sprintf(`
SELECT d.project_id, pv.id, pr.name, COALESCE(pv.version, ''), pv.support_eol_date, d.id, d.name
FROM deployed_product dp
JOIN deployment d ON d.id = dp.deployment_id
JOIN product pr ON pr.id = dp.product_id
JOIN product_version pv ON pv.id = dp.version_id
WHERE d.project_id IN %s
  AND (dp.active IS NULL OR dp.active = TRUE)
  AND (pv.support_eol_date::timestamp AT TIME ZONE 'UTC') < now()
ORDER BY dp.created_on DESC, dp.id`, accountHealthDetailProjects)

// healthDetailPriority is the priority a case is listed under: its severity, or
// "Unspecified" when it has none (or one that maps to nothing).
func healthDetailPriority(severity *string) string {
	if severity != nil {
		if mapped := string(caseSeverityFromEnum[*severity]); mapped != "" {
			return mapped
		}
	}
	return "Unspecified"
}

// GetAccountHealthDetail implements AccountRepository. A NotFoundError is
// returned when the account does not exist.
func (r *accountRepo) GetAccountHealthDetail(ctx context.Context, accountID string) (AccountHealthDetailRow, error) {
	out, err := r.healthDetailProjects(ctx, accountID)
	if err != nil {
		return AccountHealthDetailRow{}, err
	}
	if len(out.Projects) == 0 {
		return out, nil
	}
	byID := make(map[string]*ProjectHealthDetailRow, len(out.Projects))
	for i := range out.Projects {
		byID[out.Projects[i].Detail.ID] = &out.Projects[i]
	}

	for _, step := range []struct {
		name string
		run  func(context.Context, string, map[string]*ProjectHealthDetailRow) error
	}{
		{"recent cases", r.healthDetailRecentCases},
		{"migrations", r.healthDetailMigrations},
		{"escalations", r.healthDetailEscalations},
		{"deployments", r.healthDetailDeployments},
		{"eol products", r.healthDetailEol},
	} {
		if err := step.run(ctx, accountID, byID); err != nil {
			return AccountHealthDetailRow{}, fmt.Errorf("account health detail, %s: %w", step.name, err)
		}
	}
	return out, nil
}

func (r *accountRepo) healthDetailProjects(ctx context.Context, accountID string) (AccountHealthDetailRow, error) {
	rows, err := r.db.Query(ctx, accountHealthDetailAccountSQL, accountID)
	if err != nil {
		return AccountHealthDetailRow{}, fmt.Errorf("query account health detail projects: %w", err)
	}
	defer rows.Close()

	var out AccountHealthDetailRow
	found := false
	for rows.Next() {
		var projectID, projectName *string
		var state *string
		if err := rows.Scan(&out.AccountName, &out.ActivationDate, &out.GoLiveMonths, &projectID, &projectName, &state); err != nil {
			return AccountHealthDetailRow{}, fmt.Errorf("scan account health detail project: %w", err)
		}
		found = true
		if projectID == nil {
			continue
		}
		out.Projects = append(out.Projects, ProjectHealthDetailRow{
			GoLiveState: stringOrEmpty(state),
			Detail: domain.ProjectHealthDetail{
				ID:                      *projectID,
				Name:                    stringOrEmpty(projectName),
				DetailedRecentCases:     []domain.HealthCasePriorityGroup{},
				DetailedAbandonedCases:  []domain.HealthCaseLink{},
				SoftwareModel:           []domain.HealthEolProduct{},
				Deployments:             []domain.HealthDeploymentRef{},
				DetailedMigrationDelays: []domain.HealthCaseLink{},
				DetailedEscalatedCases:  []domain.HealthCaseLink{},
			},
		})
	}
	if err := rows.Err(); err != nil {
		return AccountHealthDetailRow{}, fmt.Errorf("iterate account health detail projects: %w", err)
	}
	if !found {
		return AccountHealthDetailRow{}, &apierror.NotFoundError{Msg: "account not found"}
	}
	return out, nil
}

func (r *accountRepo) healthDetailRecentCases(ctx context.Context, accountID string, byID map[string]*ProjectHealthDetailRow) error {
	rows, err := r.db.Query(ctx, accountHealthDetailRecentCasesSQL, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID, caseID, number string
		var severity *string
		if err := rows.Scan(&projectID, &caseID, &number, &severity); err != nil {
			return err
		}
		p := byID[projectID]
		if p == nil {
			continue
		}
		d := &p.Detail
		d.HasRecentCases = true
		d.TotalRecentCases++
		priority := healthDetailPriority(severity)
		idx := -1
		for i := range d.DetailedRecentCases {
			if d.DetailedRecentCases[i].Priority == priority {
				idx = i
				break
			}
		}
		if idx < 0 {
			d.DetailedRecentCases = append(d.DetailedRecentCases, domain.HealthCasePriorityGroup{Priority: priority, Cases: []domain.HealthCaseInfo{}})
			idx = len(d.DetailedRecentCases) - 1
		}
		g := &d.DetailedRecentCases[idx]
		g.Count++
		g.Cases = append(g.Cases, domain.HealthCaseInfo{ID: caseID, Number: number})
	}
	return rows.Err()
}

func (r *accountRepo) healthDetailMigrations(ctx context.Context, accountID string, byID map[string]*ProjectHealthDetailRow) error {
	rows, err := r.db.Query(ctx, accountHealthDetailMigrationsSQL, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID, caseID, number string
		var abandoned bool
		if err := rows.Scan(&projectID, &caseID, &number, &abandoned); err != nil {
			return err
		}
		p := byID[projectID]
		if p == nil {
			continue
		}
		link := domain.HealthCaseLink{ID: caseID, Number: number}
		p.Detail.HasMigrationDelays = true
		p.Detail.DetailedMigrationDelays = append(p.Detail.DetailedMigrationDelays, link)
		if abandoned {
			p.Detail.HasAbandonedCases = true
			p.Detail.DetailedAbandonedCases = append(p.Detail.DetailedAbandonedCases, link)
		}
	}
	return rows.Err()
}

func (r *accountRepo) healthDetailEscalations(ctx context.Context, accountID string, byID map[string]*ProjectHealthDetailRow) error {
	rows, err := r.db.Query(ctx, accountHealthDetailEscalationsSQL, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID, escalationID, number string
		if err := rows.Scan(&projectID, &escalationID, &number); err != nil {
			return err
		}
		p := byID[projectID]
		if p == nil {
			continue
		}
		p.Detail.HasEscalatedCases = true
		p.Detail.DetailedEscalatedCases = append(p.Detail.DetailedEscalatedCases,
			domain.HealthCaseLink{EscalationID: escalationID, Number: number, EscalationNumber: number})
	}
	return rows.Err()
}

func (r *accountRepo) healthDetailDeployments(ctx context.Context, accountID string, byID map[string]*ProjectHealthDetailRow) error {
	rows, err := r.db.Query(ctx, accountHealthDetailDeploymentsSQL, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var projectID, id, name string
		var active bool
		if err := rows.Scan(&projectID, &id, &name, &active); err != nil {
			return err
		}
		p := byID[projectID]
		if p == nil || !active {
			continue
		}
		p.Detail.Deployments = append(p.Detail.Deployments, domain.HealthDeploymentRef{ID: id, Name: name})
	}
	return rows.Err()
}

func (r *accountRepo) healthDetailEol(ctx context.Context, accountID string, byID map[string]*ProjectHealthDetailRow) error {
	rows, err := r.db.Query(ctx, accountHealthDetailEolSQL, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()

	// One entry per product version of a project, in first-seen order; each
	// lists a deployment once however many of its deployed products run it.
	type versionKey struct{ projectID, versionID string }
	type deploymentKey struct {
		versionKey
		deploymentID string
	}
	entryOf := map[versionKey]int{}
	seen := map[deploymentKey]bool{}
	for rows.Next() {
		var projectID, versionID, product, version, deploymentID, deploymentName string
		var eolDate time.Time
		if err := rows.Scan(&projectID, &versionID, &product, &version, &eolDate, &deploymentID, &deploymentName); err != nil {
			return err
		}
		p := byID[projectID]
		if p == nil {
			continue
		}
		d := &p.Detail
		d.IsUsingEolProduct = true
		vk := versionKey{projectID, versionID}
		idx, ok := entryOf[vk]
		if !ok {
			d.SoftwareModel = append(d.SoftwareModel, domain.HealthEolProduct{
				Name:        product + " " + version,
				EolDate:     eolDate.UTC().Format(time.RFC3339),
				Deployments: []domain.HealthDeploymentRef{},
			})
			idx = len(d.SoftwareModel) - 1
			entryOf[vk] = idx
		}
		dk := deploymentKey{vk, deploymentID}
		if !seen[dk] {
			seen[dk] = true
			d.SoftwareModel[idx].Deployments = append(d.SoftwareModel[idx].Deployments,
				domain.HealthDeploymentRef{ID: deploymentID, Name: deploymentName})
		}
	}
	return rows.Err()
}
