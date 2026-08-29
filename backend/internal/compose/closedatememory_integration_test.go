// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a rep's "no" means to the nightly close-date sweep.
//
// The sweep runs every night over the same deals, so a refusal it does not
// remember is a question asked again every morning until the rep gives in. The
// memory is durable and has no expiry, which is why the second half of this file
// matters as much as the first: an identity drawn too wide would let ONE refusal
// bury every future correction on that deal, and it would fail silently —
// proposals simply stop appearing, and nobody can point at when they stopped.
//
// SQL, all of it: what the sweep staged, and what the identity matched.

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

// A refused date is not proposed again the next night.
//
// The pending check the sweep already had cannot do this: a rejection clears
// 'pending', so the next pass saw nothing standing and staged the same proposal
// over again.
func TestARejectedCloseDateIsNotProposedAgainTomorrow(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Asked once", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	e.rejectCorrection(t, id)

	// Tomorrow's pass, over a deal whose date the rep has not touched.
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingCorrections(t, id); got != 0 {
		t.Errorf("after a rejection the next sweep staged %d corrections, want 0 — "+
			"the rep is being asked the same question again", got)
	}
}

// A refusal on one deal says nothing about another.
//
// This is how a too-wide identity fails, and it is the failure that hides: the
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

// A refusal survives the proposal changing under it.
//
// The proposed date is recomputed against "today" on every pass, so it differs
// from one night to the next while the rep's answer has not. An identity
// carrying that date would recognise nothing, and the whole-payload diff hash
// the staging falls back to has the same defect — which is why the memory is
// keyed on the deal and the date the rep actually holds.
func TestARejectedCloseDateStaysRefusedWhenTheProposalMoves(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Moves nightly", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	e.rejectCorrection(t, id)

	// The deal goes quieter, so the next pass computes a different date from the
	// same situation. The question is still the one the rep answered.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET occurred_at = occurred_at - interval '20 days'
		   WHERE id IN (SELECT activity_id FROM activity_link WHERE deal_id = $1)`,
		id); err != nil {
		t.Fatalf("ageing the deal's correspondence: %v", err)
	}
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingCorrections(t, id); got != 0 {
		t.Errorf("a moved proposal was staged over a rejection: %d pending — "+
			"the memory is keyed on something that moves with the calendar", got)
	}
}

// A rep who sets their own date is asked again when THAT one goes stale.
//
// The other half of the identity, and the reason it is not the deal alone. A
// refusal is remembered forever, so a deal-only key would mean one "no" ends
// close-date hygiene on that deal permanently: the rep refuses, sets a date
// themselves, it slips in turn, and nobody ever tells them.
func TestANewStandingDateIsCorrectedAgainAfterAnEarlierRefusal(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Set it myself", e.late, stringp("commit"), intp(-10), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	e.rejectCorrection(t, id)

	// The rep puts their own date on the deal, and it is overdue in its turn.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET expected_close_date = current_date - 4,
		        close_date_provisional = false
		  WHERE id = $1`, id); err != nil {
		t.Fatalf("setting the rep's own date: %v", err)
	}

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingCorrections(t, id); got != 1 {
		t.Errorf("a date the rep set themselves went stale and raised %d corrections, want 1 — "+
			"one refusal has ended close-date hygiene on this deal for good", got)
	}
}
