// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Which rows a hiding rule is holding back — the half the counts could not
// answer.
//
// Asserted by ID rather than by length, for the reason the counts next door are
// asserted as differences: the lane template is shared across this package's
// tests, so a message another test seeded is in the database too and a length
// would pass or fail on which tests ran.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func hiddenRows(t *testing.T, e *loadEnv, rule HiddenRule) []WaitingReply {
	t.Helper()
	got, err := hiddenStore(e).HiddenWaitingRows(e.as(), time.Now(), rule)
	if err != nil {
		t.Fatalf("reading the rows behind %s: %v", rule, err)
	}
	return got
}

func holds(rows []WaitingReply, id ids.UUID) bool {
	for _, row := range rows {
		if row.ActivityID == id {
			return true
		}
	}
	return false
}

// The row behind the figure. A reader who sees "1 set aside" can now ask which
// one, and gets the message they set aside rather than the whole queue.
func TestTheSetAsideRuleNamesTheRowItHeldBack(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Contact', $2, 'seed', 'system')`, contact, e.rep)
	shown := e.seedWait(t, "Still waiting", "contact_id", contact)
	aside := e.seedWait(t, "Set aside", "contact_id", contact)
	e.exec(t, `INSERT INTO activity_reader_state (activity_id, reader_id, state, set_by)
		VALUES ($1, $2, 'not_mine', 'system')`, aside, e.rep)

	rows := hiddenRows(t, e, HiddenRuleSetAside)

	if !holds(rows, aside) {
		t.Error("the set-aside message is not among the rows behind its own figure")
	}
	// The difference, not the relaxed read. A rule relaxed alone still leaves
	// every other rule in force, so a reader clicking "1 set aside" must not
	// receive the queue they can already see.
	if holds(rows, shown) {
		t.Error("a message the queue already shows was returned as hidden")
	}
}

// A thread judged not-sales is hidden from the whole workspace and never lifts,
// which makes it the rule most worth being able to inspect.
func TestTheNotSalesRuleNamesTheRowItHeldBack(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Contact', $2, 'seed', 'system')`, contact, e.rep)
	shown := e.seedWait(t, "Still waiting", "contact_id", contact)
	judged := e.seedWait(t, "Newsletter", "contact_id", contact)
	// Keyed on the THREAD, the way SetThreadNotSales writes it — an activity id
	// here would be judged by a rule that reads thread_key and match nothing.
	e.exec(t, `INSERT INTO activity_sales_state (thread_key, kind, channel_provider, set_by)
		SELECT a.thread_key, a.kind, coalesce(a.channel_provider, ''), 'system'
		  FROM activity a WHERE a.id = $1`, judged)

	rows := hiddenRows(t, e, HiddenRuleNotSales)

	if !holds(rows, judged) {
		t.Error("the not-sales thread is not among the rows behind its own figure")
	}
	if holds(rows, shown) {
		t.Error("a message the queue already shows was returned as hidden")
	}
}

// Each rule answers for itself. A row hidden by one must not appear behind
// another, or the list repeats the failure the separate figures exist to avoid
// — a reader could not tell which rule to look at.
func TestARowHeldBackByOneRuleIsNotListedUnderAnother(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Contact', $2, 'seed', 'system')`, contact, e.rep)
	aside := e.seedWait(t, "Set aside only", "contact_id", contact)
	e.exec(t, `INSERT INTO activity_reader_state (activity_id, reader_id, state, set_by)
		VALUES ($1, $2, 'not_mine', 'system')`, aside, e.rep)

	for _, rule := range []HiddenRule{HiddenRuleNotSales, HiddenRulePastHorizon, HiddenRuleUnlinked} {
		if holds(hiddenRows(t, e, rule), aside) {
			t.Errorf("a set-aside row was listed under %s", rule)
		}
	}
}

// A queue hiding nothing lists nothing. Without this the assertions above would
// be satisfied by a read that returned every row for every rule.
func TestARuleHidingNothingListsNothingOfThisTestsOwn(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Contact', $2, 'seed', 'system')`, contact, e.rep)
	shown := e.seedWait(t, "Nothing hides this", "contact_id", contact)

	for _, rule := range []HiddenRule{
		HiddenRuleSetAside, HiddenRuleNotSales, HiddenRulePastHorizon,
		HiddenRuleUnlinked, HiddenRuleColleagues,
	} {
		if holds(hiddenRows(t, e, rule), shown) {
			t.Errorf("an unhidden message was listed behind %s", rule)
		}
	}
}

// An unknown rule is refused rather than answered empty: "nothing is behind
// that rule" and "there is no such rule" are different answers, and returning
// the first for the second lets a typo read as a clean queue.
func TestAnUnknownRuleIsRefusedRatherThanAnsweredEmpty(t *testing.T) {
	e := setupLoad(t)

	_, err := hiddenStore(e).HiddenWaitingRows(e.as(), time.Now(), HiddenRule("no_such_rule"))

	if !errors.Is(err, ErrUnknownHiddenRule) {
		t.Fatalf("err = %v, want ErrUnknownHiddenRule", err)
	}
}
