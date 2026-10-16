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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sampleAlert() EscalationAlert {
	return EscalationAlert{
		Audience: "WSO2 API Manager", Rung: "L3", RungRole: "ABT lead",
		Attempt: 1, Priority: "P1", IncidentRef: "INC0012345",
		Title: "Gateway returning 500s in production", RecipientName: "Siluni Perera",
		Instruction: "Update the ticket status to Work In Progress to stop further notifications.",
		Rule:        "R2", PortalURL: "https://csm.example/operations/incidents/abc", Elapsed: "18m",
	}
}

// The header is what a reader sees first in a busy space, so the rung and the
// incident have to be in it.
func TestEscalationCard_Header(t *testing.T) {
	card := buildEscalationCard(sampleAlert()).CardsV2[0].Card
	if card.Header == nil {
		t.Fatal("the card has no header; the rung would not be visible at a glance")
	}
	if !strings.Contains(card.Header.Title, "L3") {
		t.Errorf("title = %q, want the rung in it", card.Header.Title)
	}
	if !strings.Contains(card.Header.Subtitle, "INC0012345") {
		t.Errorf("subtitle = %q, want the incident reference", card.Header.Subtitle)
	}
}

// LEVEL_0 is the notification level. Section 3.0 says it is "represented as
// the initial escalation level (but not an actual escalation level)", so a
// card claiming an incident was escalated to it contradicts the document and
// tells a reader it has been through a rung it has not.
func TestEscalationCard_Level0IsNotAnEscalation(t *testing.T) {
	a := sampleAlert()
	a.Rung = "L1"
	title := buildEscalationCard(a).CardsV2[0].Card.Header.Title
	if strings.Contains(title, "Escalated") {
		t.Errorf("title = %q; L1 is a notification, not an escalation", title)
	}
	if !strings.Contains(title, "Notifying") {
		t.Errorf("title = %q, want it to say the rotation is being notified", title)
	}

	a.Rung = "L4"
	if title := buildEscalationCard(a).CardsV2[0].Card.Header.Title; !strings.Contains(title, "Escalated") {
		t.Errorf("title = %q; a real rung is an escalation", title)
	}
}

// Everything a reader needs to act without opening anything.
func TestEscalationCard_Body(t *testing.T) {
	body := buildEscalationCard(sampleAlert()).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text
	for _, want := range []string{
		"ABT lead",
		"Siluni Perera",
		"Priority P1",
		"unattended 18m",
		"Update the ticket status",
		`<a href="https://csm.example/operations/incidents/abc">View incident</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the card body is missing %q:\n%s", want, body)
		}
	}
}

// A phone number must never reach a room: a space is an audience, and this
// repository keeps numbers out of places people read.
func TestEscalationCard_CarriesNoPhoneNumber(t *testing.T) {
	a := sampleAlert()
	a.RecipientName = "Siluni Perera"
	body := buildEscalationCard(a).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text
	if strings.Contains(body, "+94") || strings.Contains(body, "+1") {
		t.Errorf("a phone number reached the card:\n%s", body)
	}
}

// A recipient name is data from a roster, so it cannot be trusted to be free
// of the markup Chat's card text interprets.
func TestEscalationCard_EscapesDynamicValues(t *testing.T) {
	a := sampleAlert()
	a.RecipientName = `<b>evil</b> & co`
	a.Title = `</font><a href="http://evil">x</a>`
	card := buildEscalationCard(a).CardsV2[0].Card
	body := card.Sections[0].Widgets[0].TextParagraph.Text
	if strings.Contains(body, "<b>evil</b>") {
		t.Errorf("a recipient name broke out of its tag:\n%s", body)
	}
	if !strings.Contains(body, "&amp;") {
		t.Errorf("an ampersand was not escaped:\n%s", body)
	}
}

// Optional fields simply do not appear, rather than leaving a dangling label.
func TestEscalationCard_OmitsEmptyFields(t *testing.T) {
	a := sampleAlert()
	a.PortalURL, a.RecipientName, a.Rule = "", "", ""
	body := buildEscalationCard(a).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text
	for _, unwanted := range []string{"Calling", "View incident", "path "} {
		if strings.Contains(body, unwanted) {
			t.Errorf("an absent field still rendered %q:\n%s", unwanted, body)
		}
	}
}

// How long an incident has gone unattended is the one thing a card must
// always carry, because a space flattens time: cards minutes apart sit in a
// list looking the same as cards seconds apart. A zero elapsed is named
// rather than left as "unattended" with nothing after it.
func TestEscalationCard_AlwaysSaysHowLongUnattended(t *testing.T) {
	a := sampleAlert()
	body := buildEscalationCard(a).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text
	if !strings.Contains(body, "unattended 18m") {
		t.Errorf("the card does not say how long it has been unattended:\n%s", body)
	}

	a.Elapsed = ""
	body = buildEscalationCard(a).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text
	if !strings.Contains(body, "just raised") {
		t.Errorf("a zero elapsed should be named, not blank:\n%s", body)
	}
}

// And where it goes next, which is what turns a card from a notice into a
// deadline.
func TestEscalationCard_SaysWhatHappensNext(t *testing.T) {
	a := sampleAlert()
	a.NextRung, a.NextIn = "L4", "7m"
	body := buildEscalationCard(a).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text
	if !strings.Contains(body, "Escalates to L4") || !strings.Contains(body, "in 7m unless acknowledged") {
		t.Errorf("the card does not say where this goes next:\n%s", body)
	}

	// The final rung has nowhere to climb, and says so rather than going
	// quiet, which would read as the escalation having stopped.
	a.NextRung, a.NextIn = "", ""
	body = buildEscalationCard(a).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text
	if !strings.Contains(body, "final rung") {
		t.Errorf("the last rung does not say it is the last:\n%s", body)
	}
}

// Every rung of one incident belongs in one conversation, or a space scatters
// an unfolding escalation through everything else being posted.
func TestEscalationCard_ThreadsByIncident(t *testing.T) {
	a := sampleAlert()
	a.ThreadKey = "incident-escalation-abc"
	msg := buildEscalationCard(a)
	if msg.Thread == nil || msg.Thread.ThreadKey != "incident-escalation-abc" {
		t.Errorf("thread = %+v, want the incident's own key", msg.Thread)
	}

	a.ThreadKey = ""
	if buildEscalationCard(a).Thread != nil {
		t.Error("a card with no key must post standalone rather than into an empty thread")
	}
}

func TestSendEscalationAlert_Validation(t *testing.T) {
	c := NewGoogleChatClient(GoogleChatConfig{})
	a := sampleAlert()
	a.Rung = ""
	if err := c.SendEscalationAlert(context.Background(), a); err == nil {
		t.Error("expected an error with no rung")
	}
	a = sampleAlert()
	a.IncidentRef = ""
	if err := c.SendEscalationAlert(context.Background(), a); err == nil {
		t.Error("expected an error with no incident reference")
	}
}

// The whole thing, over HTTP, to the space its product routes to.
func TestSendEscalationAlert_PostsToTheSpace(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewGoogleChatClient(GoogleChatConfig{
		AudienceSpaces: []GoogleChatAudienceSpace{{Audience: "WSO2 API Manager", WebhookURL: srv.URL}},
	})
	if err := c.SendEscalationAlert(context.Background(), sampleAlert()); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nothing reached the space")
	}
	if _, ok := got["cardsV2"]; !ok {
		t.Errorf("the posted message is not a card: %v", got)
	}
}

// The call cannot always be placed - no telephony account, or a recipient the
// rota knows only by e-mail - so the card carries what it would have said.
func TestBuildEscalationCard_CarriesTheVoiceScript(t *testing.T) {
	script := "WSO2 Support Alert. Priority, P0."
	msg := buildEscalationCard(EscalationAlert{
		Rung: "L2", Priority: "P0", IncidentRef: "INC-1", VoiceScript: script,
	})

	sections := msg.CardsV2[0].Card.Sections
	if len(sections) != 2 {
		t.Fatalf("sections = %d, want the body and the voice script", len(sections))
	}
	if sections[1].Header == "" {
		t.Error("the voice section has no header; it reads as more of the card's own prose")
	}
	if got := sections[1].Widgets[0].TextParagraph.Text; !strings.Contains(got, script) {
		t.Errorf("voice section = %q, want it to contain %q", got, script)
	}
}

// Without one, the section is absent rather than empty: an empty labelled
// block reads as "the call says nothing".
func TestBuildEscalationCard_NoVoiceScriptNoSection(t *testing.T) {
	msg := buildEscalationCard(EscalationAlert{Rung: "L2", Priority: "P0", IncidentRef: "INC-1"})
	if got := len(msg.CardsV2[0].Card.Sections); got != 1 {
		t.Errorf("sections = %d, want only the body", got)
	}
}

// The script is quoted text from elsewhere. Markup inside it must render as
// characters, not as card formatting.
func TestBuildEscalationCard_VoiceScriptIsEscaped(t *testing.T) {
	msg := buildEscalationCard(EscalationAlert{
		Rung: "L2", IncidentRef: "INC-1", VoiceScript: `Account, <b>Acme</b> & Co.`,
	})
	got := msg.CardsV2[0].Card.Sections[1].Widgets[0].TextParagraph.Text
	if strings.Contains(got, "<b>Acme</b>") {
		t.Errorf("voice section = %q, want the markup escaped", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("voice section = %q, want the ampersand escaped", got)
	}
}

// Reading an escalation back afterwards needs more than "unattended 12m":
// which team, which rota window, when it was raised, when this rung went off,
// and who was supposed to have it first.
func TestBuildEscalationCard_CarriesTheContextAReaderNeeds(t *testing.T) {
	body := buildEscalationCard(EscalationAlert{
		Rung: "L2", Priority: "P0", IncidentRef: "WSO2-1042",
		Team: "Vega", Rotation: "CRE_EVENING",
		ReportedAt: "2026-09-28 19:42 IST", Firing: "2026-09-28 19:43 IST",
		OnCall: "Ashens",
	}).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text

	for _, want := range []string{"Vega", "CRE_EVENING", "2026-09-28 19:42 IST", "2026-09-28 19:43 IST", "Ashens"} {
		if !strings.Contains(body, want) {
			t.Errorf("card body missing %q:\n%s", want, body)
		}
	}
}

// The rung time sits beside the person called, so a thread reads as who was called when.
func TestBuildEscalationCard_CallingLineCarriesTheRungTime(t *testing.T) {
	body := buildEscalationCard(EscalationAlert{
		Rung: "L1", RungRole: "SaaS SRE L1 Support", Priority: "P0", IncidentRef: "WSO2-1042",
		RecipientName: "Jane Doe", Firing: "2026-10-13 15:21 IST",
	}).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text

	if !strings.Contains(body, "Calling Jane Doe (2026-10-13 15:21 IST)") {
		t.Errorf("card body missing the timed calling line:\n%s", body)
	}
	if strings.Contains(body, "This rung ") {
		t.Errorf("rung time repeated on its own line:\n%s", body)
	}
}

// An incident's rung is a short reply under its "Incident Reported" card, which already shows the incident.
func TestBuildEscalationReply(t *testing.T) {
	msg := buildEscalationReply(EscalationAlert{
		Rung: "L2", RungRole: "SaaS SRE L2 Support", IncidentRef: "INC0100914",
		RecipientName: "Jane Doe", Firing: "2026-10-13 15:26 IST", ThreadKey: "incident-abc",
	})
	card := msg.CardsV2[0].Card
	if card.Header.Title != "Escalated to L2" || card.Header.Subtitle != "#INC0100914" {
		t.Errorf("header = %q / %q", card.Header.Title, card.Header.Subtitle)
	}
	body := card.Sections[0].Widgets[0].TextParagraph.Text
	if body != "<b>SaaS SRE L2 Support</b><br>Calling Jane Doe (2026-10-13 15:26 IST)" {
		t.Errorf("body = %q", body)
	}
	if msg.Thread == nil || msg.Thread.ThreadKey != "incident-abc" {
		t.Errorf("thread = %+v, want the incident card's thread", msg.Thread)
	}
	if title := buildEscalationReply(EscalationAlert{Rung: "L1"}).CardsV2[0].Card.Header.Title; title != "Notifying L1" {
		t.Errorf("first rung title = %q, want Notifying L1", title)
	}
}

func TestBuildEscalationUnansweredReply(t *testing.T) {
	msg := buildEscalationUnansweredReply(EscalationUnanswered{
		Chain: "SaaS SRE", IncidentRef: "INC0100950", Called: []string{"L1 Jane Doe", "L2 John Roe"}, ThreadKey: "incident-abc",
	})
	card := msg.CardsV2[0].Card
	if card.Header.Title != "Unanswered" || card.Header.Subtitle != "#INC0100950" {
		t.Errorf("header = %q / %q", card.Header.Title, card.Header.Subtitle)
	}
	body := card.Sections[0].Widgets[0].TextParagraph.Text
	if body != "<b>Nobody acknowledged this incident</b><br>Called L1 Jane Doe, L2 John Roe<br>No further calls will be made." {
		t.Errorf("body = %q", body)
	}
	if msg.Thread == nil || msg.Thread.ThreadKey != "incident-abc" {
		t.Errorf("thread = %+v, want the incident card's thread", msg.Thread)
	}
}

func TestBuildEscalationHandoffReply(t *testing.T) {
	card := buildEscalationHandoffReply(EscalationHandoff{
		SMETeam: "Choreo Runtime", By: "lead@wso2.com", Paging: true, IncidentRef: "INC0100950",
	}).CardsV2[0].Card
	if card.Header.Title != "Escalated to Choreo Runtime SMEs" || card.Header.Subtitle != "#INC0100950" {
		t.Errorf("header = %q / %q", card.Header.Title, card.Header.Subtitle)
	}
	body := card.Sections[0].Widgets[0].TextParagraph.Text
	want := "<b>Escalated by lead@wso2.com</b>"
	if body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if body := buildEscalationHandoffReply(EscalationHandoff{SMETeam: "Choreo Runtime", IncidentRef: "INC1"}).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text; !strings.Contains(body, "No Choreo Runtime SME could be paged") {
		t.Errorf("unpaged body = %q", body)
	}
}

// Every one of them is absent on some rung or some deployment: a configured
// roster resolves no rota window, and only the first rung has an on-call
// name. A card must not carry an empty label for any of them.
func TestBuildEscalationCard_OmitsContextItDoesNotHave(t *testing.T) {
	body := buildEscalationCard(EscalationAlert{
		Rung: "L3", Priority: "P0", IncidentRef: "WSO2-1042",
	}).CardsV2[0].Card.Sections[0].Widgets[0].TextParagraph.Text

	for _, absent := range []string{"team ", "rotation ", "Reported ", "This rung ", "On call was"} {
		if strings.Contains(body, absent) {
			t.Errorf("card body has an empty %q label:\n%s", absent, body)
		}
	}
}

func TestBuildEscalationUnansweredCard(t *testing.T) {
	msg := buildEscalationUnansweredCard(EscalationUnanswered{
		Audience: "Apollo SRE", Chain: "SaaS SRE", IncidentRef: "INC0099001", Title: "Latency <alert>",
		Priority: "HIGH", Team: "Apollo", Called: []string{"L1 Jane", "L2 Ravi", "L3 Mia"},
		Instruction: "Assign the incident to yourself.", PortalURL: "https://csm.example/incidents/1", ThreadKey: "incident-escalation-1",
	})
	card := msg.CardsV2[0].Card
	if !strings.Contains(card.Header.Title, "Unanswered: SaaS SRE paging ended") {
		t.Errorf("title = %q", card.Header.Title)
	}
	if card.Header.Subtitle != "INC0099001 - Latency <alert>" {
		t.Errorf("subtitle = %q", card.Header.Subtitle)
	}
	body := card.Sections[0].Widgets[0].TextParagraph.Text
	for _, want := range []string{"Nobody acknowledged", "no further calls", "Called: L1 Jane, L2 Ravi, L3 Mia", "team Apollo", "View incident"} {
		if !strings.Contains(body, want) {
			t.Errorf("body has no %q: %s", want, body)
		}
	}
	if msg.Thread == nil || msg.Thread.ThreadKey != "incident-escalation-1" {
		t.Errorf("thread = %+v; want the ladder's own", msg.Thread)
	}
}

func TestBuildEscalationHandoffCard(t *testing.T) {
	for _, paging := range []bool{true, false} {
		msg := buildEscalationHandoffCard(EscalationHandoff{
			Audience: "Apollo SRE", SMETeam: "Choreo Runtime", By: "lead@example.com", Paging: paging,
			IncidentRef: "INC1", Title: "Latency", PortalURL: "https://csm.example/incidents/1", ThreadKey: "incident-escalation-1",
		})
		card := msg.CardsV2[0].Card
		if !strings.Contains(card.Header.Title, "Escalated to Choreo Runtime SMEs") {
			t.Errorf("title = %q", card.Header.Title)
		}
		body := card.Sections[0].Widgets[0].TextParagraph.Text
		want := "Paging the on-duty Choreo Runtime SMEs now"
		if !paging {
			want = "No Choreo Runtime SME could be paged"
		}
		for _, w := range []string{"Escalated by lead@example.com. SRE paging stopped.", want, "View incident"} {
			if !strings.Contains(body, w) {
				t.Errorf("paging=%v: body has no %q: %s", paging, w, body)
			}
		}
		if msg.Thread == nil || msg.Thread.ThreadKey != "incident-escalation-1" {
			t.Errorf("thread = %+v", msg.Thread)
		}
	}
}

func TestBuildEscalationStoppedCard(t *testing.T) {
	full := buildEscalationStoppedCard(EscalationStopped{
		Audience: "CRE Case Paging", Chain: "CRE", IncidentRef: "CS0379024", Title: "Duplicate <users>",
		Priority: "CRITICAL", Outcome: "Acknowledged", Detail: "Acknowledged by eng@wso2.com (assigned and a public comment).",
		Answered: true, Paged: []string{"L1 Jane", "L2 Ravi"}, Cancelled: 5,
		PortalURL: "https://csm.example/cases/1", PortalLabel: "View case", ThreadKey: "incident-escalation-1",
	})
	card := full.CardsV2[0].Card
	if card.Header.Title != "✅ Acknowledged: CRE paging stopped" {
		t.Errorf("title = %q", card.Header.Title)
	}
	if card.Header.Subtitle != "CS0379024 - Duplicate <users>" {
		t.Errorf("subtitle = %q", card.Header.Subtitle)
	}
	body := card.Sections[0].Widgets[0].TextParagraph.Text
	for _, want := range []string{"eng@wso2.com", "5 pending calls cancelled.", "Paged: L1 Jane, L2 Ravi", "Priority CRITICAL", "View case"} {
		if !strings.Contains(body, want) {
			t.Errorf("body has no %q: %s", want, body)
		}
	}
	if full.Thread == nil || full.Thread.ThreadKey != "incident-escalation-1" {
		t.Errorf("thread = %+v; want the ladder's own", full.Thread)
	}

	reply := buildEscalationStoppedCard(EscalationStopped{
		Audience: "Apollo SRE Group", Chain: "SaaS SRE", IncidentRef: "INC1", Title: "Latency", Priority: "HIGH",
		Outcome: "Severity changed to CRITICAL", Restarted: true, Cancelled: 1,
		PortalURL: "https://csm.example/incidents/1", Reply: true, ThreadKey: "incident-1",
	})
	card = reply.CardsV2[0].Card
	if card.Header.Title != "\U0001F501 Severity changed to CRITICAL: SaaS SRE paging restarts" || card.Header.Subtitle != "#INC1" {
		t.Errorf("reply header = %q / %q", card.Header.Title, card.Header.Subtitle)
	}
	body = card.Sections[0].Widgets[0].TextParagraph.Text
	if !strings.Contains(body, "1 pending call cancelled.") || strings.Contains(body, "Priority") || strings.Contains(body, "href") {
		t.Errorf("a reply under the incident card should stay short: %s", body)
	}

	closed := buildEscalationStoppedCard(EscalationStopped{IncidentRef: "CS1", Outcome: "Case closed"})
	if got := closed.CardsV2[0].Card.Header.Title; got != "⏹ Case closed: CRE paging stopped" {
		t.Errorf("closed title = %q", got)
	}
}
