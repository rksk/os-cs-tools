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
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// Case Alerts settings are the chat rooms, ServiceNow group routes and other
// ladder values an admin saves on the Team Schedule's Alert Settings tab
// (entity-service, migration 0233). They are read only when
// PAGING_CONFIG_SOURCE says so, once at start, and only replace what they
// name: a room, route or value not saved there keeps the file's and env's.
// See MergeCaseAlertSettings.

// CaseAlertSettings is GET /team-schedule/case-alerts/settings/runtime: the
// enabled rooms whose URL entity-service could open, the enabled routes and
// the saved ladder values.
type CaseAlertSettings struct {
	Revision int64            `json:"revision"`
	Rooms    []CaseAlertRoom  `json:"rooms"`
	Routes   []CaseAlertRoute `json:"routes"`
	Values   []CaseAlertValue `json:"values"`
}

// CaseAlertRoom is a saved room. TeamKey is empty for the ladder's shared
// room (cre.chat, sre.chat); otherwise it is an SRE team's (sre.teamChats) or,
// on ladder SME, an SME team's (sme.teamChats).
type CaseAlertRoom struct {
	Ladder   string `json:"ladder"`
	TeamKey  string `json:"teamKey,omitempty"`
	Label    string `json:"label"`
	Teamless bool   `json:"teamless"`
	// WebhookURL is empty for a team row saved only for its channel.
	WebhookURL string `json:"webhookUrl"`
	// Channel is how an SRE team's alerts go out (chat, call, both, log);
	// empty is the ladder's (sre.teamChannels).
	Channel string `json:"channel,omitempty"`
}

// CaseAlertRoute is a saved route: a group to a team (teams.aliases), or (SRE)
// to a default rota (sre.teams.defaultGroups).
type CaseAlertRoute struct {
	Ladder    string `json:"ladder"`
	GroupName string `json:"groupName"`
	TeamKey   string `json:"teamKey,omitempty"`
	RotaCode  string `json:"rotaCode,omitempty"`
}

// CaseAlertSettings reads the saved settings, with the webhook URLs opened.
// Only this service's client may; entity-service refuses anyone else.
func (c *EntityClient) CaseAlertSettings(ctx context.Context) (CaseAlertSettings, error) {
	raw, err := c.do(ctx, http.MethodGet, "/team-schedule/case-alerts/settings/runtime", nil)
	if err != nil {
		return CaseAlertSettings{}, err
	}
	var s CaseAlertSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		return CaseAlertSettings{}, fmt.Errorf("decode case alert settings: %w", err)
	}
	return s, nil
}

// ReportCaseAlertSettingsApplied tells entity-service what this process
// loaded, so the Alert Settings tab can tell saved from running. summary holds
// names and sources only, never a URL.
func (c *EntityClient) ReportCaseAlertSettingsApplied(ctx context.Context, revision int64, mode string, summary map[string]any) error {
	body, err := json.Marshal(map[string]any{"revision": revision, "mode": mode, "summary": summary})
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodPut, "/team-schedule/case-alerts/settings/applied", body)
	return err
}

// caseAlertRoomEnv is the environment variable a saved room's URL is put in,
// so the engine reads it the way it reads every other webhook
// (Chat.WebhookURLEnv) and nothing in it changes.
func caseAlertRoomEnv(ladder, team string) string {
	name := "CASE_ALERTS_ROOM_" + strings.ToUpper(ladder)
	if team != "" {
		name += "_" + strings.ToUpper(nonEnvChar.ReplaceAllString(team, "_"))
	}
	return name
}

var nonEnvChar = regexp.MustCompile(`[^A-Za-z0-9_]`)

// MergeCaseAlertSettings lays the saved settings over cfg and returns the
// result, the environment variables it needs (variable -> webhook URL), and
// one line per thing it changed. cfg itself is never changed, so a caller
// that only compares (shadow mode) can keep using it.
//
//   - A saved room replaces that ladder's shared chat, or that SRE team's
//     teamChats entry; its URL goes in its own variable. Its teamless flag puts
//     the team in or out of sre.teamlessChats.
//   - A saved SME room replaces that SME team's sme.teamChats entry, or adds
//     one; an SME team with no room of its own still uses sme.chat.
//   - A saved route sets teams.aliases (to a team) or adds the group to
//     sre.teams.defaultGroups (to sre.teams.defaultRota); a route to any other
//     rota is skipped, since a ladder has one default rota.
//   - A saved ladder value replaces that one key of the ladder (teams.abts,
//     timing.interval, heads.cre, ...); see caseAlertKeys.
//   - Anything not saved keeps the file's value.
//
// The result passes the same checks a file does; an error means it would
// not, and the caller keeps cfg.
func MergeCaseAlertSettings(cfg Config, s CaseAlertSettings) (Config, map[string]string, []string, error) {
	if len(s.Rooms) == 0 && len(s.Routes) == 0 && len(s.Values) == 0 {
		// Nothing saved: the file's configuration, as it was loaded.
		return cfg, map[string]string{}, nil, nil
	}
	out := cfg
	out.CRE = cloneLadderChats(cfg.CRE)
	out.SRE = cloneLadderChats(cfg.SRE)
	out.SME.TeamChats = maps.Clone(cfg.SME.TeamChats)
	env := map[string]string{}
	var changes []string

	// Values first: they never touch a room or a route, and setting a list
	// replaces it rather than writing into the file's.
	if err := mergeCaseAlertValues(&out, s.Values, &changes); err != nil {
		return cfg, nil, changes, err
	}

	for _, r := range s.Rooms {
		ladder := strings.ToUpper(strings.TrimSpace(r.Ladder))
		team := teamKeyFor(r.TeamKey)
		if ch := strings.TrimSpace(r.Channel); ch != "" {
			if err := mergeTeamChannel(&out, ladder, team, ch, &changes); err != nil {
				return cfg, nil, changes, err
			}
		}
		url := strings.TrimSpace(r.WebhookURL)
		if url == "" {
			// No URL here (a row saved for its channel, or kept without
			// its URL): the file's room stays.
			continue
		}
		name := caseAlertRoomEnv(ladder, team)
		room := Chat{Audience: strings.TrimSpace(r.Label), WebhookURLEnv: name}
		switch {
		case ladder == "CRE" && team == "":
			changes = append(changes, chatChange("cre.chat", cfg.CRE.Chat, room))
			out.CRE.Chat = room
		case ladder == "SRE" && team == "":
			changes = append(changes, chatChange("sre.chat", cfg.SRE.Chat, room))
			out.SRE.Chat = room
		case ladder == "SRE":
			key := teamChatKey(out.SRE.TeamChats, team)
			if was, ok := cfg.SRE.TeamChats[key]; ok {
				changes = append(changes, chatChange("sre.teamChats."+team, was, room))
			} else {
				// No room of its own in the file: its cards went to sre.chat.
				changes = append(changes, fmt.Sprintf("sre.teamChats.%s: none (used sre.chat) -> saved room %q", team, room.Audience))
			}
			out.SRE.TeamChats[key] = room
			had := slices.ContainsFunc(out.SRE.TeamlessChats, func(k string) bool { return teamKeyFor(k) == team })
			switch {
			case r.Teamless && !had:
				out.SRE.TeamlessChats = append(out.SRE.TeamlessChats, team)
				changes = append(changes, "sre.teamlessChats: + "+team)
			case !r.Teamless && had:
				out.SRE.TeamlessChats = slices.DeleteFunc(out.SRE.TeamlessChats, func(k string) bool { return teamKeyFor(k) == team })
				changes = append(changes, "sre.teamlessChats: - "+team)
			}
		case ladder == "SME" && team != "":
			if out.SME.TeamChats == nil {
				out.SME.TeamChats = map[string]Chat{}
			}
			key := teamChatKey(out.SME.TeamChats, team)
			if was, ok := cfg.SME.TeamChats[key]; ok {
				changes = append(changes, chatChange("sme.teamChats."+team, was, room))
			} else {
				changes = append(changes, fmt.Sprintf("sme.teamChats.%s: none (used sme.chat) -> saved room %q", team, room.Audience))
			}
			out.SME.TeamChats[key] = room
		default:
			changes = append(changes, fmt.Sprintf("skipped a %s room for team %q: only SRE and SME teams have rooms of their own", ladder, team))
			continue
		}
		env[name] = url
	}

	for _, r := range s.Routes {
		ladder := strings.ToUpper(strings.TrimSpace(r.Ladder))
		group := strings.ToLower(strings.TrimSpace(r.GroupName))
		team, rota := teamKeyFor(r.TeamKey), strings.TrimSpace(r.RotaCode)
		var l *LadderConfig
		switch ladder {
		case "CRE":
			l = &out.CRE
		case "SRE":
			l = &out.SRE
		default:
			continue
		}
		prefix := strings.ToLower(ladder) + ".teams."
		switch {
		case group == "":
		case team != "":
			// The file may spell a group in any case ("SRE IaaS"); the saved
			// route takes over that same entry rather than adding a second.
			key := sameGroupKey(l.Teams.Aliases, group)
			if old, ok := l.Teams.Aliases[key]; !ok || teamKeyFor(old) != team {
				changes = append(changes, fmt.Sprintf("%saliases[%s]: %q -> %q", prefix, group, old, team))
				l.Teams.Aliases[key] = team
			}
			if slices.ContainsFunc(l.Teams.DefaultGroups, sameGroup(group)) {
				l.Teams.DefaultGroups = slices.DeleteFunc(l.Teams.DefaultGroups, sameGroup(group))
				changes = append(changes, prefix+"defaultGroups: - "+group)
			}
		case rota != "" && ladder == "SRE" && strings.EqualFold(rota, l.Teams.DefaultRota):
			if key := sameGroupKey(l.Teams.Aliases, group); l.Teams.Aliases[key] != "" {
				delete(l.Teams.Aliases, key)
				changes = append(changes, fmt.Sprintf("%saliases[%s]: removed", prefix, group))
			}
			if !slices.ContainsFunc(l.Teams.DefaultGroups, sameGroup(group)) {
				l.Teams.DefaultGroups = append(l.Teams.DefaultGroups, group)
				changes = append(changes, prefix+"defaultGroups: + "+group)
			}
		default:
			changes = append(changes, fmt.Sprintf("skipped route %q -> rota %q: not %sdefaultRota (%q)", group, rota, prefix, l.Teams.DefaultRota))
		}
	}

	if err := out.validate(); err != nil {
		return cfg, nil, changes, err
	}
	return out, env, changes, nil
}

// mergeTeamChannel sets an SRE team's own channel. Only an SRE team has one;
// anything else is skipped and said so. A channel the file would not read
// refuses the merge.
func mergeTeamChannel(out *Config, ladder, team, raw string, changes *[]string) error {
	if ladder != "SRE" || team == "" {
		*changes = append(*changes, fmt.Sprintf("skipped a channel for %s room %q: only an SRE team has a channel of its own", ladder, team))
		return nil
	}
	ch, err := ParseChannel(raw)
	if err != nil {
		return fmt.Errorf("sre team %s: %w", team, err)
	}
	if was, ok := out.SRE.TeamChannels[team]; !ok || was != ch {
		*changes = append(*changes, fmt.Sprintf("sre team %s: alerts by %s (the ladder: %s; a team never does more than its ladder)",
			team, ch, out.SRE.Channel))
	}
	out.SRE.TeamChannels[team] = ch
	return nil
}

// cloneLadderChats copies the maps and slices of l a merge may change, so the
// merge never writes into the caller's config.
func cloneLadderChats(l LadderConfig) LadderConfig {
	l.TeamChats = maps.Clone(l.TeamChats)
	if l.TeamChats == nil {
		l.TeamChats = map[string]Chat{}
	}
	l.TeamlessChats = slices.Clone(l.TeamlessChats)
	l.TeamChannels = maps.Clone(l.TeamChannels)
	if l.TeamChannels == nil {
		l.TeamChannels = map[string]Channel{}
	}
	l.Teams.Aliases = maps.Clone(l.Teams.Aliases)
	if l.Teams.Aliases == nil {
		l.Teams.Aliases = map[string]string{}
	}
	l.Teams.DefaultGroups = slices.Clone(l.Teams.DefaultGroups)
	return l
}

// sameGroup matches a group name whatever its case or surrounding spaces, as
// the resolver does.
func sameGroup(group string) func(string) bool {
	return func(g string) bool { return strings.EqualFold(strings.TrimSpace(g), group) }
}

// sameGroupKey is the aliases key that already stands for group, whatever its
// case, else group itself.
func sameGroupKey(aliases map[string]string, group string) string {
	for k := range aliases {
		if sameGroup(group)(k) {
			return k
		}
	}
	return group
}

// teamChatKey is the teamChats key that already stands for team, whatever
// its case, else team itself.
func teamChatKey(chats map[string]Chat, team string) string {
	for k := range chats {
		if teamKeyFor(k) == team {
			return k
		}
	}
	return team
}

// chatChange describes a room replaced by a saved one, by label and variable
// name only -- never a URL.
func chatChange(where string, was, now Chat) string {
	from := strings.TrimSpace(was.WebhookURLEnv)
	if from == "" {
		from = "GOOGLE_CHAT_SPACES"
	}
	return fmt.Sprintf("%s: %q via %s -> saved room %q", where, was.Audience, from, now.Audience)
}
