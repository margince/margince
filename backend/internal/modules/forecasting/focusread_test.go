// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ranked is a frozen focus row at rank, priced unless masked.
func ranked(rank int, masked bool) Contribution {
	c := openDeal(ids.NewV7().String(), int64(1000-rank), 3)
	c.Focus = &Focus{Rank: rank}
	if masked {
		c.AmountMinor, c.BaseMinor, c.WeightedMinor = nil, nil, 0
	}
	return c
}

func passThrough(_ context.Context, _ pgx.Tx, rows []Contribution) ([]Contribution, string, error) {
	return rows, "", nil
}

func TestAMaskedDealLeavesTheListAndTheRestAreNumberedAgain(t *testing.T) {
	t.Parallel()
	first, masked, third := ranked(1, false), ranked(2, true), ranked(3, false)
	h := Handlers{readerRows: passThrough}
	got, err := h.readerFocus(t.Context(), nil, FocusRead{Teams: []TeamFocus{{Deals: []Contribution{first, masked, third}}}})
	if err != nil {
		t.Fatal(err)
	}
	list := got.Teams[0]
	if list.Masked != 1 || len(list.Deals) != 2 {
		t.Fatalf("the list keeps %d deals and counts %d masked, want 2 and 1", len(list.Deals), list.Masked)
	}
	if list.Deals[0].DealID != first.DealID || list.Deals[0].Focus.Rank != 1 ||
		list.Deals[1].DealID != third.DealID || list.Deals[1].Focus.Rank != 2 {
		t.Errorf("the visible deals rank %d and %d, want 1 and 2 with no gap where the masked one stood",
			list.Deals[0].Focus.Rank, list.Deals[1].Focus.Rank)
	}
}

func TestATeamWhoseGoalTheReaderMayNotReadShowsNoStandingAndNoList(t *testing.T) {
	t.Parallel()
	behind := false
	readable := map[ids.UUID]bool{}
	open, closed := ids.NewV7(), ids.NewV7()
	readable[open] = true
	h := Handlers{goalReadable: func(_ context.Context, _ pgx.Tx, scope crmcontracts.ReportingScope) (bool, error) {
		return readable[ids.UUID(*scope.Id)], nil
	}}
	got, err := h.readableGoals(t.Context(), nil, FocusRead{Teams: []TeamFocus{
		{Team: open, OnTarget: &behind, Deals: []Contribution{ranked(1, false)}},
		{Team: closed, OnTarget: &behind, Deals: []Contribution{ranked(1, false)}, Masked: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Teams[0].OnTarget == nil || len(got.Teams[0].Deals) != 1 {
		t.Errorf("the readable team lost its standing or list: %+v", got.Teams[0])
	}
	if hidden := got.Teams[1]; hidden.OnTarget != nil || len(hidden.Deals) != 0 || hidden.Masked != 0 {
		t.Errorf("the unreadable team still shows %+v, want no standing, no deals and no count", hidden)
	}
}

func TestAListReadADayLaterCountsDaysToCloseOnTheReadersDay(t *testing.T) {
	t.Parallel()
	deal := ranked(1, false)
	deal.Focus.Reason = FocusReason{
		ClosesInDays: new(3),
		Signals:      []Signal{{Code: "no_next_step", ClosesInDays: new(3)}, {Code: "quiet", Days: new(9)}},
	}
	wire, err := focusDealToWire(deal, focusDay.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if wire.Reason.ClosesInDays == nil || *wire.Reason.ClosesInDays != 2 {
		t.Errorf("a deal frozen 3 days from close reads %v a day later, want 2", wire.Reason.ClosesInDays)
	}
	if wire.Reason.Signals[0].ClosesInDays != nil || wire.Reason.Signals[1].Days == nil {
		t.Errorf("the signals read %+v, want the frozen day count dropped from no_next_step only", wire.Reason.Signals)
	}
}

func TestNoNextStepIsACloseRiskOnlyWithinTheRecoveryHorizon(t *testing.T) {
	t.Parallel()
	noStep := []Signal{{Code: "no_next_step"}}
	if got := closeRisks(FocusReason{ClosesInDays: new(10), Signals: noStep}); got != 1 {
		t.Errorf("no next step ten days from close counts %d risks, want 1", got)
	}
	if got := closeRisks(FocusReason{ClosesInDays: new(30), Signals: noStep}); got != 0 {
		t.Errorf("no next step a month from close counts %d risks, want 0", got)
	}
}
