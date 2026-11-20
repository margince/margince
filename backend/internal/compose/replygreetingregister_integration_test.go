// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const duMail = "Hallo, ich wollte dich fragen, ob du im Mai Zeit hast. Ich schicke dir dann die Unterlagen, wie du es wolltest."

func (o *owedEnv) contactWith(t *testing.T, first, address, register string) {
	last := "Surname"
	if _, err := o.e.Contacts.CreateContact(o.e.Admin(), contacts.CreateContactInput{FullName: first + " " + last, FirstName: &first, LastName: &last, Source: "manual", Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}}}); err != nil {
		t.Fatal(err)
	}
	o.e.WsExec(t, `UPDATE contact SET greeting_register = NULLIF($2, '') WHERE id = (SELECT contact_id FROM contact_email WHERE email = $1)`, address, register)
}

func TestTheStoredRegisterOfTheDeliveredContactDecidesTheReplyGreeting(t *testing.T) {
	const buyer = "buyer@alpha.test"
	cc := func(from string) owedMail {
		return owedMail{from: from, cc: buyer, subject: "Pricing", messageID: pricingRoot, body: alphaAsks, at: pricingStart, authenticated: true}
	}
	type tcase struct {
		mail                     owedMail
		modelErr                 error
		wantIn, wantOut, opening string
	}
	for name, tc := range map[string]tcase{
		"formal beats a du thread": {pricingMail(buyer, "", pricingRoot, duMail, 0, true), nil, `"register":"Sie","greeting_register":"formal"`, "", ""},
		"our own seat's contact":   {pricingMail("", buyer, "ours@ws.example", duMail, 30, false), nil, `"greeting_register":"formal"`, "familiar", ""},
		"a copied contact":         {cc("stranger@unknown.test"), nil, "", `"greeting_register"`, ""},
		"a domain colleague":       {cc("billing@ws.example"), nil, `"greeting_register":"formal".*"recipient":"Dietmar"`, "Zora", ""},
		"model down":               {pricingMail(buyer, "", pricingRoot, alphaAsks, 0, true), errors.New("model down"), "", "", "Dear Dietmar Surname,"},
	} {
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			o.e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('ws.example', 'admin', true)`)
			o.contactWith(t, "Dietmar", buyer, "formal")
			mail, own, ownSeat := tc.mail, o.seat, tc.mail.from == ""
			if strings.HasSuffix(mail.from, "@ws.example") {
				o.contactWith(t, "Zora", mail.from, "familiar")
			}
			if ownSeat {
				o.contactWith(t, "Sam", own, "familiar")
				o.capture(t, pricingMail(buyer, own, pricingRoot, duMail, 0, true))
			}
			mail.from, mail.to = cmp.Or(mail.from, own), cmp.Or(mail.to, own)
			anchor := o.capture(t, mail)
			if ownSeat {
				o.e.WsExec(t, `UPDATE activity_participant SET contact_id = (SELECT contact_id FROM contact_email WHERE email = $2 LIMIT 1) WHERE activity_id = $1 AND (user_id IS NOT NULL OR address = $2)`, anchor, own)
			}
			brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Re: Pricing","body":"Hallo,\n\nDanke.\n\nBis bald."}`}, err: tc.modelErr}
			drafter := newReplyDrafter(o.e.Pool, brain, slog.New(slog.DiscardHandler))
			draft, err := drafter.DraftEmailWithProvenance(o.e.As(o.owner, []ids.UUID{o.e.Team1}, integration.AdminPerms), anchor, draftfloor.Steering{})
			if err != nil {
				t.Fatal(err)
			}
			prompt := fmt.Sprint(brain.request.Messages)
			if found, _ := regexp.MatchString(tc.wantIn, prompt); !found || tc.wantOut != "" && strings.Contains(prompt, tc.wantOut) || !strings.HasPrefix(draft.Body, tc.opening) {
				t.Errorf("prompt = %s\nbody = %q", prompt, draft.Body)
			}
		})
	}
}
