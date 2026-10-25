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

// replyLangDraft drafts a reply to the anchor as Rep1 and returns the language
// the model was told to write in.
func replyLangDraft(t *testing.T, e *integration.Env, anchor crmcontracts.Activity) string {
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
			return lang
		}
	}
	t.Fatalf("the prompt names no output language: %s", content)
	return ""
}

// storeOldRuleLanguage records the label an older quote rule wrote at capture.
// No writer today stores English for German sender text, so the row is set by hand.
func storeOldRuleLanguage(t *testing.T, e *integration.Env, activity crmcontracts.Activity, lang string) {
	t.Helper()
	e.WsExec(t, `UPDATE activity SET language = $2 WHERE id = $1`, ids.UUID(activity.Id), lang)
}

func TestTheAddresseesGermanReplyStoredAsEnglishIsDraftedInGerman(t *testing.T) {
	e := integration.Setup(t)
	addressee := e.SeedContact(t, "Dietmar Kunde", &e.Rep1)
	replyLangMail(t, e, "outbound", addressee, replyLangEnglish, 0)
	anchor := replyLangMail(t, e, "inbound", addressee, replyLangGerman, time.Hour)
	storeOldRuleLanguage(t, e, anchor, "en")
	if got := replyLangDraft(t, e, anchor); got != "de" {
		t.Fatalf("output language = %q, want the addressee's German over the stored English", got)
	}
}

func TestTheStoredLanguageHoldsWhenTheAddresseeWroteNothing(t *testing.T) {
	e := integration.Setup(t)
	addressee := e.SeedContact(t, "Dietmar Kunde", &e.Rep1)
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
	addressee := e.SeedContact(t, "Dietmar Kunde", &e.Rep1)
	colleague := e.SeedContact(t, "Anne Kollegin", &e.Rep1)
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
