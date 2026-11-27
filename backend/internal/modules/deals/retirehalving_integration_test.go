// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// writeMarginceProbability sets a deal's own probability as Margince, through
// the audit row the writers leave, with the evidence its writer states.
func (e *configEnv) writeMarginceProbability(t *testing.T, deal ids.DealID, value int, evidence map[string]any) {
	t.Helper()
	ctx := principal.SystemActing(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()), "system:test")
	if err := e.store.Tx(ctx, func(tx pgx.Tx) error {
		var held *int
		var heldBy *string
		if err := tx.QueryRow(ctx, `SELECT win_probability_override, win_probability_set_by FROM deal WHERE id = $1`, deal.UUID).Scan(&held, &heldBy); err != nil {
			return err
		}
		p := storekit.NewPatch()
		setWinProbability(p, held, heldBy, &value, SourceMargince)
		if _, err := tx.Exec(ctx, `UPDATE deal SET win_probability_override = $2, win_probability_set_by = 'margince' WHERE id = $1`, deal.UUID, value); err != nil {
			return err
		}
		_, err := storekit.AuditWithEvidence(ctx, tx, "update", "deal", deal.UUID, p.Before(), p.After(), evidence)
		return err
	}); err != nil {
		t.Fatalf("writing the probability as Margince: %v", err)
	}
}

func (e *configEnv) retireHalved(t *testing.T) int {
	t.Helper()
	ctx := principal.SystemActing(principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7()), "system:test")
	cleared := 0
	if err := e.store.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		_, cleared, err = e.store.RetireHalvedProbabilitiesTx(ctx, tx, ids.Nil, 50)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return cleared
}

// The retired nightly halving is cleared where the deal still holds the value
// that write left. A probability a colleague set afterwards, and one a
// verdict set afterwards, are other writes and stay.
func TestTheRetiredHalvingIsClearedOnlyWhereItStillStands(t *testing.T) {
	t.Parallel()
	e := setupConfigEnv(t)
	halved, from, _ := seedOwnedDeal(t, e)
	var pipeline ids.PipelineID
	if err := e.owner.QueryRow(context.Background(), `SELECT pipeline_id FROM stage WHERE id = $1`, from).Scan(&pipeline); err != nil {
		t.Fatal(err)
	}
	owner := ids.From[ids.UserKind](e.admin)
	another := func() ids.DealID {
		created, err := e.store.CreateDeal(e.asDealWriter(), CreateDealInput{Name: "Quiet", Source: "manual", PipelineID: pipeline, StageID: from, OwnerID: &owner, OwnerExact: true})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.DealKind](ids.UUID(created.Id))
	}
	byHand, byVerdict := another(), another()
	sweep := map[string]any{CloseDateCorrectionKind: "downgrade_and_review"}
	for _, deal := range []ids.DealID{halved, byHand, byVerdict} {
		e.writeMarginceProbability(t, deal, 45, sweep)
	}
	human := 60
	if _, err := e.store.UpdateDeal(e.asDealWriter(), byHand, UpdateDealInput{WinProbability: &human}); err != nil {
		t.Fatal(err)
	}
	e.writeMarginceProbability(t, byVerdict, 30, map[string]any{CloseDateCorrectionKind: "deal_verdict"})

	if got := e.retireHalved(t); got != 1 {
		t.Fatalf("cleared %d deals, want only the one still at the halved value", got)
	}
	holds := func(deal ids.DealID) (int, string) {
		var value *int
		var by *string
		if err := e.owner.QueryRow(context.Background(), `SELECT win_probability_override, win_probability_set_by FROM deal WHERE id = $1`, deal.UUID).Scan(&value, &by); err != nil {
			t.Fatal(err)
		}
		if value == nil || by == nil {
			return -1, ""
		}
		return *value, *by
	}
	if value, _ := holds(halved); value != -1 {
		t.Errorf("the halved deal still holds %d, want it back on its stage", value)
	}
	if value, by := holds(byHand); value != 60 || by != "human" {
		t.Errorf("the hand-set deal holds %d by %s, want 60 by human", value, by)
	}
	if value, by := holds(byVerdict); value != 30 || by != "margince" {
		t.Errorf("the verdict-set deal holds %d by %s, want 30 by margince", value, by)
	}
	var audited int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1
		   AND before->>'win_probability_override' = '45' AND after->>'win_probability_override' IS NULL`, halved.UUID).Scan(&audited); err != nil || audited != 1 {
		t.Errorf("the halved deal has %d audit rows for its clearing (%v), want one", audited, err)
	}
	if got := e.retireHalved(t); got != 0 {
		t.Fatalf("a replay cleared %d deals, want none", got)
	}
}
