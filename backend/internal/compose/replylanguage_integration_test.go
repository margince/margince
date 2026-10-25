// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	replyLangGerman  = "Vielen Dank für das Angebot, wir prüfen es mit unserem Team und melden uns nächste Woche bei Ihnen zurück."
	replyLangEnglish = "Thanks for the update, we plan to review the proposal with our team next week and come back to you."
)

var replyLangStart = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

// replyLangMail logs one email on the "angebot" thread, linked to a contact.
// The log path then stamps that contact as its sender or recipient.
func replyLangMail(t *testing.T, e *integration.Env, direction string, with ids.UUID, body string, offset time.Duration) crmcontracts.Activity {
	t.Helper()
	at, subject := replyLangStart.Add(offset), "Angebot"
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, ThreadKey: "angebot",
		Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: with}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

// replyLangContact is a contact Rep1 owns, reachable at one address.
func replyLangContact(t *testing.T, e *integration.Env, name, email string) ids.UUID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.UUID(contact.Id)
}

// replyLangDraft drafts a reply to the anchor as Rep1 and returns the language
// the model was told to write in.
func replyLangDraft(t *testing.T, e *integration.Env, anchor crmcontracts.Activity) string {
	t.Helper()
	lang, _ := replyLangPrompt(t, e, anchor)
	return lang
}

// replyLangPrompt is replyLangDraft with the whole prompt the model was sent.
func replyLangPrompt(t *testing.T, e *integration.Env, anchor crmcontracts.Activity) (string, string) {
	t.Helper()
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Angebot","body":"Gern."}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	drafter.envelope = draftfloor.NewResolver().
		WithClock(func() time.Time { return replyLangStart.Add(30 * 24 * time.Hour) }).
		WithUserLanguage(func(context.Context) string { return "vi" })
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	if _, err := drafter.DraftEmailWithProvenance(reader, ids.UUID(anchor.Id), draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	content := brain.request.Messages[0].Content
	for _, lang := range []string{"de", "en", "vi"} {
		if strings.Contains(content, `"output_language":"`+lang+`"`) {
			return lang, content
		}
	}
	t.Fatalf("the prompt names no output language: %s", content)
	return "", ""
}

// storeOldRuleLanguage records the label an older quote rule wrote at capture.
// No writer today stores English for German sender text, so the row is set by hand.
func storeOldRuleLanguage(t *testing.T, e *integration.Env, activity crmcontracts.Activity, lang string) {
	t.Helper()
	e.WsExec(t, `UPDATE activity SET language = $2 WHERE id = $1`, ids.UUID(activity.Id), lang)
}

func TestTheAddresseesGermanReplyStoredAsEnglishIsDraftedInGerman(t *testing.T) {
	e := integration.Setup(t)
	addressee := replyLangContact(t, e, "Dietmar Kunde", "dietmar@kunde.test")
	replyLangMail(t, e, "outbound", addressee, replyLangEnglish, 0)
	anchor := replyLangMail(t, e, "inbound", addressee, replyLangGerman, time.Hour)
	storeOldRuleLanguage(t, e, anchor, "en")
	if got := replyLangDraft(t, e, anchor); got != "de" {
		t.Fatalf("output language = %q, want the addressee's German over the stored English", got)
	}
}

func TestTheStoredLanguageHoldsWhenTheAddresseeWroteNothing(t *testing.T) {
	e := integration.Setup(t)
	addressee := replyLangContact(t, e, "Dietmar Kunde", "dietmar@kunde.test")
	anchor := replyLangMail(t, e, "outbound", addressee, replyLangGerman, 0)
	storeOldRuleLanguage(t, e, anchor, "en")
	if got := replyLangDraft(t, e, anchor); got != "en" {
		t.Fatalf("output language = %q, want the stored label while the addressee has written nothing", got)
	}
}

// longReplyThread is a thread whose only German is the addressee's first mail,
// older than a full page of English mail from a copied colleague.
func longReplyThread(t *testing.T, e *integration.Env) (oldest, anchor crmcontracts.Activity) {
	t.Helper()
	addressee := replyLangContact(t, e, "Dietmar Kunde", "dietmar@kunde.test")
	colleague := replyLangContact(t, e, "Anne Kollegin", "anne@kunde.test")
	oldest = replyLangMail(t, e, "inbound", addressee, replyLangGerman, 0)
	for i := range replyEvidenceRows + 1 {
		replyLangMail(t, e, "inbound", colleague, replyLangEnglish, time.Duration(i+1)*time.Minute)
	}
	anchor = replyLangMail(t, e, "inbound", addressee, "ok", 2*time.Hour)
	return oldest, anchor
}

func TestTheAddresseesMailOlderThanTheThreadPageStillDecidesTheLanguage(t *testing.T) {
	e := integration.Setup(t)
	_, anchor := longReplyThread(t, e)
	if got := replyLangDraft(t, e, anchor); got != "de" {
		t.Fatalf("output language = %q, want the addressee's German from before the newest %d messages", got, replyEvidenceRows)
	}
}

func TestTheAddresseesMailTheDrafterMayNotReadDoesNotDecideTheLanguage(t *testing.T) {
	e := integration.Setup(t)
	oldest, anchor := longReplyThread(t, e)
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, ids.UUID(oldest.Id))
	if got := replyLangDraft(t, e, anchor); got != "en" {
		t.Fatalf("output language = %q, want English: the German mail is limited to participants the drafter is not among", got)
	}
}

func TestAnOutboundReplyFollowsTheContactItIsDeliveredToNotTheGreetedSender(t *testing.T) {
	e := integration.Setup(t)
	buyer := replyLangContact(t, e, "Dietmar Kunde", "dietmar@kunde.test")
	seat := replyLangContact(t, e, "Sofia Meier", "sofia.private@mail.test")
	replyLangMail(t, e, "inbound", buyer, replyLangGerman, 0)
	replyLangMail(t, e, "inbound", seat, replyLangEnglish, time.Hour)
	anchor := replyLangMail(t, e, "outbound", buyer, "ok", 2*time.Hour)
	// Our rep sent the anchor and is on file as a contact too. Capture writes
	// such a seat's participant row with both the user and the contact.
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, user_id, contact_id) VALUES ($1, 'from', $2, $3)`,
		ids.UUID(anchor.Id), e.Rep1, seat)
	recipient, err := e.Activities.ReplyRecipientFor(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(anchor.Id)))
	if err != nil || recipient.FullName != "Sofia Meier" {
		t.Fatalf("precondition: the greeting names %+v (err %v), want the seat's contact", recipient, err)
	}
	if got := replyLangDraft(t, e, anchor); got != "de" {
		t.Fatalf("output language = %q, want the German of the buyer the reply is delivered to", got)
	}
}

func TestASharedMailboxsOwnTextOutranksAStoredLabelWhileTheGreetingStaysNameless(t *testing.T) {
	e := integration.Setup(t)
	mailbox := replyLangContact(t, e, "Steireif Anna", "anna@steireif.test")
	anchor := replyLangMail(t, e, "inbound", mailbox, replyLangGerman, 0)
	storeOldRuleLanguage(t, e, anchor, "en")
	lang, prompt := replyLangPrompt(t, e, anchor)
	if lang != "de" {
		t.Fatalf("output language = %q, want the shared mailbox's own German over the stored English", lang)
	}
	if strings.Contains(prompt, `"recipient":`) {
		t.Fatalf("a shared mailbox is greeted by name: %s", prompt)
	}
}

func TestAnAddresseeMessageDatedAfterTheDraftingInstantDoesNotDecideTheLanguage(t *testing.T) {
	e := integration.Setup(t)
	addressee := replyLangContact(t, e, "Dietmar Kunde", "dietmar@kunde.test")
	colleague := replyLangContact(t, e, "Anne Kollegin", "anne@kunde.test")
	replyLangMail(t, e, "inbound", colleague, replyLangEnglish, 0)
	anchor := replyLangMail(t, e, "inbound", addressee, "ok", time.Hour)
	replyLangMail(t, e, "inbound", addressee, replyLangGerman, 60*24*time.Hour)
	if got := replyLangDraft(t, e, anchor); got != "en" {
		t.Fatalf("output language = %q, want English: the German mail is dated after the draft is written", got)
	}
}
