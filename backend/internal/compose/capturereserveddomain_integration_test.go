// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// capture.skip_reserved_domain_proposals over the real path: the mail lands
// through the production sink into a shared mailbox, the model answers below
// the confidence floor so the sender retires to `unsure`, and the review sweep
// decides whether a human is asked.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAReservedDomainSenderRaisesNoContactProposal(t *testing.T) {
	e := setupWithOwnMailbox(t)
	cfg := CaptureConfig{SkipReservedDomainProposals: true}
	reserved := reviewQuestionThroughRealCapture(t, e, cfg, "qa@mail.example.com", "reserved-1")
	ordinary := reviewQuestionThroughRealCapture(t, e, cfg, "lena@kontor-nord.de", "reserved-2")

	if n := counterpartyProposals(t, e, reserved); n != 0 {
		t.Errorf("%d contact proposals for a sender on example.com, want 0", n)
	}
	if got := dispositionStatus(t, e, reserved); got != capture.PendingStatusRejected {
		t.Errorf("reserved sender's question is %q, want rejected — left unsure it returns on every pass", got)
	}
	if n := countIn(t, e, `
		SELECT count(*) FROM activity a JOIN capture_pending_counterparty p ON p.activity_id = a.id
		 WHERE p.id = $1 AND a.archived_at IS NULL`, reserved); n != 1 {
		t.Error("the reserved sender's mail left the timeline; only the proposal may be withheld")
	}
	if n := counterpartyProposals(t, e, ordinary); n != 1 {
		t.Errorf("%d contact proposals for an ordinary sender with the rule on, want 1", n)
	}
}

func TestTheRuleSwitchedOffProposesAReservedDomainSender(t *testing.T) {
	e := setupWithOwnMailbox(t)
	disposition := reviewQuestionThroughRealCapture(t, e, CaptureConfig{}, "buyer@acme.test", "reserved-off-1")

	if n := counterpartyProposals(t, e, disposition); n != 1 {
		t.Errorf("%d contact proposals for a .test sender with the rule off, want 1", n)
	}
}

// reviewQuestionThroughRealCapture captures one inbound mail from sender, lets
// a model answer it below the confidence floor, and runs the review sweep.
// Returns the sender's ledger row.
func reviewQuestionThroughRealCapture(
	t *testing.T, e *integration.Env, cfg CaptureConfig, sender, sourceID string,
) ids.UUID {
	t.Helper()
	captureInboundThroughRealSink(t, e, e.Rep1, sourceID, sender, "thread-"+sourceID)
	disposition, queued := openDisposition(t, e, sender)
	if !queued {
		t.Fatalf("capturing mail from %s opened no question", sender)
	}
	brain := &scriptedVerdictBrain{confidence: map[string]float64{disposition.String(): 0.2}}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, cfg, slog.Default())
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := engine.RunWorkspace(ctx, 0); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}
	if got := dispositionStatus(t, e, disposition); got != capture.PendingStatusUnsure {
		t.Fatalf("after a low-confidence answer %s is %q, want unsure", sender, got)
	}
	if err := engine.StageReviewsWorkspace(ctx, 0); err != nil {
		t.Fatalf("staging reviews: %v", err)
	}
	return disposition
}

func counterpartyProposals(t *testing.T, e *integration.Env, disposition ids.UUID) int {
	t.Helper()
	return countIn(t, e, `
		SELECT count(*) FROM approval
		 WHERE kind = 'capture_counterparty' AND proposed_change->>'disposition_id' = $1::text`, disposition)
}
