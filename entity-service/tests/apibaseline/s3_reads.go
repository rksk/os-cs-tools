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

import "time"

func init() {
	// -------------------------------------------------------- search and lists

	// Main: case search with no filter is newest first and pages with limit/offset; countOnly returns only the total.
	reg("case-search", "51-case-search-pagination", "200; limit/offset window; countOnly empty list with total", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/search", M{}).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"pagination": M{"limit": 2, "offset": 0}}).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"pagination": M{"limit": 2, "offset": 2}}).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"pagination": M{"limit": 2, "offset": 500}}).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"countOnly": true}).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"pagination": M{"limit": -1}})
		c.Do(csm, "POST", "/cases/search", M{"pagination": M{"limit": 100000}})
	})
	// Main: filters by state, severity, type and free text narrow the list; sorting by severity ascending is honoured.
	reg("case-search", "52-case-search-filters-and-sort", "200 filtered lists; 400 for unknown field or sort", func(c *Ctx) {
		f := func(field, op string, vals ...string) M {
			return M{"filters": M{"filters": []M{{"field": field, "op": op, "values": vals}}}}
		}
		c.Do(csm, "POST", "/cases/search", f("state", "in", "closed")).Expect(200)
		c.Do(csm, "POST", "/cases/search", f("severity", "in", "critical", "medium")).Expect(200)
		c.Do(csm, "POST", "/cases/search", f("type", "in", "case")).Expect(200)
		c.Do(csm, "POST", "/cases/search", f("projectId", "in", pOther)).Expect(200)
		c.Do(csm, "POST", "/cases/search", f("state", "notIn", "closed")).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"filters": M{"searchQuery": "Baseline"}}).Expect(200)
		// Main: tied severities come back in no defined order, so only the severity sequence is compared.
		c.Do(csm, "POST", "/cases/search", M{"sortBy": M{"field": "severity", "order": "asc"}, "pagination": M{"limit": 50}}).Expect(200).Only(severitySequence)
		c.Do(csm, "POST", "/cases/search", M{"sortBy": M{"field": "createdOn", "order": "asc"}, "pagination": M{"limit": 3}}).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"sortBy": M{"field": "bogus"}}).Expect(400)
		c.Do(csm, "POST", "/cases/search", f("bogus", "eq", "x")).Expect(400)
		c.Do(csm, "POST", "/cases/search", M{"filters": M{"filters": []M{{"field": "state", "operator": "in"}}}}).Expect(400)
	})
	// Main: a customer's search is scoped to their registered project; the other project's customer sees different rows; m2m sees all.
	reg("case-search", "53-case-search-scope-by-caller", "customer lists only own project; sam only other; m2m all", func(c *Ctx) {
		c.Do(cust, "POST", "/cases/search", M{}).Expect(200)
		c.Do(custOth, "POST", "/cases/search", M{}).Expect(200)
		c.Do(m2m, "POST", "/cases/search", M{}).Expect(200)
		c.Do(direct, "POST", "/cases/search", M{"countOnly": true}).Expect(200)
		c.Do(anon, "POST", "/cases/search", M{}).Expect(401)
	})
	// Main: aggregation by state and by severity, and the 30 day volume trend answer 200.
	reg("case-search", "54-case-aggregate-and-trend", "200 grouped counts; trend has a 30 point series", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/aggregate", M{"groupBy": "state"}).Expect(200)
		c.Do(csm, "POST", "/cases/aggregate", M{"groupBy": "severity"}).Expect(200)
		c.Do(csm, "POST", "/cases/aggregate", M{"groupBy": "bogus"})
		c.Do(csm, "POST", "/cases/volume-trend", M{}).Expect(200)
		c.Do(cust, "POST", "/cases/aggregate", M{"groupBy": "state"})
	})

	// ------------------------------------------------- project and deployment reads

	// Main: project reads for an agent: by id, metadata, stats and case stats.
	reg("project-read", "55-project-reads", "200 for each; counts reflect the cases made by this suite", func(c *Ctx) {
		c.Do(csm, "GET", "/projects/"+pEx, nil).Expect(200)
		c.Do(csm, "GET", "/projects/"+pEx+"/metadata", nil).Expect(200)
		c.Do(csm, "GET", "/projects/"+pEx+"/stats", nil).Expect(200)
		c.Do(csm, "GET", "/projects/"+pEx+"/cases/stats", nil).Expect(200)
		c.Do(csm, "GET", "/projects/"+pEx+"/deployments/stats", nil).Expect(200)
		c.Do(csm, "GET", "/projects/"+nowhereID, nil)
	})
	// Main: project search and project access for customers: own project readable, other project not.
	reg("project-read", "56-project-search-and-access", "200 lists; customer gets own project, 404 for the other", func(c *Ctx) {
		c.Do(csm, "POST", "/projects/search", M{}).Expect(200)
		c.Do(cust, "POST", "/projects/search", M{}).Expect(200)
		c.Do(cust, "GET", "/projects/"+pEx, nil).Expect(200)
		c.Do(cust, "GET", "/projects/"+pOther, nil)
		c.Do(csm, "POST", "/projects/aggregate", M{"groupBy": "state"})
	})
	// Main: deployments, deployed products and products are listed and readable.
	reg("project-read", "57-deployments-and-products", "200 lists with seeded rows", func(c *Ctx) {
		c.Do(csm, "POST", "/deployments/search", M{}).Expect(200)
		c.Do(cust, "POST", "/deployments/search", M{}).Expect(200)
		c.Do(csm, "POST", "/deployed-products/search", M{}).Expect(200)
		c.Do(csm, "POST", "/products/search", M{}).Expect(200)
		c.Do(csm, "GET", "/products/00000000-0000-0000-0000-000000000511", nil)
		c.Do(csm, "POST", "/products/00000000-0000-0000-0000-000000000511/versions/search", M{})
		c.Do(csm, "POST", "/accounts/search", M{}).Expect(200)
		c.Do(csm, "GET", "/accounts/00000000-0000-0000-0000-000000000301", nil).Expect(200)
	})
	// Main: reference data: metadata, teams, team members, groups.
	reg("reference", "58-reference-data", "200 metadata/teams/members/groups", func(c *Ctx) {
		c.Do(csm, "GET", "/metadata", nil).Expect(200)
		c.Do(csm, "POST", "/teams/search", M{}).Expect(200)
		c.Do(csm, "GET", "/teams/"+teamABT+"/members", nil).Expect(200)
		c.Do(csm, "POST", "/groups/search", M{}).Expect(200)
		c.Do(csm, "GET", "/sla-duration-policy", nil).Expect(200)
	})

	// ---------------------------------------------------------- users and roles

	// Main: /users/me reports the caller's type, roles and groups for each kind of caller.
	reg("users", "59-users-me-by-caller", "200 per caller with userType and roles; unknown 403; anonymous 401", func(c *Ctx) {
		for _, a := range []Actor{csm, csmAl, cust, custOth, direct} {
			c.Do(a, "GET", "/users/me", nil).Expect(200)
		}
		c.Do(unknown, "GET", "/users/me", nil)
		c.Do(anon, "GET", "/users/me", nil).Expect(401)
		c.Do(m2m, "GET", "/users/me", nil)
	})
	// Main: lookup by id, by id list and search by e-mail, free text and active flag.
	reg("users", "60-user-lookup-and-search", "200; inactive user found with active=false; 404 for unknown id", func(c *Ctx) {
		c.Do(csm, "GET", "/users/"+uAlice, nil).Expect(200)
		c.Do(csm, "GET", "/users/"+uDormant, nil).Expect(200)
		c.Do(csm, "GET", "/users/"+nowhereID, nil).Expect(404)
		c.Do(csm, "POST", "/users/by-ids", M{"ids": []string{uAlice, uBob}}).Expect(200)
		c.Do(csm, "POST", "/users/search", M{"filters": M{"emails": []string{"alice.perera@example.com"}}}).Expect(200)
		c.Do(csm, "POST", "/users/search", M{"filters": M{"searchQuery": "fern"}}).Expect(200)
		c.Do(csm, "POST", "/users/search", M{"filters": M{"active": false}}).Expect(200)
		c.Do(csm, "POST", "/users/search", M{"pagination": M{"limit": 3, "offset": 0}}).Expect(200)
		c.Do(cust, "POST", "/users/search", M{"filters": M{"emails": []string{"alice.perera@example.com"}}})
	})
	// Main: creating a user validates its fields, and a customer user is created with the customer role; a repeat is refused.
	reg("users", "61-user-create-and-update", "400 validation; 201 customer user; repeat refused; phone update 200", func(c *Ctx) {
		c.Do(csm, "POST", "/users", M{"firstName": "x"}).Expect(400)
		c.Do(csm, "POST", "/users", M{"email": "new.person@example.net"}).Expect(400)
		r := c.Do(csm, "POST", "/users", M{"email": "new.person@example.net", "firstName": "New", "lastName": "Person", "roles": []string{"customer"}})
		r.Capture("id", "newuser")
		c.Do(csm, "POST", "/users", M{"email": "new.person@example.net", "firstName": "New", "lastName": "Person", "roles": []string{"customer"}})
		c.Do(csm, "POST", "/users", M{"email": "staff.person@example.net", "firstName": "Staff", "lastName": "Person", "roles": []string{"internal"}})
		c.Do(csm, "PATCH", "/users/"+uAlice, M{"phone": "+15550100000"}).Expect(200)
		c.Do(csm, "GET", "/users/"+uAlice, nil).Expect(200)
		c.Do(cust, "PATCH", "/users/"+uAlice, M{"phone": "+15550100001"})
	})

	// -------------------------------------------------------------- error shapes

	// Main: wrong method and unknown route have their own plain-text bodies; a body with an unknown field is 400.
	reg("errors", "62-transport-error-shapes", "405 Method Not Allowed; 404 page not found; 400 unknown field; 400 malformed JSON", func(c *Ctx) {
		c.Do(csm, "DELETE", "/cases/"+seedCase1, nil).Expect(405)
		c.Do(csm, "GET", "/nonexistent", nil).Expect(404)
		c.Do(csm, "POST", "/cases/search", M{"bogusField": 1}).Expect(400)
		c.Do(csm, "POST", "/cases/search", "[1,2").Expect(400)
		c.Do(csm, "POST", "/cases/search", "").Expect(400)
		c.Do(csm, "GET", "/health", nil)
	})
	// Main: 409 appears for conflicting state in time cards and de-escalation; 403 for a creator approving own change.
	reg("errors", "63-conflict-and-forbidden-bodies", "409 and 403 bodies carry code and message (errorCode where defined)", func(c *Ctx) {
		c.Do(csmAl, "PATCH", "/time-cards/"+c.V("tc1"), M{"state": "rejected", "leadComment": "late"}).Expect(409)
		c.Do(csm, "POST", "/change-requests/"+c.V("cr1")+"/approvals/decision", M{"decision": "approved"}).Expect(403)
		c.Do(csm, "POST", "/cases/"+c.V("case3")+"/escalations", M{"action": "DEESCALATE", "reason": "x"}).Expect(409)
	})

	// ------------------------------------------------------------- idempotency

	// Main: repeated reads and repeated no-op patches return the same body; repeated creates make distinct records.
	reg("idempotency", "64-repeat-reads-patches-creates", "GET twice identical; same-severity PATCH twice 200; two creates give two cases", func(c *Ctx) {
		c.Do(csm, "GET", "/cases/"+c.V("case4"), nil).Expect(200)
		c.Do(csm, "GET", "/cases/"+c.V("case4"), nil).Expect(200)
		patchCase(c, csm, c.V("case4"), M{"severity": "catastrophic"}).Expect(200)
		patchCase(c, csm, c.V("case4"), M{"severity": "catastrophic"}).Expect(200)
		a := c.Do(csm, "POST", "/cases", newCaseBody("low", "Repeat create")).Expect(201)
		b := c.Do(csm, "POST", "/cases", newCaseBody("low", "Repeat create")).Expect(201)
		a.Capture("case.id", "dupA")
		b.Capture("case.id", "dupB")
		c.Do(csm, "POST", "/cases/search", M{"filters": M{"searchQuery": "Repeat create"}}).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"filters": M{"searchQuery": "Repeat create"}}).Expect(200)
	})
	// Main: the persisted state after the whole run: counts per table read straight from the database.
	reg("idempotency", "65-final-database-counts", "row counts match the records created by the steps above", func(c *Ctx) {
		c.SQLRecord("work items by type (without incident tasks, which a background job creates)", `SELECT string_agg(type::text||'='||n, ',' ORDER BY type::text) FROM (SELECT type, count(*) AS n FROM work_item WHERE type <> 'INCIDENT_TASK' GROUP BY type) t`)
		c.SQLRecord("cases by state", `SELECT string_agg(state::text||'='||n, ',' ORDER BY state::text) FROM (SELECT state, count(*) AS n FROM "case" GROUP BY state) t`)
		c.SQLRecord("comments by type", `SELECT string_agg(type::text||'='||n, ',' ORDER BY type::text) FROM (SELECT type, count(*) AS n FROM comment GROUP BY type) t`)
		c.SQLRecord("time cards by state", `SELECT string_agg(state::text||'='||n, ',' ORDER BY state::text) FROM (SELECT state, count(*) AS n FROM time_card GROUP BY state) t`)
		c.SQLRecord("sla clocks", `SELECT count(*) FROM sla`)
	})
	// Main: a background job turns each incident into follow-up tasks within seconds; after waiting for it the count is stable.
	reg("idempotency", "66-incident-report-tasks-settle", "incident tasks created by the background job: 3 after the run", func(c *Ctx) {
		var n string
		for i := 0; i < 40; i++ {
			n = c.SQL(`SELECT count(*) FROM work_item WHERE type = 'INCIDENT_TASK'`)
			if n != "0" {
				time.Sleep(2 * time.Second)
				n = c.SQL(`SELECT count(*) FROM work_item WHERE type = 'INCIDENT_TASK'`)
				break
			}
			time.Sleep(time.Second)
		}
		c.SQLRecord("incident tasks", `SELECT count(*) FROM work_item WHERE type = 'INCIDENT_TASK'`)
		c.Do(csm, "POST", "/incident-tasks/search", M{}).Expect(200)
	})
}

// severitySequence reduces a case search body to its severities in order.
func severitySequence(body any) any {
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	var out []any
	rows, _ := m["cases"].([]any)
	for _, r := range rows {
		out = append(out, lookup(r, "severity"))
	}
	return M{"severities": out, "total": m["total"]}
}
