// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The first customer a partner sourced and won activates it. The tests drive
// the real win, the envelope it put in the outbox, and the real consumer.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// partnerOnReady puts the fixture's partner on the ready stage, one step short
// of an active one, so an activation shows as a move.
func partnerOnReady(t *testing.T, e *Env, fx accrualFixture, attribution string) {
	t.Helper()
	admin := e.As(e.AdminUser, nil, commissionAdminPerms)
	if _, err := e.Contacts.UpsertPartner(admin, contacts.UpsertPartnerInput{
		CompanyID: fx.partner, PartnerRole: "consulting", RelationshipStage: new("ready"),
	}); err != nil {
		t.Fatalf("moving the partner to ready: %v", err)
	}
	if _, err := e.Deals.UpdateDeal(admin, fx.deal, deals.UpdateDealInput{PartnerAttribution: &attribution}); err != nil {
		t.Fatalf("attributing the deal %s: %v", attribution, err)
	}
}

// winAndActivate wins the deal and answers a delivery of the envelope the win
// emitted, so a caller can deliver it twice.
func winAndActivate(t *testing.T, e *Env, fx accrualFixture) func() error {
	t.Helper()
	if _, err := e.Deals.AdvanceDeal(e.As(e.AdminUser, nil, commissionAdminPerms), fx.deal, deals.AdvanceDealInput{
		ToStageID: fx.won, WonWithoutContractReason: WonByImport(),
	}); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}
	env := lastStageChanged(t, e, fx.deal)
	consumer := compose.NewPartnerActivation(e.Pool, contacts.NewStore(e.DB()), slog.New(slog.DiscardHandler))
	return func() error { return consumer.HandleEvent(context.Background(), env) }
}

func TestASourcedWinActivatesItsPartnerOnce(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	partnerOnReady(t, e, fx, "sourced")
	deliver := winAndActivate(t, e, fx)
	if err := deliver(); err != nil {
		t.Fatalf("the activation consumer refused the win: %v", err)
	}
	admin := e.As(e.AdminUser, nil, commissionAdminPerms)
	activated, err := e.Contacts.GetPartner(admin, fx.partner)
	if err != nil {
		t.Fatal(err)
	}
	if activated.ActivationBasis == nil || *activated.ActivationBasis != "customer_won" ||
		activated.ActivationDealId == nil || ids.UUID(*activated.ActivationDealId) != fx.deal.UUID ||
		*activated.RelationshipStage != "activated" || activated.ActivatedAt == nil {
		t.Fatalf("after a sourced win the partner reads basis %v, deal %v, stage %s; want customer_won through the deal, on activated",
			activated.ActivationBasis, activated.ActivationDealId, *activated.RelationshipStage)
	}

	if err := deliver(); err != nil {
		t.Fatalf("a replayed win was refused: %v", err)
	}
	replayed, err := e.Contacts.GetPartner(admin, fx.partner)
	if err != nil {
		t.Fatal(err)
	}
	if *replayed.Version != *activated.Version || !replayed.ActivatedAt.Equal(*activated.ActivatedAt) {
		t.Errorf("a replayed win changed the partner: version %d to %d", *activated.Version, *replayed.Version)
	}
}

func TestAnInfluencedWinNeverActivatesItsPartner(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	partnerOnReady(t, e, fx, "influenced")
	deliver := winAndActivate(t, e, fx)
	if err := deliver(); err != nil {
		t.Fatalf("the activation consumer refused the win: %v", err)
	}
	partner, err := e.Contacts.GetPartner(e.As(e.AdminUser, nil, commissionAdminPerms), fx.partner)
	if err != nil {
		t.Fatal(err)
	}
	if partner.ActivationBasis != nil || *partner.RelationshipStage != "ready" {
		t.Errorf("an influenced win left the partner with basis %v on %s; want unactivated on ready",
			partner.ActivationBasis, *partner.RelationshipStage)
	}
}
