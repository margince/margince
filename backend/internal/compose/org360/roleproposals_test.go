// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/proposeroles"
)

// A contact's job title is carried into the prompt only so the instruction can
// say what it is NOT. What a human typed wins; a purchased claim fills a blank
// and never seconds an answer somebody gave.
func TestTitleOfPrefersWhatAPersonTypedOverWhatWasBought(t *testing.T) {
	t.Parallel()
	typed, bought := "Head of Fleet", "VP Operations"
	if got := titleOf(contactCard{title: &typed, providerTitle: &bought}); got != typed {
		t.Fatalf("a bought title displaced one a person typed: %q", got)
	}
	if got := titleOf(contactCard{providerTitle: &bought}); got != bought {
		t.Fatalf("a bought title did not fill the blank: %q", got)
	}
	empty := ""
	if got := titleOf(contactCard{title: &empty, providerTitle: &bought}); got != bought {
		t.Fatalf("an empty typed title beat a bought one: %q", got)
	}
	if got := titleOf(contactCard{}); got != "" {
		t.Fatalf("a contact with no title anywhere read as %q", got)
	}
}

// Nothing to read is an ANSWER, and it must not be credited to a model that
// was never asked. Reporting the lane as the author of an empty reading would
// tell an operator their model produced it.
func TestAnAccountWithNothingToReadIsNotCreditedToTheModel(t *testing.T) {
	t.Parallel()
	out := emptyProposalResult()
	if out.GeneratedBy != "deterministic" {
		t.Fatalf("an unasked reading was credited to %q", out.GeneratedBy)
	}
	if out.Written == nil {
		t.Fatal("written is nil, so the wire would carry null rather than an empty list")
	}
	if len(out.Written) != 0 || out.Skipped != 0 {
		t.Fatalf("an empty reading reported %d written and %d skipped", len(out.Written), out.Skipped)
	}
}

// The gate refuses any source this call did not supply, so every id reaching
// the wire is one of ours and parses. A source that somehow did not is the
// honest zero rather than a panic: the client renders no link.
func TestAnUnparseableSourceBecomesTheEmptyIDRatherThanAPanic(t *testing.T) {
	t.Parallel()
	if got := mustParseActivity("not-a-uuid"); got.String() != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("an unreadable source id became %q", got)
	}
}

// The window is what keeps a role a statement about THIS deal. Without it a
// contact who signed off a budget on a closed deal two years ago reads as this
// one's economic buyer, and the card gives a reader no way to tell.
func TestTheReadingWindowIsBoundedToAYear(t *testing.T) {
	t.Parallel()
	if proposalWindowDays > 366 {
		t.Fatalf("the reading window is %d days, long enough to read a role off a closed deal", proposalWindowDays)
	}
}

// Both caps bound one model call's cost and latency. A committee is a dozen
// people; past that the account's contacts are a mailing list, and reading all
// of them spends a premium call on people nobody is selling to.
func TestTheCandidateAndMessageCapsStayBounded(t *testing.T) {
	t.Parallel()
	if proposalCandidates > 25 {
		t.Fatalf("%d candidates per call is a mailing list, not a committee", proposalCandidates)
	}
	if proposalMessages > 20 {
		t.Fatalf("%d messages per contact is a thread dump, not evidence", proposalMessages)
	}
}

// Parse reads the shape and nothing more: every judgement about whether a
// proposal deserves a record belongs to Gate, which is the one place those
// rules are spelled.
func TestParseLeavesEveryJudgementToTheGate(t *testing.T) {
	t.Parallel()
	// A proposal the gate will certainly refuse — no candidates at all — still
	// parses, because refusing it is not Parse's job.
	got, err := proposeroles.Parse(`{"proposals":[{"person_id":"p-1","role":"chief_wizard",
		"evidence_snippet":"x","source_id":"a-1","confidence":9}]}`)
	if err != nil {
		t.Fatalf("a well-formed reply was refused at the decode step: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("decoded %d proposals from a reply carrying one", len(got))
	}
	if len(proposeroles.Gate(got, nil)) != 0 {
		t.Fatal("the gate kept a proposal for a call that offered nobody")
	}
}

// A malformed reply and an empty one are different facts about the lane.
// Reading the first as the second would report a broken model as an account
// whose contacts said nothing about who buys.
func TestAMalformedReplyIsNotReadAsNoRolesFound(t *testing.T) {
	t.Parallel()
	if _, err := proposeroles.Parse("I could not determine any roles."); err == nil {
		t.Fatal("prose parsed as an empty reading")
	}
}
