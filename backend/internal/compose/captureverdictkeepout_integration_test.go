// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// `keep_out` against a sender the ledger never judged.
//
// The contract's promise is unconditional — "no record, and the mail this
// sender already brought in is destroyed" — but the effect ran only through the
// verdict engine's noise arm, which fires when a capture_pending_counterparty
// row is judged. A decision recorded for a sender whose row was settled before
// it, or who never opened one, destroyed nothing until a NEW message arrived.
// The contact half was already covered by the reconcile sweep; this is the mail
// half.
//
// Its own file because the authority is different from the one
// captureverdictnoise_integration_test.go is about. That one draws its bounds
// against a FORGED From turning a model's guess into permission; here the
// authority is a seat that typed the address, and the two bounds that answer a
// forger — the verdict's reach window and the bulk corroboration destruction
// asks for — have nothing to answer.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepingEngine is the engine with no brain: these cases judge nothing, they
// run the sweeps that act on a decision already recorded.
func sweepingEngine(e *integration.Env) *CounterpartyVerdictEngine {
	return NewCounterpartyVerdictEngine(e.Pool, nil, CaptureConfig{}, slog.Default())
}

func TestAKeepOutReachesMailTheLedgerNeverJudged(t *testing.T) {
	e := integration.Setup(t)
	// No seedPendingDisposition: this sender has no ledger row at all, which is
	// the whole case. An ordinary message, not a bulk one — a human's decision
	// is not the model's guess, so it does not wait for corroboration.
	activityID := seedCapturedMail(t, e, "ex.supplier@oldco.example", "invoice 4471")
	seedSenderOverride(t, e, e.Rep1, "ex.supplier@oldco.example", capture.OverrideKeepOut)

	engine := sweepingEngine(e)
	ws := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := engine.HideNoiseStragglersWorkspace(ws); err != nil {
		t.Fatalf("straggler sweep: %v", err)
	}
	if n := countIn(t, e,
		`SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NOT NULL`, activityID); n != 1 {
		t.Fatal("a standing keep_out left the sender's mail visible — the decision promises " +
			"the mail they already brought in is destroyed, and nothing had judged them")
	}

	// And destroyed, once the undo window a mistaken click needs has passed.
	backdateArchive(t, e, activityID)
	if err := engine.RedactNoiseWorkspace(ws, capture.NoiseUndoWindow, 0); err != nil {
		t.Fatalf("redaction sweep: %v", err)
	}
	if n := countIn(t, e,
		`SELECT count(*) FROM activity WHERE id = $1 AND subject IS NULL AND body IS NULL`,
		activityID); n != 1 {
		t.Fatal("the mail was hidden and kept — `keep_out` says destroyed, and the bulk " +
			"corroboration exists to doubt a MODEL rather than the seat whose mailbox this is")
	}
	if n := rawCaptureRows(t, e, activityID); n != 0 {
		t.Errorf("%d provider original(s) survived the destruction, want 0", n)
	}
}

// The scope rule is the whole of what keeps one seat's decision off a
// colleague's record, and it is the SAME rule the machine arm is held to — so
// the case that would make this dangerous is the one that must still be refused.
func TestAKeepOutStillCannotReachMailTheWorkspaceCorrespondsWith(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedCapturedMail(t, e, "cfo@bigcorp.example", "re: our contract renewal")
	seedOutboundMail(t, e, "cfo@bigcorp.example", "our proposal")
	seedSenderOverride(t, e, e.Rep1, "cfo@bigcorp.example", capture.OverrideKeepOut)

	if err := sweepingEngine(e).HideNoiseStragglersWorkspace(
		principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("straggler sweep: %v", err)
	}
	if n := countIn(t, e,
		`SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, activityID); n != 1 {
		t.Fatal("one seat's keep_out hid mail from an address the workspace writes to — " +
			"writing to someone is the signal that they are a counterparty, and it " +
			"calls the effect off for a human decision exactly as it does for a verdict")
	}
}

// A `business` decision is not a keep_out, and must move no mail. Without this
// the two cases above would pass on a corpus that read every override.
func TestABusinessDecisionMovesNoMail(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedCapturedMail(t, e, "dana.olsen@partner.example", "quarterly note")
	seedSenderOverride(t, e, e.Rep1, "dana.olsen@partner.example", capture.OverrideBusiness)

	if err := sweepingEngine(e).HideNoiseStragglersWorkspace(
		principal.WithWorkspaceID(context.Background(), e.WS)); err != nil {
		t.Fatalf("straggler sweep: %v", err)
	}
	if n := countIn(t, e,
		`SELECT count(*) FROM activity WHERE id = $1 AND archived_at IS NULL`, activityID); n != 1 {
		t.Fatal("a `business` decision hid the sender's mail — the sweep is reading every " +
			"override rather than the one that disowns a sender")
	}
}
