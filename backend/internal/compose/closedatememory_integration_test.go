// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a rep's "no" means to the nightly close-date sweep.
//
// Every test here calls nextDay between the two passes, and that is the point
// rather than a detail. The sweep proposes "today plus a stage-worth of the
// usual pace", so two passes on the same day propose the same date and a memory
// keyed on ANY date passes — which is how an earlier version of this fix went
// green while remembering a refusal for exactly one night.
//
// The memory is durable and has no expiry, which is why the second half of this
// file matters as much as the first: a key drawn too wide would let ONE refusal
// bury every future correction on that deal, and it would fail silently —
// proposals simply stop appearing, and nobody can point at when they stopped.
//
// SQL, all of it: what the sweep staged, and what the memory matched.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rejectCorrection turns down the correction standing against one deal.
func (e *closeDateEnv) rejectCorrection(t *testing.T, dealID ids.UUID) {
	t.Helper()
	var approvalID ids.ApprovalID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM approval WHERE kind = 'close_date_correction'
		   AND target_entity_id = $1 AND status = 'pending'`,
		dealID).Scan(&approvalID); err != nil {
		t.Fatalf("no staged correction to reject: %v", err)
	}
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("rejecting the correction: %v", err)
	}
}

// proposedDateFor reads back the date the standing correction offers, so a test
// can state that two passes really did propose DIFFERENT dates.
func (e *closeDateEnv) proposedDateFor(t *testing.T, dealID ids.UUID) string {
	t.Helper()
	var proposed string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT proposed_change ->> 'expected_close_date' FROM approval
		  WHERE kind = 'close_date_correction' AND target_entity_id = $1
		  ORDER BY created_at DESC LIMIT 1`, dealID).Scan(&proposed); err != nil {
		t.Fatalf("reading the proposed date: %v", err)
	}
	return proposed
}

// A refused date is not proposed again the next night.
//
// The pending check the sweep already had cannot do this: a rejection clears
// 'pending', so the next pass saw nothing standing and staged the same question
// over again.
func TestARejectedCloseDateIsNotProposedAgainTomorrow(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Asked once", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	e.rejectCorrection(t, id)

	e.nextDay()
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingCorrections(t, id); got != 0 {
		t.Errorf("after a rejection the next night staged %d corrections, want 0 — "+
			"the rep is being asked the same question again", got)
	}
}

// The date really does move between two nights.
//
// Without this the test above proves nothing a same-day pair would not, and a
// memory keyed on the proposed date would pass it while forgetting every
// refusal by morning. This is the mutation that catches that.
func TestTomorrowsPassProposesADifferentDate(t *testing.T) {
	e := setupCloseDate(t)
	tonight := e.seedSweepDeal(t, "Swept tonight", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	// The same situation one day on, on a deal the first pass never saw. Both
	// stand in the same stage at the same distance, so the only thing separating
	// their proposals is which day the sweep believes it is.
	e.nextDay()
	tomorrow := e.seedSweepDeal(t, "Swept tomorrow", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	first, second := e.proposedDateFor(t, tonight), e.proposedDateFor(t, tomorrow)
	if first == second {
		t.Errorf("both nights proposed %s, so no test in this file can tell a memory "+
			"keyed on the date from one keyed on the question", first)
	}
}

// A refusal on one deal says nothing about another.
//
// This is how a too-wide key fails, and it is the failure that hides: the
// pipeline quietly stops raising corrections and every test about staging still
// passes, because each one seeds its own deal.
func TestARejectedCloseDateOnOneDealDoesNotSilenceAnother(t *testing.T) {
	e := setupCloseDate(t)
	declined := e.seedSweepDeal(t, "Said no here", e.late, stringp("commit"), intp(-10), 3)
	other := e.seedSweepDeal(t, "Never asked", e.late, stringp("commit"), intp(-12), 4)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	e.rejectCorrection(t, declined)

	e.nextDay()
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingCorrections(t, declined); got != 0 {
		t.Errorf("the refused deal was asked again: %d pending", got)
	}
	if got := e.pendingCorrections(t, other); got != 1 {
		t.Errorf("a deal nobody refused has %d pending corrections, want 1 — "+
			"one rejection has silenced a deal it was never about", got)
	}
}

// A rep who sets their own date is asked again when THAT one goes stale.
//
// The other half of the key, and the reason it is not the deal alone. A refusal
// is remembered forever, so a deal-only key would mean one "no" ends close-date
// hygiene on that deal permanently: the rep refuses, sets a date themselves, it
// slips in turn, and nobody ever tells them.
func TestANewStandingDateIsCorrectedAgainAfterAnEarlierRefusal(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Set it myself", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	e.rejectCorrection(t, id)

	// The rep puts their own date on the deal, and it is overdue in its turn.
	// close_date_provisional goes false, which is what says the next correction
	// is about THEIR date rather than about the machine's guess.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET expected_close_date = current_date - 4,
		        close_date_provisional = false
		  WHERE id = $1`, id); err != nil {
		t.Fatalf("setting the rep's own date: %v", err)
	}

	e.nextDay()
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingCorrections(t, id); got != 1 {
		t.Errorf("a date the rep set themselves went stale and raised %d corrections, want 1 — "+
			"one refusal has ended close-date hygiene on this deal for good", got)
	}
}

// A deal that advances is asked again.
//
// The key is how far the deal still has to go, so moving it forward a stage is
// a genuinely different question: the guess is drawn from a shorter distance.
// Without this the memory would be keyed on the deal alone, with the permanent
// silence that implies.
func TestADealThatAdvancesAStageIsCorrectedAgain(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Advanced", e.early, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	e.rejectCorrection(t, id)

	// Into the last open stage: one stage to go rather than two.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET stage_id = $2 WHERE id = $1`, id, e.late); err != nil {
		t.Fatalf("advancing the deal a stage: %v", err)
	}

	e.nextDay()
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingCorrections(t, id); got != 1 {
		t.Errorf("a deal that advanced a stage raised %d corrections, want 1 — "+
			"a refusal made at one distance is silencing a question about another", got)
	}
}
