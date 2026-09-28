// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The needs-you lane, bound the way the server binds it.
//
// The lane reaches the staged queue through an adapter of four lines — the
// status it filters on, the limit it forwards, the engine it asks and the
// binding itself — and every one of those is invisible to a suite that spells
// its own. So the service under test here is the one newMagicService returns,
// and the only thing this file supplies is the proposal.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A DECISION WAITING REACHES THE RECEIPT THROUGH THE ENGINE THAT HOLDS IT.
//
// One engine stages the proposal and serves the lane, so the row drawn here is
// the row the inbox decides from rather than a second reading of the queue, and
// the row it returns is shaped by the engine's authority filter and its
// target-label freeze rather than by a fixture's idea of them.
func TestADecisionWaitingReachesTheReceiptThroughTheRealApprovalsEngine(t *testing.T) {
	e := integration.Setup(t)
	since := time.Now().Add(-time.Hour)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Weber GmbH — Phase 2", pipeline, open, &e.Rep1)

	queue := approvalsServiceWithEffects(e.Pool)
	change, hash, err := diffhash.Canonical(json.RawMessage(`{"stage": "won"}`))
	if err != nil {
		t.Fatal(err)
	}
	staged, err := queue.Stage(e.AgentCtx(), approvals.StageInput{
		Kind: "advance_deal", ProposedChange: change, DiffHash: hash,
		TargetType: "deal", TargetID: deal, Summary: "a summary naming nothing",
	})
	if err != nil {
		t.Fatalf("staging the decision: %v", err)
	}

	receipt, err := newMagicService(e.Pool, queue, time.Now).
		Read(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	var line crmcontracts.MagicLine
	var found bool
	for _, candidate := range receipt.NeedsYou {
		if ids.UUID(candidate.Id) == staged.UUID {
			line, found = candidate, true
		}
	}
	if !found {
		t.Fatalf("a proposal the engine staged is absent from needs_you: %+v", receipt.NeedsYou)
	}
	if line.Summary.Key != "magic.action.approval_advance_deal" {
		t.Errorf("summary key = %q, want the sentence advance_deal asks", line.Summary.Key)
	}
	// The caption the engine froze at staging, which is what the approver was
	// shown; a lane resolving it itself would name whatever the record became.
	if line.Summary.Values == nil || (*line.Summary.Values)["target"] != "Weber GmbH — Phase 2" {
		t.Errorf("summary values = %v, want the target the engine recorded", line.Summary.Values)
	}
	if line.Entity == nil || ids.UUID(line.Entity.Id) != deal {
		t.Errorf("entity = %+v, want the deal the proposal is about", line.Entity)
	}
	if receipt.Totals.NeedsYou != len(receipt.NeedsYou) {
		t.Errorf("totals.needs_you says %d over %d drawn lines",
			receipt.Totals.NeedsYou, len(receipt.NeedsYou))
	}
}
