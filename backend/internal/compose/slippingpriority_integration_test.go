// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"testing"
	"time"
)

func TestOverdueDealIsFoundBehindMoreThanOnePageOfCurrentDeals(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	now := time.Date(2090, 1, 15, 12, 0, 0, 0, time.UTC)
	store := e.Deals.WithClock(func() time.Time { return now.AddDate(0, 0, -7) })
	closes := now.AddDate(0, 0, -1)
	overdue, err := store.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Renewal recovery", PipelineID: pipeline, StageID: stage, Source: "manual", ExpectedClose: &closes})
	if err != nil {
		t.Fatal(err)
	}
	future := now.AddDate(0, 0, 30)
	for i := 0; i < 105; i++ {
		if _, err := store.CreateDeal(e.Admin(), deals.CreateDealInput{Name: fmt.Sprintf("Current deal %d", i), PipelineID: pipeline, StageID: stage, Source: "manual", ExpectedClose: &future}); err != nil {
			t.Fatal(err)
		}
	}
	// A long quiet threshold isolates the overdue-close path in this test.
	candidates, cut, err := quietDealScanWithClock(e.Pool, 100000, func() time.Time { return now })(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	if cut {
		t.Fatal("current deals consumed the risk scan's bound")
	}
	if len(candidates) != 1 || candidates[0].DealID != ids.UUID(overdue.Id) || !candidates[0].CloseOverdue {
		t.Fatalf("expected the overdue renewal alone, got %d candidates", len(candidates))
	}
}
