// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The Close column shows a closed deal's closed day and an open deal's
// forecast, so sorting by close_day orders by what the column shows. The won
// deal keeps a forecast a year out, which the forecast sort ranks last.
func TestTheDealsListSortsByTheCloseDayItDraws(t *testing.T) {
	e := Setup(t)
	pipeline, openStage, wonStage := DealFixture(t, e)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, dealCFVPerms)

	today := clocktest.Now(t).UTC().Truncate(24 * time.Hour)
	create := func(name string, forecast time.Time) ids.UUID {
		t.Helper()
		d, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: openStage, ExpectedClose: &forecast,
		})
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		return ids.UUID(d.Id)
	}
	openDeal := create("Open next month", today.AddDate(0, 1, 0))
	wonDeal := create("Won with a stale forecast", today.AddDate(1, 0, 0))
	if _, err := e.Deals.AdvanceDeal(ctx, ids.From[ids.DealKind](wonDeal), wonInput(wonStage)); err != nil {
		t.Fatalf("winning the deal: %v", err)
	}

	listed := func(spec string) []ids.UUID {
		t.Helper()
		rows, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{Sort: &spec})
		if err != nil {
			t.Fatalf("ListDeals(sort=%s): %v", spec, err)
		}
		out := make([]ids.UUID, len(rows))
		for i, d := range rows {
			out[i] = ids.UUID(d.Id)
		}
		return out
	}
	assertIDOrder(t, listed("expected_close_date"), []ids.UUID{openDeal, wonDeal}, "forecast ascending")
	assertIDOrder(t, listed("close_day"), []ids.UUID{wonDeal, openDeal}, "close day ascending")
	assertIDOrder(t, listed("-close_day"), []ids.UUID{openDeal, wonDeal}, "close day descending")
}
