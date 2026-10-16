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
	"errors"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/viewer"
)

// entityCustomerHealthClient is the subset of internal/entity.CustomerEntityClient
// the account health detail needs: the one health-detail request.
type entityCustomerHealthClient interface {
	GetAccountHealthDetail(ctx context.Context, accountID string) (entity.CHAccountHealthDetail, error)
}

// postgresCustomerHealthClient implements customerHealthDetailClient entirely
// against entity-service (Postgres). entity-service computes the per-project
// risk flags and the items behind them (recent cases, delayed and abandoned
// migrations, end-of-life software, escalations, go-live status) in one
// request; this client only reshapes the answer into the portal's wire format.
type postgresCustomerHealthClient struct {
	entity entityCustomerHealthClient
}

// NewPostgresCustomerHealthClient builds a postgresCustomerHealthClient.
// entity is the same *entity.CustomerEntityClient every other CS Portal
// handler uses.
func NewPostgresCustomerHealthClient(entity entityCustomerHealthClient) *postgresCustomerHealthClient {
	return &postgresCustomerHealthClient{entity: entity}
}

// GetCustomerHealthDetail implements customerHealthDetailClient with exactly one
// entity-service request (GET /accounts/{id}/health-detail), however many
// projects and cases the account has. A 404 (no such account) becomes
// viewer.ErrAccountNotFound; every other error is returned unchanged for
// the handler's upstream-error mapping. The account lookup is deliberately
// unscoped: a CS engineer may legitimately view a deactivated account's
// health history.
func (c *postgresCustomerHealthClient) GetCustomerHealthDetail(ctx context.Context, accountID string) (*viewer.AccountDetail, error) {
	detail, err := c.entity.GetAccountHealthDetail(ctx, accountID)
	if err != nil {
		var apiErr *apierror.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, viewer.ErrAccountNotFound
		}
		return nil, err
	}
	return accountDetailFromEntity(detail), nil
}

// accountDetailFromEntity maps entity-service's health detail onto the
// portal's AccountDetail. Every id becomes the flat 32-character form (see
// uuidToSysID) and every list is a non-nil slice, so the JSON always carries
// [] rather than null. A group's case list and a link's fields are omitted
// when empty, as before.
func accountDetailFromEntity(d entity.CHAccountHealthDetail) *viewer.AccountDetail {
	projects := make([]viewer.ProjectDetail, 0, len(d.CustomerProjects))
	for _, p := range d.CustomerProjects {
		recent := make([]viewer.CaseGroup, 0, len(p.DetailedRecentCases))
		for _, g := range p.DetailedRecentCases {
			group := viewer.CaseGroup{Priority: g.Priority, Count: g.Count}
			if len(g.Cases) > 0 {
				group.Cases = make([]viewer.CaseInfo, 0, len(g.Cases))
				for _, cs := range g.Cases {
					group.Cases = append(group.Cases, viewer.CaseInfo{SysID: uuidToSysID(cs.ID), Number: cs.Number})
				}
			}
			recent = append(recent, group)
		}
		software := make([]viewer.EolProduct, 0, len(p.SoftwareModel))
		for _, s := range p.SoftwareModel {
			software = append(software, viewer.EolProduct{Name: s.Name, EolDate: s.EolDate, Deployments: deploymentRefsFromEntity(s.Deployments)})
		}
		projects = append(projects, viewer.ProjectDetail{
			SysID:                   uuidToSysID(p.ID),
			Name:                    p.Name,
			HasRecentCases:          p.HasRecentCases,
			DetailedRecentCases:     recent,
			TotalRecentCases:        p.TotalRecentCases,
			HasAbandonedCases:       p.HasAbandonedCases,
			DetailedAbandonedCases:  caseLinksFromEntity(p.DetailedAbandonedCases),
			IsUsingEolProduct:       p.IsUsingEolProduct,
			SoftwareModel:           software,
			Deployments:             deploymentRefsFromEntity(p.Deployments),
			HasMigrationDelays:      p.HasMigrationDelays,
			DetailedMigrationDelays: caseLinksFromEntity(p.DetailedMigrationDelays),
			HasEscalatedCases:       p.HasEscalatedCases,
			DetailedEscalatedCases:  caseLinksFromEntity(p.DetailedEscalatedCases),
			GoLiveStatus:            viewer.GoLiveStatus{Status: p.GoLiveStatus.Status, IsRisk: p.GoLiveStatus.IsRisk, State: p.GoLiveStatus.State},
		})
	}
	return &viewer.AccountDetail{AccountName: d.AccountName, CustomerProjects: projects}
}

func deploymentRefsFromEntity(in []entity.CHHealthDeploymentRef) []viewer.Deployment {
	out := make([]viewer.Deployment, 0, len(in))
	for _, d := range in {
		out = append(out, viewer.Deployment{SysID: uuidToSysID(d.ID), Name: d.Name})
	}
	return out
}

func caseLinksFromEntity(in []entity.CHHealthCaseLink) []viewer.CaseLink {
	out := make([]viewer.CaseLink, 0, len(in))
	for _, l := range in {
		link := viewer.CaseLink{Number: l.Number, EscalationNumber: l.EscalationNumber}
		if l.ID != "" {
			link.SysID = uuidToSysID(l.ID)
		}
		if l.EscalationID != "" {
			link.EscalationSysID = uuidToSysID(l.EscalationID)
		}
		out = append(out, link)
	}
	return out
}

// customerHealthAccountLifeCycle/customerHealthAccountRequiresProject scope
// every customer-health account query to the confirmed, correct rule for
// this feature: account.life_cycle = 'Customer' and the account has at
// least one project.
//
// This supersedes an earlier version of this fix, which guessed "active"
// (account.deactivation_date IS NULL) was the right scope, reasoned from an
// unrelated account search/picker endpoint's own exclusion of accounts
// whose status was "lost". That guess was wrong on two counts, found once
// the correct rule was confirmed and checked directly against staging data:
// (1) the right field is account.life_cycle -- a distinct column from
// account.classification (a business-segment axis: Enterprise/Partner/
// Cloud/OEM) -- not deactivation_date at all; deactivation_date IS NULL
// matched far more accounts than the real endpoint's count, nowhere close.
// (2) the account must also have at least one project -- missing this
// alone still overcounts, since life_cycle = 'Customer' alone still matches
// a much larger set than the real endpoint's count. Confirmed together:
// life_cycle = 'Customer' AND at least one project matches exactly the
// account count the real endpoint reports for the identical unfiltered
// request. Notably the correct rule has NO active/inactive check at all --
// some of those matching accounts are themselves deactivated
// (deactivation_date IS NOT NULL) and must still be included, which is why
// this is no longer combined with an Active filter.
const (
	customerHealthAccountLifeCycle       = "Customer"
	customerHealthAccountRequiresProject = true
)

// parseCustomerHealthRisks splits the comma-separated risks query param into
// the "risks" filter entity-service's POST /accounts/search takes
// ("hasNoGoLive", "hasGoneLive", "hasRecentCases", "noSupportCases6mo",
// "hasEolProduct", "hasAbandonedMigrations", "hasMigrationDelays",
// "hasRecentEscalations"). Blank entries are dropped; an unrecognized key is
// passed through and matches nothing, entity-service's call to make.
func parseCustomerHealthRisks(risks *string) []string {
	if risks == nil {
		return nil
	}
	var out []string
	for _, r := range strings.Split(*risks, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// uuidToSysID strips the dashes out of an entity-service UUID to match the
// flat 32-character sys_id shape every other customer-health response field
// already uses -- internal/risk's own uuidToSysID (private to that package,
// so not reusable directly from here) documents the same convention: the
// webapp compares a project's sysId field-for-field across endpoints (e.g.
// matching GetCustomerHealthDetail's own ProjectDetail.SysID against
// GetAccountHealthStatus's ProjectSysID to show each project's current
// review status), so every project/deployment/case id this file places into
// a response must be in the same flat shape those other endpoints already
// use, or the two will never compare equal.
//
// Found live, a real, confirmed bug: ProjectDetail.SysID (and every other
// sys_id-shaped field constructed in this file) was left as the raw, dashed
// UUID entity-service itself uses -- so a project's health-status/action
// items could never be matched back to its own card in the UI, and marking
// a project healthy/at-risk never appeared to do anything, because the two
// ids being compared never actually matched.
func uuidToSysID(uuid string) string {
	return strings.ReplaceAll(uuid, "-", "")
}
