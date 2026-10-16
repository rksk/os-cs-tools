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

package paging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

// The ladder values are the rest of this file's cre: and sre: sections that
// can be saved on the Team Schedule's Alert Settings tab, one key each, in the
// file's own shape (entity-service checks each one's limits). A saved value
// replaces that key of the file; a key not saved keeps the file's value.
//
// What decides whether paging runs and what it may cost -- enabled, channel,
// safety, phones.source, testCall, sme, cre.rules -- is not among them and
// stays in the file.

// CaseAlertValue is a saved ladder value.
type CaseAlertValue struct {
	Ladder string          `json:"ladder"`
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value"`
}

// caseAlertKey is one key: which ladders have it, how a saved value is laid
// over a ladder, and how a ladder's value is read back for the report (false
// when the file leaves it unset).
type caseAlertKey struct {
	ladders []string
	set     func(l *LadderConfig, raw json.RawMessage) error
	get     func(l LadderConfig) (any, bool)
	// private values are logged as changed, never with the value (heads).
	private bool
}

var (
	caseAlertBoth = []string{"CRE", "SRE"}
	caseAlertCRE  = []string{"CRE"}
	caseAlertSRE  = []string{"SRE"}
)

// caseAlertKeys are the ladder values, by the file's name.
var caseAlertKeys = map[string]caseAlertKey{
	"trigger.priorities":       listKey(caseAlertBoth, func(l *LadderConfig) *[]string { return &l.Start.Priorities }),
	"trigger.teams":            listKey(caseAlertBoth, func(l *LadderConfig) *[]string { return &l.Start.Teams }),
	"trigger.excludeTeams":     listKey(caseAlertBoth, func(l *LadderConfig) *[]string { return &l.Start.ExcludeTeams }),
	"trigger.shifts":           listKey(caseAlertBoth, func(l *LadderConfig) *[]string { return &l.Start.Shifts }),
	"trigger.requireKnownTeam": boolKey(caseAlertBoth, func(l *LadderConfig) *bool { return &l.Start.RequireKnownTeam }),
	"onLeave": {
		ladders: caseAlertBoth,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			var v OnLeave
			if err := strictJSON(raw, &v); err != nil {
				return err
			}
			l.OnLeave = v
			return nil
		},
		get: func(l LadderConfig) (any, bool) {
			v := OnLeave(strings.ToLower(strings.TrimSpace(string(l.OnLeave))))
			if v == "" {
				v = OnLeaveSkip
			}
			return string(v), true
		},
	},
	"teams.abtType": stringKey(caseAlertBoth, func(l *LadderConfig) *string { return &l.Teams.ABTType }),
	"teams.abts":    listKey(caseAlertBoth, func(l *LadderConfig) *[]string { return &l.Teams.ABTs }),

	"teams.americas":                  stringKey(caseAlertCRE, func(l *LadderConfig) *string { return &l.Teams.Americas }),
	"teams.leadership":                stringKey(caseAlertCRE, func(l *LadderConfig) *string { return &l.Teams.Leadership }),
	"teams.teamLeadsToCall":           intKey(caseAlertCRE, func(l *LadderConfig) *int { return &l.Teams.TeamLeadsToCall }),
	"teams.teamLeads":                 listKey(caseAlertCRE, func(l *LadderConfig) *[]string { return &l.Teams.TeamLeads }),
	"teams.americasWeekendRotaShifts": listKey(caseAlertCRE, func(l *LadderConfig) *[]string { return &l.Teams.AmericasWeekendRotaShifts }),
	"teams.rotaTeams":                 listKey(caseAlertCRE, func(l *LadderConfig) *[]string { return &l.Teams.RotaTeams }),
	"teams.unassignedTeamLead":        stringKey(caseAlertCRE, func(l *LadderConfig) *string { return &l.Teams.UnassignedTeamLead }),
	"teams.unassignedTeamLeadCount":   intKey(caseAlertCRE, func(l *LadderConfig) *int { return &l.Teams.UnassignedTeamLeadCount }),
	"teams.rotaMembersToCall": {
		ladders: caseAlertCRE,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			var v map[string]int
			if err := strictJSON(raw, &v); err != nil {
				return err
			}
			l.Teams.RotaMembersToCall = v
			return nil
		},
		get: func(l LadderConfig) (any, bool) {
			if l.Teams.RotaMembersToCall == nil {
				return nil, false
			}
			return maps.Clone(l.Teams.RotaMembersToCall), true
		},
	},
	"heads.cre":         headKey(func(l *LadderConfig) *Person { return &l.Heads.CRE }),
	"heads.cs":          headKey(func(l *LadderConfig) *Person { return &l.Heads.CS }),
	"alertDuty.tiers":   listKey(caseAlertCRE, func(l *LadderConfig) *[]string { return &l.AlertDuty.Tiers }),
	"alertDuty.perTeam": intKey(caseAlertCRE, func(l *LadderConfig) *int { return &l.AlertDuty.PerTeam }),
	"acknowledgement.requireBoth": {
		ladders: caseAlertCRE,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			var v bool
			if err := strictJSON(raw, &v); err != nil {
				return err
			}
			l.Acknowledgement.RequireBoth = &v
			return nil
		},
		get: func(l LadderConfig) (any, bool) {
			if l.Acknowledgement.RequireBoth == nil {
				return nil, false
			}
			return *l.Acknowledgement.RequireBoth, true
		},
	},

	"teams.defaultRota":  stringKey(caseAlertSRE, func(l *LadderConfig) *string { return &l.Teams.DefaultRota }),
	"timing.initialWait": durationKey(func(l *LadderConfig) *Duration { return &l.Timing.InitialWait }, true),
	"timing.interval":    durationKey(func(l *LadderConfig) *Duration { return &l.Timing.Interval }, false),
	"timing.includeL4":   boolKey(caseAlertSRE, func(l *LadderConfig) *bool { return &l.Timing.IncludeL4 }),
	"unansweredChat":     boolKey(caseAlertSRE, func(l *LadderConfig) *bool { return &l.UnansweredChat }),
	"smeHandoffChat":     boolKey(caseAlertSRE, func(l *LadderConfig) *bool { return &l.SMEHandoffChat }),
}

// smeCaseAlertKey is one key of the file's sme: section that can be saved
// (ladder SME, 0238): how a saved value is laid over it, and read back.
type smeCaseAlertKey struct {
	set func(s *SMEConfig, raw json.RawMessage) error
	get func(s SMEConfig) (any, bool)
}

// smeCaseAlertKeys are the SME page's values, by the file's name. Whether it
// runs, its channel, safety and chat rooms are not among them.
var smeCaseAlertKeys = map[string]smeCaseAlertKey{
	"timing.initialWait": smeDurationKey(func(s *SMEConfig) *Duration { return &s.Timing.InitialWait }, true),
	"timing.interval":    smeDurationKey(func(s *SMEConfig) *Duration { return &s.Timing.Interval }, false),
	"onLeave": {
		set: func(s *SMEConfig, raw json.RawMessage) error {
			var v OnLeave
			if err := strictJSON(raw, &v); err != nil {
				return err
			}
			s.OnLeave = v
			return nil
		},
		get: func(s SMEConfig) (any, bool) {
			v := OnLeave(strings.ToLower(strings.TrimSpace(string(s.OnLeave))))
			if v == "" {
				v = OnLeaveSkip
			}
			return string(v), true
		},
	},
	"unansweredChat": {
		set: func(s *SMEConfig, raw json.RawMessage) error { return strictJSON(raw, &s.UnansweredChat) },
		get: func(s SMEConfig) (any, bool) { return s.UnansweredChat, true },
	},
	"teams": {
		set: func(s *SMEConfig, raw json.RawMessage) error {
			var v map[string]string
			if err := strictJSON(raw, &v); err != nil {
				return err
			}
			s.Teams = v
			return nil
		},
		get: func(s SMEConfig) (any, bool) {
			if s.Teams == nil {
				return nil, false
			}
			return maps.Clone(s.Teams), true
		},
	},
}

func smeDurationKey(field func(*SMEConfig) *Duration, zeroIsSet bool) smeCaseAlertKey {
	return smeCaseAlertKey{
		set: func(s *SMEConfig, raw json.RawMessage) error {
			var v string
			if err := strictJSON(raw, &v); err != nil {
				return err
			}
			d, err := time.ParseDuration(strings.TrimSpace(v))
			if err != nil {
				return fmt.Errorf("%q is not a duration", v)
			}
			*field(s) = Duration(d)
			return nil
		},
		get: func(s SMEConfig) (any, bool) {
			d := time.Duration(*field(&s))
			if d == 0 && !zeroIsSet {
				return "", false
			}
			return fileDuration(d), true
		},
	}
}

// strictJSON decodes raw into v, refusing what v does not have room for.
func strictJSON(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("not the shape this setting takes: %w", err)
	}
	return nil
}

func listKey(ladders []string, field func(*LadderConfig) *[]string) caseAlertKey {
	return caseAlertKey{
		ladders: ladders,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			var v []string
			if err := strictJSON(raw, &v); err != nil {
				return err
			}
			if v == nil {
				v = []string{}
			}
			*field(l) = v
			return nil
		},
		get: func(l LadderConfig) (any, bool) {
			// Absent from the file is unset; written as [] it is a value.
			v := *field(&l)
			if v == nil {
				return nil, false
			}
			return slices.Clone(v), true
		},
	}
}

func boolKey(ladders []string, field func(*LadderConfig) *bool) caseAlertKey {
	return caseAlertKey{
		ladders: ladders,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			return strictJSON(raw, field(l))
		},
		get: func(l LadderConfig) (any, bool) { return *field(&l), true },
	}
}

func intKey(ladders []string, field func(*LadderConfig) *int) caseAlertKey {
	return caseAlertKey{
		ladders: ladders,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			return strictJSON(raw, field(l))
		},
		get: func(l LadderConfig) (any, bool) { return *field(&l), true },
	}
}

// stringKey is a text value; an empty one is left out of the report, as
// unset in the file -- the code's default applies either way.
func stringKey(ladders []string, field func(*LadderConfig) *string) caseAlertKey {
	return caseAlertKey{
		ladders: ladders,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			return strictJSON(raw, field(l))
		},
		get: func(l LadderConfig) (any, bool) {
			v := strings.TrimSpace(*field(&l))
			return v, v != ""
		},
	}
}

// durationKey is an SRE timing value. zeroIsSet says whether zero is a value
// (initialWait: call at once) or unset (interval: the five-minute default).
func durationKey(field func(*LadderConfig) *Duration, zeroIsSet bool) caseAlertKey {
	return caseAlertKey{
		ladders: caseAlertSRE,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			var s string
			if err := strictJSON(raw, &s); err != nil {
				return err
			}
			d, err := time.ParseDuration(strings.TrimSpace(s))
			if err != nil {
				return fmt.Errorf("%q is not a duration", s)
			}
			*field(l) = Duration(d)
			return nil
		},
		get: func(l LadderConfig) (any, bool) {
			d := time.Duration(*field(&l))
			if d == 0 && !zeroIsSet {
				return "", false
			}
			return fileDuration(d), true
		},
	}
}

// fileDuration writes a duration the way the file does: 5m, 30s, 0s.
func fileDuration(d time.Duration) string {
	switch {
	case d == 0:
		return "0s"
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d%time.Second == 0:
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	return d.String()
}

// caseAlertHead is a head as saved on the tab: who, not how to reach them --
// the number comes from their profile, as for everyone else the ladder calls.
type caseAlertHead struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func headKey(field func(*LadderConfig) *Person) caseAlertKey {
	return caseAlertKey{
		ladders: caseAlertCRE,
		private: true,
		set: func(l *LadderConfig, raw json.RawMessage) error {
			var h caseAlertHead
			if err := strictJSON(raw, &h); err != nil {
				return err
			}
			*field(l) = Person{Name: strings.TrimSpace(h.Name), Email: strings.TrimSpace(h.Email)}
			return nil
		},
		get: func(l LadderConfig) (any, bool) {
			p := *field(&l)
			if !p.Set() {
				return nil, false
			}
			return caseAlertHead{Name: p.Name, Email: p.Email}, true
		},
	}
}

// ladderFor is the ladder a saved value is for, in out.
func ladderFor(out *Config, ladder string) *LadderConfig {
	switch ladder {
	case "CRE":
		return &out.CRE
	case "SRE":
		return &out.SRE
	}
	return nil
}

// usableCaseAlertValue is the ladder and spec of a saved value this build can
// apply, or why not.
func usableCaseAlertValue(v CaseAlertValue) (string, caseAlertKey, string) {
	ladder := strings.ToUpper(strings.TrimSpace(v.Ladder))
	key := strings.TrimSpace(v.Key)
	spec, known := caseAlertKeys[key]
	where := strings.ToLower(ladder) + "." + key
	if ladder == "SME" {
		if _, ok := smeCaseAlertKeys[key]; !ok {
			return "", spec, fmt.Sprintf("skipped %s: not a setting this build knows", where)
		}
		return ladder, spec, ""
	}
	switch {
	case ladder != "CRE" && ladder != "SRE":
		return "", spec, fmt.Sprintf("skipped a value for ladder %q", v.Ladder)
	case !known:
		return "", spec, fmt.Sprintf("skipped %s: not a setting this build knows", where)
	case !slices.Contains(spec.ladders, ladder):
		return "", spec, fmt.Sprintf("skipped %s: not a %s setting", where, ladder)
	}
	return ladder, spec, ""
}

// SavedCaseAlertKeys is which keys of each ladder the saved values set, as
// MergeCaseAlertSettings applies them.
func SavedCaseAlertKeys(s CaseAlertSettings) map[string]map[string]bool {
	out := map[string]map[string]bool{"CRE": {}, "SRE": {}, "SME": {}}
	for _, v := range s.Values {
		if ladder, _, skip := usableCaseAlertValue(v); skip == "" {
			out[ladder][strings.TrimSpace(v.Key)] = true
		}
	}
	return out
}

// mergeCaseAlertValues lays the saved values over out, describing each change
// in changes. A key this build does not know, or one saved for the other
// ladder, is skipped and said so: entity-service may be newer than this
// service.
func mergeCaseAlertValues(out *Config, values []CaseAlertValue, changes *[]string) error {
	sorted := slices.Clone(values)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Ladder+" "+sorted[i].Key < sorted[j].Ladder+" "+sorted[j].Key
	})
	for _, v := range sorted {
		ladder, spec, skip := usableCaseAlertValue(v)
		if skip != "" {
			*changes = append(*changes, skip)
			continue
		}
		if ladder == "SME" {
			if err := mergeSMEValue(out, v, changes); err != nil {
				return err
			}
			continue
		}
		l := ladderFor(out, ladder)
		where := strings.ToLower(ladder) + "." + strings.TrimSpace(v.Key)
		was, wasSet := spec.get(*l)
		if err := spec.set(l, v.Value); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		now, _ := spec.get(*l)
		if wasSet && sameJSON(was, now) {
			continue
		}
		if spec.private {
			*changes = append(*changes, where+": changed")
			continue
		}
		from := "unset"
		if wasSet {
			from = compactJSON(was)
		}
		*changes = append(*changes, fmt.Sprintf("%s: %s -> %s", where, from, compactJSON(now)))
	}
	return nil
}

// mergeSMEValue lays one saved value over the file's sme: section.
func mergeSMEValue(out *Config, v CaseAlertValue, changes *[]string) error {
	key := strings.TrimSpace(v.Key)
	spec := smeCaseAlertKeys[key]
	where := "sme." + key
	was, wasSet := spec.get(out.SME)
	if err := spec.set(&out.SME, v.Value); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	now, _ := spec.get(out.SME)
	if wasSet && sameJSON(was, now) {
		return nil
	}
	from := "unset"
	if wasSet {
		from = compactJSON(was)
	}
	*changes = append(*changes, fmt.Sprintf("%s: %s -> %s", where, from, compactJSON(now)))
	return nil
}

func compactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func sameJSON(a, b any) bool { return compactJSON(a) == compactJSON(b) }

// fileOnlyCaseAlertValues is what only the file can set, shown read-only on
// the tab so nobody has to open the file to see what a ladder may do: whether
// it runs, its channel, its caps, where numbers come from. The allowed
// numbers are counted, never listed.
func fileOnlyCaseAlertValues(cfg Config, l LadderConfig) map[string]any {
	out := map[string]any{
		"enabled":                           cfg.Enabled && l.Enabled,
		"channel":                           string(l.Channel),
		"safety.maxCallsPerLadder":          l.Safety.MaxCallsPerLadder,
		"safety.allowedNumbers":             len(l.Safety.AllowedNumbers),
		"safety.callWithoutVerifiedLeads":   l.Safety.CallWithoutVerifiedLeads,
		"safety.callHeadsWithoutLowerTiers": l.Safety.CallHeadsWithoutLowerTiers,
		"phones.source":                     l.PhoneSource(),
	}
	if lvl, capped := l.CapLevel(); capped {
		out["safety.maxLevel"] = int(lvl)
	}
	return out
}

// RunningCaseAlertValues is what cfg runs with, key by key, for the Case
// Alerts tab: each ladder value's value and where it came from ("saved" for
// a key in saved, else "file"). A key the file leaves unset is left out.
// Heads carry a name and an email, never a phone number. fileOnly is what
// only the file sets (fileOnlyCaseAlertValues).
func RunningCaseAlertValues(cfg Config, saved map[string]map[string]bool) map[string]any {
	fileOnly := map[string]any{}
	for _, ladder := range caseAlertBoth {
		fileOnly[ladder] = fileOnlyCaseAlertValues(cfg, *ladderFor(&cfg, ladder))
	}
	fileOnly["SME"] = map[string]any{
		"enabled":               cfg.Enabled && cfg.SME.Enabled,
		"channel":               string(cfg.SME.Channel),
		"safety.allowedNumbers": len(cfg.SME.Safety.AllowedNumbers),
	}
	ladders := map[string]any{}
	sme := map[string]any{}
	for key, spec := range smeCaseAlertKeys {
		v, ok := spec.get(cfg.SME)
		if !ok {
			continue
		}
		source := "file"
		if saved["SME"][key] {
			source = "saved"
		}
		sme[key] = map[string]any{"value": v, "source": source}
	}
	ladders["SME"] = sme
	for _, ladder := range caseAlertBoth {
		l := ladderFor(&cfg, ladder)
		out := map[string]any{}
		for key, spec := range caseAlertKeys {
			if !slices.Contains(spec.ladders, ladder) {
				continue
			}
			v, ok := spec.get(*l)
			if !ok {
				continue
			}
			source := "file"
			if saved[ladder][key] {
				source = "saved"
			}
			out[key] = map[string]any{"value": v, "source": source}
		}
		ladders[ladder] = out
	}
	return map[string]any{"ladders": ladders, "fileOnly": fileOnly}
}
