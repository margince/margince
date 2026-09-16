// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The ruleset stamp and the re-judge sweep, against a real database.
//
// Nothing in the unit lane reaches any of it. The stale predicate is a WHERE
// the database evaluates, the CAS is another, and the sweep's whole point is
// which rows it can reach — each can be wrong in a way that compiles and simply
// corrects the wrong messages, or none.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	rulesetOld = "prompts-old"
	rulesetNew = "prompts-new"
)

// dbNow is the database's clock, which is the one owed_verdict_at is stamped
// with. A test comparing against the Go process's own clock would compare two
// unsynchronised sources and pass or fail on the skew between them.
func dbNow(t *testing.T, e *loadEnv) time.Time {
	t.Helper()
	var at time.Time
	if err := e.owner.QueryRow(e.as(), `SELECT now()`).Scan(&at); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	return at
}

// verdictAndRuleset reads back what a row was judged as, and under which rules.
func verdictAndRuleset(t *testing.T, e *loadEnv, id ids.UUID) (verdict, ruleset string) {
	t.Helper()
	var v, r *string
	if err := e.owner.QueryRow(e.as(),
		`SELECT owed_verdict, owed_verdict_ruleset FROM activity WHERE id = $1`, id).Scan(&v, &r); err != nil {
		t.Fatalf("reading the verdict back: %v", err)
	}
	if v != nil {
		verdict = *v
	}
	if r != nil {
		ruleset = *r
	}
	return verdict, ruleset
}

// A verdict reached under older rules is replaced, and the audit row says what
// it replaced.
//
// The before-image is the half worth asserting. This write became a REPLACING
// one, so the gate that exempted it no longer applies, and an audit row saying
// a field changed without saying from what is not recoverable afterwards.
func TestAVerdictUnderAnOlderRulesetIsReplaced(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	activity := e.waitingFrom(t, "Dienstag 14 Uhr", "buyer@customer.test", e.buyer(t))

	if _, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs, rulesetOld, dbNow(t, e)); err != nil {
		t.Fatal(err)
	}
	applied, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictAsksUs, rulesetNew, dbNow(t, e))
	if err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("a verdict judged under newer rules did not replace one judged under older rules")
	}
	verdict, ruleset := verdictAndRuleset(t, e, activity)
	if verdict != OwedVerdictAsksUs || ruleset != rulesetNew {
		t.Errorf("row reads %q under %q, want %q under %q", verdict, ruleset, OwedVerdictAsksUs, rulesetNew)
	}

	var before, after string
	if err := e.owner.QueryRow(e.as(), `
		SELECT coalesce(before->>'owed_verdict', ''), coalesce(after->>'owed_verdict', '')
		  FROM audit_log WHERE entity_id = $1 AND action = 'update'
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`, activity).Scan(&before, &after); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if before != OwedVerdictInformsUs || after != OwedVerdictAsksUs {
		t.Errorf("audit row says %q → %q, want %q → %q", before, after, OwedVerdictInformsUs, OwedVerdictAsksUs)
	}
}

// The same rules judging twice is still two opinions, and the first stands.
func TestAVerdictUnderTheSameRulesetDoesNotOverwrite(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	activity := e.waitingFrom(t, "Anything", "buyer@customer.test", e.buyer(t))

	if _, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictAsksUs, rulesetOld, dbNow(t, e)); err != nil {
		t.Fatal(err)
	}
	applied, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs, rulesetOld, dbNow(t, e))
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("a second judgement under the same rules overwrote the first")
	}
	if verdict, _ := verdictAndRuleset(t, e, activity); verdict != OwedVerdictAsksUs {
		t.Errorf("the row reads %q, want the first verdict %q to stand", verdict, OwedVerdictAsksUs)
	}
}

// A worker holding an old ruleset must not overwrite a verdict written while it
// was thinking.
//
// THE RACE THIS CLOSES, which a bare `ruleset <> $current` test does not: the
// classifier reads a batch, spends a model call, then writes. Across a rolling
// deploy an old binary can hold a pre-deploy stamp over that gap, land after a
// new binary judged the row, and pass the inequality while overwriting a
// strictly newer answer with a strictly older one. Difference is not order; the
// read instant is what supplies the order.
func TestAStaleWorkerDoesNotOverwriteANewerVerdict(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	activity := e.waitingFrom(t, "Anything", "buyer@customer.test", e.buyer(t))

	// The old worker read its batch here, BEFORE the new worker wrote — on the
	// DATABASE's clock, which is where production takes it from and the only
	// one owed_verdict_at can be compared against.
	readAt := dbNow(t, e)
	if _, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictAsksUs, rulesetNew, dbNow(t, e)); err != nil {
		t.Fatal(err)
	}
	applied, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs, rulesetOld, readAt)
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("a worker holding older rules overwrote a verdict written after it read its batch")
	}
	verdict, ruleset := verdictAndRuleset(t, e, activity)
	if verdict != OwedVerdictAsksUs || ruleset != rulesetNew {
		t.Errorf("row reads %q under %q, want the newer %q under %q to stand",
			verdict, ruleset, OwedVerdictAsksUs, rulesetNew)
	}
}

// A judgement made before rulesets existed is stale, and the sweep reaches it.
//
// The pre-migration state cannot be produced by the writer any more — it stamps
// every verdict — so the UPDATE below is the honest way to seed the one row
// shape that only history can make.
func TestALegacyVerdictWithNoRulesetIsStale(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	activity := e.waitingFrom(t, "Judged long ago", "buyer@customer.test", e.buyer(t))
	if _, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs, rulesetOld, dbNow(t, e)); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE activity SET owed_verdict_ruleset = NULL WHERE id = $1`, activity)

	rows, _, err := store.OwedRestale(asClassifier(e), rulesetNew, 100, 400, 400)
	if err != nil {
		t.Fatalf("reading the re-judge backlog: %v", err)
	}
	if !containsCandidate(rows, activity) {
		t.Fatal("a verdict carrying no ruleset was not treated as stale, so nothing can ever re-judge it")
	}
}

// The sweep reaches a row the waiting queue would not show.
//
// THE SELF-EXCLUSION THIS AVOIDS: past the waiting horizon a row survives that
// queue only when it carries request evidence — `asks_us`, an accepted task or
// a scheduling label — and a row wrongly judged informs_us carries none. Built
// on the queue, the sweep could never reach the rows it exists to correct, and
// the wrong verdict would keep itself wrong forever.
func TestTheSweepReachesARowTheQueueWouldNotShow(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	// Older than any horizon the queue derives, and judged informs_us with no
	// label and no task — invisible to the queue on both counts.
	activity := e.waitingAgedFrom(t, "Dienstag 14 Uhr", "buyer@customer.test", e.buyer(t), 400)
	if _, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictInformsUs, rulesetOld, dbNow(t, e)); err != nil {
		t.Fatal(err)
	}

	stale, _, err := store.OwedRestale(asClassifier(e), rulesetNew, 100, 400, 400)
	if err != nil {
		t.Fatalf("reading the re-judge backlog: %v", err)
	}
	if !containsCandidate(stale, activity) {
		t.Error("the sweep cannot reach a wrongly-judged row the waiting queue has aged out — " +
			"the wrong verdict keeps itself wrong")
	}
	// And the unjudged backlog does not carry it: the two populations are
	// disjoint, which is what keeps the sweep off new mail's budget.
	if unjudged(t, e)[activity] {
		t.Error("a judged row appeared in the unjudged backlog, so the two reads overlap")
	}
}

// A row already judged under the current rules is left alone.
func TestACurrentVerdictIsNotSweptAgain(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	activity := e.waitingFrom(t, "Anything", "buyer@customer.test", e.buyer(t))
	if _, err := store.SetOwedVerdict(asClassifier(e), activity, OwedVerdictAsksUs, rulesetNew, dbNow(t, e)); err != nil {
		t.Fatal(err)
	}

	stale, _, err := store.OwedRestale(asClassifier(e), rulesetNew, 100, 400, 400)
	if err != nil {
		t.Fatalf("reading the re-judge backlog: %v", err)
	}
	if containsCandidate(stale, activity) {
		t.Error("a verdict judged under the current rules was swept again, so every pass re-judges the whole tree")
	}
}

func containsCandidate(rows []OwedCandidate, id ids.UUID) bool {
	for _, row := range rows {
		if row.ID == id {
			return true
		}
	}
	return false
}
