// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// flagsMissingGreeting runs the shape check for a draft to the named recipient.
func flagsMissingGreeting(body, firstName, lastName string) bool {
	for _, finding := range draftcheck.Formatting(body, draftcheck.Grounds{FirstName: firstName, LastName: lastName}) {
		if finding.Rule == draftcheck.RuleMissingGreeting {
			return true
		}
	}
	return false
}

// The reported draft opens on the message; the others open on a sentence
// that merely has a comma early on, which is not a greeting either.
func TestADraftWithNoGreetingLineIsAFinding(t *testing.T) {
	for _, body := range []string{
		"It has been a while since we met at the fair, and I wanted to pick up the demo.\n\nWould Wednesday at 15:00 work?",
		"Thanks for your note, Marcus.\n\nCan we talk on Friday?",
		"Wednesday works, see you then.",
		"Seit unserem Treffen sind zehn Tage vergangen.\n\nPasst Ihnen Mittwoch um 15 Uhr?",
		"Marcus Greven ist heute nicht da, ich melde mich morgen.",
	} {
		if !flagsMissingGreeting(body, "Marcus", "Greven") {
			t.Errorf("a draft that greets nobody passed: %q", body)
		}
	}
}

// Every greeting a draft really opens with passes, in each language and
// register: on a greeting word, or on the recipient's own name however long.
func TestADraftThatOpensWithAGreetingPasses(t *testing.T) {
	for _, tc := range []struct{ body, first, last string }{
		{"Hi Marcus,\n\nthe demo slots are Wednesday 15:00 or Friday 14:00.", "Marcus", "Greven"},
		{"Dear Anne Weiss,\n\nthank you for your time at the fair.", "Anne", "Weiss"},
		{"Hello,\n\nfollowing the fair, here are two slots.", "", ""},
		{"Guten Tag Anne-Marie Weiß-Konrad,\n\nanbei zwei Termine.", "Anne-Marie", "Weiß-Konrad"},
		{"Sehr geehrter Herr Dr. Weiss,\n\nanbei zwei Termine.", "Klaus", "Weiss"},
		{"Greven,\n\nwir haben die Frage über Michael Grodd offen.", "Marcus", "Greven"},
		{"Marcus!\n\nPasst Donnerstag?", "Marcus", "Greven"},
		{"Maria de los Ángeles García López,\n\nadjunto dos fechas.", "Maria", "de los Ángeles García López"},
		{"Chào anh Nguyễn Văn An,\n\nxin gửi anh hai lịch hẹn.", "An", "Nguyễn Văn"},
		{"Hallo Marcus, kurz zu Donnerstag.\n\nPasst 14 Uhr?", "Marcus", "Greven"},
	} {
		if flagsMissingGreeting(tc.body, tc.first, tc.last) {
			t.Errorf("a draft that opens with a greeting was flagged: %q", tc.body)
		}
	}
}

// A greeted body is never given a second greeting, and an ungreeted one is
// given the floor's greeting by name.
func TestEnsureGreetingLeavesAGreetedBodyAlone(t *testing.T) {
	english := draftfloor.Envelope{Language: "en", ConversationState: "fresh"}
	const body = "Hi Marcus,\n\nWednesday works."
	if got := draftcheck.EnsureGreeting(body, english, "Marcus", "Greven"); got != body {
		t.Errorf("a greeted body was changed to %q", got)
	}
	got := draftcheck.EnsureGreeting("It has been a while since the fair.", english, "", "")
	if got != "Hello,\n\nIt has been a while since the fair." {
		t.Errorf("an ungreeted body with no recipient name = %q", got)
	}
}

// A Sie draft is repaired formally, by full name, and a du draft familiarly.
func TestEnsureGreetingKeepsTheRegister(t *testing.T) {
	const body = "Seit unserem Treffen sind zehn Tage vergangen."
	sie := draftfloor.Envelope{Language: "de", ConversationState: "weeks", Register: "Sie"}
	if got := draftcheck.EnsureGreeting(body, sie, "Dietmar", "Rietsch"); got != "Guten Tag Dietmar Rietsch,\n\n"+body {
		t.Errorf("a Sie draft was repaired as %q", got)
	}
	du := draftfloor.Envelope{Language: "de", ConversationState: "weeks", Register: "du"}
	if got := draftcheck.EnsureGreeting(body, du, "Dietmar", "Rietsch"); got != "Hallo Dietmar,\n\n"+body {
		t.Errorf("a du draft was repaired as %q", got)
	}
}
