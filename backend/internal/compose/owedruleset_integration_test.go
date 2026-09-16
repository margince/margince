// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The pass re-judging what older rules decided, end to end.
//
// The store's own tests prove the stale predicate and the CAS. This proves the
// engine actually asks the second question: that a prompt change reaches rows
// the old prompt judged, and that a row already judged under the current rules
// costs nothing.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// rulesetOf reads which rules a row was judged under.
func rulesetOf(t *testing.T, e *integration.Env, id ids.UUID) string {
	t.Helper()
	var ruleset *string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT owed_verdict_ruleset FROM activity WHERE id = $1`, id).Scan(&ruleset)
	})
	if err != nil {
		t.Fatalf("reading the ruleset: %v", err)
	}
	if ruleset == nil {
		return ""
	}
	return *ruleset
}

// stampVerdict puts a row in the state an earlier build would have left it in.
func stampVerdict(t *testing.T, e *integration.Env, id ids.UUID, verdict, ruleset string) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			UPDATE activity SET owed_verdict = $2, owed_verdict_at = now(), owed_verdict_ruleset = $3
			WHERE id = $1`, id, verdict, ruleset)
		return err
	})
	if err != nil {
		t.Fatalf("stamping the prior verdict: %v", err)
	}
}

// A prompt change reaches what the old prompt judged.
//
// This is the whole promise of the stamp: the defect that opened this work was
// a verdict nothing could revisit, so correcting the rules left every message
// already judged under the old ones wrong for good.
func TestAVerdictUnderAnOlderRulesetIsJudgedAgain(t *testing.T) {
	e := integration.Setup(t)
	activity := seedWaitingMail(t, e, "Dienstag 14 Uhr wuerde bei uns passen")
	stampVerdict(t, e, activity, activities.OwedVerdictInformsUs, "prompts-someoldbuild")

	brain := &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95}
	runOwedWorker(t, e, brain)

	got := verdictOf(t, e, activity)
	if got == nil || *got != activities.OwedVerdictAsksUs {
		t.Fatalf("verdict = %v, want the message re-judged as %q", got, activities.OwedVerdictAsksUs)
	}
	if ruleset := rulesetOf(t, e, activity); ruleset != owedRuleset {
		t.Errorf("ruleset = %q, want the rules that judged it now, %q", ruleset, owedRuleset)
	}
}

// A row judged under the current rules is not re-read.
//
// Without this the sweep would re-judge the whole installation on every pass,
// forever, at one model call per ten rows.
func TestAVerdictUnderTheCurrentRulesetIsLeftAlone(t *testing.T) {
	e := integration.Setup(t)
	activity := seedWaitingMail(t, e, "Already judged by this build")
	stampVerdict(t, e, activity, activities.OwedVerdictInformsUs, owedRuleset)

	brain := &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95}
	runOwedWorker(t, e, brain)

	if len(brain.prompts) != 0 {
		t.Errorf("the pass spent %d model call(s) on a row already judged under these rules", len(brain.prompts))
	}
	if got := verdictOf(t, e, activity); got == nil || *got != activities.OwedVerdictInformsUs {
		t.Errorf("verdict = %v, want the existing %q untouched", got, activities.OwedVerdictInformsUs)
	}
}

// New mail is judged before history is re-read.
//
// The two populations have separate reads and separate budgets so a sweep can
// never delay a customer who wrote this morning. On the deploy that moves the
// prompt every judged row is stale at once, which is exactly when that matters.
func TestNewMailIsJudgedBeforeTheSweep(t *testing.T) {
	e := integration.Setup(t)
	fresh := seedWaitingMail(t, e, "A question nobody has judged")
	stale := seedWaitingMail(t, e, "Judged under rules that have moved")
	stampVerdict(t, e, stale, activities.OwedVerdictInformsUs, "prompts-someoldbuild")

	brain := &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95}
	runOwedWorker(t, e, brain)

	if got := verdictOf(t, e, fresh); got == nil {
		t.Error("the unjudged message was left unjudged while the pass re-read history")
	}
	if got := verdictOf(t, e, stale); got == nil || *got != activities.OwedVerdictAsksUs {
		t.Errorf("the stale message was not re-judged: %v", got)
	}
}
