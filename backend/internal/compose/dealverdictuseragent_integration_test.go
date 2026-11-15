// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAUsersAgentEditProtectsTheDealFromNightlyChanges(t *testing.T) {
	v := setupDealVerdict(t)
	ctx := scopedArchiveAgent(t, v.Env)
	amount := int64(3_000_000)
	if _, err := v.Deals.UpdateDeal(ctx, ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	var read dealVerdictRead
	if err := database.WithWorkspaceTx(v.Admin(), v.Pool, func(tx pgx.Tx) error {
		return readVerdictEdits(v.Admin(), tx, v.deal, &read)
	}); err != nil {
		t.Fatal(err)
	}
	if at := read.state.HumanEditedAt[deals.VerdictAmountCorrection]; !at.After(v.now) {
		t.Fatalf("agent edit at %s was not read as the user's edit after %s", at, v.now)
	}
	v.run(t)
	if got := v.field(t, "amount_minor"); got != "3000000" {
		t.Fatalf("amount = %s, want the user's agent edit kept", got)
	}
}

func TestTheVerdictWriteRechecksAUsersAgentEdit(t *testing.T) {
	v := setupDealVerdict(t)
	amount := int64(3_000_000)
	if _, err := v.Deals.UpdateDeal(scopedArchiveAgent(t, v.Env), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	var read dealVerdictRead
	if err := database.WithWorkspaceTx(v.Admin(), v.Pool, func(tx pgx.Tx) error {
		var err error
		read, err = readDealVerdict(v.Admin(), tx, v.deal, nil, v.now, v.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	result, err := v.Deals.ApplyVerdict(v.Admin(), deals.VerdictInput{
		DealID: ids.From[ids.DealKind](v.deal), Owner: v.Rep1, Seen: read.state.Seen,
		Changes:       []deals.VerdictChange{{Correction: deals.VerdictAmountCorrection, AmountMinor: 4_500_000, NewestCitedAt: v.now}},
		Allowed:       func(context.Context, pgx.Tx, string) (bool, error) { return true, nil },
		EvidenceHolds: func(context.Context, pgx.Tx) (bool, error) { return true, nil },
		Record: func(context.Context, pgx.Tx, []deals.VerdictChange) error {
			t.Error("a protected edit was overwritten")
			return nil
		},
		AsOwner: ownersAgentTx(v.pass.users, v.Rep1, marginceActorID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Outcomes) != 1 || result.Outcomes[0] != deals.VerdictDroppedHuman {
		t.Fatalf("outcomes = %v, want the user's edit protected", result.Outcomes)
	}
	if got := v.field(t, "amount_minor"); got != "3000000" {
		t.Fatalf("amount = %s, want the user's agent edit kept", got)
	}
}

func TestAutomaticDealEditsDoNotBecomeUserEdits(t *testing.T) {
	v := setupDealVerdict(t)
	actor, ok := principal.Actor(v.Admin())
	if !ok {
		t.Fatal("the administrator has no actor")
	}
	actor.Type, actor.ID = principal.PrincipalAgent, "agent:automatic"
	actor.OnBehalfOf, actor.PassportID = v.Rep1, ids.Nil
	ctx := principal.WithActor(v.Admin(), actor)
	amount := int64(3_000_000)
	if _, err := v.Deals.UpdateDeal(ctx, ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	var read dealVerdictRead
	if err := database.WithWorkspaceTx(v.Admin(), v.Pool, func(tx pgx.Tx) error {
		return readVerdictEdits(v.Admin(), tx, v.deal, &read)
	}); err != nil {
		t.Fatal(err)
	}
	if at := read.state.HumanEditedAt[deals.VerdictAmountCorrection]; at.After(v.now) {
		t.Fatalf("automatic edit became a user edit at %s", at)
	}
	v.run(t)
	if got := v.field(t, "amount_minor"); got != "4500000" {
		t.Fatalf("amount = %s, want automatic work to remain eligible for correction", got)
	}
}
