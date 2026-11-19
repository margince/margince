// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const familiarGermanMail = "Hallo, ich wollte dich fragen, ob du im Mai Zeit hast. Ich schicke dir dann die Unterlagen, wie du es wolltest."
const formalGermanMail = "Guten Tag, vielen Dank für Ihre Nachricht. Ich sende Ihnen die Unterlagen, wie Sie es wünschen."

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
