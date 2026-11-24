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

func init() {
	// ----------------------------------------------------------- service request

	// Main: the catalog offers one category with one item for the deployed product, and the item has one mandatory question.
	reg("service-request", "33-catalog-read", "200 catalog with one item; variables list the mandatory question", func(c *Ctx) {
		c.Do(csm, "POST", "/catalogs/search", M{"deployedProductId": dpGw}).Expect(200)
		c.Do(csm, "POST", "/catalogs/search", M{}).Expect(400)
		c.Do(csm, "GET", "/catalogs/"+catID+"/items/"+catItemID+"/variables", nil).Expect(200)
		c.Do(csm, "GET", "/catalogs/"+catID+"/items/"+nowhereID+"/variables", nil)
		c.Do(cust, "POST", "/catalogs/search", M{"deployedProductId": dpGw}).Expect(200)
	})
	// Main: a service request is created through the case endpoint with a catalog item and answers; missing parts are 400.
	reg("service-request", "34-create-service-request", "201 with answers; 400 without variables/catalog", func(c *Ctx) {
		body := M{"type": "service_request", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw,
			"subject": "Baseline service request", "description": "Fictional request",
			"catalogId": catID, "catalogItemId": catItemID,
			"variables": []M{{"id": catVarID, "value": "Fictional reason"}}}
		r := c.Do(csm, "POST", "/cases", body).Expect(201)
		r.Capture("case.id", "sr1")
		nv := M{"type": "service_request", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw,
			"subject": "x", "description": "x", "catalogId": catID, "catalogItemId": catItemID}
		c.Do(csm, "POST", "/cases", nv).Expect(400)
		nc := M{"type": "service_request", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw, "subject": "x"}
		c.Do(csm, "POST", "/cases", nc).Expect(400)
		bad := M{"type": "service_request", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw,
			"subject": "x", "description": "x", "catalogId": catID, "catalogItemId": nowhereID,
			"variables": []M{{"id": catVarID, "value": "x"}}}
		c.Do(csm, "POST", "/cases", bad)
		c.Do(cust, "POST", "/cases", body)
		c.Do(csm, "GET", "/cases/"+c.V("sr1"), nil).Expect(200)
	})
	// Main: a service request moves open, work_in_progress, closed (with resolution) and reads back closed.
	reg("service-request", "35-service-request-close", "200 each step; closed state and resolved fields read back", func(c *Ctx) {
		patchCase(c, csm, c.V("sr1"), M{"state": "closed"}).Expect(400)
		patchCase(c, csm, c.V("sr1"), M{"state": "work_in_progress"}).Expect(200)
		patchCase(c, csm, c.V("sr1"), M{"state": "closed", "resolutionCode": "SOLVED_FIXED_THE_ISSUE", "cause": "UNKNOWN", "closeNotes": "Done"}).Expect(200)
		c.Do(csm, "GET", "/cases/"+c.V("sr1"), nil).Expect(200)
		c.SQLRecord("sr1 persisted", `SELECT state||'|'||COALESCE(resolution_code::text,'-') FROM service_request WHERE id='`+c.V("sr1")+`'`)
		c.Do(csm, "POST", "/cases/search", M{"filters": M{"filters": []M{{"field": "type", "op": "in", "values": []string{"service_request"}}}}}).Expect(200)
	})
	// Main: engagements and security report analyses are created through the same endpoint and validated per type.
	reg("case-create", "36-create-engagement-and-sra", "201 engagement; 400 missing engagementType; SRA per its own rules", func(c *Ctx) {
		e := M{"type": "engagement", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw,
			"subject": "Baseline engagement", "description": "Fictional", "engagementType": "onboarding", "engagementPaymentType": "foc"}
		c.Do(csm, "POST", "/cases", e).Expect(201)
		e2 := M{"type": "engagement", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw, "subject": "x", "description": "x"}
		c.Do(csm, "POST", "/cases", e2).Expect(400)
		s := M{"type": "security_report_analysis", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw,
			"subject": "Baseline SRA", "description": "Fictional"}
		c.Do(csm, "POST", "/cases", s)
	})

	// ----------------------------------------------------------------- incident

	// Main: an incident is created with a service, impact and urgency; priority is derived; an unknown service is 400.
	reg("incident", "37-create-incident", "201 with derived priority; 400 unknown service/missing fields", func(c *Ctx) {
		r := c.Do(csm, "POST", "/incidents", M{"callerId": uJane, "category": "SERVICE_INTERRUPTION", "serviceId": serviceID,
			"impact": "HIGH", "urgency": "HIGH", "subject": "Baseline incident", "assignedEngineerId": uAlice}).Expect(201)
		r.Capture("incident.id", "inc1")
		r2 := c.Do(csm, "POST", "/incidents", M{"callerId": uJane, "category": "INQUIRY", "serviceId": serviceID,
			"impact": "LOW", "urgency": "LOW", "subject": "Baseline incident to cancel"}).Expect(201)
		r2.Capture("incident.id", "inc2")
		c.Do(csm, "POST", "/incidents", M{"callerId": uJane, "category": "INQUIRY", "serviceId": nowhereID,
			"impact": "LOW", "urgency": "LOW", "subject": "x"}).Expect(400)
		c.Do(csm, "POST", "/incidents", M{"subject": "x"}).Expect(400)
		c.Do(cust, "POST", "/incidents", M{"callerId": uDave, "category": "INQUIRY", "serviceId": serviceID,
			"impact": "LOW", "urgency": "LOW", "subject": "Customer incident"})
		c.Do(csm, "GET", "/incidents/"+c.V("inc1"), nil).Expect(200)
		c.Do(csm, "GET", "/incidents/create-defaults", nil).Expect(200)
	})
	// Main: incident NEW to IN_PROGRESS is accepted; resolve and close demand a resolution code and notes (400 otherwise).
	reg("incident", "38-incident-resolve-and-close", "200 in progress; 400 without code/notes; 200 resolved; 200 closed", func(c *Ctx) {
		id := c.V("inc1")
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "IN_PROGRESS"}).Expect(200)
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "RESOLVED"}).Expect(400)
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "CLOSED"}).Expect(400)
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "RESOLVED", "resolutionCode": "SOLVED_PERMANENTLY", "resolutionNotes": "Fictional fix"}).Expect(200)
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "CLOSED", "resolutionCode": "SOLVED_PERMANENTLY", "resolutionNotes": "Fictional fix"})
		c.Do(csm, "GET", "/incidents/"+id, nil).Expect(200)
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "IN_PROGRESS"})
	})
	// Main: an incident can be cancelled from NEW; a cancelled incident is then moved to IN_PROGRESS without complaint (baseline quirk).
	reg("incident", "39-incident-cancel", "200 cancelled; follow-up move out of cancelled accepted on main", func(c *Ctx) {
		id := c.V("inc2")
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "CANCELLED"}).Expect(200)
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "IN_PROGRESS"})
		c.Do(csm, "PATCH", "/incidents/"+id, M{"state": "BOGUS"}).Expect(400)
		c.Do(csm, "PATCH", "/incidents/"+nowhereID, M{"state": "IN_PROGRESS"}).Expect(404)
	})
	// Main: incident search, aggregate and activity listing answer 200 for an agent.
	reg("incident", "40-incident-search-aggregate", "200 search, aggregate by state, empty activity list", func(c *Ctx) {
		c.Do(csm, "POST", "/incidents/search", M{}).Expect(200)
		c.Do(csm, "POST", "/incidents/search", M{"pagination": M{"limit": 1, "offset": 1}}).Expect(200)
		c.Do(csm, "POST", "/incidents/aggregate", M{"groupBy": "state"}).Expect(200)
		c.Do(csm, "POST", "/incidents/"+c.V("inc1")+"/activities/search", M{}).Expect(200)
		c.Do(cust, "POST", "/incidents/search", M{})
	})

	// ---------------------------------------------------------- change request

	// Main: a normal change is created in state new with a number; a body without type is 400.
	reg("change-request", "41-create-change-request", "201 state new; 400 without type", func(c *Ctx) {
		body := M{"subject": "Baseline change", "type": "normal", "projectId": pEx, "groupId": teamABT,
			"justification": "Fictional", "implementationPlan": "Fictional plan", "backoutPlan": "Fictional",
			"testPlan": "Fictional", "riskImpactAnalysis": "Fictional", "risk": "low", "impact": "low",
			"priority": "low", "category": "software",
			"plannedStartDate": "2027-01-10T10:00:00Z", "plannedEndDate": "2027-01-10T12:00:00Z"}
		r := c.Do(csm, "POST", "/change-requests", body).Expect(201)
		r.Capture("changeRequest.id", "cr1")
		c.Do(csm, "POST", "/change-requests", M{"subject": "x"}).Expect(400)
		c.Do(csm, "GET", "/change-requests/"+c.V("cr1"), nil).Expect(200)
		c.Do(cust, "GET", "/change-requests/"+c.V("cr1"), nil).Expect(404)
		c.Do(csm, "GET", "/change-requests/"+c.V("cr1")+"/approvals", nil).Expect(200)
		c.Do(csm, "GET", "/change-requests/"+nowhereID, nil).Expect(404)
	})
	// Main: requesting approval starts the Peer then CAB stages; the group members are asked; the creator cannot decide.
	reg("change-request", "42-request-approval-and-stages", "200 to assess; approvals list peer stage with two requested approvers", func(c *Ctx) {
		c.Do(csm, "PATCH", "/change-requests/"+c.V("cr1"), M{"state": "closed"}).Expect(400)
		c.Do(csm, "PATCH", "/change-requests/"+c.V("cr1"), M{"state": "assess"}).Expect(200)
		c.Do(csm, "GET", "/change-requests/"+c.V("cr1")+"/approvals", nil).Expect(200).Only(sortApprovers)
		c.Do(csm, "POST", "/change-requests/"+c.V("cr1")+"/approvals/decision", M{"decision": "approved"}).Expect(403)
	})
	// Main: an approver's approval completes the peer stage and opens the CAB stage; the approval list shows both stages.
	reg("change-request", "43-approver-decides", "200 approved; stages list peer approved and CAB pending", func(c *Ctx) {
		c.Do(csmAl, "POST", "/change-requests/"+c.V("cr1")+"/approvals/decision", M{"decision": "approved"}).Expect(200)
		c.Do(csmAl, "GET", "/change-requests/"+c.V("cr1")+"/approvals", nil).Expect(200).Only(sortApprovers)
		c.Do(csmAl, "POST", "/change-requests/"+c.V("cr1")+"/approvals/decision", M{"decision": "bogus"})
		c.Do(csmAl, "GET", "/change-requests/"+c.V("cr1"), nil).Expect(200)
	})
	// Main: change request search, aggregate and link options answer 200.
	reg("change-request", "44-change-request-search", "200 search/aggregate/link options", func(c *Ctx) {
		c.Do(csm, "POST", "/change-requests/search", M{}).Expect(200)
		c.Do(csm, "POST", "/change-requests/aggregate", M{"groupBy": "state"}).Expect(200)
		c.Do(csm, "POST", "/change-requests/link-options", M{})
		c.Do(cust, "POST", "/change-requests/search", M{})
	})

	// ---------------------------------------------------------------- time card

	// Main: a time card is created submitted with its approver; invalid input is 400.
	reg("time-card", "45-create-time-cards", "201 submitted; a card without approvers is also accepted (201); empty body 400", func(c *Ctx) {
		mk := M{"caseId": seedCase1, "projectId": pEx, "date": "2026-11-20", "approverIds": []string{uAlice},
			"timeAnalyzing": 30, "workLogComment": "Fictional work"}
		c.Do(csm, "POST", "/time-cards", mk).Expect(201).Capture("timeCard.id", "tc1")
		c.Do(csm, "POST", "/time-cards", mk).Expect(201).Capture("timeCard.id", "tc2")
		c.Do(csm, "POST", "/time-cards", M{"caseId": seedCase1, "projectId": pEx, "date": "2026-11-20"}).Expect(201)
		c.Do(csm, "POST", "/time-cards", M{}).Expect(400)
		c.Do(cust, "POST", "/time-cards", mk)
	})
	// Main: only an eligible approver other than the submitter may approve; approving twice is 409.
	reg("time-card", "46-approve-time-card", "403 for a non-approver; 200 approve; 409 second approve", func(c *Ctx) {
		c.Do(direct, "PATCH", "/time-cards/"+c.V("tc1"), M{"state": "approved"}).Expect(403)
		c.Do(csmAl, "PATCH", "/time-cards/"+c.V("tc1"), M{"state": "approved", "leadComment": "Looks right"}).Expect(200)
		c.Do(csmAl, "PATCH", "/time-cards/"+c.V("tc1"), M{"state": "approved"}).Expect(409)
	})
	// Main: rejecting needs a comment (400); with one it works; a rejected card can no longer be edited or deleted (409).
	reg("time-card", "47-reject-time-card", "400 without comment; 200 reject; 409 edit/delete after", func(c *Ctx) {
		c.Do(csmAl, "PATCH", "/time-cards/"+c.V("tc2"), M{"state": "rejected"}).Expect(400)
		c.Do(csmAl, "PATCH", "/time-cards/"+c.V("tc2"), M{"state": "rejected", "leadComment": "Needs more detail"}).Expect(200)
		c.Do(csm, "PATCH", "/time-cards/"+c.V("tc2"), M{"workLogComment": "edit after reject"}).Expect(409)
		c.Do(csm, "DELETE", "/time-cards/"+c.V("tc2"), nil).Expect(409)
		c.Do(csm, "PATCH", "/time-cards/"+nowhereID, M{"state": "approved"}).Expect(404)
	})
	// Main: time card search with state filter, aggregate by state and the per-case list answer 200.
	reg("time-card", "48-time-card-search", "200 filters/aggregate/per-case", func(c *Ctx) {
		c.Do(csm, "POST", "/time-cards/search", M{}).Expect(200)
		c.Do(csm, "POST", "/time-cards/search", M{"filters": M{"states": []string{"approved"}}}).Expect(200)
		c.Do(csm, "POST", "/time-cards/aggregate", M{"groupBy": "state"}).Expect(200)
		c.Do(csm, "POST", "/cases/time-cards/search", M{}).Expect(200)
		c.Do(csm, "GET", "/projects/"+pEx+"/time-cards/stats", nil)
	})

	// -------------------------------------------------------------- attachments

	// Main: an upload can be started on an open case (a pending record is created); size, closed case and missing parent are refused.
	reg("attachments", "49-attachment-upload-metadata", "201 pending record; 400 oversize; closed case refused", func(c *Ctx) {
		up := func(parent string, size int64) M {
			return M{"type": "work_item", "parentId": parent, "name": "note.txt", "contentType": "text/plain", "sizeBytes": size}
		}
		c.Do(csm, "POST", "/attachments/upload", up(c.V("case4"), 12)).Expect(201)
		c.Do(cust, "POST", "/attachments/upload", up(c.V("case2"), 12))
		c.Do(csm, "POST", "/attachments/upload", up(c.V("case4"), 99999999999)).Expect(400)
		c.Do(csm, "POST", "/attachments/upload", up(c.V("case1"), 12))
		c.Do(csm, "POST", "/attachments/upload", up(nowhereID, 12))
		c.Do(csm, "POST", "/attachments/upload", M{"type": "bogus"}).Expect(400)
	})
	// Main: attachment metadata search needs a reference type and id; a case with only pending uploads lists none.
	reg("attachments", "50-attachment-search", "400 without referenceType; 200 empty list for confirmed-only listing", func(c *Ctx) {
		c.Do(csm, "POST", "/attachments/search", M{"referenceId": seedCase1}).Expect(400)
		c.Do(csm, "POST", "/attachments/search", M{"referenceId": c.V("case4"), "referenceType": "case"}).Expect(200)
		c.Do(csm, "POST", "/attachments/search", M{}).Expect(400)
		c.Do(cust, "POST", "/attachments/search", M{"referenceId": seedCase3, "referenceType": "case"})
	})
}

// sortApprovers orders each stage's approvers by name: main returns the
// approvers of one stage in no defined order (they are created in the same
// instant), which differs from run to run.
func sortApprovers(body any) any {
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	stages, _ := m["approvals"].([]any)
	for _, s := range stages {
		if sm, ok := s.(map[string]any); ok {
			if arr, ok := sm["approvers"].([]any); ok {
				sortRows(arr, []string{"name"})
			}
		}
	}
	return body
}
