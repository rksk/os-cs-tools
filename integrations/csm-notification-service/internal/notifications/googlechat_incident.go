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

package notifications

import (
	"context"
	"fmt"
	"strings"
)

// IncidentAudiencePrefix namespaces INCIDENT_CHAT_SPACES entries so an assignment group never collides with a GOOGLE_CHAT_SPACES audience.
const IncidentAudiencePrefix = "incident/"

// IncidentAudience is the Chat audience key of an incident's assignment group.
func IncidentAudience(assignmentGroup string) string {
	return IncidentAudiencePrefix + strings.ToLower(strings.TrimSpace(assignmentGroup))
}

// IncidentGroups is the assignment groups whose INCIDENT_CHAT_SPACES spaces get a group's incident messages: its INCIDENT_CHAT_FANOUT list, else the group itself.
func IncidentGroups(fanout map[string][]string, group string) []string {
	key := strings.ToLower(strings.TrimSpace(group))
	if key == "" {
		return nil
	}
	for g, targets := range fanout {
		if strings.ToLower(strings.TrimSpace(g)) == key && len(targets) > 0 {
			return targets
		}
	}
	return []string{group}
}

// IncidentCreatedAlert is SendIncidentCreatedAlert's card content.
type IncidentCreatedAlert struct {
	// IncidentID keys the card's thread (IncidentThreadKey).
	IncidentID       string
	Number           string
	Team             string
	Priority         string
	ShortDescription string
	Service          string
	Environment      string
	Category         string
	State            string
	IncidentLink     string
}

// IncidentAssignedAlert is SendIncidentAssignedAlert's card content.
type IncidentAssignedAlert struct {
	// IncidentID keys the card's thread (IncidentThreadKey).
	IncidentID   string
	Number       string
	AssigneeName string
	UpdatedBy    string
	IncidentLink string
}

// incidentPriorityLabels renders an incident priority the way ServiceNow's incident card does.
var incidentPriorityLabels = map[string]struct{ word, label string }{
	"CRITICAL": {"Critical", "P1 - Critical"},
	"HIGH":     {"High", "P2 - High"},
	"MODERATE": {"Moderate", "P3 - Moderate"},
	"LOW":      {"Low", "P4 - Low"},
	"PLANNING": {"Planning", "P5 - Planning"},
	"P1":       {"Critical", "P1 - Critical"},
	"P2":       {"High", "P2 - High"},
	"P3":       {"Moderate", "P3 - Moderate"},
	"P4":       {"Low", "P4 - Low"},
	"P5":       {"Planning", "P5 - Planning"},
}

// SendIncidentCreatedAlert posts ServiceNow's "<team> | <priority> Priority Incident Reported" card.
func (c *GoogleChatClient) SendIncidentCreatedAlert(ctx context.Context, audience string, a IncidentCreatedAlert) error {
	if a.IncidentID == "" {
		return fmt.Errorf("notifications: incidentId is required")
	}
	priority := incidentPriorityLabels[strings.ToUpper(strings.TrimSpace(a.Priority))]
	title := "Priority Incident Reported"
	if priority.word != "" {
		title = priority.word + " " + title
	}
	if strings.TrimSpace(a.Team) != "" {
		title = strings.ToUpper(strings.TrimSpace(a.Team)) + " | " + title
	}
	subtitle := "#" + incidentRef(a.Number, a.IncidentID)
	for _, part := range []string{a.Service, a.Environment} {
		if strings.TrimSpace(part) != "" {
			subtitle += " | " + strings.TrimSpace(part)
		}
	}

	summary := []chatCardWidget{{TextParagraph: &chatTextParagraph{
		Text: caseAlertLine(`<b>Short Description:</b><br>%s`, orEmDash(a.ShortDescription)),
	}}}
	if a.IncidentLink != "" {
		summary = append(summary, chatCardWidget{ButtonList: &chatButtonList{
			Buttons: []chatButton{{Text: "View Incident", OnClick: chatOnClick{OpenLink: chatOpenLink{URL: a.IncidentLink}}}},
		}})
	}
	details := []string{
		caseAlertLine(`<b>Category:</b> %s`, orEmDash(strings.ToLower(a.Category))),
		caseAlertLine(`<b>Priority:</b> %s`, orEmDash(priority.label)),
		caseAlertLine(`<b>State:</b> %s`, orEmDash(humanizeEnumLabel(a.State))),
	}
	sections := []chatCardSection{
		{Widgets: summary},
		{
			Header:      "Incident Details",
			Collapsible: true,
			Widgets:     []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: strings.Join(details, "<br>")}}},
		},
	}

	msg := chatCardMessage{
		CardsV2: []chatCardWrapper{{CardID: "incident-created-alert", Card: chatCard{
			Header:   &chatCardHeader{Title: title, Subtitle: subtitle},
			Sections: sections,
		}}},
		Thread: &chatThread{ThreadKey: IncidentThreadKey(a.IncidentID)},
	}
	return c.sendCardToAudience(ctx, audience, a.Number, a.IncidentID, msg)
}

// SendIncidentAssignedAlert posts ServiceNow's "Incident Acknowledged." reply under the incident's card.
func (c *GoogleChatClient) SendIncidentAssignedAlert(ctx context.Context, audience string, a IncidentAssignedAlert) error {
	if a.IncidentID == "" {
		return fmt.Errorf("notifications: incidentId is required")
	}
	lines := []string{caseAlertLine(`Incident acknowledged by %s.`, orEmDash(a.AssigneeName))}
	if a.UpdatedBy != "" {
		lines = append(lines, caseAlertLine(`Incident updated by %s.`, a.UpdatedBy))
	}
	// The link keeps the reply usable on its own in a space that never got the card (the group changed since creation).
	if a.IncidentLink != "" {
		lines = append(lines, caseAlertLine(`<a href="%s">View incident</a>`, a.IncidentLink))
	}
	msg := chatCardMessage{
		CardsV2: []chatCardWrapper{{CardID: "incident-assigned-alert", Card: chatCard{
			Header:   &chatCardHeader{Title: "Incident Acknowledged.", Subtitle: incidentRef(a.Number, a.IncidentID)},
			Sections: []chatCardSection{{Widgets: []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: strings.Join(lines, "<br>")}}}}},
		}}},
		Thread: &chatThread{ThreadKey: IncidentThreadKey(a.IncidentID)},
	}
	return c.sendCardToAudience(ctx, audience, a.Number, a.IncidentID, msg)
}

// incidentRef is the incident's number, or its id when an older publisher sent no number.
func incidentRef(number, incidentID string) string {
	if strings.TrimSpace(number) != "" {
		return number
	}
	return incidentID
}

// IncidentThreadKey keeps an incident's created card and its replies, the paging rungs included, in one Chat thread.
func IncidentThreadKey(incidentID string) string {
	return "incident-" + incidentID
}

// humanizeEnumLabel turns an UPPER_SNAKE value such as IN_PROGRESS into "In Progress".
func humanizeEnumLabel(v string) string {
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(v)), "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
