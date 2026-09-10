// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a rep's "leave my deals alone" means to the nightly close-date sweep.
//
// This file used to test what a rep's REFUSAL meant, when the sweep wrote a
// date and then raised a card asking them to confirm it. There is no card now:
// the correction is applied and the morning's receipt carries it with a way
// back, so a refusal has nothing to land on and the memory that mattered is the
// reversal — which correctionrevert_integration_test.go holds.
//
// What is left is the setting itself, and it is worth its own file because the
// failure it prevents is silent in both directions: a rep who switched the
// sweep off and still finds their dates moved has no way to tell, and one who
// left it on and sees nothing move cannot tell that either.
//
// Every test calls nextDay between passes, for the reason the old file gave:
// the sweep proposes "today plus a stage-worth of the usual pace", so two
// passes on one day propose the same date and a test keyed on any date passes.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leaveMeAlone records that this rep wants close dates left to them.
func (e *closeDateEnv) leaveMeAlone(t *testing.T, user ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO approval_autonomy_policy (id, user_id, kind, mode)
		 VALUES ($1, $2, 'close_date_correction', 'manual')`,
		ids.NewV7(), user); err != nil {
		t.Fatalf("recording the rep's setting: %v", err)
	}
}

// A rep who has said nothing gets the correction applied.
//
// The default, and it is the whole shape of this change: the alternative asked
// them to confirm a date the sweep had already written, which expired
// unanswered and left the deal standing on a machine's guess with nothing on
// the page saying so.
func TestADealIsCorrectedForARepWhoHasSaidNothing(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Nobody objected", e.early, stringp("commit"), intp(-10), 3)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	if !swept.provisional {
		t.Error("the deal was not corrected; a rep who has expressed no preference " +
			"gets the sweep's hygiene, which is what the setting defaults to")
	}
}

// And a rep who switched it off is left alone.
func TestADealIsLeftAloneForARepWhoAskedToBe(t *testing.T) {
	e := setupCloseDate(t)
	e.leaveMeAlone(t, e.Rep1)
	id := e.seedSweepDeal(t, "Hands off", e.early, stringp("commit"), intp(-10), 3)
	before := e.readSwept(t, id)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	after := e.readSwept(t, id)
	if after.provisional != before.provisional || !sameDay(after.expectedClose, before.expectedClose) {
		t.Errorf("a rep who turned close-date corrections off had a deal moved from "+
			"%v to %v anyway", dayOf(before.expectedClose), dayOf(after.expectedClose))
	}
}

// A rep on the third rung is not consenting either.
//
// `veto` means "show me before it lands", and reading the setting as a two-way
// switch would take it as a yes — the failure mode of comparing against
// "not manual" instead of against auto.
func TestADealIsLeftAloneForARepOnVeto(t *testing.T) {
	e := setupCloseDate(t)
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO approval_autonomy_policy (id, user_id, kind, mode, veto_window)
		 VALUES ($1, $2, 'close_date_correction', $3, '1 hour')`,
		ids.NewV7(), e.Rep1, "veto"); err != nil {
		// The rung the schema allows and nothing writes yet. Spelled as the
		// literal because approvals declares no constant for it — the mode
		// exists in the CHECK constraint and in this test alone.
		t.Fatalf("recording the rep's veto setting: %v", err)
	}
	id := e.seedSweepDeal(t, "Show me first", e.early, stringp("commit"), intp(-10), 3)
	before := e.readSwept(t, id)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	after := e.readSwept(t, id)
	if !sameDay(after.expectedClose, before.expectedClose) {
		t.Errorf("a rep on veto had a deal moved from %v to %v; veto asks to see a "+
			"change before it lands, which is not consent to it landing",
			dayOf(before.expectedClose), dayOf(after.expectedClose))
	}
}

// sameDay compares two nullable close dates by the day they name.
func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// dayOf renders a nullable close date for a failure message.
func dayOf(at *time.Time) string {
	if at == nil {
		return "none"
	}
	return at.Format(time.DateOnly)
}
