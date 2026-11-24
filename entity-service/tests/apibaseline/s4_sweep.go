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

// Route sweep: every parameter-free GET and every list/aggregate POST in the
// published contract that the focused scenarios above do not already call, sent
// once as an internal agent with an empty body. The sweep records the status
// and the response shape of each; it is breadth, not depth. The lists were
// generated from the contract of origin/main, so routes that exist only on a
// newer branch are not in them (they are listed as not covered in the report).

var sweepGets = []string{
	"/banners",
	"/carbon-kernel-releases",
	"/carbon-platform-history",
	"/case-escalation-access/deescalators",
	"/instances/daily-usages/imports",
	"/instances/unresolved-products",
	"/knowledge-bases",
	"/outages/metadata",
	"/plg/analytics/dashboard",
	"/plg/lifecycle",
	"/plg/playbooks",
	"/plg/products",
	"/portal-banners",
	"/portal-banners/manage",
	"/products/vulnerabilities/meta",
	"/release-lines",
	"/reports/long-open-cases",
	"/reports/migration-cases",
	"/reports/mttr",
	"/reports/query-hours",
	"/runbooks",
	"/scheduled-tasks/attempts",
	"/skills/categories",
	"/skills/levels",
	"/skills/me",
	"/skills/me/history",
	"/specialist-handoff-teams",
	"/team-schedule/case-alerts/settings",
	"/team-schedule/case-alerts/settings/runtime",
	"/team-schedule/catalogue",
	"/team-schedule/members",
	"/team-schedule/my-lead-teams",
	"/team-schedule/on-duty",
	"/team-schedule/paging-chain",
	"/team-schedule/paging-contacts",
	"/team-schedule/paging-readiness",
	"/team-schedule/rota-access",
	"/teams/chat-webhooks",
	"/teams/group-emails",
	"/users/me/dashboards",
	"/users/me/shared-dashboards",
}

var sweepPosts = []string{
	"/accounts/by-health-status/search",
	"/accounts/health-summaries/search",
	"/announcement-requests/search",
	"/call-requests/search",
	"/cases/feedback/aggregate",
	"/cases/feedback/search",
	"/comments/search",
	"/configuration-items/search",
	"/conversations/search",
	"/customer-engagements/allocations/search",
	"/deployed-products/projects/search",
	"/event-publish-failures/search",
	"/incident-tasks/aggregate",
	"/instances/metrics/search",
	"/instances/metrics/stats/search",
	"/instances/search",
	"/instances/usages/search",
	"/instances/usages/stats/search",
	"/invoices/search",
	"/kb-articles/search",
	"/kb-manager-groups/search",
	"/kb-manager-users/search",
	"/onboarding-steps/search",
	"/opportunities/search",
	"/outages/search",
	"/pending-verifications/search",
	"/plg/organizations/aggregate",
	"/plg/organizations/search",
	"/plg/registrations/aggregate",
	"/plg/registrations/search",
	"/plg/users/search",
	"/plg/work-queue/search",
	"/problems/aggregate",
	"/problems/search",
	"/product-versions/search",
	"/products/vulnerabilities/search",
	"/project-opportunity-links/search",
	"/search",
	"/service-offerings/search",
	"/services/search",
	"/skills/search",
	"/skills/users/search",
	"/tags/search",
	"/tasks/search",
	"/team-schedule/absences/search",
	"/team-schedule/assignments/search",
	"/team-schedule/engagement-allocations/search",
	"/team-schedule/holiday-availability/search",
	"/team-schedule/public-holidays/search",
}

func init() {
	// Main: parameter-free reads answer 200 (or a defined 4xx when the feature needs configuration this database lacks).
	reg("route-sweep", "67-get-sweep", "each GET answers with a stable status and shape", func(c *Ctx) {
		for _, path := range sweepGets {
			c.Do(csm, "GET", path, nil)
		}
	})
	// Main: list and aggregate endpoints answer 200 with an empty body filter, or 400 naming the missing field.
	reg("route-sweep", "68-search-sweep", "each POST list/aggregate answers with a stable status and shape", func(c *Ctx) {
		for _, path := range sweepPosts {
			c.Do(csm, "POST", path, M{})
		}
	})
}
