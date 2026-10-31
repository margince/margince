// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore_test

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestLanguageEvidenceReadsTheRecipientsOwnMailFirst(t *testing.T) {
	t.Parallel()
	in, out := crmcontracts.ActivityDirectionInbound, crmcontracts.ActivityDirectionOutbound
	rows := []crmcontracts.Activity{
		turnRow(crmcontracts.ActivityKindEmail, out, 1, "our newest"),
		turnRow(crmcontracts.ActivityKindEmail, in, 5, "their newer"),
		turnRow(crmcontracts.ActivityKindEmail, in, 9, "their older"),
		turnRow(crmcontracts.ActivityKindEmail, out, 3, "our middle"),
	}
	got := draftcore.LanguageEvidenceOf(rows, turnsNow, time.UTC)
	want := []string{"their newer", "their older", "our newest", "our middle"}
	if len(got) != len(want) {
		t.Fatalf("got %d strings %q, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if !strings.HasSuffix(got[i], w) || !strings.HasPrefix(got[i], "Pricing\n") {
			t.Errorf("evidence[%d] = %q, want subject then %q", i, got[i], w)
		}
	}
}

func TestLanguageEvidenceDropsWithheldAndEmptyRowsAndStaysBounded(t *testing.T) {
	t.Parallel()
	in := crmcontracts.ActivityDirectionInbound
	withheld := turnRow(crmcontracts.ActivityKindEmail, in, 1, "secret words")
	state := crmcontracts.ActivityContentStateWithheld
	withheld.ContentState = &state
	empty := turnRow(crmcontracts.ActivityKindEmail, in, 2, "")
	empty.Subject = nil
	rows := []crmcontracts.Activity{withheld, empty}
	for i := range 20 {
		rows = append(rows, turnRow(crmcontracts.ActivityKindEmail, in, 3+i, strings.Repeat("long ", 500)))
	}
	got := draftcore.LanguageEvidenceOf(rows, turnsNow, time.UTC)
	if want := draftcore.LanguageEvidenceTurns; len(got) != want {
		t.Errorf("got %d strings, want %d", len(got), want)
	}
	for _, text := range got {
		if strings.Contains(text, "secret") || len([]rune(text)) > draftcore.LanguageEvidenceRunes {
			t.Errorf("evidence leaked a withheld row or exceeded the rune bound: %d runes", len([]rune(text)))
		}
	}
}

func TestLanguageEvidenceKeepsAnOldInboundMessageBehindManyNewerOutboundOnes(t *testing.T) {
	t.Parallel()
	in, out := crmcontracts.ActivityDirectionInbound, crmcontracts.ActivityDirectionOutbound
	var rows []crmcontracts.Activity
	for i := range draftcore.LanguageEvidenceTurns + 3 {
		rows = append(rows, turnRow(crmcontracts.ActivityKindEmail, out, i+1, "our newer mail"))
	}
	rows = append(rows, turnRow(crmcontracts.ActivityKindEmail, in, 40, "their old reply"))
	got := draftcore.LanguageEvidenceOf(rows, turnsNow, time.UTC)
	if len(got) == 0 || !strings.HasSuffix(got[0], "their old reply") {
		t.Errorf("first evidence = %q, want their old inbound reply ahead of newer outbound mail", got)
	}
}

func TestLanguageEvidencePrefersTheAddresseesMailOverAnotherParticipants(t *testing.T) {
	t.Parallel()
	in := string(crmcontracts.ActivityDirectionInbound)
	turns := []draftcore.Turn{
		{Direction: in, SenderID: "cc-colleague", At: turnsNow, Text: "colleague newest"},
		{Direction: in, SenderID: "addressee", At: turnsNow.Add(-48 * time.Hour), Text: "addressee older"},
		{Direction: in, At: turnsNow.Add(-24 * time.Hour), Text: "unknown sender"},
	}
	got := draftcore.LanguageEvidence(turns, "addressee")
	want := []string{"addressee older", "colleague newest", "unknown sender"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("evidence[%d] = %q, want %q", i, got[i], w)
		}
	}
	if first := draftcore.LanguageEvidence(turns, "")[0]; first != "colleague newest" {
		t.Errorf("with no addressee the newest inbound leads, got %q", first)
	}
}

func TestAddresseeEvidenceIsOnlyTheAddresseesOwnTextNewestFirst(t *testing.T) {
	t.Parallel()
	in, out := string(crmcontracts.ActivityDirectionInbound), string(crmcontracts.ActivityDirectionOutbound)
	mail, chat, call := crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage, crmcontracts.ActivityKindCall
	turns := []draftcore.Turn{
		{Kind: mail, Direction: in, SenderID: "addressee", At: turnsNow.Add(-72 * time.Hour), Subject: "Pricing", Text: "addressee oldest"},
		{Kind: mail, Direction: in, SenderID: "cc-colleague", At: turnsNow, Text: "colleague newest"},
		{Kind: mail, Direction: out, SenderID: "addressee", At: turnsNow, Text: "our mail"},
		{Kind: chat, Direction: in, SenderID: "addressee", At: turnsNow.Add(-time.Hour), Text: "addressee newest"},
		{Kind: mail, Direction: in, SenderID: "addressee", At: turnsNow.Add(-2 * time.Hour), Subject: "subject only"},
		{Kind: call, Direction: in, SenderID: "addressee", At: turnsNow, Text: "the rep's call note", Logged: true},
	}
	got := draftcore.AddresseeEvidence(turns, "addressee")
	want := []string{"addressee newest", "addressee oldest"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("addressee evidence = %q, want %q with no subject and no call note", got, want)
	}
	if none := draftcore.AddresseeEvidence(turns, ""); len(none) != 0 {
		t.Errorf("with no addressee there is no addressee evidence, got %q", none)
	}
}

// A colleague writing from one of our own domains arrives inbound. Ranked by
// direction, that mail was the counterparty's best language sample, so a draft
// mirrored our own writing back at them. The row's voice says who wrote it.
func TestLanguageEvidenceSkipsAColleaguesOwnInboundMail(t *testing.T) {
	t.Parallel()
	in := string(crmcontracts.ActivityDirectionInbound)
	turns := []draftcore.Turn{
		{Direction: in, ByColleague: true, At: turnsNow, Text: "our own words"},
		{Direction: in, At: turnsNow.Add(-24 * time.Hour), Text: "their words"},
	}
	got := draftcore.LanguageEvidence(turns, "")
	if len(got) == 0 || got[0] != "their words" {
		t.Fatalf("language evidence leads with %v, want their words first", got)
	}
}

// One answer for whose word a turn is, so two readers cannot disagree. A
// colleague writing from one of our own domains arrives inbound, and that is
// the case direction alone gets wrong.
func TestATurnSaysWhoseWordItIs(t *testing.T) {
	t.Parallel()
	in := string(crmcontracts.ActivityDirectionInbound)
	out := string(crmcontracts.ActivityDirectionOutbound)
	for _, c := range []struct {
		name  string
		turn  draftcore.Turn
		their bool
	}{
		{"their inbound mail", draftcore.Turn{Direction: in}, true},
		{"our colleague's inbound mail", draftcore.Turn{Direction: in, ByColleague: true}, false},
		{"our outbound mail", draftcore.Turn{Direction: out}, false},
		{"a row naming no direction", draftcore.Turn{}, false},
	} {
		if got := c.turn.TheirWord(); got != c.their {
			t.Errorf("%s: TheirWord() = %v, want %v", c.name, got, c.their)
		}
	}
}
