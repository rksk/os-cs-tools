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

package domain

import (
	"strings"
	"testing"
)

// abtTeamRegistryFixture is a representative registry in the configured wire
// form: two classified teams, one unclassified (2-field) team, and a
// hyphenated display name. Every name here is an invented placeholder -- real
// team names are organisation vocabulary and never appear in this repo.
const abtTeamRegistryFixture = "alpha|Alpha Team|CRE,beta|Beta SRE Group|SRE,delta|Delta-Two|cre,gamma|Gamma Team"

// resetAbtRegistry clears the package-level registry so the calling test starts
// clean, and registers a cleanup that clears it again on exit. Without the
// cleanup the last test to run leaves its teams in place, which any later test
// in this package would silently inherit -- making results order-dependent.
func resetAbtRegistry(t *testing.T) {
	t.Helper()
	clearAbtRegistry()
	t.Cleanup(clearAbtRegistry)
}

func clearAbtRegistry() {
	abtMu.Lock()
	defer abtMu.Unlock()
	abtTeams = nil
}

// mustSetRegistry parses raw and installs it, failing the test on a parse error.
func mustSetRegistry(t *testing.T, raw string) {
	t.Helper()
	teams, err := ParseAbtTeamRegistry(raw)
	if err != nil {
		t.Fatalf("ParseAbtTeamRegistry(%q) returned error: %v", raw, err)
	}
	SetAbtTeams(teams)
}

// TestAbtRegistry_StartsClean deliberately does NOT call resetAbtRegistry: it asserts that
// whatever ran before it left the package-level registry empty. It is the test that makes
// resetAbtRegistry's cleanup load-bearing -- drop the cleanup and this fails under
// -shuffle=on whenever it is not scheduled first.
func TestAbtRegistry_StartsClean(t *testing.T) {
	abtMu.RLock()
	teams := abtTeams
	abtMu.RUnlock()

	if teams != nil {
		t.Fatalf("registry not clean on entry: %d teams; an earlier test leaked state", len(teams))
	}
}

func TestParseAbtTeamRegistry_PopulatesCorrectly(t *testing.T) {
	resetAbtRegistry(t)
	mustSetRegistry(t, abtTeamRegistryFixture)

	names := AbtGroupNames()
	if len(names) != 4 {
		t.Fatalf("AbtGroupNames() returned %d names, want 4: %v", len(names), names)
	}

	// Three fields, family CRE.
	alpha, ok := FindAbtTeamByGroupName("Alpha Team")
	if !ok {
		t.Fatalf("expected to find Alpha Team")
	}
	if alpha.TeamKey != "alpha" || alpha.Family != AbtFamilyCRE {
		t.Fatalf("Alpha Team = %+v, want teamKey=alpha family=cre", alpha)
	}

	// Three fields, family SRE, multi-word display name.
	beta, ok := FindAbtTeamByGroupName("Beta SRE Group")
	if !ok {
		t.Fatalf("expected to find Beta SRE Group")
	}
	if beta.TeamKey != "beta" || beta.Family != AbtFamilySRE {
		t.Fatalf("Beta SRE Group = %+v, want teamKey=beta family=sre", beta)
	}

	// A lowercase family in config normalizes the same way an uppercase one does.
	delta, ok := FindAbtTeamByGroupName("Delta-Two")
	if !ok {
		t.Fatalf("expected to find Delta-Two")
	}
	if delta.TeamKey != "delta" || delta.Family != AbtFamilyCRE {
		t.Fatalf("Delta-Two = %+v, want teamKey=delta family=cre", delta)
	}

	// Two fields: no family, and that is not an error.
	gamma, ok := FindAbtTeamByGroupName("Gamma Team")
	if !ok {
		t.Fatalf("expected to find Gamma Team")
	}
	if gamma.TeamKey != "gamma" || gamma.Family != "" {
		t.Fatalf("Gamma Team = %+v, want teamKey=gamma family=\"\"", gamma)
	}

	// Lookup by key works for the same rows.
	if team, ok := FindAbtTeamByKey("beta"); !ok || team.DisplayName != "Beta SRE Group" {
		t.Fatalf("FindAbtTeamByKey(\"beta\") = %+v, %t; want the Beta SRE Group row", team, ok)
	}
	if _, ok := FindAbtTeamByKey("nope"); ok {
		t.Fatalf("expected no match for an unknown team key")
	}

	// Unknown name -> no match.
	if _, ok := FindAbtTeamByGroupName("Does Not Exist"); ok {
		t.Fatalf("expected no match for unknown group name")
	}

	// AbtTeams preserves configured order.
	all := AbtTeams()
	if len(all) != 4 || all[0].TeamKey != "alpha" || all[3].TeamKey != "gamma" {
		t.Fatalf("AbtTeams() = %+v, want the 4 configured rows in order", all)
	}
}

// TestParseAbtTeamRegistry_TrimsWhitespace covers the value someone types into
// a deployment platform's web form, with spaces around the separators.
func TestParseAbtTeamRegistry_TrimsWhitespace(t *testing.T) {
	resetAbtRegistry(t)
	mustSetRegistry(t, "  alpha | Alpha Team | CRE , beta | Beta Team ")

	if _, ok := FindAbtTeamByGroupName("Alpha Team"); !ok {
		t.Fatalf("expected \"Alpha Team\" to match after trimming; got names %v", AbtGroupNames())
	}
	alpha, _ := FindAbtTeamByKey("alpha")
	if alpha.TeamKey != "alpha" || alpha.DisplayName != "Alpha Team" || alpha.Family != AbtFamilyCRE {
		t.Fatalf("alpha = %+v, want every field trimmed", alpha)
	}
	beta, ok := FindAbtTeamByKey("beta")
	if !ok || beta.DisplayName != "Beta Team" {
		t.Fatalf("beta = %+v, %t; want DisplayName \"Beta Team\"", beta, ok)
	}
}

// TestParseAbtTeamRegistry_GroupSysID covers the new optional 4th field: the
// backing group's sysid, used only for the integrationCsTeamIds case filter.
func TestParseAbtTeamRegistry_GroupSysID(t *testing.T) {
	resetAbtRegistry(t)
	mustSetRegistry(t, "alpha|Alpha Team|CRE|d1e42a1234567890abcdef1234567890,beta|Beta Team|SRE,gamma|Gamma Team")

	// Four fields: family and groupSysID both populated.
	alpha, ok := FindAbtTeamByKey("alpha")
	if !ok {
		t.Fatalf("expected to find alpha")
	}
	if alpha.Family != AbtFamilyCRE || alpha.GroupSysID != "d1e42a1234567890abcdef1234567890" {
		t.Fatalf("alpha = %+v, want family=cre groupSysID=d1e42a1234567890abcdef1234567890", alpha)
	}

	// Three fields: family populated, groupSysID left empty (not configured).
	beta, ok := FindAbtTeamByKey("beta")
	if !ok {
		t.Fatalf("expected to find beta")
	}
	if beta.Family != AbtFamilySRE || beta.GroupSysID != "" {
		t.Fatalf("beta = %+v, want family=sre groupSysID=\"\"", beta)
	}

	// Two fields: neither family nor groupSysID configured.
	gamma, ok := FindAbtTeamByKey("gamma")
	if !ok {
		t.Fatalf("expected to find gamma")
	}
	if gamma.Family != "" || gamma.GroupSysID != "" {
		t.Fatalf("gamma = %+v, want family=\"\" groupSysID=\"\"", gamma)
	}
}

// TestParseAbtTeamRegistry_TrailingCommaTolerated: a blank row is skipped, not
// rejected -- a trailing comma is not worth failing a deploy over.
func TestParseAbtTeamRegistry_TrailingCommaTolerated(t *testing.T) {
	teams, err := ParseAbtTeamRegistry("alpha|Alpha Team,")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("got %d teams, want 1: %+v", len(teams), teams)
	}
}

func TestParseAbtTeamRegistry_RejectsMalformedRows(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantIn  string // substring the error must name
		wantRow string // the offending row must be quoted in the error
	}{
		{
			name:    "one field",
			raw:     "alpha|Alpha Team,beta",
			wantIn:  "row 2",
			wantRow: "beta",
		},
		{
			name:    "five fields",
			raw:     "alpha|Alpha Team|CRE|d1e42a1234567890abcdef1234567890|extra",
			wantIn:  "row 1",
			wantRow: "alpha|Alpha Team|CRE|d1e42a1234567890abcdef1234567890|extra",
		},
		{
			name:    "empty teamKey",
			raw:     "alpha|Alpha Team,|Beta Team|SRE",
			wantIn:  "teamKey is empty",
			wantRow: "|Beta Team|SRE",
		},
		{
			name:    "whitespace-only teamKey",
			raw:     "   |Alpha Team",
			wantIn:  "teamKey is empty",
			wantRow: "|Alpha Team",
		},
		{
			// The dangerous one: an empty display name is matched verbatim
			// against the upstream group name, so it resolves zero members
			// without surfacing an error anywhere.
			name:    "empty displayName",
			raw:     "alpha|",
			wantIn:  "displayName is empty",
			wantRow: "alpha|",
		},
		{
			name:    "whitespace-only displayName",
			raw:     "alpha|   |CRE",
			wantIn:  "displayName is empty",
			wantRow: "alpha|   |CRE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			teams, err := ParseAbtTeamRegistry(tc.raw)
			if err == nil {
				t.Fatalf("ParseAbtTeamRegistry(%q) = %+v, nil; want an error", tc.raw, teams)
			}
			if teams != nil {
				t.Fatalf("ParseAbtTeamRegistry(%q) returned teams %+v alongside its error; want nil", tc.raw, teams)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("error %q does not mention %q", err, tc.wantIn)
			}
			if !strings.Contains(err.Error(), tc.wantRow) {
				t.Fatalf("error %q does not name the offending row %q", err, tc.wantRow)
			}
		})
	}
}

// TestAbtRegistry_UnsetConfig_EmptyRegistryNoPanic: an unset variable is legal.
// The service must start and every lookup must return empty rather than panic.
func TestAbtRegistry_UnsetConfig_EmptyRegistryNoPanic(t *testing.T) {
	resetAbtRegistry(t)

	teams, err := ParseAbtTeamRegistry("")
	if err != nil {
		t.Fatalf("ParseAbtTeamRegistry(\"\") returned error %v, want nil", err)
	}
	if len(teams) != 0 {
		t.Fatalf("ParseAbtTeamRegistry(\"\") = %+v, want no teams", teams)
	}
	SetAbtTeams(teams)

	names := AbtGroupNames()
	if names == nil {
		t.Fatalf("AbtGroupNames() returned nil, want empty non-nil slice")
	}
	if len(names) != 0 {
		t.Fatalf("AbtGroupNames() = %v, want empty", names)
	}
	if _, ok := FindAbtTeamByGroupName("Alpha Team"); ok {
		t.Fatalf("expected no match against an empty registry")
	}
	if _, ok := FindAbtTeamByKey("alpha"); ok {
		t.Fatalf("expected no key match against an empty registry")
	}
	if got := AbtTeams(); len(got) != 0 {
		t.Fatalf("AbtTeams() = %+v, want empty", got)
	}
}

// TestSetAbtTeams_CopiesInput guards against the caller mutating the slice it
// handed over and silently rewriting the live registry.
func TestSetAbtTeams_CopiesInput(t *testing.T) {
	resetAbtRegistry(t)

	teams := []AbtTeam{{TeamKey: "alpha", DisplayName: "Alpha Team"}}
	SetAbtTeams(teams)
	teams[0].DisplayName = "Mutated"

	got, ok := FindAbtTeamByKey("alpha")
	if !ok || got.DisplayName != "Alpha Team" {
		t.Fatalf("registry = %+v after the caller mutated its slice; want DisplayName \"Alpha Team\"", got)
	}
}

// TestParseAbtTeamRegistry_AllFourFamilies covers the widened family enum.
// The two legacy values ("cre", "sre") must keep parsing exactly as before --
// registries already deployed carry them -- while the two ABT variants are
// new. Case is irrelevant on every one of them.
func TestParseAbtTeamRegistry_AllFourFamilies(t *testing.T) {
	cases := []struct {
		raw  string
		want AbtFamily
	}{
		{"cre-abt", AbtFamilyCREAbt},
		{"CRE-ABT", AbtFamilyCREAbt},
		{"Cre-Abt", AbtFamilyCREAbt},
		{"cre", AbtFamilyCRE},
		{"CRE", AbtFamilyCRE},
		{"sre-abt", AbtFamilySREAbt},
		{"SRE-ABT", AbtFamilySREAbt},
		{"sre", AbtFamilySRE},
		{"SRE", AbtFamilySRE},
		{"", ""},
		{"  ", ""},
	}

	for _, tc := range cases {
		teams, err := ParseAbtTeamRegistry("key|Display Name|" + tc.raw)
		if err != nil {
			t.Fatalf("ParseAbtTeamRegistry(family=%q) returned error: %v", tc.raw, err)
		}
		if len(teams) != 1 {
			t.Fatalf("ParseAbtTeamRegistry(family=%q) returned %d teams, want 1", tc.raw, len(teams))
		}
		if teams[0].Family != tc.want {
			t.Fatalf("family %q parsed to %q, want %q", tc.raw, teams[0].Family, tc.want)
		}
	}
}

// TestParseAbtTeamRegistry_RejectsUnknownFamily locks in the reversal of the
// old pass-through behaviour: an unrecognised family now fails the deploy,
// naming the row, rather than producing a team no picker will ever show.
func TestParseAbtTeamRegistry_RejectsUnknownFamily(t *testing.T) {
	for _, raw := range []string{"sre_abt", "abt", "cre-abts", "platform"} {
		row := "key|Display Name|" + raw
		_, err := ParseAbtTeamRegistry(row)
		if err == nil {
			t.Fatalf("ParseAbtTeamRegistry(%q) returned no error, want a rejection", row)
		}
		if !strings.Contains(err.Error(), raw) {
			t.Fatalf("error %q does not name the offending value %q", err, raw)
		}
		if !strings.Contains(err.Error(), "row 1") {
			t.Fatalf("error %q does not name the offending row", err)
		}
	}
}

// TestParseAbtTeamRegistry_FourFieldAbtFamily proves the widened values still
// work in the four-field shape, where family sits between the display name
// and the backing group sysid.
func TestParseAbtTeamRegistry_FourFieldAbtFamily(t *testing.T) {
	teams, err := ParseAbtTeamRegistry("alpha|Alpha Team|sre-abt|d1e42a1234567890abcdef1234567890")
	if err != nil {
		t.Fatalf("ParseAbtTeamRegistry returned error: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("got %d teams, want 1", len(teams))
	}
	if teams[0].Family != AbtFamilySREAbt {
		t.Fatalf("family = %q, want %q", teams[0].Family, AbtFamilySREAbt)
	}
	if teams[0].GroupSysID != "d1e42a1234567890abcdef1234567890" {
		t.Fatalf("groupSysID = %q, want the configured sysid", teams[0].GroupSysID)
	}
}
