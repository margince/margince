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
	turns := []draftcore.Turn{
		{SideKnown: true, SenderID: "cc-colleague", At: turnsNow, Text: "colleague newest"},
		{SideKnown: true, SenderID: "addressee", At: turnsNow.Add(-48 * time.Hour), Text: "addressee older"},
		{SideKnown: true, At: turnsNow.Add(-24 * time.Hour), Text: "unknown sender"},
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
	mail, chat, call := crmcontracts.ActivityKindEmail, crmcontracts.ActivityKindMessage, crmcontracts.ActivityKindCall
	turns := []draftcore.Turn{
		{Kind: mail, SideKnown: true, SenderID: "addressee", At: turnsNow.Add(-72 * time.Hour), Subject: "Pricing", Text: "addressee oldest"},
		{Kind: mail, SideKnown: true, SenderID: "cc-colleague", At: turnsNow, Text: "colleague newest"},
		{Kind: mail, Ours: true, SideKnown: true, SenderID: "addressee", At: turnsNow, Text: "our mail"},
		{Kind: chat, SideKnown: true, SenderID: "addressee", At: turnsNow.Add(-time.Hour), Text: "addressee newest"},
		{Kind: mail, SideKnown: true, SenderID: "addressee", At: turnsNow.Add(-2 * time.Hour), Subject: "subject only"},
		{Kind: call, SideKnown: true, SenderID: "addressee", At: turnsNow, Text: "the rep's call note", Logged: true},
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
	in := crmcontracts.ActivityDirectionInbound
	ours := turnRow(crmcontracts.ActivityKindEmail, in, 1, "our own words")
	ours.Voice = authoredBy(crmcontracts.ActivityAuthorKindColleague)
	turns := draftcore.Conversation([]crmcontracts.Activity{
		ours, turnRow(crmcontracts.ActivityKindEmail, in, 24, "their words"),
	}, nil, turnsNow, time.UTC, 6)
	got := draftcore.LanguageEvidence(turns, "")
	if len(got) == 0 || !strings.HasSuffix(got[0], "their words") {
		t.Fatalf("language evidence leads with %q, want their words first", got)
	}
}

// Conversation reads whose word a turn is from the row's author, never its
// transport, so its readers never combine the two themselves. A logged call
// keeps its direction: the voice there names who wrote the record down.
func TestATurnSaysWhoseWordItIs(t *testing.T) {
	t.Parallel()
	in, out := crmcontracts.ActivityDirectionInbound, crmcontracts.ActivityDirectionOutbound
	mail := crmcontracts.ActivityKindEmail
	for _, c := range []struct {
		name       string
		row        crmcontracts.Activity
		their, our bool
	}{
		{"their inbound mail", turnRow(mail, in, 1, "x"), true, false},
		{"our colleague's inbound mail", withAuthor(turnRow(mail, in, 1, "x"), crmcontracts.ActivityAuthorKindColleague), false, true},
		{"the contact's mail sent from here", withAuthor(turnRow(mail, out, 1, "x"), crmcontracts.ActivityAuthorKindContactRecord), true, false},
		{"our outbound mail", turnRow(mail, out, 1, "x"), false, true},
		{"an inbound call a colleague logged", withAuthor(turnRow(crmcontracts.ActivityKindCall, in, 1, "x"), crmcontracts.ActivityAuthorKindColleague), true, false},
		{"a note naming no direction", turnRow(crmcontracts.ActivityKindNote, "", 1, "x"), false, false},
	} {
		turns := draftcore.Conversation([]crmcontracts.Activity{c.row}, nil, turnsNow, time.UTC, 6)
		if len(turns) != 1 {
			t.Fatalf("%s: got %d turns, want 1", c.name, len(turns))
		}
		if their, our := turns[0].TheirWord(), turns[0].OurWord(); their != c.their || our != c.our {
			t.Errorf("%s: TheirWord=%v OurWord=%v, want %v and %v", c.name, their, our, c.their, c.our)
		}
	}
}

func authoredBy(kind crmcontracts.ActivityAuthorKind) *crmcontracts.ActivityVoice {
	return &crmcontracts.ActivityVoice{Author: &crmcontracts.ActivityAuthor{Kind: kind}}
}

func withAuthor(row crmcontracts.Activity, kind crmcontracts.ActivityAuthorKind) crmcontracts.Activity {
	row.Voice = authoredBy(kind)
	return row
}
