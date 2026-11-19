// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const (
	familiarGermanMail = "Hallo, ich wollte dich fragen, ob du im Mai Zeit hast. Ich schicke dir dann die Unterlagen, wie du es wolltest."
	formalGermanMail   = "Guten Tag, vielen Dank für Ihre Nachricht. Ich sende Ihnen die Unterlagen, wie Sie es wünschen."
)

// setGreetingRegister stores the register on the contact writing from address.
func (o *owedEnv) setGreetingRegister(t *testing.T, address, register string) {
	t.Helper()
	contact := o.contactWriting(t, address)
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE contact SET greeting_register = NULLIF($2, '') WHERE id = $1`, contact, register)
		return err
	}); err != nil {
		t.Fatalf("storing the register: %v", err)
	}
}

// replyPrompt drafts a reply to the anchor and returns the prompt the model saw.
func replyPrompt(t *testing.T, o *owedEnv, anchor ids.UUID) string {
	t.Helper()
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Re: Pricing","body":"Hallo,\n\nDanke.\n\nBis bald."}`}}
	drafter := newReplyDrafter(o.e.Pool, brain, slog.New(slog.DiscardHandler))
	reader := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	if _, err := drafter.DraftEmailWithProvenance(reader, anchor, draftfloor.Steering{}); err != nil {
		t.Fatal(err)
	}
	return brain.request.Messages[0].Content
}

func TestAStoredGreetingRegisterReachesTheReplyPromptAndOutranksTheThread(t *testing.T) {
	for name, tc := range map[string]struct {
		mail, stored, wantRegister, wantStored string
	}{
		"formal stored on a familiar thread":   {familiarGermanMail, "formal", `"register":"Sie"`, `"greeting_register":"formal"`},
		"familiar stored on a formal thread":   {formalGermanMail, "familiar", `"register":"du"`, `"greeting_register":"familiar"`},
		"nothing stored keeps the thread's du": {familiarGermanMail, "", `"register":"du"`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			o.contact(t, "Dietmar Rietsch", "buyer@alpha.test")
			anchor := o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, tc.mail, 0, true))
			o.setGreetingRegister(t, "buyer@alpha.test", tc.stored)
			prompt := replyPrompt(t, o, anchor)
			if !strings.Contains(prompt, tc.wantRegister) {
				t.Errorf("the prompt lacks %s: %s", tc.wantRegister, prompt)
			}
			if tc.wantStored == "" && strings.Contains(prompt, `"greeting_register"`) {
				t.Errorf("an unset register reached the prompt: %s", prompt)
			}
			if tc.wantStored != "" && !strings.Contains(prompt, tc.wantStored) {
				t.Errorf("the prompt lacks %s: %s", tc.wantStored, prompt)
			}
		})
	}
}

// namedContact is a contact with both name parts, so a formal greeting has a
// surname to use.
func (o *owedEnv) namedContact(t *testing.T, first, last, address, register string) {
	t.Helper()
	if _, err := o.e.Contacts.CreateContact(o.e.Admin(), contacts.CreateContactInput{
		FullName: first + " " + last, FirstName: &first, LastName: &last, Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("creating %s %s: %v", first, last, err)
	}
	o.setGreetingRegister(t, address, register)
}

func TestAFollowUpToOurOwnMailTakesTheRegisterOfWhoItIsDeliveredTo(t *testing.T) {
	o := setupOwed(t)
	o.namedContact(t, "Dietmar", "Rietsch", "buyer@alpha.test", "formal")
	o.namedContact(t, "Sam", "Colleague", o.seat, "familiar")
	o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, familiarGermanMail, 0, true))
	ours := o.capture(t, pricingMail(o.seat, "buyer@alpha.test", "pricing-ours@ws.example", familiarGermanMail, 30, false))
	// Our own seat's address has a contact, linked to the sender participant.
	// A ranking that ignored seats would take that contact's register.
	o.e.WsExec(t, `UPDATE activity_participant SET contact_id = (SELECT contact_id FROM contact_email WHERE email = $2 LIMIT 1)
		WHERE activity_id = $1 AND (user_id IS NOT NULL OR address = $2)`, ours, o.seat)
	prompt := replyPrompt(t, o, ours)
	if !strings.Contains(prompt, `"greeting_register":"formal"`) || strings.Contains(prompt, `"greeting_register":"familiar"`) {
		t.Errorf("a follow-up took the register of someone other than the addressee: %s", prompt)
	}
}

func TestACcdContactDoesNotLendItsRegisterToAnUnknownSender(t *testing.T) {
	o := setupOwed(t)
	o.namedContact(t, "Dietmar", "Rietsch", "buyer@alpha.test", "formal")
	mail := pricingMail("stranger@unknown.test", o.seat, pricingRoot, familiarGermanMail, 0, true)
	mail.cc = "buyer@alpha.test"
	anchor := o.capture(t, mail)
	if prompt := replyPrompt(t, o, anchor); strings.Contains(prompt, `"greeting_register"`) {
		t.Errorf("a CC lent its register to the unknown sender: %s", prompt)
	}
}

// With the model down, the fallback draft opens in the stored register in every
// language, through the same greeting the model's draft is checked against.
func TestTheFallbackDraftOpensInTheStoredRegister(t *testing.T) {
	for name, tc := range map[string]struct {
		mail, stored, wantOpening string
	}{
		"de formal":   {formalGermanMail, "formal", "Guten Tag Dietmar Rietsch,"},
		"de familiar": {formalGermanMail, "familiar", "Hallo Dietmar,"},
		"en formal":   {alphaAsks, "formal", "Dear Dietmar Rietsch,"},
		"en familiar": {alphaAsks, "familiar", "Hi Dietmar,"},
	} {
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			o.namedContact(t, "Dietmar", "Rietsch", "buyer@alpha.test", tc.stored)
			anchor := o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, tc.mail, 0, true))
			brain := &replyBrainStub{err: errors.New("model down")}
			drafter := newReplyDrafter(o.e.Pool, brain, slog.New(slog.DiscardHandler))
			reader := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
			draft, err := drafter.DraftEmailWithProvenance(reader, anchor, draftfloor.Steering{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(draft.Body, tc.wantOpening) {
				t.Errorf("fallback body = %q, want it to open %q", draft.Body, tc.wantOpening)
			}
		})
	}
}

func TestTheNoModelActivityPathOpensInTheStoredRegister(t *testing.T) {
	o := setupOwed(t)
	o.namedContact(t, "Dietmar", "Rietsch", "buyer@alpha.test", "formal")
	anchor := o.capture(t, pricingMail("buyer@alpha.test", o.seat, pricingRoot, alphaAsks, 0, true))
	drafter := newReplyDrafter(o.e.Pool, &replyBrainStub{}, slog.New(slog.DiscardHandler))
	reader := o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms)
	activity, err := drafter.store.GetActivityContent(reader, ids.From[ids.ActivityKind](anchor), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	_, body, _ := drafter.store.FloorDraft(reader, anchor, activity, textlang.English, nil)
	if !strings.HasPrefix(body, "Dear Dietmar Rietsch,") {
		t.Errorf("floor body = %q, want the formal opening", body)
	}
}

// A colleague known only by our domain is skipped as the addressee.
// Greeting and register then belong to the external contact the reply goes to.
func TestAColleagueKnownOnlyByDomainLendsNeitherNameNorRegister(t *testing.T) {
	o := setupOwed(t)
	o.e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('ws.example', 'admin', true)`)
	o.namedContact(t, "Bea", "Billing", "billing@ws.example", "familiar")
	o.namedContact(t, "Dietmar", "Rietsch", "buyer@alpha.test", "formal")
	mail := pricingMail("billing@ws.example", o.seat, pricingRoot, alphaAsks, 0, true)
	mail.cc = "buyer@alpha.test"
	anchor := o.capture(t, mail)
	prompt := replyPrompt(t, o, anchor)
	for _, want := range []string{`"recipient":"Dietmar"`, `"recipient_last_name":"Rietsch"`, `"greeting_register":"formal"`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt lacks %s: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Bea") {
		t.Errorf("the colleague known only by domain was greeted: %s", prompt)
	}
}
