// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package offernetdeals_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/offernetdeals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// netDealFixture is a deal owned by Rep1 whose accepted offer carries tax.
// It is left as an accept wrote it before a deal's value left tax out.
// The deal holds the offer's gross, and when acceptWroteIt the deal's own audit
// row for that accept says so.
type netDealFixture struct {
	deal       ids.UUID
	offer      ids.UUID
	net, gross int64
}

type dealSeed struct {
	name, taxRate string
	acceptWroteIt bool
	closed        bool
}

func taskUnderTest(e *integration.Env) datatask.Task {
	return offernetdeals.DataTask(e.DB(), compose.DealsInstallation(), notices.NewStore(e.DB()))
}

func seedGrossPricedDeal(t *testing.T, e *integration.Env, pipeline ids.PipelineID, open, won ids.StageID, in dealSeed) netDealFixture {
	t.Helper()
	deal := e.SeedDeal(t, in.name, pipeline, open, &e.Rep1)
	dealID := ids.From[ids.DealKind](deal)
	if in.closed {
		amount, currency, reason := int64(100000), "EUR", "imported"
		if _, err := e.Deals.UpdateDeal(e.Admin(), dealID, deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
			t.Fatalf("pricing the deal: %v", err)
		}
		if _, err := e.Deals.AdvanceDeal(e.Admin(), dealID, deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: &reason}); err != nil {
			t.Fatalf("closing the deal: %v", err)
		}
	}
	offerDesk := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.OfferDeskPerms)
	description, price, taxRate := "Retainer", int64(23820), in.taxRate
	created, err := e.Deals.CreateOffer(offerDesk, dealID, deals.CreateOfferInput{
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
	if in.closed {
		e.WsExec(t, `UPDATE deal SET amount_minor_base = $2 WHERE id = $1`, deal, fx.gross)
	}
	if !in.acceptWroteIt {
		return fx
	}
	// The accept of that time re-priced the deal to the gross and said so in the
	// deal's audit row. The audit log is append-only, so that row is written
	// again through the audit writer, after the real accept's.
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, err := storekit.AuditWithEvidence(e.Admin(), tx, "update", "deal", deal,
			map[string]any{"amount_minor": fx.net}, map[string]any{"amount_minor": fx.gross},
			map[string]any{"offer_id": offer.UUID})
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
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	pipeline, open, won := integration.DealFixture(t, e)
	fx := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Taxed", taxRate: "19.00", acceptWroteIt: true})
	if fx.net == fx.gross {
		t.Fatal("the fixture offer carries no tax")
	}
	if n, _, err := taskUnderTest(e).Steps[0].Estimate(ctx, e.Pool, 10); err != nil || n != 1 {
		t.Fatalf("estimate = %d, %v; want the one gross-priced deal", n, err)
	}

	e.RunDataTask(ctx, t, taskUnderTest(e))
	e.RunDataTask(ctx, t, taskUnderTest(e))

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
		 AND action = 'update' AND evidence ? 'offer_id' AND before ->> 'amount_minor' = $2 AND after ->> 'amount_minor' = $3`, fx.deal, strconv.FormatInt(fx.gross, 10), strconv.FormatInt(fx.net, 10)); got != 1 {
		t.Fatalf("%d audit rows record the correction, want one", got)
	}
}

func TestTheOfferNetTaskLeavesAloneWhatIsNotAGrossFromAnAccept(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")

	pipeline, open, won := integration.DealFixture(t, e)
	retyped := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Retyped", taxRate: "19.00", acceptWroteIt: true})
	e.WsExec(t, `UPDATE deal SET amount_minor = 777000 WHERE id = $1`, retyped.deal)

	typedByHand := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Typed by hand", taxRate: "19.00"})

	untaxed := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Untaxed", taxRate: "0.00", acceptWroteIt: true})

	lines := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Lines", taxRate: "19.00", acceptWroteIt: true})
	e.WsExec(t, `UPDATE deal SET commercial_mode = 'lines' WHERE id = $1`, lines.deal)

	e.RunDataTask(ctx, t, taskUnderTest(e))

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
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	pipeline, open, won := integration.DealFixture(t, e)
	fx := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Rolled back", taxRate: "19.00", acceptWroteIt: true})
	task := taskUnderTest(e)
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

func TestTheOfferNetTaskLeavesADealAloneThatSomeoneRetypedAwayAndBackToTheGross(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	pipeline, open, won := integration.DealFixture(t, e)
	fx := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Back and forth", taxRate: "19.00", acceptWroteIt: true})
	for _, amount := range []int64{777000, fx.gross} {
		if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](fx.deal), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
			t.Fatalf("retyping the amount to %d: %v", amount, err)
		}
	}

	e.RunDataTask(ctx, t, taskUnderTest(e))

	if got := storedDealAmount(t, e, fx.deal); got != fx.gross {
		t.Fatalf("deal amount = %d, want the figure someone typed, %d, left alone", got, fx.gross)
	}
	if got := netValueNotices(t, e, fx.deal); got != 0 {
		t.Fatalf("%d notices, want none", got)
	}
}

func TestTheOfferNetTaskTellsTheOwnerTheDealHasWhenItIsCorrected(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	pipeline, open, won := integration.DealFixture(t, e)
	fx := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Reassigned", taxRate: "19.00", acceptWroteIt: true})
	newOwner := ids.From[ids.UserKind](e.Rep2)
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](fx.deal), deals.UpdateDealInput{OwnerID: &newOwner}); err != nil {
		t.Fatalf("reassigning the deal: %v", err)
	}

	e.RunDataTask(ctx, t, taskUnderTest(e))

	if got := e.WsCount(t, `SELECT count(*) FROM notice WHERE target_id = $1 AND recipient_user_id = $2 AND kind = 'deal_value_net_basis'`, fx.deal, e.Rep2); got != 1 {
		t.Fatalf("the new owner got %d notices, want one", got)
	}
	if got := netValueNotices(t, e, fx.deal); got != 1 {
		t.Fatalf("%d notices in all, want only the new owner's", got)
	}
}

func TestTheOfferNetTaskLeavesADealAloneWhoseCurrencyWasRestated(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	pipeline, open, won := integration.DealFixture(t, e)
	fx := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Restated", taxRate: "19.00", acceptWroteIt: true})
	e.WsExec(t, `UPDATE deal SET currency = 'USD' WHERE id = $1`, fx.deal)

	e.RunDataTask(ctx, t, taskUnderTest(e))

	if got := storedDealAmount(t, e, fx.deal); got != fx.gross {
		t.Fatalf("deal amount = %d, want it left at %d", got, fx.gross)
	}
}

func TestTheOfferNetTaskReFreezesAClosedDealOnTheNetAndLeavesItsCommissionAlone(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	pipeline, open, won := integration.DealFixture(t, e)
	fx := seedGrossPricedDeal(t, e, pipeline, open, won, dealSeed{name: "Closed", taxRate: "19.00", acceptWroteIt: true, closed: true})
	partner := e.SeedCompany(t, "Northgate Partners", nil)
	e.WsExec(t, `INSERT INTO commission_entry (id, deal_id, partner_company_id, attribution_at_accrual, basis_amount_minor,
		rate_bps, amount_minor, currency, fx_rate_to_base, captured_by)
		VALUES ($1, $2, $3, 'sourced', $4, 1000, 10000, 'EUR', 1.0, 'system:test')`, ids.NewV7(), fx.deal, partner, fx.gross)

	e.RunDataTask(ctx, t, taskUnderTest(e))

	if got := int64(e.WsCount(t, `SELECT amount_minor_base FROM deal WHERE id = $1`, fx.deal)); got != fx.net {
		t.Fatalf("frozen base amount = %d, want the net %d", got, fx.net)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM commission_entry WHERE deal_id = $1 AND basis_amount_minor = $2 AND amount_minor = 10000`, fx.deal, fx.gross); got != 1 {
		t.Fatalf("the commission entry changed or was restated")
	}
}
