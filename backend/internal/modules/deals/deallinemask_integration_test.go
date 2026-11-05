// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskedSeller may change the deal but not read its amount.
func (e *reviewEnv) maskedSeller() context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), e.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.admin.String(), UserID: e.admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"sales"}, RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{
				"deal": {Read: true, Update: true}, "product": {Read: true}, "offer": {Read: true},
			},
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: dealAmountField, Condition: principal.MaskAlways}},
		},
	})
}

// A reader the amount is masked from reads the lines without a single number:
// a quantity beside a catalogue price would rebuild the amount. The products
// read and the line writes hold the same mask.
func TestAMaskedReaderGetsNoNumberOfTheLines(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	addProductLine(t, e, dealID, 0, bananas, "100")
	months, count := 1, 12
	if _, err := e.store.AddDealLine(e.productCtx(), dealID, AddDealLineInput{
		ExpectedRevision: 1, Description: new("Cold storage"), Quantity: new("1"), UnitPriceMinor: new(int64(5_000)),
		BillingModel: new(BillingRecurring), IntervalMonths: &months, IntervalCount: &count,
	}); err != nil {
		t.Fatalf("adding the recurring line: %v", err)
	}

	state, err := e.store.GetDealLines(e.maskedSeller(), dealID)
	if err != nil {
		t.Fatalf("the masked read: %v", err)
	}
	if !state.MoneyWithheld || state.CommittedNetMinor != nil || state.KnownCommittedNetMinor != nil || state.Writable {
		t.Fatalf("the masked state is %+v, want every figure withheld and no write", state)
	}
	for _, line := range state.Lines {
		if line.Quantity != nil || line.UnitPriceMinor != nil || line.DiscountPct != nil || line.TaxRate != nil ||
			line.IntervalCount != nil || line.BillingIntervalMonths != nil || line.LineNetMinor != nil || line.CommittedNetMinor != nil {
			t.Fatalf("a masked line carries a number: %+v", line)
		}
	}
	products, err := e.store.ListDealProducts(e.maskedSeller(), dealID)
	if err != nil {
		t.Fatalf("the masked products read: %v", err)
	}
	if products[0].AmountMinor != nil || products[0].KnownAmountMinor != nil {
		t.Fatalf("a masked product row carries its value: %+v", products[0])
	}
	if _, err := e.store.AddDealLine(e.maskedSeller(), dealID, AddDealLineInput{ExpectedRevision: 2, ProductID: &bananas, Quantity: new("1")}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a masked seller's line write answered %v, want permission denied", err)
	}
}

// What a typed amount means is told only to a reader who may read it: the
// comparison with an accepted offer's gross would otherwise disclose it.
func TestAMaskedReaderIsNotToldTheAmountBasis(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	if _, err := e.store.UpdateDeal(e.productCtx(), dealID, UpdateDealInput{AmountMinor: new(int64(100_000)), Currency: new("EUR")}); err != nil {
		t.Fatalf("typing an estimate: %v", err)
	}
	open, err := e.store.GetDealLines(e.productCtx(), dealID)
	if err != nil || open.AmountBasis == nil {
		t.Fatalf("the reader of the amount got basis %v (%v), want one", open.AmountBasis, err)
	}
	masked, err := e.store.GetDealLines(e.maskedSeller(), dealID)
	if err != nil {
		t.Fatalf("the masked read: %v", err)
	}
	if masked.AmountBasis != nil {
		t.Fatalf("the masked reader was told basis %s", *masked.AmountBasis)
	}
}

// Every flow that patches the deal row meets the same refusal on a deal
// priced from lines; only the lines' own writer passes it.
func TestNoPatchWritesTheMoneyOfALinesDeal(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	addProductLine(t, e, dealID, 0, bananas, "100")

	err := e.store.Tx(e.productCtx(), func(tx pgx.Tx) error {
		lock, err := storekit.LockRow(e.productCtx(), tx, dealTable, dealID.UUID, storekit.LiveOnly)
		if err != nil {
			return err
		}
		p := storekit.NewPatch()
		p.Set(amountField, int64(20_000), int64(1))
		return applyDealPatchLocked(e.productCtx(), tx, p, lock)
	})
	var owned *LineOwnedValueError
	if !errors.As(err, &owned) {
		t.Fatalf("a locked patch of the amount answered %v, want deal_has_lines", err)
	}
	wantDealAmount(t, e, dealID, 20_000)
}

// The nightly verdict does not reprice a deal its lines price: the amount
// correction is dropped, not applied and not failed.
func TestAnAmountVerdictLeavesALinesDealAlone(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	bananas := seedPricedProduct(t, e, "Bananas", 200, "EUR")
	addProductLine(t, e, dealID, 0, bananas, "100")
	amount, currency := int64(20_000), "EUR"
	result, err := e.store.ApplyVerdict(e.productCtx(), VerdictInput{
		DealID: dealID, Owner: e.admin,
		Seen:          VerdictValues{AmountMinor: &amount, Currency: &currency},
		Changes:       []VerdictChange{{Correction: VerdictAmountCorrection, AmountMinor: 99_000, NewestCitedAt: clocktest.Now(t).Add(time.Hour)}},
		Allowed:       func(context.Context, pgx.Tx, string) (bool, error) { return true, nil },
		EvidenceHolds: func(context.Context, pgx.Tx) (bool, error) { return true, nil },
		Record:        func(context.Context, pgx.Tx, []VerdictChange) error { return nil },
	})
	if err != nil {
		t.Fatalf("applying the verdict: %v", err)
	}
	if len(result.Outcomes) != 1 || result.Outcomes[0] != VerdictDroppedLines {
		t.Fatalf("the amount verdict came out %v, want %s", result.Outcomes, VerdictDroppedLines)
	}
	wantDealAmount(t, e, dealID, 20_000)
}
