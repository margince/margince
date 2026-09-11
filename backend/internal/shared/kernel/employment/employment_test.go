// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package employment

// What the predicates actually say.
//
// Every gate around this package holds that there is ONE definition. None of
// them holds what the definition IS — so the one line carrying the whole
// notice-period rule could be quietly narrowed to a null check and every
// census would go on reporting a clean tree, because it would still be the
// only spelling.
//
// That is the failure this file exists for: uniqueness and correctness are
// different claims, and a tree can hold the first perfectly while losing the
// second.

import (
	"strings"
	"testing"
)

func TestACurrentEmploymentIsADateComparisonAndNotANullCheck(t *testing.T) {
	t.Parallel()

	got := IsCurrentSQL("r.ended_at")

	// Somebody serving three months' notice still works there. Reading the
	// column's presence as "gone" took a contact off their employer's contact
	// list the day their notice was filed — with no way back, because ended_at
	// cannot be cleared through the API.
	if !strings.Contains(got, "current_date") {
		t.Errorf("the predicate is %q, with no date comparison in it: a contact serving notice "+
			"drops off their employer's contact list the day it is filed", got)
	}
	if !strings.Contains(got, "r.ended_at IS NULL") {
		t.Errorf("the predicate is %q and does not admit a NULL: somebody with no end date at all "+
			"is the ordinary case and would read as departed", got)
	}
	// OR, not AND: the two arms are alternatives — no end date, or an end date
	// still ahead. Joined with AND the predicate is unsatisfiable and nobody
	// works anywhere.
	if !strings.Contains(got, " OR ") {
		t.Errorf("the predicate is %q: its two arms are alternatives, and joined any other way "+
			"it answers false for everyone", got)
	}
}

// The date expression is placed, not assumed. Every caller passes a different
// one — a column on a read, an alias on a join, the incoming value on a write —
// and a predicate that hard-coded one would silently answer about the wrong row.
func TestThePredicateAsksAboutTheColumnItWasGiven(t *testing.T) {
	t.Parallel()

	if got := IsCurrentSQL("emp.ended_at"); strings.Contains(got, "r.ended_at") {
		t.Errorf("asked about emp.ended_at and answered %q", got)
	}
	if got := IsCurrentSQL("$3"); !strings.Contains(got, "$3") {
		t.Errorf("a bind parameter did not reach the predicate: %q", got)
	}
}

// The three questions about is_current_primary are three, and two of them are
// deliberately DATE-BLIND: they are the index's own predicates, and asking them
// with the date comparison would read a slot as free while the index still held
// it — a 409 on a write that should have been a skip.
func TestTheSlotPredicatesStayDateBlind(t *testing.T) {
	t.Parallel()

	for name, got := range map[string]string{
		"the live-employment slot": LiveSlotSQL("r"),
		"the current-primary slot": CurrentPrimarySlotSQL("r"),
	} {
		if strings.Contains(got, "current_date") {
			t.Errorf("%s asks a date question (%q): the index does not, so the guard would think "+
				"the slot was free while the index still held it", name, got)
		}
	}

	// And the READER's question is not: it pairs the flag with the currency
	// test, which is what stops a new reader trusting a flag written months ago.
	if got := CurrentPrimarySQL("r"); !strings.Contains(got, "current_date") {
		t.Errorf("the reader's predicate is %q and asks no date question: somebody goes on "+
			"counting at a company after their last day has passed", got)
	}
}
