// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// netDealFixture is a deal owned by Rep1 whose accepted offer carries tax, left
// in the state the accept wrote before a deal's value left tax out: the deal
// holds the offer's gross and, when acceptWroteIt, the accept's audit row says the
// accept put it there.
type netDealFixture struct {
	deal       ids.UUID
	offer      ids.UUID
	net, gross int64
}

func seedGrossPricedDeal(t *testing.T, e *integration.Env, name, taxRate string, acceptWroteIt bool) netDealFixture {
	t.Helper()
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, name, pipeline, open, &e.Rep1)
	offerDesk := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.OfferDeskPerms)
	description, price := "Retainer", int64(23820)
	created, err := e.Deals.CreateOffer(offerDesk, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate}},
	})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := e.Deals.SendOffer(offerDesk, offer, nil); err != nil {
		t.Fatalf("send offer: %v", err)
	}
	accepted, err := e.Deals.AcceptOffer(offerDesk, offer, nil)
	if err != nil {
		t.Fatalf("accept offer: %v", err)
	}
	fx := netDealFixture{deal: deal, offer: offer.UUID, net: *accepted.NetMinor, gross: *accepted.GrossMinor}
	e.WsExec(t, `UPDATE deal SET amount_minor = $2 WHERE id = $1`, deal, fx.gross)
	if !acceptWroteIt {
		return fx
	}
	// The accept of that time recorded the gross it wrote. The audit log is
	// append-only, so the old accept's row is written again through the audit
	// writer beside the real accept's.
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, err := storekit.Audit(e.Admin(), tx, "update", "offer", offer.UUID,
			map[string]any{"status": "sent"}, map[string]any{"status": "accepted", "deal_amount_minor": fx.gross})
		return err
	}); err != nil {
		t.Fatalf("writing the earlier accept's audit row: %v", err)
	}
	return fx
}

func storedDealAmount(t *testing.T, e *integration.Env, deal ids.UUID) int64 {
	t.Helper()
	return int64(e.WsCount(t, `SELECT amount_minor FROM deal WHERE id = $1`, deal))
}

func netValueNotices(t *testing.T, e *integration.Env, deal ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM notice WHERE target_id = $1 AND kind = 'deal_value_net_basis'`, deal)
}

func TestTheOfferNetTaskCorrectsAGrossPricedDealAndTellsItsOwnerOnce(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	fx := seedGrossPricedDeal(t, e, "Taxed", "19.00", true)
	if fx.net == fx.gross {
		t.Fatal("the fixture offer carries no tax")
	}
	if n, _, err := offerNetDealValuesDataTask(e.Pool).Steps[0].Estimate(ctx, e.Pool, 10); err != nil || n != 1 {
		t.Fatalf("estimate = %d, %v; want the one gross-priced deal", n, err)
	}

	e.RunDataTask(ctx, t, offerNetDealValuesDataTask(e.Pool))
	e.RunDataTask(ctx, t, offerNetDealValuesDataTask(e.Pool))

	if got := storedDealAmount(t, e, fx.deal); got != fx.net {
		t.Fatalf("deal amount = %d, want the offer's net %d", got, fx.net)
	}
	if got := netValueNotices(t, e, fx.deal); got != 1 {
		t.Fatalf("%d notices about the deal across two runs, want one", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE target_id = $1 AND recipient_user_id = $2`, fx.deal, e.Rep1); got != 1 {
		t.Fatalf("the notice did not reach the deal's owner")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1
		 AND action = 'update' AND evidence ? 'offer_id' AND after ->> 'amount_minor' = $2`, fx.deal, strconv.FormatInt(fx.net, 10)); got != 1 {
		t.Fatalf("%d audit rows record the correction, want one", got)
	}
}

func TestTheOfferNetTaskLeavesAloneWhatIsNotAGrossFromAnAccept(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")

	retyped := seedGrossPricedDeal(t, e, "Retyped", "19.00", true)
	e.WsExec(t, `UPDATE deal SET amount_minor = 777000 WHERE id = $1`, retyped.deal)

	typedByHand := seedGrossPricedDeal(t, e, "Typed by hand", "19.00", false)

	untaxed := seedGrossPricedDeal(t, e, "Untaxed", "0.00", true)

	lines := seedGrossPricedDeal(t, e, "Lines", "19.00", true)
	e.WsExec(t, `UPDATE deal SET commercial_mode = 'lines' WHERE id = $1`, lines.deal)

	e.RunDataTask(ctx, t, offerNetDealValuesDataTask(e.Pool))

	for name, c := range map[string]struct {
		deal ids.UUID
		want int64
	}{
		"a retyped amount": {retyped.deal, 777000}, "no accept row behind the amount": {typedByHand.deal, typedByHand.gross},
		"an offer without tax": {untaxed.deal, untaxed.gross}, "a lines deal": {lines.deal, lines.gross},
	} {
		if got := storedDealAmount(t, e, c.deal); got != c.want {
			t.Errorf("%s: deal amount = %d, want it left at %d", name, got, c.want)
		}
		if got := netValueNotices(t, e, c.deal); got != 0 {
			t.Errorf("%s: %d notices, want none", name, got)
		}
	}
}

func TestTheOfferNetTaskRollsBackItsNoticeWithItsDomainWrite(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	fx := seedGrossPricedDeal(t, e, "Rolled back", "19.00", true)
	task := offerNetDealValuesDataTask(e.Pool)
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	e.RunDataTask(ctx, t, task)

	if got := storedDealAmount(t, e, fx.deal); got != fx.gross {
		t.Fatalf("the correction survived a rolled-back batch: amount %d", got)
	}
	if got := netValueNotices(t, e, fx.deal); got != 0 {
		t.Fatalf("a notice escaped the rollback: %d", got)
	}
}
