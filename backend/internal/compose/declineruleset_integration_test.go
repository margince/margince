// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A decline is final only for the prompt that declined it, end to end.
//
// Each row is put in its prior state by the store's own writer, under a digest
// no current prompt carries — exactly what a build shipping older wording left
// behind. The sweeps then run under the prompts this build ships.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/capturelabel"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/owedverdict"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// retiredPrompt is a digest no prompt in this build renders to.
const retiredPrompt = "prompts-retired"

// classifyPassFor is the classify sweep and the context it runs under.
func classifyPassFor(e *integration.Env, brain completer) (*CaptureClassifier, context.Context) {
	return NewCaptureClassifier(e.Pool, brain, slog.New(slog.DiscardHandler)),
		principal.WithWorkspaceID(context.Background(), e.WS)
}

// owedSweepContext is the principal judgeWorkspace hands the owed pass.
func owedSweepContext(e *integration.Env) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: activities.OwedVerdictCapturedBy,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

// declinedUnder reads which prompt a row's decline names, or "" for none.
func declinedUnder(t *testing.T, e *integration.Env, id ids.UUID, column string) string {
	t.Helper()
	var ruleset *string
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT `+pgx.Identifier{column}.Sanitize()+` FROM activity WHERE id = $1`, id).Scan(&ruleset)
	})
	if err != nil {
		t.Fatalf("reading %s: %v", column, err)
	}
	if ruleset == nil {
		return ""
	}
	return *ruleset
}

func TestTheClassifyPassOffersAMessageDeclinedUnderAnotherPrompt(t *testing.T) {
	e := integration.Setup(t)
	stale := seedUnlabeledEmail(t, e, "please send the offer")
	// The control: declined under THIS prompt, and carrying the marker, so any
	// call that re-sent it would be counted as withheld.
	current := seedUnlabeledEmail(t, e, "Invoice "+hostileMarker)

	brain := &withholdingWhere{inner: &scriptedClassifyBrain{}}
	classifier, ctx := classifyPassFor(e, brain)
	for id, ruleset := range map[ids.UUID]string{stale: retiredPrompt, current: capturelabel.Ruleset} {
		if recorded, err := classifier.store.MarkCaptureLabelDeclined(ctx, id, ruleset); err != nil || !recorded {
			t.Fatalf("recording the prior decline: recorded=%v err=%v", recorded, err)
		}
	}
	if err := classifier.RunWorkspace(ctx, 0); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if labelOf(t, e, stale) == nil {
		t.Error("a message declined under a retired prompt was not offered to the current one")
	}
	if brain.withheld != 0 {
		t.Errorf("a message declined under the current prompt was sent %d times; it stays declined", brain.withheld)
	}
}

// Re-offering must not become the per-tick loop the stamp ended: a message the
// current prompt declines too is asked once, re-stamped, and left alone.
func TestTheClassifyPassRestampsAStaleDeclineOnce(t *testing.T) {
	e := integration.Setup(t)
	hostile := seedUnlabeledEmail(t, e, "Invoice "+hostileMarker)
	brain := &withholdingWhere{inner: &scriptedClassifyBrain{}}
	classifier, ctx := classifyPassFor(e, brain)
	if recorded, err := classifier.store.MarkCaptureLabelDeclined(ctx, hostile, retiredPrompt); err != nil || !recorded {
		t.Fatalf("recording the prior decline: recorded=%v err=%v", recorded, err)
	}

	for pass := 1; pass <= 2; pass++ {
		if err := classifier.RunWorkspace(ctx, 0); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		if brain.withheld != 2 {
			t.Fatalf("after pass %d the message had been sent %d times, want 2 — its batch and itself, once", pass, brain.withheld)
		}
	}
	if got := declinedUnder(t, e, hostile, "capture_label_declined_ruleset"); got != capturelabel.Ruleset {
		t.Errorf("the decline names %q, want the prompt that declined it now, %q", got, capturelabel.Ruleset)
	}
}

func TestTheOwedPassOffersAMessageDeclinedUnderAnotherPrompt(t *testing.T) {
	e := integration.Setup(t)
	stale := seedWaitingMail(t, e, "Dienstag 14 Uhr wuerde bei uns passen")
	current := seedWaitingMail(t, e, "Invoice "+hostileMarker)
	store := activities.NewStore(InstallationDB(e.Pool))
	ctx := owedSweepContext(e)
	for id, ruleset := range map[ids.UUID]string{stale: retiredPrompt, current: owedverdict.Ruleset} {
		if recorded, err := store.MarkOwedVerdictDeclined(ctx, id, ruleset); err != nil || !recorded {
			t.Fatalf("recording the prior decline: recorded=%v err=%v", recorded, err)
		}
	}

	brain := &withholdingWhere{inner: &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95}}
	for pass := 1; pass <= 2; pass++ {
		runOwedWorker(t, e, brain)
	}

	if got := verdictOf(t, e, stale); got == nil || *got != activities.OwedVerdictAsksUs {
		t.Errorf("a message declined under a retired prompt was left %v; want it judged %q", got, activities.OwedVerdictAsksUs)
	}
	if brain.withheld != 0 {
		t.Errorf("a message declined under the current prompt was sent %d times; it stays declined", brain.withheld)
	}
}

// The owed pass's half of the same guarantee: a message declined under a
// retired prompt that the current prompt declines too is offered on the first
// pass, re-stamped under the current prompt, and not offered on the second.
func TestTheOwedPassRestampsAStaleDeclineOnce(t *testing.T) {
	e := integration.Setup(t)
	hostile := seedWaitingMail(t, e, "Invoice "+hostileMarker)
	store := activities.NewStore(InstallationDB(e.Pool))
	if recorded, err := store.MarkOwedVerdictDeclined(owedSweepContext(e), hostile, retiredPrompt); err != nil || !recorded {
		t.Fatalf("recording the prior decline: recorded=%v err=%v", recorded, err)
	}

	brain := &withholdingWhere{inner: &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: 0.95}}
	runOwedWorker(t, e, brain)
	offeredOnce := brain.withheld
	if offeredOnce == 0 {
		t.Fatal("the first pass never sent the message declined under a retired prompt; it is owed one offering")
	}
	if got := declinedUnder(t, e, hostile, "owed_verdict_declined_ruleset"); got != owedverdict.Ruleset {
		t.Fatalf("the decline names %q, want the prompt that declined it now, %q", got, owedverdict.Ruleset)
	}

	runOwedWorker(t, e, brain)
	if brain.withheld != offeredOnce {
		t.Errorf("the second pass sent the message again (%d calls, want %d); a decline under the current prompt stands",
			brain.withheld, offeredOnce)
	}
	if got := verdictOf(t, e, hostile); got != nil {
		t.Errorf("the declined message was judged %q; it stays unjudged", *got)
	}
}
