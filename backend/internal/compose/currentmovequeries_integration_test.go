// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCurrentAdviceBatchesOwnerQueriesAcrossTheWholeCandidateSet(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	var wanted []ids.UUID
	for range 40 {
		deal := e.SeedDeal(t, "Demo expansion", pipeline, stage, &e.Rep1)
		subject := "Confirm scope"
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: &subject, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
		}); err != nil {
			t.Fatal(err)
		}
		wanted = append(wanted, deal)
	}
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	read := func(deals []ids.UUID) int64 {
		t.Helper()
		ctx, meter := database.WithStatementMeter(reader)
		moves, err := cards.CurrentMoves(ctx, deals)
		if err != nil || len(moves) != len(deals) {
			t.Fatalf("current candidate actions=%d/%d, %v", len(moves), len(deals), err)
		}
		return meter.Cost().Statements
	}
	one, page := read(wanted[:1]), read(wanted)
	if one == 0 || page > one+2 {
		t.Fatalf("current advice grew queries per candidate: one=%d, forty=%d", one, page)
	}
	t.Logf("actual owner-query count: one=%d, forty=%d", one, page)
}
