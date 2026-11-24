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
	// Main: a free-text tag is attached to a case (201, same tag returned when repeated), listed globally and removed (204).
	reg("case-tags", "69-case-tags", "201 attach, 201 repeat returns same tag, 400 empty label, 204 detach", func(c *Ctx) {
		r := c.Do(csm, "POST", "/cases/"+c.V("case4")+"/tags", M{"label": "baseline-tag"}).Expect(201)
		r.Capture("id", "tag1")
		c.Do(csm, "POST", "/cases/"+c.V("case4")+"/tags", M{"label": "baseline-tag"}).Expect(201)
		c.Do(csm, "POST", "/cases/"+c.V("case4")+"/tags", M{"label": ""}).Expect(400)
		c.Do(csm, "GET", "/tags/search", nil).Expect(200)
		c.Do(csm, "POST", "/cases/search", M{"filters": M{"filters": []M{{"field": "tag", "op": "in", "values": []string{"baseline-tag"}}}}})
		c.Do(csm, "DELETE", "/cases/"+c.V("case4")+"/tags/"+c.V("tag1"), nil).Expect(204)
		c.Do(csm, "DELETE", "/cases/"+c.V("case4")+"/tags/"+c.V("tag1"), nil)
	})
	// Main: call requests are created for a case by an agent or the project's customer; a case id is required for the list.
	reg("call-request", "70-call-requests", "201 for agent and customer; list needs caseId (400); list returns them", func(c *Ctx) {
		body := func(id string) M {
			return M{"caseId": id, "reason": "Fictional reason", "utcTimes": []string{"2027-02-01T10:00:00Z"}, "durationInMinutes": 30}
		}
		c.Do(csm, "POST", "/call-requests", body(c.V("case4"))).Expect(201)
		c.Do(cust, "POST", "/call-requests", body(seedCase1)).Expect(201)
		c.Do(custOth, "POST", "/call-requests", body(seedCase1))
		c.Do(csm, "POST", "/call-requests", M{"caseId": c.V("case4")}).Expect(400)
		c.Do(csm, "POST", "/call-requests/search", M{}).Expect(400)
		c.Do(csm, "POST", "/call-requests/search", M{"caseId": c.V("case4")}).Expect(200)
	})
	// Main: a conversation is started on a project by an agent or a customer; the list shows both.
	reg("conversation", "71-conversations", "201 for agent and customer; search lists them; 400 without message", func(c *Ctx) {
		b := M{"projectId": pEx, "initialMessage": "Fictional question"}
		c.Do(csm, "POST", "/conversations", b).Expect(201).Capture("conversation.id", "conv1")
		c.Do(cust, "POST", "/conversations", b).Expect(201)
		c.Do(csm, "POST", "/conversations", M{"projectId": pEx}).Expect(400)
		c.Do(csm, "POST", "/conversations/search", M{}).Expect(200)
		c.Do(csm, "GET", "/conversations/"+c.V("conv1"), nil)
	})
	// Main: feedback on an open case is refused with 409; reading it before any is submitted is 404.
	reg("case-feedback", "72-case-feedback", "409 submit on open case; 404 read with none submitted", func(c *Ctx) {
		c.Do(csm, "POST", "/cases/"+c.V("case4")+"/feedback", M{"emojiId": nowhereID}).Expect(409)
		c.Do(csm, "GET", "/cases/"+c.V("case4")+"/feedback", nil).Expect(404)
	})
	// Main: a deployed product is added to a deployment (201) and shows up in the project's deployments stats.
	reg("deployment", "73-create-deployed-product", "201; deployment stats count changes", func(c *Ctx) {
		b := M{"projectId": pEx, "deploymentId": depProd, "productId": "00000000-0000-0000-0000-000000000512",
			"versionId": "00000000-0000-0000-0000-000000000523", "cores": 2}
		c.Do(csm, "POST", "/deployed-products", b).Expect(201)
		c.Do(csm, "POST", "/deployed-products", M{"projectId": pEx}).Expect(400)
		c.Do(csm, "POST", "/deployed-products/search", M{}).Expect(200)
		c.Do(csm, "GET", "/projects/"+pEx+"/stats", nil).Expect(200)
		c.Do(csm, "POST", "/projects/"+pEx+"/contacts/search", M{}).Expect(200)
		c.Do(csm, "POST", "/accounts/00000000-0000-0000-0000-000000000301/contacts/search", M{}).Expect(200)
	})
}
