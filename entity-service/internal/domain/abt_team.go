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
	"fmt"
	"log"
	"strings"
	"sync"
)

// AbtFamily classifies a team along two axes at once: its discipline
// (customer renewal/expansion vs site reliability) and whether it is an
// account-based team (ABT) or the wider non-ABT organisation. Not every team
// has a family assigned -- it is empty for the unclassified teams.
//
// The "-abt" variants are the ones a dashboard team picker offers: a
// dashboard scoped to a discipline lists only that discipline's ABTs. The
// bare variants classify a member of the discipline who is not on an ABT.
type AbtFamily string

const (
	// AbtFamilyCREAbt identifies a Customer Renewal & Expansion
	// account-based team.
	AbtFamilyCREAbt AbtFamily = "cre-abt"
	// AbtFamilyCRE identifies a Customer Renewal & Expansion team that is
	// not an account-based team.
	AbtFamilyCRE AbtFamily = "cre"
	// AbtFamilySREAbt identifies a Site Reliability Engineering
	// account-based team.
	AbtFamilySREAbt AbtFamily = "sre-abt"
	// AbtFamilySRE identifies a Site Reliability Engineering team that is
	// not an account-based team.
	AbtFamilySRE AbtFamily = "sre"
)

// validAbtFamilies is the closed set of family values the registry accepts.
// It is closed deliberately: consumers branch on the family (a dashboard's
// team picker filters to "sre-abt", the default-dashboard choice keys off
// "cre"/"sre"), so a typo like "sre_abt" would not error anywhere -- it would
// just make the team invisible in every picker. Failing the deploy is the
// only place that mistake is cheap to catch.
var validAbtFamilies = map[AbtFamily]bool{
	AbtFamilyCREAbt: true,
	AbtFamilyCRE:    true,
	AbtFamilySREAbt: true,
	AbtFamilySRE:    true,
}

// AbtTeam is one of WSO2's Account-Based Teams. Team names are organisation
// vocabulary and are never hardcoded in this repo: the registry is supplied as
// deployment configuration (see ParseAbtTeamRegistry) and installed once at
// startup. The registry is a flat list; there is no sub-team nesting.
type AbtTeam struct {
	TeamKey string
	// DisplayName is the exact group name in the backing data source; it is
	// matched verbatim against group-members/search's groupName, so an empty
	// or misspelt value silently resolves zero members. ParseAbtTeamRegistry
	// rejects an empty one for that reason.
	DisplayName string
	// Family may be empty -- not every team has a family assigned.
	Family AbtFamily
	// GroupSysID is the backing data source's sys_user_group sysid for this
	// team, distinct from DisplayName: DisplayName is matched verbatim for
	// membership resolution (resolveAbtTeam), while GroupSysID is the id that
	// backs case-search's integrationCsTeamIds filter. It is optional -- a
	// team with no configured id simply cannot be used to scope that filter,
	// which degrades gracefully rather than erroring.
	GroupSysID string
}

var (
	abtMu    sync.RWMutex
	abtTeams []AbtTeam
)

// SetAbtTeams installs the ABT team registry. It is called once during startup
// with the parsed contents of the team-registry configuration, and by tests.
// An empty registry is legal (no teams configured) and is warned about rather
// than rejected: team names cannot be defaulted in this repo, so a deployment
// that has not set the variable yet must still start and serve every other
// endpoint.
func SetAbtTeams(teams []AbtTeam) {
	abtMu.Lock()
	defer abtMu.Unlock()
	abtTeams = append([]AbtTeam(nil), teams...)
	if len(abtTeams) == 0 {
		log.Printf("abtteam: team registry is empty; team filters and the team catalogue will return nothing")
	}
}

// AbtGroupNames returns the backing data source's group display name for every
// configured ABT team, suitable for a single groupNames-IN membership query.
// Returns an empty slice if no teams are configured.
func AbtGroupNames() []string {
	abtMu.RLock()
	defer abtMu.RUnlock()

	names := make([]string, 0, len(abtTeams))
	for _, team := range abtTeams {
		if team.DisplayName != "" {
			names = append(names, team.DisplayName)
		}
	}
	return names
}

// FindAbtTeamByKey looks a team up by its registry key. ok is false if no
// configured team matches.
func FindAbtTeamByKey(key string) (team AbtTeam, ok bool) {
	abtMu.RLock()
	defer abtMu.RUnlock()
	for _, t := range abtTeams {
		if t.TeamKey == key {
			return t, true
		}
	}
	return AbtTeam{}, false
}

// AbtTeams returns every configured team. Empty if none are configured.
func AbtTeams() []AbtTeam {
	abtMu.RLock()
	defer abtMu.RUnlock()
	out := make([]AbtTeam, len(abtTeams))
	copy(out, abtTeams)
	return out
}

// FindAbtTeamByGroupName looks up the ABT team whose group name in the backing
// data source exactly matches groupName. ok is false if no configured team
// matches.
func FindAbtTeamByGroupName(groupName string) (team AbtTeam, ok bool) {
	abtMu.RLock()
	defer abtMu.RUnlock()

	for _, t := range abtTeams {
		if t.DisplayName == groupName {
			return t, true
		}
	}
	return AbtTeam{}, false
}

// ParseAbtTeamRegistry parses the team registry from its flat, single-line
// configuration form:
//
//	teamKey|Display Name|FAMILY|groupSysID,teamKey|Display Name,...
//
// Rows are separated by ",", fields within a row by "|". A row carries two
// fields (key and display name), three (plus the family), or four (plus the
// backing group's sysid, used only to scope the integrationCsTeamIds case
// filter). family is a real optional middle field, not a slot that can be
// skipped: a groupSysID cannot be supplied without a family alongside it, so
// a 2-field-plus-id shape is not accepted -- pad the family field (even
// empty, e.g. "key|Name||sysid") if a team needs an id but no family.
// Whitespace around every field is trimmed, so a value pasted into a web
// form survives. A wholly blank row is skipped, which tolerates a trailing
// comma.
//
// The single-line shape is deliberate: the deployment platform's configuration
// UI is one-dimensional and stringifies nested collections, so a structured
// (nested-array) registry cannot be deployed at all. Do not reintroduce one.
//
// An empty string yields no teams and no error. Any malformed row is an error
// naming the offending row, so a typo stops a deploy instead of silently
// degrading team resolution at the first request.
func ParseAbtTeamRegistry(raw string) ([]AbtTeam, error) {
	rows := strings.Split(raw, ",")
	teams := make([]AbtTeam, 0, len(rows))

	for i, row := range rows {
		if strings.TrimSpace(row) == "" {
			continue
		}

		fields := strings.Split(row, "|")
		if len(fields) < 2 || len(fields) > 4 {
			return nil, fmt.Errorf(
				"team registry row %d (%q): expected 2, 3, or 4 %q-separated fields (teamKey|displayName[|family[|groupSysID]]), got %d",
				i+1, strings.TrimSpace(row), "|", len(fields))
		}
		for j := range fields {
			fields[j] = strings.TrimSpace(fields[j])
		}

		if fields[0] == "" {
			return nil, fmt.Errorf("team registry row %d (%q): teamKey is empty", i+1, strings.TrimSpace(row))
		}
		// An empty display name is the dangerous one: it is matched verbatim
		// against the group name upstream, so it would resolve zero members
		// without any error surfacing anywhere.
		if fields[1] == "" {
			return nil, fmt.Errorf("team registry row %d (%q): displayName is empty", i+1, strings.TrimSpace(row))
		}

		team := AbtTeam{TeamKey: fields[0], DisplayName: fields[1]}
		if len(fields) >= 3 {
			family, err := parseAbtFamily(fields[2])
			if err != nil {
				return nil, fmt.Errorf("team registry row %d (%q): %w", i+1, strings.TrimSpace(row), err)
			}
			team.Family = family
		}
		if len(fields) == 4 {
			team.GroupSysID = fields[3]
		}
		teams = append(teams, team)
	}

	return teams, nil
}

// parseAbtFamily normalizes a configured family value (in any case, e.g.
// "SRE-ABT") into this package's lowercase AbtFamily constants. An empty
// value is legal and yields the empty family: not every team is classified.
//
// Any other value is an error rather than being passed through, which is a
// deliberate reversal of this parser's earlier behaviour. Passing an unknown
// value through was safe while nothing branched on the family; now that the
// dashboard team picker and default-dashboard selection both do, an
// unrecognised family silently removes a team from every picker instead of
// erroring anywhere. A rejected deploy is the cheaper failure.
func parseAbtFamily(family string) (AbtFamily, error) {
	trimmed := strings.TrimSpace(family)
	if trimmed == "" {
		return "", nil
	}
	normalized := AbtFamily(strings.ToLower(trimmed))
	if !validAbtFamilies[normalized] {
		return "", fmt.Errorf(
			"unknown family %q: expected one of %q, %q, %q, %q, or empty",
			trimmed, AbtFamilyCREAbt, AbtFamilyCRE, AbtFamilySREAbt, AbtFamilySRE)
	}
	return normalized, nil
}
