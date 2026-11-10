// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func withBody(act crmcontracts.Activity, body string) crmcontracts.Activity {
	act.Body = &body
	return act
}

// Where only our side has written, the follow-up still reads what we said.
func TestOurOwnSentMailReachesAFollowUp(t *testing.T) {
	at := func(hours int) time.Time { return draftedAt.Add(-time.Duration(hours) * time.Hour) }
	note := logged(crmcontracts.ActivityKindNote, at(1), "unrelated: office move")
	recent := FoldRecent([]crmcontracts.Activity{
		note,
		withBody(mail(crmcontracts.ActivityDirectionOutbound, at(48), "Pricing"), "Second ask: please confirm the 40 seats."),
		withBody(mail(crmcontracts.ActivityDirectionOutbound, at(240), "Pricing"), "First ask: can we send a quote?"),
	}, draftedAt, time.UTC)

	if len(recent) != 3 {
		t.Fatalf("read %d turns, want 3", len(recent))
	}
	if recent[1].Said != "Second ask: please confirm the 40 seats." || recent[2].Said != "First ask: can we send a quote?" {
		t.Errorf("our two mails carry %q and %q, want their text", recent[1].Said, recent[2].Said)
	}
	if recent[0].Record == "" || recent[0].Said != "" {
		t.Errorf("the logged note must be context in record, got record %q said %q", recent[0].Record, recent[0].Said)
	}
	if recent[1].Snippet != "" || recent[2].Snippet != "" {
		t.Error("our own words must not be presented as the snippet of something they wrote")
	}
}

func TestATurnIsBoundedTheSameWhoeverWroteIt(t *testing.T) {
	long := strings.Repeat("word ", 400)
	recent := FoldRecent([]crmcontracts.Activity{
		withBody(mail(crmcontracts.ActivityDirectionOutbound, draftedAt.Add(-time.Hour), "Out"), long),
		withBody(mail(crmcontracts.ActivityDirectionInbound, draftedAt.Add(-2*time.Hour), "In"), long),
	}, draftedAt, time.UTC)
	if got, want := len([]rune(recent[0].Said)), len([]rune(recent[1].Snippet)); got != want || got > draftcore.TurnTextRunes {
		t.Errorf("our mail is read at %d runes and theirs at %d, want one bound of %d", got, want, draftcore.TurnTextRunes)
	}
}

// A new message keeps its own subject even when only our side has written.
func TestANewMessageKeepsItsSubjectWithOutboundHistory(t *testing.T) {
	at := func(hours int) time.Time { return draftedAt.Add(-time.Duration(hours) * time.Hour) }
	view := viewWith(
		mail(crmcontracts.ActivityDirectionOutbound, at(48), "Pricing for 40 seats"),
		mail(crmcontracts.ActivityDirectionOutbound, at(240), "Pricing for 40 seats"),
	)
	in := FromView(view, Request{Envelope: envelopeAt(textlang.English, convstate.BandWeeks)})
	const answer = `{"subject":"Quick follow-up","body":"Hello Jonas Becker,\n\nI wanted to check whether the 40 seats still work for you.\n\nCould you confirm by Friday?","reasoning":[]}`

	draft, _, err := Write(context.Background(), &sequencedLane{answers: []string{answer, answer}}, in, draftvoice.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Subject != "Quick follow-up" {
		t.Errorf("served subject %q, want the new message's subject", draft.Subject)
	}

}
