// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An entry's basis IS the deal's amount, copied at accrual, so a seat that may
// not read that amount on the deal must not read it off the ledger either. The
// basis is an `int` on the wire and cannot carry a withheld value, so the row
// leaves the read.
//
// The deal's mask differs from the partner's in the half that matters here: it
// CAN be conditioned on write authority, and the answer is a property of the
// deal row rather than of the entry. So the two tests below are one invariant —
// withheld where the caller could not change the deal, present where they
// could — and a fix that simply emptied the ledger would pass the first alone.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealAmountSeat is the ledger reader, with whatever masks the case gives it.
// The update verb is present throughout because write authority is the verb AND
// the row arm: a seat without it holds authority over nothing, and a condition
// that could never lift would make the second test agree with the first for the
// wrong reason.
func dealAmountSeat(masks ...principal.FieldMask) principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"commission": {Read: true},
			"deal":       {Read: true, Update: true},
		},
		RowScope:   principal.RowScopeTeam,
		FieldMasks: masks,
	}
}

func dealAmountMask(condition principal.MaskCondition) principal.FieldMask {
	return principal.FieldMask{Object: "deal", Field: "amount_minor", Condition: condition}
}

func TestAMaskedSeatReadsNoCommissionEntryPricedFromTheDealAmount(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)

	// The control first, or the emptiness below says nothing: the same seat
	// without the mask must reach the entry, so what removes it is the mask and
	// not the row scope.
	unmasked := e.As(e.Rep1, []ids.UUID{e.Team1}, dealAmountSeat())
	page, err := fx.ledger.List(unmasked, commissions.ListInput{DealID: &fx.deal})
	if err != nil {
		t.Fatalf("listing the ledger unmasked: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("the seat reads %d entry(ies) before any mask, want the one accrued — "+
			"the masked assertion below would be vacuous", len(page.Data))
	}

	masked := e.As(e.Rep1, []ids.UUID{e.Team1}, dealAmountSeat(dealAmountMask(principal.MaskAlways)))
	page, err = fx.ledger.List(masked, commissions.ListInput{DealID: &fx.deal})
	if err != nil {
		t.Fatalf("listing the ledger masked: %v", err)
	}
	if len(page.Data) != 0 {
		t.Errorf("the ledger listed %d entry(ies) to a seat that may not read the deal's "+
			"amount; basis_amount_minor IS that amount", len(page.Data))
	}

	// A sum over one entry is that entry, and its basis is the deal's amount.
	summary, err := fx.ledger.Summary(masked)
	if err != nil {
		t.Fatalf("summarising the ledger: %v", err)
	}
	if len(summary.Data) != 0 {
		t.Errorf("the open-liability summary answered %d row(s) built from a basis the "+
			"caller may not read", len(summary.Data))
	}

	entry := onlyLedgerEntry(t, e, fx)
	if _, err := fx.ledger.GetCommissionEntry(masked, entry); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading one entry under the mask = %v, want ErrNotFound", err)
	}
}

// The condition lifts on a deal this seat could change, and the fixture's deal
// is Rep1's own — so the entry stays. A mask that reached the ledger through
// the group closure could not answer this: "the deals you may write" is a
// question the entry has no column for.
func TestAConditionedDealMaskLeavesTheEntriesOfDealsTheSeatMayWrite(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)

	owner := e.As(e.Rep1, []ids.UUID{e.Team1},
		dealAmountSeat(dealAmountMask(principal.MaskOutsideWriteAuthority)))
	page, err := fx.ledger.List(owner, commissions.ListInput{DealID: &fx.deal})
	if err != nil {
		t.Fatalf("listing the ledger: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("the ledger listed %d entry(ies) to the owner of the deal it prices; a mask "+
			"conditioned on write authority lifts on a row they could change", len(page.Data))
	}
	if page.Data[0].BasisAmountMinor == 0 {
		t.Error("the basis came back zero to a seat the mask lifts for")
	}
}
