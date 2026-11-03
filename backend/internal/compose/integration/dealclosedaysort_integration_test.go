// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The Close column shows a closed deal's closed day and an open deal's
// forecast, so sorting by close_day orders by what the column shows. The days
// are cut in the installation zone, and a deal with nothing to show sits last.
func TestTheDealsListSortsByTheCloseDayItDraws(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Asia/Ho_Chi_Minh"'::jsonb WHERE key = 'installation.timezone'`)
	zone, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	pipeline, openStage, wonStage := DealFixture(t, e)
	lost := lostStage(t, e)
	ctx := e.Admin()

	now := clocktest.Now(t)
	e.Deals.WithClock(func() time.Time { return now })
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	create := func(name string, forecast *time.Time) ids.DealID {
		t.Helper()
		d, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: openStage, ExpectedClose: forecast,
		})
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		return ids.From[ids.DealKind](ids.UUID(d.Id))
	}
	forecast := func(days int) *time.Time { d := today.AddDate(0, 0, days); return &d }
	closeDeal := func(id ids.DealID, to ids.StageID, in deals.AdvanceDealInput, closedOn *time.Time) {
		t.Helper()
		in.ToStageID = to
		if _, err := e.Deals.AdvanceDeal(ctx, id, in); err != nil {
			t.Fatalf("closing: %v", err)
		}
		if closedOn != nil {
			if _, err := e.Deals.CorrectCloseDay(ctx, id, deals.CorrectCloseDayInput{ClosedOn: *closedOn}); err != nil {
				t.Fatalf("correcting the close day: %v", err)
			}
		}
	}
	reason := "verbal"
	lostReason := "went with a rival"

	openDated := create("Open next month", forecast(30))
	openUndated := create("Open with no forecast", nil)
	won := create("Won with a stale forecast", forecast(365))
	closeDeal(won, wonStage, deals.AdvanceDealInput{WonWithoutContractReason: &reason}, forecast(-5))
	lostDeal := create("Lost with a forecast", forecast(60))
	closeDeal(lostDeal, lost, deals.AdvanceDealInput{LostReason: &lostReason, LostReasonCode: new("other")}, forecast(-2))

	// 10:00Z and 18:30Z on one UTC date are one day in UTC. In Ho Chi Minh
	// the later one is already the next morning.
	utcDay := today.AddDate(0, 0, -8)
	earlyInZone := create("Closed before midnight there", nil)
	closeDeal(earlyInZone, wonStage, deals.AdvanceDealInput{WonWithoutContractReason: &reason}, nil)
	e.WsExec(t, `UPDATE deal SET closed_at = $2 WHERE id = $1`, earlyInZone, utcDay.Add(10*time.Hour))
	lateInZone := create("Closed after midnight there", nil)
	closeDeal(lateInZone, wonStage, deals.AdvanceDealInput{WonWithoutContractReason: &reason}, nil)
	e.WsExec(t, `UPDATE deal SET closed_at = $2 WHERE id = $1`, lateInZone, utcDay.Add(18*time.Hour+30*time.Minute))

	ascending := []ids.DealID{earlyInZone, lateInZone, won, lostDeal, openDated, openUndated}
	listed := func(spec string, limit int, advance time.Duration) []ids.DealID {
		t.Helper()
		var out []ids.DealID
		var cursor *string
		for range 10 {
			page, next, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{Sort: &spec, Limit: &limit, Cursor: cursor})
			if err != nil {
				t.Fatalf("ListDeals(sort=%s): %v", spec, err)
			}
			for _, d := range page {
				out = append(out, ids.From[ids.DealKind](ids.UUID(d.Id)))
			}
			if !next.HasMore {
				return out
			}
			cursor = &next.NextCursor
			now = now.Add(advance)
		}
		t.Fatalf("listing by %s did not end", spec)
		return nil
	}
	if got := listed("close_day", 50, 0); !slices.Equal(got, ascending) {
		t.Errorf("close day ascending = %v, want %v", got, ascending)
	}
	dated := slices.Clone(ascending[:len(ascending)-1])
	slices.Reverse(dated)
	// Nulls stay last in both directions.
	descending := append(slices.Clone(dated), openUndated)
	if got := listed("-close_day", 50, 0); !slices.Equal(got, descending) {
		t.Errorf("close day descending = %v, want %v", got, descending)
	}
	if got := listed("close_day", 2, 24*time.Hour); !slices.Equal(got, ascending) {
		t.Errorf("close day paged across local midnight = %v, want %v", got, ascending)
	}
	if got := listed("expected_close_date", 50, 0); slices.Equal(got, ascending) {
		t.Error("the forecast sort agrees with the close day sort, so the fixture does not tell them apart")
	}
}
