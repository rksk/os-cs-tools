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

package repository_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// A change request's tags: the same work_item_tag rows, read by the same
// helper the case detail uses, written by the same CaseRepository.AddCaseTag /
// RemoveCaseTag (which take a work item id). Same harness as the customer-group
// tests (newCustomerGroupFlow, DSN-gated by CHANGE_REQUEST_TEST_DSN); run as a
// superuser and as csm_app like the rest of the suite.
//
// The migrated shape is what the sync leaves behind from the previous system:
// a CHG-numbered change request written by 'sn-sync', its stage unlabeled with
// UPPER_SNAKE states, and tag / work_item_tag rows whose ids are sys_id-derived
// (the 32-hex sys_id with hyphens inserted), created_by 'sn-sync' / 'system',
// nothing else set -- work_item_tag.sn_sys_id (migration 0135) set on some rows
// and NULL on others. Nothing here depends on anything only a portal-written tag
// row has.

// Tag rows the sync would write. The uuids are the sys_ids they derive from.
const (
	migratedTagSysID1   = "9c2e4b1f0a7d4e3b8c6f1a2b3c4d5e61"
	migratedTagSysID2   = "9c2e4b1f0a7d4e3b8c6f1a2b3c4d5e62"
	migratedEntrySysID1 = "7b1d3a9e5c2f4a6b8d0e2f4a6c8e0a71"
	migratedEntrySysID2 = "7b1d3a9e5c2f4a6b8d0e2f4a6c8e0a72"
)

// sysIDUUID is the sync's id for a sys_id: the 32 hex digits as a uuid.
func sysIDUUID(sysID string) string {
	return sysID[0:8] + "-" + sysID[8:12] + "-" + sysID[12:16] + "-" + sysID[16:20] + "-" + sysID[20:32]
}

// tagLabels is the tags' labels, sorted case-insensitively: the read orders by
// name, and the database's collation decides where an upper-case label falls.
func tagLabels(tags []domain.Tag) string {
	labels := make([]string, 0, len(tags))
	for _, t := range tags {
		labels = append(labels, t.Label)
	}
	sort.Slice(labels, func(i, j int) bool { return strings.ToLower(labels[i]) < strings.ToLower(labels[j]) })
	return strings.Join(labels, ",")
}

// tagID is the id of the tag labelled label among tags, "" when absent.
func tagID(tags []domain.Tag, label string) string {
	for _, t := range tags {
		if t.Label == label {
			return t.ID
		}
	}
	return ""
}

// seedMigratedTag attaches a tag the sync migrated to the change request: a tag
// row and a work_item_tag row with sys_id-derived ids, 'sn-sync' metadata and
// (when entrySysID is given) the attachment's sn_sys_id. The rows go with the
// change request (ON DELETE CASCADE on work_item_tag); the tag row is removed by
// the flow's cleanup below.
func (f *crFlow) seedMigratedTag(id, tagSysID, label string, entrySysID *string) string {
	f.t.Helper()
	tagID := sysIDUUID(tagSysID)
	f.execSQL(`INSERT INTO tag (id, created_on, updated_on, created_by, updated_by, name)
	           VALUES ($1::uuid, now() - interval '400 days', now() - interval '400 days', 'sn-sync', 'system', $2)
	           ON CONFLICT (id) DO NOTHING`, tagID, label)
	f.t.Cleanup(func() { _, _ = f.scoped.Exec(f.sys, `DELETE FROM tag WHERE id = $1::uuid`, tagID) })
	var entryID any
	if entrySysID != nil {
		entryID = sysIDUUID(*entrySysID)
	} else {
		entryID = sysIDUUID(strings.Repeat("0", 8) + tagSysID[8:])
	}
	f.execSQL(`INSERT INTO work_item_tag (id, created_on, updated_on, created_by, updated_by, work_item_id, tag_id, sn_sys_id)
	           VALUES ($1::uuid, now() - interval '300 days', now() - interval '300 days', 'sn-sync', 'sn-sync', $2, $3::uuid, $4)`,
		entryID, id, tagID, entrySysID)
	return tagID
}

// migratedChangeRequestWithTags is a change request the way the sync leaves one
// (migratedInCustomerApproval: an unlabeled stage asking Alice and Bob with
// UPPER_SNAKE states, the previous system's customer group, our boxes false),
// CHG-numbered and written by the sync, carrying two migrated tags: one whose
// attachment has the previous system's label_entry sys_id, one without.
func (f *crFlow) migratedChangeRequestWithTags() (id, taggedWithSysID, taggedWithoutSysID string) {
	f.t.Helper()
	id = f.migratedInCustomerApproval()
	f.execSQL(`UPDATE work_item SET number = 'CHG0041007', created_by = 'sn-sync', updated_by = 'sn-sync' WHERE id = $1`, id)
	entry := migratedEntrySysID1
	taggedWithSysID = f.seedMigratedTag(id, migratedTagSysID1, "Migrated-Window", &entry)
	taggedWithoutSysID = f.seedMigratedTag(id, migratedTagSysID2, "legacy platform", nil)
	return id, taggedWithSysID, taggedWithoutSysID
}

func (f *crFlow) tagsOf(id string) []domain.Tag {
	f.t.Helper()
	return f.get(id).Tags
}

// Native: a change request created here carries no tags (an empty array, not
// null), the case tag path attaches and detaches tags on it, and the detail
// reads them back the way the case detail does (by label).
func TestChangeRequestTagsIntegration_DetailCarriesTagsAddedThroughTheCasePath(t *testing.T) {
	f := newCustomerGroupFlow(t)
	vis := visStrictSinceLongAgo()
	f.useVisibility(vis)
	cases := repository.NewCaseRepository(f.scoped, vis)
	id := f.createWithProject(domain.ChangeRequestTypeNormal, sp(crScopeProjectA), true, false)
	staff := crFlowEmail(crFlowCreatorID)

	if tags := f.tagsOf(id); tags == nil || len(tags) != 0 {
		t.Fatalf("a new change request's tags = %#v, want an empty array", tags)
	}

	second, err := cases.AddCaseTag(f.sys, id, "zebra-rollout", staff)
	if err != nil {
		t.Fatalf("AddCaseTag on a change request: %v", err)
	}
	first, err := cases.AddCaseTag(f.sys, id, "apim-upgrade", staff)
	if err != nil {
		t.Fatalf("AddCaseTag on a change request: %v", err)
	}
	// Idempotent by label, case-insensitively, like a case's.
	again, err := cases.AddCaseTag(f.sys, id, "APIM-Upgrade", staff)
	if err != nil || again.ID != first.ID {
		t.Fatalf("a repeat AddCaseTag = %+v, %v; want the existing tag %s", again, err, first.ID)
	}
	tags := f.tagsOf(id)
	if got := tagLabels(tags); got != "apim-upgrade,zebra-rollout" {
		t.Fatalf("detail tags = %q, want apim-upgrade,zebra-rollout", got)
	}
	// Ordered by name, like the case read, with the tag's own id.
	if tags[0].ID != first.ID || tags[1].ID != second.ID {
		t.Fatalf("detail tag ids = %v / %v, want %s / %s (by name)", tags[0].ID, tags[1].ID, first.ID, second.ID)
	}

	if err := cases.RemoveCaseTag(f.sys, id, second.ID, staff); err != nil {
		t.Fatalf("RemoveCaseTag on a change request: %v", err)
	}
	if got := tagLabels(f.tagsOf(id)); got != "apim-upgrade" {
		t.Fatalf("detail tags after the removal = %q, want apim-upgrade", got)
	}
	err = cases.RemoveCaseTag(f.sys, id, second.ID, staff)
	f.wantNotFound("removing a tag that is no longer on the change request", err)

	// The PATCH receipt is the same read.
	receipt, err := f.patch(id, domain.PatchChangeRequestRequest{Justification: sp("tagged")})
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	if got := tagLabels(receipt.Tags); got != "apim-upgrade" {
		t.Fatalf("the PATCH receipt's tags = %q, want apim-upgrade", got)
	}
}

// Migrated: the detail returns the tags the sync attached (sys_id-derived ids,
// 'sn-sync' metadata, sn_sys_id set or NULL), a native add lands next to them,
// and a migrated tag is removed by the same path, whatever its sn_sys_id.
func TestChangeRequestTagsIntegration_MigratedTagsAreReadAddedAlongsideAndRemoved(t *testing.T) {
	f := newCustomerGroupFlow(t)
	vis := visStrictSinceLongAgo()
	f.useVisibility(vis)
	cases := repository.NewCaseRepository(f.scoped, vis)
	id, withSysID, withoutSysID := f.migratedChangeRequestWithTags()
	staff := crFlowEmail(crFlowCreatorID)

	cr := f.get(id)
	if cr.Number != "CHG0041007" || cr.State == nil || *cr.State != "customer_approval" {
		t.Fatalf("the migrated change request reads %q in %v; the fixture is not the migrated shape", cr.Number, cr.State)
	}
	if got := tagLabels(cr.Tags); got != "legacy platform,Migrated-Window" {
		t.Fatalf("migrated tags = %q, want legacy platform,Migrated-Window", got)
	}
	if tagID(cr.Tags, "legacy platform") != withoutSysID || tagID(cr.Tags, "Migrated-Window") != withSysID {
		t.Fatalf("migrated tag ids = %+v, want the sync's %s / %s", cr.Tags, withoutSysID, withSysID)
	}

	// A native add next to the migrated ones; the migrated label is found by
	// name, not re-created.
	native, err := cases.AddCaseTag(f.sys, id, "hotfix", staff)
	if err != nil {
		t.Fatalf("AddCaseTag beside migrated tags: %v", err)
	}
	same, err := cases.AddCaseTag(f.sys, id, "migrated-window", staff)
	if err != nil || same.ID != withSysID {
		t.Fatalf("AddCaseTag of a migrated label = %+v, %v; want the sync's tag %s", same, err, withSysID)
	}
	if got := tagLabels(f.tagsOf(id)); got != "hotfix,legacy platform,Migrated-Window" {
		t.Fatalf("tags after the native add = %q", got)
	}
	var created string
	if err := f.scoped.QueryRow(f.sys, `SELECT created_by FROM work_item_tag WHERE work_item_id = $1 AND tag_id = $2::uuid`, id, native.ID).Scan(&created); err != nil || created != staff {
		t.Fatalf("the native attachment's created_by = %q, %v; want %s", created, err, staff)
	}
	var migratedRows int
	if err := f.scoped.QueryRow(f.sys, `SELECT COUNT(*) FROM work_item_tag WHERE work_item_id = $1 AND created_by = 'sn-sync'`, id).Scan(&migratedRows); err != nil || migratedRows != 2 {
		t.Fatalf("migrated attachments after the native add = %d, %v; want the 2 untouched", migratedRows, err)
	}

	// Removing a migrated tag: with the previous system's sys_id, and without.
	for _, tagID := range []string{withSysID, withoutSysID} {
		if err := cases.RemoveCaseTag(f.sys, id, tagID, staff); err != nil {
			t.Fatalf("RemoveCaseTag of migrated tag %s: %v", tagID, err)
		}
	}
	if got := tagLabels(f.tagsOf(id)); got != "hotfix" {
		t.Fatalf("tags after removing the migrated ones = %q, want hotfix", got)
	}
	// The tag rows themselves stay (the vocabulary is shared, a removal only
	// detaches), exactly as for a case.
	var tagRows int
	if err := f.scoped.QueryRow(f.sys, `SELECT COUNT(*) FROM tag WHERE id IN ($1::uuid, $2::uuid)`, withSysID, withoutSysID).Scan(&tagRows); err != nil || tagRows != 2 {
		t.Fatalf("migrated tag rows after detaching = %d, %v; want both kept", tagRows, err)
	}
}

// Customers never see tags: a designated contact reads the change request (it is
// visible to them) with tags null, a contact who was never asked cannot reach a
// hidden change request's tags at all (404, the same as every other case path), and
// the migrated shape behaves the same.
func TestChangeRequestTagsIntegration_CustomersNeverSeeOrReachTags(t *testing.T) {
	f := newCustomerGroupFlow(t)
	vis := visStrictSinceLongAgo()
	f.useVisibility(vis)
	cases := repository.NewCaseRepository(f.scoped, vis)
	staff := crFlowEmail(crFlowCreatorID)

	t.Run("native", func(t *testing.T) {
		id := f.createWithProject(domain.ChangeRequestTypeNormal, sp(crScopeProjectA), true, false)
		tag, err := cases.AddCaseTag(f.sys, id, "internal-only", staff)
		if err != nil {
			t.Fatalf("AddCaseTag: %v", err)
		}
		alice, sam := asContact(crScopeUserA1), asContact(crScopeUserSecurity)

		// In New nobody was asked: hidden, so the tag calls are 404 for Alice too.
		_, err = cases.AddCaseTag(alice, id, "a-tag", crFlowEmail(crScopeUserA1))
		f.wantNotFound("AddCaseTag on a hidden change request", err)
		f.wantNotFound("RemoveCaseTag on a hidden change request", cases.RemoveCaseTag(alice, id, tag.ID, crFlowEmail(crScopeUserA1)))

		f.driveToCustomerApproval(id)
		// Designated: Alice reads the change request, without its tags.
		cr, err := f.getAsContact(id, crScopeUserA1)
		if err != nil {
			t.Fatalf("GET as a designated contact: %v", err)
		}
		if cr.Tags != nil {
			t.Fatalf("a customer's read carries tags %v; want null", cr.Tags)
		}
		// Sam was never asked: still hidden, still 404 on the tag path.
		_, err = cases.AddCaseTag(sam, id, "a-tag", crFlowEmail(crScopeUserSecurity))
		f.wantNotFound("AddCaseTag as a contact who was never asked", err)
		f.wantNotFound("RemoveCaseTag as a contact who was never asked", cases.RemoveCaseTag(sam, id, tag.ID, crFlowEmail(crScopeUserSecurity)))
		// Staff still read them.
		if got := tagLabels(f.tagsOf(id)); got != "internal-only" {
			t.Fatalf("staff read = %q, want internal-only", got)
		}
	})

	t.Run("migrated", func(t *testing.T) {
		// A migrated change request predates the cutover: it is a LEGACY row (its
		// unlabeled stage designates nobody), visible to its project's registered
		// contacts in Customer Approval -- so no cutover here, as in production
		// for these rows.
		legacy := repository.CRVisibility{}
		f.useVisibility(legacy)
		cases := repository.NewCaseRepository(f.scoped, legacy)
		id, withSysID, _ := f.migratedChangeRequestWithTags()
		// Alice (asked by the sync's stage) and Sam (registered, never asked) both
		// read it, neither with its tags.
		for _, u := range []string{crScopeUserA1, crScopeUserSecurity} {
			cr, err := f.getAsContact(id, u)
			if err != nil {
				t.Fatalf("GET of a migrated change request as %s: %v", u, err)
			}
			if cr.Tags != nil {
				t.Fatalf("a customer's read of a migrated change request carries tags %v; want null", cr.Tags)
			}
		}
		// Carol is a registered contact of project B: the change request does not
		// exist for her, tags included.
		carol := asContact(crScopeUserB1)
		_, err := cases.AddCaseTag(carol, id, "a-tag", crFlowEmail(crScopeUserB1))
		f.wantNotFound("AddCaseTag on a migrated change request hidden from the caller", err)
		f.wantNotFound("RemoveCaseTag on a migrated change request hidden from the caller", cases.RemoveCaseTag(carol, id, withSysID, crFlowEmail(crScopeUserB1)))
		if got := tagLabels(f.tagsOf(id)); got != "legacy platform,Migrated-Window" {
			t.Fatalf("the migrated tags after the refused calls = %q", got)
		}
	})
}
