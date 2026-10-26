// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
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

func logReplyMessage(ctx context.Context, t *testing.T, e *integration.Env, thread, subject, body string, offset time.Duration) crmcontracts.Activity {
	t.Helper()
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC).Add(offset)
	direction := "outbound"
	row, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, ThreadKey: thread,
		Direction: &direction, OccurredAt: &at, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestReplyDraftUsesTheSelectedMessageAndReadableThreadContext(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	anchor := logReplyMessage(ctx, t, e, "pricing", "Re: RE: Pricing", "Selected proposal", 0)
	logReplyMessage(ctx, t, e, "pricing", "Delivery", "Agreed delivery window", time.Hour)
	logReplyMessage(ctx, t, e, "unrelated", "Unrelated", "Other customer secret", 2*time.Hour)
	colleague := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	held := logReplyMessage(colleague, t, e, "pricing", "Private planning", "Hidden colleague note", 2*time.Hour)
	_, err := e.Activities.SetAudience(colleague, ids.From[ids.ActivityKind](ids.UUID(held.Id)), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Wrong target","body":"Could we confirm the proposal?"}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	result, err := drafter.DraftEmailWithProvenance(ctx, ids.UUID(anchor.Id), draftfloor.Steering{Intent: "Ask for confirmation"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Subject != "Re: Pricing" {
		t.Fatalf("reply subject = %q", result.Subject)
	}
	if len(brain.request.Messages) != 1 {
		t.Fatalf("model messages = %d", len(brain.request.Messages))
	}
	content := brain.request.Messages[0].Content
	if !strings.Contains(content, "Selected proposal") || !strings.Contains(content, "Agreed delivery window") ||
		strings.Contains(content, "Other customer secret") || strings.Contains(content, "Hidden colleague note") ||
		!strings.Contains(content, "later than the selected message") {
		t.Fatalf("selected message and thread isolation lost: %s", content)
	}
	if strings.Count(content, "Selected proposal") != 1 || strings.Contains(content, `"thread":"inbound_mail"`) {
		t.Fatalf("outbound anchor duplicated or treated as inbound: %s", content)
	}
	// The anchor's own date, so the gap is sized from a day rather than from
	// the band's name.
	if !strings.Contains(content, `"occurred_at":"2026-09-01T09:00:00Z"`) {
		t.Fatalf("the selected message's date never reached the model: %s", content)
	}
}

func TestAReplyRewriteCarriesTheDraftOnScreenIntoThePrompt(t *testing.T) {
	e := integration.Setup(t)
	anchor := logReplyMessage(e.Admin(), t, e, "pricing", "Pricing", "Selected proposal", 0)
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Pricing","body":"Shorter."}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	steering := draftfloor.Steering{Intent: "shorter", RewriteOf: "Hallo Marek, bitte bestätigen Sie die Lieferung."}
	if _, err := drafter.DraftEmailWithProvenance(e.Admin(), ids.UUID(anchor.Id), steering); err != nil {
		t.Fatal(err)
	}
	content := brain.request.Messages[0].Content
	if !strings.Contains(content, `"rewrite_of":"Hallo Marek, bitte bestätigen Sie die Lieferung."`) {
		t.Fatalf("the draft on screen never reached the model: %s", content)
	}
	if !strings.Contains(brain.request.System, "rewrite_of, when present") {
		t.Fatalf("the reply prompt does not say what rewrite_of means: %s", brain.request.System)
	}
	if !strings.Contains(content, `"output_language":"de"`) && !strings.Contains(content, `"language":"de"`) {
		t.Fatalf("a rewrite lost the language of the draft it revises: %s", content)
	}
}

func TestAShortReplyToAnEnglishThreadIsDraftedInEnglishWhateverTheAppLanguage(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	logReplyMessage(ctx, t, e, "pilot", "Pilot", "Thanks for the update, we plan to review the proposal with our team next week and come back to you.", 0)
	anchor := logReplyMessage(ctx, t, e, "pilot", "Pilot", "ok", time.Hour)
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Pilot","body":"Great."}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	drafter.envelope = draftfloor.NewResolver().WithUserLanguage(func(context.Context) string { return "vi" })
	if _, err := drafter.DraftEmailWithProvenance(ctx, ids.UUID(anchor.Id), draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	if content := brain.request.Messages[0].Content; !strings.Contains(content, `"output_language":"en"`) {
		t.Fatalf("the thread's English lost to the app language: %s", content)
	}
}

// A text-only adapter is still governed by the HTTP surface's selected subject.
type plainReplyStub struct{}

func (plainReplyStub) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	return "Another subject", "An editable draft", nil
}

func TestReplyEndpointNormalizesTheSubjectForEveryDraftAdapter(t *testing.T) {
	e := integration.Setup(t)
	anchor := logReplyMessage(e.Admin(), t, e, "", "Re: Pricing", "Selected proposal", 0)
	for name, drafter := range map[string]activities.EmailDrafter{
		"deterministic": nil,
		"plain":         plainReplyStub{},
		"provenance":    newReplyDrafter(e.Pool, &replyBrainStub{response: model.Response{Text: `{"subject":"Another subject","body":"Please confirm."}`}}, slog.New(slog.DiscardHandler)),
	} {
		t.Run(name, func(t *testing.T) {
			handler := activities.NewHandlers(e.DB()).WithEmailDrafter(drafter)
			response := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/draft-email", nil).WithContext(e.Admin())
			handler.DraftEmail(response, request, anchor.Id)
			var draft crmcontracts.EmailDraft
			if err := json.Unmarshal(response.Body.Bytes(), &draft); err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 || draft.Subject != "Re: Pricing" || draft.Body == "" {
				t.Fatalf("reply endpoint: status %d, draft %+v", response.Code, draft)
			}
		})
	}
}

func TestAnOldInboundMessageStillDecidesTheReplyLanguageBehindManyNewerOutboundOnes(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	inbound, subject := "inbound", "Angebot"
	german := "Vielen Dank für das Angebot, wir prüfen es mit unserem Team und melden uns nächste Woche bei Ihnen zurück."
	if _, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &german, ThreadKey: "angebot",
		Direction: &inbound, OccurredAt: &at, Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	var anchor crmcontracts.Activity
	for i := range replyContextTurns + 4 {
		anchor = logReplyMessage(ctx, t, e, "angebot", "Angebot", "ok", time.Duration(i+1)*time.Hour)
	}
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Angebot","body":"Gern."}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	drafter.envelope = draftfloor.NewResolver().WithUserLanguage(func(context.Context) string { return "en" })
	if _, err := drafter.DraftEmailWithProvenance(ctx, ids.UUID(anchor.Id), draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	if content := brain.request.Messages[0].Content; !strings.Contains(content, `"output_language":"de"`) {
		t.Fatalf("the older German inbound message was cut before the language was decided: %s", content)
	}
}

func TestAReadableCopiedContactDoesNotSetTheReplyLanguageWhenTheSenderIsPrivate(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	inbound, subject := "inbound", "Angebot"
	logInbound := func(body string, offset time.Duration) crmcontracts.Activity {
		when := at.Add(offset)
		row, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
			Kind: "email", Subject: &subject, Body: &body, ThreadKey: "angebot",
			Direction: &inbound, OccurredAt: &when, Source: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	english := logInbound("Thanks for the update, we plan to review the proposal with our team next week and come back to you.", 0)
	german := logInbound("Vielen Dank für das Angebot, wir prüfen es mit unserem Team und melden uns nächste Woche bei Ihnen zurück.", time.Hour)
	anchor := logInbound("ok", 2*time.Hour)

	copied := e.SeedContact(t, "Anne Copied", &e.Rep1)
	sender := e.SeedContact(t, "Dietmar Private", &e.Rep1)
	e.MakeCapturePrivate(t, "contact", sender, e.Rep2)
	participate := func(activity crmcontracts.Activity, role string, contact ids.UUID) {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, contact_id) VALUES ($1, $2, $3)`,
			ids.UUID(activity.Id), role, contact)
	}
	participate(english, "from", copied)
	participate(german, "from", sender)
	participate(anchor, "from", sender)
	participate(anchor, "cc", copied)

	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Angebot","body":"Gern."}`}}
	drafter := newReplyDrafter(e.Pool, brain, slog.New(slog.DiscardHandler))
	drafter.envelope = draftfloor.NewResolver().WithUserLanguage(func(context.Context) string { return "en" })
	recipient, err := e.Activities.ReplyRecipientFor(ctx, ids.From[ids.ActivityKind](ids.UUID(anchor.Id)))
	if err != nil || recipient.FullName != "Anne Copied" {
		t.Fatalf("precondition: recipient = %+v, err %v; want the copied contact named", recipient, err)
	}
	if _, err := drafter.DraftEmailWithProvenance(ctx, ids.UUID(anchor.Id), draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	if content := brain.request.Messages[0].Content; !strings.Contains(content, `"output_language":"de"`) {
		t.Fatalf("the copied contact's English mail set the language of a reply to someone else: %s", content)
	}
}
