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
	"html"
	"strings"
)

// EscalationAlert is one rung of an incident call-escalation ladder, delivered
// to a Google Chat space instead of (or alongside) a phone call.
//
// READ THIS BEFORE TREATING CHAT AS THE PAGING CHANNEL. The ladder exists to
// wake someone up. A chat message does not: the specification already sends
// one to the Incident Monitor space the moment an incident is raised, and the
// ladder exists precisely because that was not enough to get an unattended
// incident attended to at three in the morning. Delivering the ladder itself
// over chat changes it from a pager into a louder version of the notification
// that already failed.
//
// It is still worth having, for two reasons. It is the only channel that can
// be exercised end to end without a telephony account, which is what makes the
// flow demonstrable today. And it gives the room visibility of an escalation
// in progress, which the calls themselves do not: a call reaches one person,
// while the card shows everyone which rung an incident has reached and who is
// being asked to pick it up.
type EscalationAlert struct {
	// Audience is the GOOGLE_CHAT_SPACES key the card is posted to. It used to
	// be the incident's product, from when Chat routed by product; routing is
	// by audience now, and no audience is ever named after a product, so every
	// card was dropped. The escalation ladder sets it from its own
	// configuration (paging-alert.yaml chat.audience).
	Audience string
	// Rung is the level being contacted, e.g. "L2".
	Rung string
	// RungRole names who that rung is, e.g. "ABT team leads".
	RungRole string
	// Attempt is which notification of this rung it is, 1-based.
	//
	// Not rendered. The escalation engine posts one card per rung, so every
	// card it builds is attempt 1, and a line reporting that would say
	// nothing. Kept on the struct because it is part of what a rung is, and a
	// caller outside that flow may have a reason to show it.
	Attempt int
	// Priority is the incident's priority, which sets the whole clock.
	Priority string
	// IncidentRef is the human-readable reference, e.g. "INC0012345".
	IncidentRef string
	// Title is the incident subject, for recognising it at a glance.
	Title string
	// RecipientName is who this rung resolved to. Deliberately a name and
	// never a phone number: a space is a room, and the ladder's own logs
	// already keep numbers out of places people read.
	RecipientName string
	// Instruction is what stops the ladder, which differs by trigger.
	Instruction string
	// Rule is the section 5.0 row that selected these recipients, so the room
	// can see which path an alert took.
	Rule string
	// PortalURL opens the incident, or the customer case a case ladder pages
	// about.
	PortalURL string
	// PortalLabel is the link's text; empty is "View incident".
	PortalLabel string
	// Elapsed is how long the incident has been unattended, e.g. "18m".
	// Empty means the ladder has only just started.
	Elapsed string
	// NextRung and NextIn say where this goes if nobody picks it up, e.g.
	// "L3" and "7m". Both empty on the final rung, which has nowhere
	// left to climb.
	//
	// These exist because a space flattens time. Cards arrive minutes apart
	// but sit in a list, and a reader scanning it cannot see from the cards
	// alone whether an incident is escalating quickly or has been quietly
	// stuck; the timestamps are there but easy to miss and hard to compare.
	// Saying how long it has gone unattended and when it climbs next puts the
	// urgency in the card rather than leaving it to be inferred.
	NextRung string
	NextIn   string
	// ThreadKey groups every rung of one incident's ladder into a single
	// conversation, so the space shows an escalation unfolding in one place
	// instead of scattering it through everything else being posted.
	ThreadKey string
	// Reply renders the rung as a short reply under the incident's "Incident Reported" card, which already carries the incident's details.
	Reply bool
	// VoiceScript is what the call would say, word for word.
	//
	// The card carries it because the call often cannot be placed: a
	// deployment with no telephony account, or a recipient the rota knows by
	// e-mail and has no number for, still needs somebody to be able to read
	// what the alert would have said. It is the spoken text verbatim, so the
	// case number appears spelled out character by character - that is not a
	// rendering mistake, it is what the recipient hears, and seeing it is the
	// point.
	//
	// Empty leaves the section off entirely.
	VoiceScript string

	// Team is who the incident is assigned to.
	Team string
	// ReportedAt is when the incident was raised, and Firing is when this
	// rung went off. Both absolute and both in one zone, because "unattended
	// 12m" answers how long but never when, and a reader piecing an
	// escalation together afterwards needs when.
	ReportedAt string
	Firing     string
	// Rotation is the rota window covering the incident, as the schedule
	// names it: CRE_EVENING, CRE_AMERICAS. Empty when the ladder did not
	// resolve anyone from the rota, which is every rung but the first and
	// every deployment still reading a configured roster.
	Rotation string
	// OnCall is who the first rung reached, carried on every later card.
	// By LEVEL_3 the question a reader has is not only "who is being called
	// now" but "who was supposed to have this an hour ago".
	OnCall string
}

// SendEscalationAlert posts one rung of the ladder to the product's space.
//
// The card is deliberately unlike this package's case.* cards: those announce
// something that happened, while this one is asking the room to act, so the
// rung and the instruction carry the weight and everything else is context.
func (c *GoogleChatClient) SendEscalationAlert(ctx context.Context, a EscalationAlert) error {
	if a.Rung == "" {
		return fmt.Errorf("notifications: rung is required")
	}
	if a.IncidentRef == "" {
		return fmt.Errorf("notifications: incidentRef is required")
	}
	// Routed by audience, as every card here now is. The ladder still names
	// its target by product, so the product string is the audience key --
	// unchanged behaviour under the renamed API, not a new routing scheme
	// invented during a rebase.
	//
	// sendCardToAudience treats an unconfigured audience as a no-op success
	// (warn, return nil). Right for an announcement -- a not-yet-onboarded
	// team must not fail the whole delivery -- and wrong for a rung, which
	// would then be recorded as delivered while reaching nobody. The
	// escalation notifier therefore checks HasAudienceSpace itself before it
	// gets here and records NO_CHAT_SPACE instead; see
	// escalation.chatNotifier.Deliver.
	if strings.TrimSpace(a.Audience) == "" {
		return fmt.Errorf("notifications: audience is required")
	}
	if a.Reply {
		return c.sendCardToAudience(ctx, a.Audience, a.IncidentRef, "", buildEscalationReply(a))
	}
	return c.sendCardToAudience(ctx, a.Audience, a.IncidentRef, "", buildEscalationCard(a))
}

// buildEscalationReply is a rung as a reply under the incident card: the rung, who it is, and who is being called when.
func buildEscalationReply(a EscalationAlert) chatCardMessage {
	lines := []string{caseAlertLine("<b>%s</b>", a.RungRole)}
	if a.RecipientName != "" {
		calling := caseAlertLine("Calling %s", a.RecipientName)
		if a.Firing != "" {
			calling += caseAlertLine(" (%s)", a.Firing)
		}
		lines = append(lines, calling)
	}
	return escalationReply("incident-escalation-rung", rungVerb(a.Rung)+" "+a.Rung, a.IncidentRef, a.ThreadKey, lines)
}

// buildEscalationCard assembles the card, split from the send so its exact
// shape can be asserted on without a webhook.
func buildEscalationCard(a EscalationAlert) chatCardMessage {

	header := fmt.Sprintf("%s %s %s", escalationSeverityMark(a.Priority), rungVerb(a.Rung), a.Rung)
	subtitle := a.IncidentRef
	if a.Title != "" {
		subtitle = a.IncidentRef + " - " + a.Title
	}

	var body strings.Builder
	body.WriteString(caseAlertLine("<b>%s</b>", a.RungRole))
	if a.RecipientName != "" {
		body.WriteString("<br>")
		body.WriteString(caseAlertLine("Calling %s", a.RecipientName))
		if a.Firing != "" {
			body.WriteString(caseAlertLine(" (%s)", a.Firing))
		}
	}
	body.WriteString("<br>")
	body.WriteString(caseAlertLine(`<font color="#5F6368">Priority %s</font>`, a.Priority))
	if a.Team != "" {
		body.WriteString(caseAlertLine(`<font color="#5F6368"> - team %s</font>`, a.Team))
	}
	if a.Rotation != "" {
		body.WriteString(caseAlertLine(`<font color="#5F6368"> - rotation %s</font>`, a.Rotation))
	}
	body.WriteString(caseAlertLine(`<font color="#5F6368"> - unattended %s</font>`, elapsedOrNew(a.Elapsed)))
	if a.Rule != "" {
		body.WriteString(caseAlertLine(`<font color="#5F6368"> - path %s</font>`, a.Rule))
	}
	if a.NextRung != "" && a.NextIn != "" {
		body.WriteString("<br>")
		body.WriteString(caseAlertLine(`<font color="#B3261E">Escalates to %s`, a.NextRung))
		body.WriteString(caseAlertLine(` in %s unless acknowledged</font>`, a.NextIn))
	} else if a.NextRung == "" {
		body.WriteString("<br>")
		body.WriteString(`<font color="#B3261E">This is the final rung. Nothing escalates past it.</font>`)
	}
	if a.ReportedAt != "" || (a.Firing != "" && a.RecipientName == "") || a.OnCall != "" {
		body.WriteString("<br>")
	}
	if a.ReportedAt != "" {
		body.WriteString(caseAlertLine(`<br><font color="#5F6368">Reported %s</font>`, a.ReportedAt))
	}
	if a.Firing != "" && a.RecipientName == "" {
		body.WriteString(caseAlertLine(`<br><font color="#5F6368">This rung %s</font>`, a.Firing))
	}
	if a.OnCall != "" {
		body.WriteString(caseAlertLine(`<br><font color="#5F6368">On call was %s</font>`, a.OnCall))
	}
	if a.Instruction != "" {
		body.WriteString("<br><br>")
		body.WriteString(caseAlertLine("<b>%s</b>", a.Instruction))
	}
	if a.PortalURL != "" {
		body.WriteString("<br>")
		label := strings.TrimSpace(a.PortalLabel)
		if label == "" {
			label = "View incident"
		}
		body.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, a.PortalURL, html.EscapeString(label)))
	}

	sections := []chatCardSection{{
		Widgets: []chatCardWidget{
			{TextParagraph: &chatTextParagraph{Text: body.String()}},
		},
	}}
	if a.VoiceScript != "" {
		// Its own section, not another line in the body: this is a quotation
		// of something said elsewhere, and running it into the card's own
		// prose invites reading it as more instructions from the card.
		sections = append(sections, chatCardSection{
			Header: "What the call says",
			Widgets: []chatCardWidget{
				{TextParagraph: &chatTextParagraph{
					Text: fmt.Sprintf(`<i><font color="#5F6368">%s</font></i>`,
						html.EscapeString(a.VoiceScript)),
				}},
			},
		})
	}

	msg := chatCardMessage{
		Thread: threadFor(a.ThreadKey),
		CardsV2: []chatCardWrapper{{
			CardID: "incident-escalation",
			Card: chatCard{
				Header:   &chatCardHeader{Title: header, Subtitle: subtitle},
				Sections: sections,
			},
		}},
	}
	return msg
}

// elapsedOrNew renders how long an incident has gone unattended, naming the
// zero case rather than leaving a card that says "unattended" and stops.
func elapsedOrNew(elapsed string) string {
	if elapsed == "" {
		return "just raised"
	}
	return elapsed
}

// threadFor groups a ladder's rungs, or leaves the message standalone when no
// key is given.
func threadFor(key string) *chatThread {
	if key == "" {
		return nil
	}
	return &chatThread{ThreadKey: key}
}

// rungVerb keeps the header honest about what L1 (LEVEL_0) is.
//
// Section 3.0 calls it the notification level and says in as many words that
// it is "represented as the initial escalation level (but not an actual
// escalation level)" - it exists to tell the rotation an incident has arrived,
// before any escalation has happened. A card announcing "Escalated to L1"
// contradicts the document it implements, and tells a reader the incident has
// already been through a rung it has not.
func rungVerb(rung string) string {
	if rung == "L1" {
		return "Notifying"
	}
	return "Escalated to"
}

// escalationSeverityMark gives the header a glyph that reads at a glance in a
// busy space. Only the two fastest priorities get the loud one: if every rung
// of every incident shouted, none of them would.
func escalationSeverityMark(priority string) string {
	switch strings.ToUpper(strings.TrimSpace(priority)) {
	case "P0", "P1", "CATASTROPHIC", "CRITICAL":
		return "\U0001F534" // red circle
	default:
		return "\U0001F7E0" // orange circle
	}
}

// EscalationUnanswered is the closing message of a ladder that ran past its
// last rung with nobody acknowledging the incident: the calls are over, and
// the team's space is told so.
type EscalationUnanswered struct {
	// Audience is the GOOGLE_CHAT_SPACES key the card is posted to.
	Audience string
	// Chain names the chain that was paged, e.g. "SaaS SRE"; empty is "SRE".
	Chain string
	// IncidentRef, Title, Priority and Team describe the incident.
	IncidentRef string
	Title       string
	Priority    string
	Team        string
	// Called is who each rung reached, in order, e.g. "L1 Jane Doe".
	Called []string
	// Instruction is what still stops the paging gesture-wise, for whoever
	// picks the incident up now.
	Instruction string
	// PortalURL opens the incident; PortalLabel is its text ("View incident"
	// when empty).
	PortalURL   string
	PortalLabel string
	// ThreadKey puts the message in the ladder's own thread, under its rung
	// cards when the ladder posted any.
	ThreadKey string
	// Reply posts it as a short reply under the incident card, like the rung replies.
	Reply bool
}

// SendEscalationUnanswered posts the closing message of an unanswered ladder.
func (c *GoogleChatClient) SendEscalationUnanswered(ctx context.Context, u EscalationUnanswered) error {
	if u.IncidentRef == "" {
		return fmt.Errorf("notifications: incidentRef is required")
	}
	if strings.TrimSpace(u.Audience) == "" {
		return fmt.Errorf("notifications: audience is required")
	}
	if u.Reply {
		return c.sendCardToAudience(ctx, u.Audience, u.IncidentRef, "", buildEscalationUnansweredReply(u))
	}
	return c.sendCardToAudience(ctx, u.Audience, u.IncidentRef, "", buildEscalationUnansweredCard(u))
}

// buildEscalationUnansweredReply is the closing message as a reply under the incident card: who was called, and that the calls are over.
func buildEscalationUnansweredReply(u EscalationUnanswered) chatCardMessage {
	lines := []string{caseAlertLine("<b>Nobody acknowledged this incident</b>")}
	if len(u.Called) > 0 {
		lines = append(lines, caseAlertLine("Called %s", strings.Join(u.Called, ", ")))
	}
	lines = append(lines, caseAlertLine("No further calls will be made."))
	return escalationReply("incident-escalation-unanswered", "Unanswered", u.IncidentRef, u.ThreadKey, lines)
}

// escalationReply is the compact card every ladder message uses under the incident card.
func escalationReply(cardID, title, incidentRef, threadKey string, lines []string) chatCardMessage {
	return chatCardMessage{
		Thread: threadFor(threadKey),
		CardsV2: []chatCardWrapper{{
			CardID: cardID,
			Card: chatCard{
				Header:   &chatCardHeader{Title: title, Subtitle: "#" + incidentRef},
				Sections: []chatCardSection{{Widgets: []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: strings.Join(lines, "<br>")}}}}},
			},
		}},
	}
}

// buildEscalationUnansweredCard assembles the closing card, split from the
// send so its shape can be asserted on without a webhook.
func buildEscalationUnansweredCard(u EscalationUnanswered) chatCardMessage {
	chain := strings.TrimSpace(u.Chain)
	if chain == "" {
		chain = "SRE"
	}
	subtitle := u.IncidentRef
	if u.Title != "" {
		subtitle = u.IncidentRef + " - " + u.Title
	}

	var body strings.Builder
	body.WriteString(caseAlertLine(`<font color="#B3261E"><b>Nobody acknowledged this incident.</b></font>`))
	body.WriteString("<br>")
	body.WriteString(caseAlertLine("The %s paging calls have ended; no further calls will be made.", chain))
	if len(u.Called) > 0 {
		body.WriteString("<br>")
		body.WriteString(caseAlertLine("Called: %s", strings.Join(u.Called, ", ")))
	}
	body.WriteString("<br>")
	body.WriteString(caseAlertLine(`<font color="#5F6368">Priority %s</font>`, u.Priority))
	if u.Team != "" {
		body.WriteString(caseAlertLine(`<font color="#5F6368"> - team %s</font>`, u.Team))
	}
	if u.Instruction != "" {
		body.WriteString("<br><br>")
		body.WriteString(caseAlertLine("<b>%s</b>", u.Instruction))
	}
	if u.PortalURL != "" {
		body.WriteString("<br>")
		label := strings.TrimSpace(u.PortalLabel)
		if label == "" {
			label = "View incident"
		}
		body.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, u.PortalURL, html.EscapeString(label)))
	}

	return chatCardMessage{
		Thread: threadFor(u.ThreadKey),
		CardsV2: []chatCardWrapper{{
			CardID: "incident-escalation-unanswered",
			Card: chatCard{
				Header: &chatCardHeader{
					Title:    fmt.Sprintf("%s Unanswered: %s paging ended", escalationSeverityMark(u.Priority), chain),
					Subtitle: subtitle,
				},
				Sections: []chatCardSection{{
					Widgets: []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: body.String()}}},
				}},
			},
		}},
	}
}

// EscalationHandoff is the one message an SRE space gets when an incident of
// its team is escalated to an SME team: the SRE calls have stopped, and who
// is paged now.
type EscalationHandoff struct {
	// Audience is the GOOGLE_CHAT_SPACES key the card is posted to.
	Audience string
	// SMETeam names the SME team paged now, e.g. "Choreo Runtime".
	SMETeam string
	// By is who escalated it (the incident's changedBy).
	By string
	// Paging is true when the SME ladder started; false when nobody on the
	// SME team could be paged (the incident's work notes say why).
	Paging bool
	// IncidentRef, Title and Priority describe the incident.
	IncidentRef string
	Title       string
	Priority    string
	// PortalURL opens the incident; PortalLabel is its text.
	PortalURL   string
	PortalLabel string
	// ThreadKey puts the message in the incident's ladder thread.
	ThreadKey string
	// Reply posts it as a short reply under the incident card, like the rung replies.
	Reply bool
}

// SendEscalationHandoff posts the escalated-to-SME message.
func (c *GoogleChatClient) SendEscalationHandoff(ctx context.Context, h EscalationHandoff) error {
	if h.IncidentRef == "" {
		return fmt.Errorf("notifications: incidentRef is required")
	}
	if strings.TrimSpace(h.Audience) == "" {
		return fmt.Errorf("notifications: audience is required")
	}
	if h.Reply {
		return c.sendCardToAudience(ctx, h.Audience, h.IncidentRef, "", buildEscalationHandoffReply(h))
	}
	return c.sendCardToAudience(ctx, h.Audience, h.IncidentRef, "", buildEscalationHandoffCard(h))
}

// buildEscalationHandoffReply is the escalated-to-SME message as a reply under the incident card.
func buildEscalationHandoffReply(h EscalationHandoff) chatCardMessage {
	team := strings.TrimSpace(h.SMETeam)
	if team == "" {
		team = "the specialist team"
	}
	lines := []string{caseAlertLine("<b>SRE paging stopped</b>")}
	if h.By != "" {
		lines = []string{caseAlertLine("<b>Escalated by %s</b>", h.By)}
	}
	if !h.Paging {
		lines = append(lines, caseAlertLine(`<font color="#B3261E">No %s SME could be paged; see the incident's work notes.</font>`, team))
	}
	return escalationReply("incident-escalation-handoff", "Escalated to "+team+" SMEs", h.IncidentRef, h.ThreadKey, lines)
}

// buildEscalationHandoffCard assembles the escalated-to-SME card.
func buildEscalationHandoffCard(h EscalationHandoff) chatCardMessage {
	team := strings.TrimSpace(h.SMETeam)
	if team == "" {
		team = "the specialist team"
	}
	subtitle := h.IncidentRef
	if h.Title != "" {
		subtitle = h.IncidentRef + " - " + h.Title
	}
	var body strings.Builder
	if h.By != "" {
		body.WriteString(caseAlertLine("Escalated by %s. SRE paging stopped.", h.By))
	} else {
		body.WriteString(caseAlertLine("SRE paging stopped."))
	}
	body.WriteString("<br>")
	if h.Paging {
		body.WriteString(caseAlertLine("Paging the on-duty %s SMEs now: L1, then L2 and L3, until someone is assigned.", team))
	} else {
		body.WriteString(caseAlertLine(`<font color="#B3261E">No %s SME could be paged; see the incident's work notes.</font>`, team))
	}
	if h.Priority != "" {
		body.WriteString("<br>")
		body.WriteString(caseAlertLine(`<font color="#5F6368">Priority %s</font>`, h.Priority))
	}
	if h.PortalURL != "" {
		body.WriteString("<br>")
		label := strings.TrimSpace(h.PortalLabel)
		if label == "" {
			label = "View incident"
		}
		body.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, h.PortalURL, html.EscapeString(label)))
	}
	return chatCardMessage{
		Thread: threadFor(h.ThreadKey),
		CardsV2: []chatCardWrapper{{
			CardID: "incident-escalation-handoff",
			Card: chatCard{
				Header: &chatCardHeader{
					Title:    fmt.Sprintf("%s Escalated to %s SMEs", escalationSeverityMark(h.Priority), team),
					Subtitle: subtitle,
				},
				Sections: []chatCardSection{{
					Widgets: []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: body.String()}}},
				}},
			},
		}},
	}
}

// EscalationStopped is the reply a ladder posts in its thread when it stops:
// somebody acknowledged it, the case closed, or a severity or priority change
// replaced it with a new chain. Without it the rung cards are the last thing
// the room sees, and nobody can tell a chain that was answered from one still
// climbing.
type EscalationStopped struct {
	// Audience is the GOOGLE_CHAT_SPACES key the reply is posted to.
	Audience string
	// Chain names the chain that stopped, e.g. "CRE" or "SaaS SRE".
	Chain string
	// IncidentRef, Title and Priority describe the record.
	IncidentRef string
	Title       string
	Priority    string
	// Outcome is what happened, as the header says it: "Acknowledged",
	// "Case closed", "Severity changed to S2".
	Outcome string
	// Detail is one sentence on it, e.g. who acknowledged and how.
	Detail string
	// Answered marks an acknowledgement; Restarted a new chain replacing this
	// one. Each changes the header's mark and wording.
	Answered  bool
	Restarted bool
	// Paged is who each rung reached, in order, e.g. "L1 Jane Doe".
	Paged []string
	// Cancelled is how many calls were still to come and will not be made.
	Cancelled int
	// PortalURL opens the record; PortalLabel is its text.
	PortalURL   string
	PortalLabel string
	// ThreadKey puts the reply under the ladder's rung cards.
	ThreadKey string
	// Reply renders it short, under an incident card that already carries
	// the record's details, as the rung replies are.
	Reply bool
}

// SendEscalationStopped posts a stopped ladder's reply.
func (c *GoogleChatClient) SendEscalationStopped(ctx context.Context, s EscalationStopped) error {
	if s.IncidentRef == "" {
		return fmt.Errorf("notifications: incidentRef is required")
	}
	if strings.TrimSpace(s.Audience) == "" {
		return fmt.Errorf("notifications: audience is required")
	}
	return c.sendCardToAudience(ctx, s.Audience, s.IncidentRef, "", buildEscalationStoppedCard(s))
}

// buildEscalationStoppedCard assembles the stopped reply.
func buildEscalationStoppedCard(s EscalationStopped) chatCardMessage {
	chain := strings.TrimSpace(s.Chain)
	if chain == "" {
		chain = "CRE"
	}
	var title string
	switch {
	case s.Restarted:
		title = fmt.Sprintf("\U0001F501 %s: %s paging restarts", s.Outcome, chain) // repeat arrows
	case s.Answered:
		title = fmt.Sprintf("✅ %s: %s paging stopped", s.Outcome, chain) // check mark
	default:
		title = fmt.Sprintf("⏹ %s: %s paging stopped", s.Outcome, chain) // stop button
	}
	subtitle := "#" + s.IncidentRef
	if !s.Reply && s.Title != "" {
		subtitle = s.IncidentRef + " - " + s.Title
	}

	var lines []string
	if s.Detail != "" {
		lines = append(lines, caseAlertLine("%s", s.Detail))
	}
	switch {
	case s.Cancelled == 1:
		lines = append(lines, caseAlertLine("1 pending call cancelled."))
	case s.Cancelled > 1:
		lines = append(lines, caseAlertLine("%s pending calls cancelled.", fmt.Sprint(s.Cancelled)))
	}
	if len(s.Paged) > 0 {
		lines = append(lines, caseAlertLine("Paged: %s", strings.Join(s.Paged, ", ")))
	}
	if !s.Reply {
		if s.Priority != "" {
			lines = append(lines, caseAlertLine(`<font color="#5F6368">Priority %s</font>`, s.Priority))
		}
		if s.PortalURL != "" {
			label := strings.TrimSpace(s.PortalLabel)
			if label == "" {
				label = "View incident"
			}
			lines = append(lines, fmt.Sprintf(`<a href="%s">%s</a>`, s.PortalURL, html.EscapeString(label)))
		}
	}

	return chatCardMessage{
		Thread: threadFor(s.ThreadKey),
		CardsV2: []chatCardWrapper{{
			CardID: "incident-escalation-stopped",
			Card: chatCard{
				Header:   &chatCardHeader{Title: title, Subtitle: subtitle},
				Sections: []chatCardSection{{Widgets: []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: strings.Join(lines, "<br>")}}}}},
			},
		}},
	}
}
