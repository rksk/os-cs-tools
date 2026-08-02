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

// AbtFamily classifies an ABT (Account-Based Team) as either a customer
// renewal/expansion team (CRE) or a site reliability team (SRE). Not every
// team has a family assigned -- it is empty for the unclassified teams.
type AbtFamily string

const (
	// AbtFamilyCRE identifies a Customer Renewal & Expansion team.
	AbtFamilyCRE AbtFamily = "cre"
	// AbtFamilySRE identifies a Site Reliability Engineering team.
	AbtFamilySRE AbtFamily = "sre"
)

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
//	teamKey|Display Name|FAMILY,teamKey|Display Name,...
//
// Rows are separated by ",", fields within a row by "|". A row carries either
// two fields (key and display name) or three (plus the family). Whitespace
// around every field is trimmed, so a value pasted into a web form survives.
// A wholly blank row is skipped, which tolerates a trailing comma.
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
		if len(fields) < 2 || len(fields) > 3 {
			return nil, fmt.Errorf(
				"team registry row %d (%q): expected 2 or 3 %q-separated fields (teamKey|displayName[|family]), got %d",
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
		if len(fields) == 3 {
			team.Family = normalizeAbtFamily(fields[2])
		}
		teams = append(teams, team)
	}

	return teams, nil
}

// normalizeAbtFamily normalizes the configured "CRE"/"SRE" (in any case) into
// this package's lowercase AbtFamily constants. Any other value (including the
// empty case) is passed through lowercased rather than rejected, so an
// unclassified team or a future family value never blocks a deploy.
func normalizeAbtFamily(family string) AbtFamily {
	switch strings.ToUpper(family) {
	case "CRE":
		return AbtFamilyCRE
	case "SRE":
		return AbtFamilySRE
	default:
		return AbtFamily(strings.ToLower(family))
	}
}
