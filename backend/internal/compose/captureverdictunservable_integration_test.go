// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A model is composed and the router will not send this task to it.
//
// The state #6190 created and nothing anticipated: the verdicts are local-only,
// and a deployment whose `local_small` rung binds a hosted provider — which is
// the dev stack and four of the five shipped presets — has a brain that
// CanJudge() reports as present and a router that refuses every call.
//
// Before this, that error left judgeOne and failed the whole pass, on every row,
// for as long as the binding stood. These say it degrades to the answer the
// product already has for "no model can answer this".

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// unservableBrain is the router's answer for a local-only task with no local
// rung: a refusal that names itself, not a provider failure.
type unservableBrain struct{ calls int }

func (b *unservableBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.calls++
	return model.Response{}, ai.ErrLocalOnlyUnservable
}

// The pass SUCCEEDS and the sender reaches a human, rather than erroring the
// job on a configuration somebody chose.
func TestAnUnservableVerdictAsksAHumanRatherThanFailingThePass(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedCapturedMail(t, e, "ada@realco.example", "quote request")
	dispositionID := seedPendingDisposition(t, e, "ada@realco.example", "realco.example", activityID)

	brain := &unservableBrain{}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("the pass failed on a binding it cannot serve: %v", err)
	}

	// Retired to a human, which is what an installation with no model at all
	// gets. The row must not stay pending: nothing else advances it, so it
	// would sit invisible and the contact would silently never be created.
	if got := dispositionStatus(t, e, dispositionID); got == capture.PendingStatusPending {
		t.Errorf("disposition is still %q — a row nobody will ever judge looks exactly like one whose turn has not come", got)
	}
}

// The refusal is not retried. It is a standing property of the binding rather
// than a fault, so asking again produces the same answer and the pass would
// spend its whole budget learning that.
func TestAnUnservableVerdictIsNotReAskedWithinThePass(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedCapturedMail(t, e, "bob@realco.example", "another quote")
	seedPendingDisposition(t, e, "bob@realco.example", "realco.example", activityID)

	brain := &unservableBrain{}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	if brain.calls > 1 {
		t.Errorf("asked the refusing router %d times for one row; the re-ask exists for a low-confidence answer, not for a binding that cannot serve", brain.calls)
	}
}
