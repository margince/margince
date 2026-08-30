// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// When two close-date corrections are the same question.
//
// This is the whole of the rejection memory's judgment, and both of its terms
// were arrived at by watching one fail: matching the proposed date alone
// silences a rep's own date when the sweep's next guess lands on the same day,
// and dropping it remembers nothing at all.

import "testing"

func TestWhenTwoCloseDateCorrectionsAreTheSameQuestion(t *testing.T) {
	t.Parallel()
	refused := CloseDateCorrection{ExpectedCloseDate: "2026-09-13"}

	// The resurrection this memory exists to stop. The sweep computes its date
	// from stage velocity, so a deal whose situation has not changed is offered
	// the same date again — and the rep has answered that already.
	sameDate := RefusalProbe{Proposed: "2026-09-13", AfterHumanEdit: false}
	if !sameDate.SameQuestionAs(refused) {
		t.Error("the same date proposed again reads as a new question; the rep is " +
			"asked what they already answered")
	}

	// A different date is a different question, however recently they refused.
	movedOn := RefusalProbe{Proposed: "2026-11-30", AfterHumanEdit: false}
	if movedOn.SameQuestionAs(refused) {
		t.Error("a different proposed date reads as already answered")
	}

	// The rep set their own date, and it has gone stale in its turn. The sweep's
	// guess for it can be the very day they refused before — this is the term
	// that keeps that from ending close-date hygiene on the deal for good.
	theirOwnDate := RefusalProbe{Proposed: "2026-09-13", AfterHumanEdit: true}
	if theirOwnDate.SameQuestionAs(refused) {
		t.Error("a date the rep set themselves is treated as the refused one, so " +
			"they are never told it went stale")
	}
}

func TestProbeForReadsTheProposalAndTheStateItCameFrom(t *testing.T) {
	t.Parallel()
	proposal := CloseDateCorrection{ExpectedCloseDate: "2026-09-13"}

	// Provisional means the deal stands at the machine's own guess, so this is
	// not a question about a date any person chose.
	if got := ProbeFor(proposal, true); got.AfterHumanEdit {
		t.Error("a correction on a provisional date reads as being about a rep's own date")
	}
	if got := ProbeFor(proposal, false); !got.AfterHumanEdit {
		t.Error("a correction on a rep's own date reads as being about the machine's guess")
	}
	if got := ProbeFor(proposal, false); got.Proposed != proposal.ExpectedCloseDate {
		t.Errorf("the probe proposes %q, the correction %q", got.Proposed, proposal.ExpectedCloseDate)
	}
}

func TestADealWithNoCloseDateStillHasAStandingValue(t *testing.T) {
	t.Parallel()
	// A real case — the `missing` flag exists for it — and it needs a value
	// rather than an absent key, because jsonb containment never matches a key
	// the payload does not carry.
	if got := StandingCloseDate(nil); got == "" {
		t.Error("a deal with no close date stands at the empty string, which reads " +
			"as an absent identity field and matches nothing")
	}
	held := "2026-08-19"
	if got := StandingCloseDate(&held); got != held {
		t.Errorf("a deal holding %q stands at %q", held, got)
	}
	// And the sentinel cannot be mistaken for a date: every real one is
	// formatted YYYY-MM-DD.
	if StandingCloseDate(nil) == held {
		t.Error("the no-date sentinel collides with a real date")
	}
}
