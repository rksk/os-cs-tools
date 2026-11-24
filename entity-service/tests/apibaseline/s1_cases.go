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

// Case scenarios. The comment above each registration is the one-line
// statement of what origin/main is expected to do (the baseline); the same
// statement is written into every result so a reader of a diff sees it.

// M is a JSON object literal.
type M = map[string]any

const (
	csmClient  = "baseline-csm-backend"
	custClient = "baseline-customer-backend"
	m2mClient  = "baseline-m2m"

	pEx        = "00000000-0000-0000-0000-000000000401"
	pOther     = "00000000-0000-0000-0000-000000000402"
	depProd    = "00000000-0000-0000-0000-000000000501"
	depOther   = "00000000-0000-0000-0000-000000000541"
	dpGw       = "00000000-0000-0000-0000-000000000531"
	dpOther    = "00000000-0000-0000-0000-000000000534"
	uJane      = "00000000-0000-0000-0000-000000000001"
	uAlice     = "00000000-0000-0000-0000-000000000011"
	uBob       = "00000000-0000-0000-0000-000000000012"
	uDormant   = "00000000-0000-0000-0000-000000000031"
	uDave      = "00000000-0000-0000-0000-000000000021"
	seedCase1  = "00000000-0000-0000-0000-000000000601"
	seedCase3  = "00000000-0000-0000-0000-000000000603"
	teamABT    = "00000000-0000-0000-0000-000000000901"
	catID      = "00000000-0000-0000-0000-000000001601"
	catItemID  = "00000000-0000-0000-0000-000000001602"
	catVarID   = "00000000-0000-0000-0000-000000001605"
	serviceID  = "00000000-0000-0000-0000-0000000000aa"
	nowhereID  = "00000000-0000-0000-0000-00000000dead"
	seedTCard1 = "00000000-0000-0000-0000-000000000801"
)

var (
	csm     = Actor{Name: "csm-agent", Client: csmClient, Email: "jane.doe@example.com"}
	csmAl   = Actor{Name: "csm-alice", Client: csmClient, Email: "alice.perera@example.com"}
	cust    = Actor{Name: "customer-dave", Client: custClient, Email: "dave.mendis@example.net"}
	custOth = Actor{Name: "customer-sam", Client: custClient, Email: "sam.other@othercorp.example"}
	direct  = Actor{Name: "internal-direct-bob", Email: "bob.fernando@example.com"}
	m2m     = Actor{Name: "m2m-client", Client: m2mClient}
	anon    = Actor{Name: "anonymous"}
	unknown = Actor{Name: "unknown-user", Client: custClient, Email: "nobody@example.net"}
	badTok  = Actor{Name: "garbage-token", Raw: map[string]string{"x-user-id-token": "not-a-token"}}
)

func newCaseBody(severity, subject string) M {
	return M{"type": "case", "projectId": pEx, "deploymentId": depProd, "deployedProductId": dpGw,
		"subject": subject, "description": "Fictional description for " + subject,
		"severity": severity, "issueType": "error"}
}

func patchCase(c *Ctx, a Actor, id string, body M) *Resp {
	return c.Do(a, "PATCH", "/cases/"+id, body)
}

func init() {
	// ---------------------------------------------------------------- creation

	// Main: an agent creates a case and gets 201 with number, internal id and state "open".
	reg("case-create", "01-create-case-as-agent", "201 with id/number/internalId and state open", func(c *Ctx) {
		r := c.Do(csm, "POST", "/cases", newCaseBody("medium", "Baseline case one")).Expect(201)
		r.Capture("case.id", "case1")
	})
	// Main: a registered customer creates a case on their own project the same way.
	reg("case-create", "02-create-case-as-customer", "201, createdBy is the customer", func(c *Ctx) {
		r := c.Do(cust, "POST", "/cases", newCaseBody("low", "Baseline customer case")).Expect(201)
		r.Capture("case.id", "case2")
	})
	// Main: a critical case gets 201 and its response and resolution clocks registered by the SLA policies in the seed.
	reg("case-create", "03-create-critical-case", "201, S1 clocks registered", func(c *Ctx) {
		r := c.Do(csm, "POST", "/cases", newCaseBody("critical", "Baseline critical case")).Expect(201)
		r.Capture("case.id", "case4")
	})
	// Main: invalid create bodies answer 400 with a plain message, a customer of another project is refused.
	reg("case-create", "04-create-case-validation", "400 for each malformed body; cross-project customer 4xx", func(c *Ctx) {
		c.Do(csm, "POST", "/cases", M{}).Expect(400)
		b := newCaseBody("medium", "x")
		delete(b, "description")
		c.Do(csm, "POST", "/cases", b).Expect(400)
		c.Do(csm, "POST", "/cases", newCaseBody("bogus", "x")).Expect(400)
		o := newCaseBody("medium", "x")
		o["deployedProductId"] = dpOther
		c.Do(csm, "POST", "/cases", o)
		c.Do(csm, "POST", "/cases", M{"type": "bogus", "projectId": pEx}).Expect(400)
		c.Do(custOth, "POST", "/cases", newCaseBody("low", "Not my project"))
		c.Do(csm, "POST", "/cases", "{not json").Expect(400)
	})

	// -------------------------------------------------------------------- reads

	// Main: GET by id returns the full case view for an agent.
	reg("case-read", "05-get-case-as-agent", "200 full case view", func(c *Ctx) {
		c.Do(csm, "GET", "/cases/"+c.V("case1"), nil).Expect(200)
	})
	// Main: a customer reads their own case but gets 404 (not 403) for another project's case.
	reg("case-read", "06-get-case-customer-scope", "200 own case, 404 for other project", func(c *Ctx) {
		c.Do(cust, "GET", "/cases/"+c.V("case2"), nil).Expect(200)
		c.Do(cust, "GET", "/cases/"+seedCase3, nil).Expect(404)
		c.Do(custOth, "GET", "/cases/"+c.V("case2"), nil).Expect(404)
		c.Do(cust, "GET", "/cases/"+c.V("case1"), nil).Expect(200)
	})
	// Main: error shapes for reads: 404 unknown id, 400 malformed id, 401 without identity, 401 on a forged token, 403 for an unknown user.
	reg("errors", "07-get-case-error-shapes", "404/400/401/401/403 bodies carry code and message", func(c *Ctx) {
		c.Do(csm, "GET", "/cases/"+nowhereID, nil).Expect(404)
		c.Do(csm, "GET", "/cases/not-a-uuid", nil).Expect(400)
		c.Do(anon, "GET", "/cases/"+seedCase1, nil).Expect(401)
		c.Do(badTok, "GET", "/cases/"+seedCase1, nil).Expect(401)
		c.Do(unknown, "GET", "/cases/"+seedCase1, nil).Expect(403)
		c.Do(m2m, "GET", "/cases/"+seedCase1, nil).Expect(200)
		c.Do(direct, "GET", "/cases/"+seedCase1, nil).Expect(200)
	})

	// ----------------------------------------------------------------- comments

	// Main: public comments and internal work notes are accepted from an agent with 201.
	reg("case-comments", "08-comment-public-and-internal-note", "201 for comment and work_note", func(c *Ctx) {
		r := c.Do(csm, "POST", "/cases/"+c.V("case2")+"/comments", M{"type": "comment", "content": "Public comment from the agent"}).Expect(201)
		r.Capture("comment.id", "comment1")
		c.Do(csm, "POST", "/cases/"+c.V("case2")+"/comments", M{"type": "work_note", "content": "Internal note from the agent"}).Expect(201)
	})
	// Main: a customer posting a work note is NOT rejected with 403: the row-level policy fails and the API answers 500.
	reg("case-comments", "09-comment-customer-work-note", "500 internal server error (RLS violation surfaces as 500)", func(c *Ctx) {
		c.Do(cust, "POST", "/cases/"+c.V("case2")+"/comments", M{"type": "work_note", "content": "Customer tries an internal note"}).Expect(500)
		c.Do(cust, "POST", "/cases/"+c.V("case2")+"/comments", M{"type": "comment", "content": "Customer public comment"}).Expect(201)
	})
	// Main: the agent sees every comment, the customer only the public ones.
	reg("case-comments", "10-comments-search-visibility", "agent sees 3 (incl. note), customer sees public ones only", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/"+c.V("case2")+"/comments/search", M{}).Expect(200)
		c.Do(cust, "POST", "/cases/"+c.V("case2")+"/comments/search", M{}).Expect(200)
		c.Do(cust, "POST", "/cases/"+c.V("case2")+"/comments/search", M{"pagination": M{"limit": 1, "offset": 1}}).Expect(200)
	})
	// Main: comment validation answers 400; reading one comment by id and its history works.
	reg("case-comments", "11-comment-validation-and-read", "400 for empty content/bad type; 200 get by id and history", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/"+c.V("case2")+"/comments", M{"type": "comment", "content": ""}).Expect(400)
		c.Do(csm, "POST", "/cases/"+c.V("case2")+"/comments", M{"type": "bogus", "content": "x"}).Expect(400)
		c.Do(csm, "POST", "/cases/"+nowhereID+"/comments", M{"type": "comment", "content": "x"})
		c.Do(csm, "GET", "/comments/"+c.V("comment1"), nil)
		c.Do(csm, "GET", "/comments/"+c.V("comment1")+"/history", nil)
	})

	// ------------------------------------------------------------ state machine

	// Main: open to closed is refused with errorCode case_invalid_state_transition.
	reg("case-state", "12-state-illegal-open-to-closed", "400 case_invalid_state_transition", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"state": "closed"}).Expect(400)
	})
	// Main: the working path open, work_in_progress, awaiting_info, waiting_on_wso2, work_in_progress is accepted step by step.
	reg("case-state", "13-state-working-path", "200 for each legal move", func(c *Ctx) {
		for _, s := range []string{"work_in_progress", "awaiting_info", "waiting_on_wso2", "work_in_progress"} {
			patchCase(c, csm, c.V("case1"), M{"state": s}).Expect(200)
		}
	})
	// Main: unknown state value, a move back to open, and a move to reopened from work_in_progress are all 400.
	reg("case-state", "14-state-illegal-moves", "400 for bogus value, wip->open, wip->reopened", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"state": "bogus"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "open"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "reopened"}).Expect(400)
	})
	// Main: patch validation: empty body is 400; two exclusive fields together are 400.
	reg("case-state", "15-patch-validation", "400 empty body; 400 conflicting fields", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "awaiting_info", "assigneeEmail": "alice.perera@example.com"})
		patchCase(c, csm, nowhereID, M{"severity": "high"}).Expect(404)
		patchCase(c, anon, c.V("case1"), M{"severity": "high"}).Expect(403) // main: 403 here although GET answers 401
	})
	// Main: an internal user can be assigned; unknown and inactive emails are refused.
	reg("case-assign", "16-assign-case", "200 for an internal active user; error for unknown/inactive", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"assigneeEmail": "alice.perera@example.com"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"assigneeEmail": "nobody@example.net"})
		patchCase(c, csm, c.V("case1"), M{"assigneeEmail": "dormant.user@example.com"})
		patchCase(c, csm, c.V("case1"), M{"assigneeEmail": "dave.mendis@example.net"})
		patchCase(c, cust, c.V("case2"), M{"assigneeEmail": "alice.perera@example.com"})
		c.Do(csm, "GET", "/cases/"+c.V("case1"), nil)
	})
	// Main: severity can be changed by an agent; an invalid value is 400; repeating the same value stays 200.
	reg("case-severity", "17-severity-change", "200 change, 400 invalid, 200 repeat of same value", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"severity": "high"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"severity": "high"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"severity": "bogus"}).Expect(400)
		patchCase(c, csm, c.V("case4"), M{"severity": "catastrophic"})
		c.Do(csm, "GET", "/sla-status", nil).Canon("statuses", "wso2CaseId", "clockType")
	})
	// Main: acknowledging twice reports alreadyAcknowledged on the second call.
	reg("case-ack", "18-acknowledge-twice", "200 both; second says alreadyAcknowledged", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"acknowledge": true}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"acknowledge": true}).Expect(200)
	})
	// Main: work state ongoing/paused and watch list updates are accepted.
	reg("case-workstate", "19-work-state-and-watchers", "200 ongoing, paused; watch list replaced", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"workState": "ongoing"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"workState": "paused"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"workState": "bogus"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"watchList": []string{"alice.perera@example.com"}}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"watchList": []string{uAlice, uBob}}).Expect(200)
		c.Do(csm, "GET", "/cases/"+c.V("case1"), nil)
	})

	// -------------------------------------------------------------- escalation

	// Main: escalation needs a reason; with one it moves the level 0 to 1 and is listed.
	reg("case-escalation", "20-escalate-case", "400 without reason; 201 with; listed afterwards", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/"+c.V("case4")+"/escalations", M{}).Expect(400)
		r := c.Do(csm, "POST", "/cases/"+c.V("case4")+"/escalations", M{"action": "ESCALATE", "reason": "Fictional escalation reason"}).Expect(201)
		r.Capture("id", "esc1")
		c.Do(csm, "GET", "/cases/"+c.V("case4")+"/escalations", nil).Expect(200)
		c.Do(csm, "POST", "/escalations/search", M{}).Expect(200)
		c.Do(csm, "GET", "/cases/"+c.V("case4"), nil).Expect(200)
	})
	// Main: de-escalation on a never-escalated case is 409; a customer's escalate request is accepted (201).
	reg("case-escalation", "21-deescalate-and-customer-escalate", "409 nothing to de-escalate; customer escalate 201", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/"+c.V("case1")+"/escalations", M{"action": "DEESCALATE", "reason": "Fictional"}).Expect(409)
		c.Do(csm, "POST", "/cases/"+c.V("case4")+"/escalations", M{"action": "DEESCALATE", "reason": "Fictional"})
		c.Do(cust, "POST", "/cases/"+c.V("case2")+"/escalations", M{"action": "ESCALATE", "reason": "Customer asks"})
	})

	// -------------------------------------------------------------- resolution

	// Main: a solution is proposed with resolution code, cause and notes; the case view shows them.
	reg("case-resolution", "22-solution-proposed-with-resolution", "200; resolvedOn set; resolution fields readable", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"state": "solution_proposed", "resolutionCode": "SOLVED_FIXED_BY_SUPPORT_GUIDANCE_PROVIDED",
			"cause": "USER_ERROR_CONFIGURATION", "closeNotes": "Fictional close notes"}).Expect(200)
		c.Do(csm, "GET", "/cases/"+c.V("case1"), nil).Expect(200)
	})
	// Main: from solution_proposed the moves to waiting_on_wso2 are accepted; open and reopened are refused; waiting_on_wso2 to solution_proposed is refused.
	reg("case-resolution", "23-solution-proposed-exits", "400 to open/reopened; 200 to waiting_on_wso2; 400 waiting->solution_proposed; 200 via wip", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"state": "reopened"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "open"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "waiting_on_wso2"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"state": "solution_proposed"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "work_in_progress"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"state": "solution_proposed"}).Expect(200)
	})
	// Main: closing with resolution code, cause and close notes returns 200; the closed case reads back with closedOn and the notes.
	reg("case-resolution", "24-close-with-resolution", "200; closedOn, resolutionCode, cause, resolutionNotes populated", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"state": "closed", "resolutionCode": "SOLVED_BY_CUSTOMER",
			"cause": "UNKNOWN", "closeNotes": "Closed in the baseline"}).Expect(200)
		c.Do(csm, "GET", "/cases/"+c.V("case1"), nil).Expect(200)
		c.SQLRecord("case1 persisted resolution", `SELECT state||'|'||COALESCE(resolution_code::text,'-')||'|'||COALESCE(cause::text,'-') FROM "case" WHERE id='`+c.V("case1")+`'`)
	})
	// Main: a second close is accepted (closed to closed) and changes nothing visible.
	reg("idempotency", "25-close-twice", "200 both times", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"state": "closed"}).Expect(200)
		patchCase(c, csm, c.V("case1"), M{"state": "closed"}).Expect(200)
	})
	// Main: a closed case cannot go back to reopened without adminReopen, and an admin reopen needs a reason.
	reg("case-resolution", "26-reopen-closed-case", "400 plain reopen; 400 reason without adminReopen; admin reopen 200", func(c *Ctx) {
		patchCase(c, csm, c.V("case1"), M{"state": "reopened"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "reopened", "reopenReason": "Fictional"}).Expect(400)
		patchCase(c, csm, c.V("case1"), M{"state": "reopened", "adminReopen": true, "reopenReason": "Fictional admin reopen"})
		patchCase(c, csm, c.V("case1"), M{"state": "work_in_progress"})
		c.Do(csm, "GET", "/cases/"+c.V("case1"), nil)
	})
	// Main: a case can be closed with NO resolution code (200), and the code reads back null.
	reg("case-resolution", "27-close-without-resolution-code", "200 accepted; resolutionCode null", func(c *Ctx) {
		r := c.Quiet(csm, "POST", "/cases", newCaseBody("low", "Baseline case three"))
		r.Capture("case.id", "case3")
		patchCase(c, csm, c.V("case3"), M{"state": "work_in_progress"}).Expect(200)
		patchCase(c, csm, c.V("case3"), M{"state": "closed"}).Expect(200)
		c.Do(csm, "GET", "/cases/"+c.V("case3"), nil).Expect(200)
	})
	// Main: resolution fields on a non-closing state and an invalid code are 400.
	reg("case-resolution", "28-resolution-validation", "400 for code on wip; 400 for unknown code", func(c *Ctx) {
		r := c.Quiet(csm, "POST", "/cases", newCaseBody("low", "Baseline case validation"))
		id := r.Capture("case.id", "case6")
		patchCase(c, csm, id, M{"state": "work_in_progress"}).Expect(200)
		patchCase(c, csm, id, M{"state": "work_in_progress", "resolutionCode": "SOLVED_BY_CUSTOMER"}).Expect(400)
		patchCase(c, csm, id, M{"state": "closed", "resolutionCode": "BOGUS"}).Expect(400)
		patchCase(c, csm, id, M{"state": "closed", "cause": "BOGUS"}).Expect(400)
	})
	// Main: a customer may move their own open case straight to closed (200) although the same move is 400 for an agent.
	reg("case-state", "29-customer-closes-own-case", "200 for the customer (agent gets 400 for the same move); another project's customer 404", func(c *Ctx) {
		patchCase(c, custOth, c.V("case2"), M{"state": "closed"}).Expect(404)
		patchCase(c, cust, c.V("case2"), M{"severity": "medium"}).Expect(200)
		patchCase(c, cust, c.V("case2"), M{"state": "closed"}).Expect(200)
		c.Do(cust, "GET", "/cases/"+c.V("case2"), nil)
	})

	// ------------------------------------------------------------- auto-closure

	// Main: a solution_proposed case that waited past the interval is returned for the first reminder; stage and role rules apply.
	reg("autoclosure", "30-autoclosure-candidates", "200 candidate listed for first_reminder; 400 bad stage; customer 403; m2m 200", func(c *Ctx) {
		r := c.Quiet(csm, "POST", "/cases", newCaseBody("medium", "Baseline autoclosure case"))
		id := r.Capture("case.id", "case5")
		patchCase(c, csm, id, M{"state": "work_in_progress"}).Expect(200)
		patchCase(c, csm, id, M{"state": "solution_proposed"}).Expect(200)
		c.SQL(`UPDATE "case" SET autoclose_enabled = true, resolved_on = now() - interval '30 days' WHERE id = '` + id + `'`)
		c.Do(csm, "POST", "/autoclosure/candidates", M{"stage": "first_reminder", "intervalDays": 7}).Expect(200)
		c.Do(csm, "POST", "/autoclosure/candidates", M{"stage": "close"}).Expect(200)
		c.Do(csm, "POST", "/autoclosure/candidates", M{"stage": "release_hold"}).Expect(200)
		c.Do(csm, "POST", "/autoclosure/candidates", M{"stage": "bogus"}).Expect(400)
		c.Do(csm, "POST", "/autoclosure/candidates", M{}).Expect(400)
		c.Do(cust, "POST", "/autoclosure/candidates", M{"stage": "close"}).Expect(403)
		c.Do(m2m, "POST", "/autoclosure/candidates", M{"stage": "first_reminder", "intervalDays": 7}).Expect(200)
	})

	// --------------------------------------------------------------------- SLA

	// Main: the S1 case has response and resolution clocks, readable through the clock list and by id.
	reg("case-sla", "31-sla-clocks-on-case", "200; one response and one resolution clock for the case", func(c *Ctx) {
		// Main: GET /sla-status has no per-case filter; it lists every active clock, tied rows in no defined order.
		c.Do(csm, "GET", "/sla-status", nil).Expect(200).Canon("statuses", "wso2CaseId", "clockType")
		c.Do(csm, "GET", "/sla-status/clock-state?workItemId="+c.V("case4")+"&target=resolution", nil)
		r := c.Do(csm, "POST", "/slas/search", M{"pagination": M{"limit": 50}}).Expect(200).Canon("slas", "task.name", "slaDefinition.target")
		if id := r.Str("slas.0.id"); id != "" {
			c.Do(csm, "GET", "/slas/"+id, nil).Expect(200)
		}
		c.Do(csm, "GET", "/slas/"+nowhereID, nil).Expect(404)
		c.Do(cust, "GET", "/sla-status", nil).Canon("statuses", "wso2CaseId", "clockType")
		c.Do(csm, "GET", "/sla-status/clock-state?workItemId="+c.V("case4")+"&target=response", nil)
	})
	// Main: the audit trail of a case lists its comments and changes, newest first.
	reg("case-activity", "32-case-activities", "200 list with comment and state entries", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/"+c.V("case1")+"/activities/search", M{}).Expect(200)
		c.Do(csm, "POST", "/cases/"+c.V("case2")+"/activities/search", M{}).Expect(200)
		c.Do(cust, "POST", "/cases/"+c.V("case2")+"/activities/search", M{}).Expect(200)
	})
}
