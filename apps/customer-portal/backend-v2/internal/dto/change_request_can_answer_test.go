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

package dto

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
)

func detailsJSON(t *testing.T, cr entity.ChangeRequest) map[string]any {
	t.Helper()
	raw, err := json.Marshal(MapChangeRequestDetails(cr))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return got
}

// customerCanAnswer is entity-service's answer for the signed-in customer and
// reaches the portal unchanged, in all three states: not computed (absent, which
// is NOT false), false, true.
func TestMapChangeRequestDetails_PassesCustomerCanAnswerThrough(t *testing.T) {
	yes, no := true, false
	for name, tc := range map[string]struct {
		in      *bool
		present bool
		want    bool
	}{
		"not computed": {nil, false, false},
		"false":        {&no, true, false},
		"true":         {&yes, true, true},
	} {
		got := detailsJSON(t, entity.ChangeRequest{CustomerCanAnswer: tc.in})
		v, present := got["customerCanAnswer"]
		if present != tc.present {
			t.Errorf("%s: customerCanAnswer present = %v, want %v (%v)", name, present, tc.present, got)
			continue
		}
		if present && v != tc.want {
			t.Errorf("%s: customerCanAnswer = %v, want %v", name, v, tc.want)
		}
	}
}

// isOnHold is whether WSO2 holds the change (so the portal can turn Propose New
// Time off); the reason is WSO2's note and never reaches the customer, and an
// absent flag stays absent (unknown is not "not held").
func TestMapChangeRequestDetails_ExposesIsOnHoldButNotTheReason(t *testing.T) {
	var held, free entity.ChangeRequest
	if err := json.Unmarshal([]byte(`{"id":"cr-1","state":"customer_approval","onHold":true,"onHoldReason":"waiting for the freeze to end"}`), &held); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"id":"cr-2","state":"customer_approval","onHold":false}`), &free); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := detailsJSON(t, held)
	if got["isOnHold"] != true {
		t.Errorf("isOnHold = %v, want true", got["isOnHold"])
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "freeze") || strings.Contains(string(raw), "onHoldReason") {
		t.Errorf("the hold's reason reached the customer: %s", raw)
	}
	if got := detailsJSON(t, free); got["isOnHold"] != false {
		t.Errorf("isOnHold = %v, want false", got["isOnHold"])
	}
	if _, present := detailsJSON(t, entity.ChangeRequest{})["isOnHold"]; present {
		t.Error("isOnHold is present when entity-service did not say")
	}
}

// entity-service's detail carries things a customer must not see (the contacts of
// the customer group, who legalNextStates lets staff move the change to, ...), and
// the new field must not widen what the customer's detail exposes beyond itself:
// the whole key set is pinned, so a field added to the DTO -- the approvals, an
// approver's identity -- has to be added here on purpose.
func TestMapChangeRequestDetails_ExposesOnlyTheCustomerFields(t *testing.T) {
	const fromEntityService = `{
		"id": "cr-1", "number": "CHG001", "subject": "s", "description": "d",
		"project": {"id": "p", "name": "P"}, "case": {"id": "c", "name": "C"},
		"plannedStartOn": "2030-03-01T09:00:00Z", "plannedEndOn": "2030-03-01T11:00:00Z",
		"impact": "low", "state": "customer_approval", "type": "normal",
		"createdOn": "2026-10-06T00:00:00Z", "updatedOn": "2026-10-06T00:00:00Z",
		"createdBy": "wso2.engineer@example.com",
		"justification": "j", "impactDescription": "i", "serviceOutage": "o",
		"communicationPlan": "c", "rollbackPlan": "r", "testPlan": "t",
		"hasCustomerApproved": false, "hasCustomerReviewed": false,
		"approvedBy": {"id": "u", "name": "U"}, "approvedOn": "2026-10-05T00:00:00Z",
		"legalNextStates": ["canceled"],
		"customerCanAnswer": true,
		"customerContacts": [{"id": "u1", "name": "Alice", "email": "alice@example.com"}],
		"approvals": [{"stage": "CAB Approval", "approvers": [{"id": "u2", "name": "Cab Member", "status": "APPROVED", "canDecide": false}]}],
		"customerApprovalRequired": true, "customerReviewRequired": false,
		"requestedBy": {"id": "u3", "name": "Requester"},
		"tags": [{"id": "t1", "label": "internal-only", "color": null}]
	}`
	var cr entity.ChangeRequest
	if err := json.Unmarshal([]byte(fromEntityService), &cr); err != nil {
		t.Fatalf("decode the entity-service detail: %v", err)
	}
	if cr.CustomerCanAnswer == nil || !*cr.CustomerCanAnswer {
		t.Fatalf("the entity type dropped customerCanAnswer: %v", cr.CustomerCanAnswer)
	}

	got := detailsJSON(t, cr)
	var keys []string
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{
		"approvedBy", "approvedOn", "case", "createdBy", "createdOn", "customerCanAnswer", "communicationPlan",
		"customerApprovalRequired", "customerReviewRequired",
		"description", "endDate", "hasCustomerApproved", "hasCustomerReviewed", "id", "impact", "impactDescription",
		"justification", "number", "project", "rollbackPlan", "serviceOutage", "startDate", "state", "testPlan",
		"title", "type", "updatedOn",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("the customer detail's keys changed:\n  got:  %v\n  want: %v", keys, want)
	}
	// tags are staff bookkeeping (the CSM portal's), never a customer's to see:
	// the entity type has no field for them, so the key cannot reach the customer.
	// customerApprovalRequired / customerReviewRequired are no longer in this list:
	// they reach a customer's read, but only when entity-service computed it for a
	// customer (see the requirement-box tests below).
	for _, internal := range []string{"customerContacts", "approvals", "approvers", "legalNextStates", "requestedBy", "canDecide", "tags"} {
		if _, present := got[internal]; present {
			t.Errorf("the customer detail exposes %q", internal)
		}
	}
}

// The two requirement boxes (does the change need the customer's approval /
// review) reach the customer only with a read entity-service computed for a
// customer on the PostgreSQL data source -- the read that carries
// customerCanAnswer -- and then exactly as sent, true or false. Without that
// signal they stay out even when the payload carries them: the previous
// system's data source sends them as false without computing them, and an
// unknown requirement must never arrive as "not required".
func TestMapChangeRequestDetails_PassesTheRequirementBoxesOnlyWithACustomerRead(t *testing.T) {
	yes, no := true, false
	for name, tc := range map[string]struct {
		canAnswer        *bool
		approval, review *bool
		wantPresent      bool
	}{
		"customer read, both ticked":          {&yes, &yes, &yes, true},
		"customer read, neither ticked":       {&no, &no, &no, true},
		"customer read, approval only":        {&no, &yes, &no, true},
		"not a customer read, boxes sent":     {nil, &yes, &no, false},
		"not a customer read, boxes as false": {nil, &no, &no, false},
		"not a customer read, boxes absent":   {nil, nil, nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			got := detailsJSON(t, entity.ChangeRequest{
				CustomerCanAnswer:        tc.canAnswer,
				CustomerApprovalRequired: tc.approval,
				CustomerReviewRequired:   tc.review,
			})
			for key, sent := range map[string]*bool{"customerApprovalRequired": tc.approval, "customerReviewRequired": tc.review} {
				v, present := got[key]
				if !tc.wantPresent {
					if present {
						t.Errorf("%s = %v, want it absent", key, v)
					}
					continue
				}
				if sent == nil {
					// A customer read always carries both (entity-service's type has
					// them as plain booleans); a nil here only means the fake sent none.
					if present {
						t.Errorf("%s = %v, want it absent when entity-service sent none", key, v)
					}
					continue
				}
				if !present || v != *sent {
					t.Errorf("%s = %v (present %v), want %v", key, v, present, *sent)
				}
			}
		})
	}
}

// The boxes are decoded from entity-service's detail as it sends them (plain
// booleans) and reach the customer under the same names and values.
func TestMapChangeRequestDetails_DecodesTheRequirementBoxesFromTheEntityDetail(t *testing.T) {
	var cr entity.ChangeRequest
	if err := json.Unmarshal([]byte(`{"id":"cr-1","state":"closed","customerCanAnswer":false,
		"customerApprovalRequired":true,"customerReviewRequired":false}`), &cr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := detailsJSON(t, cr)
	if got["customerApprovalRequired"] != true || got["customerReviewRequired"] != false {
		t.Fatalf("boxes = %v / %v, want true / false", got["customerApprovalRequired"], got["customerReviewRequired"])
	}
}
