// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type moveQueryCounter struct{ queries atomic.Int32 }

func (c *moveQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.queries.Add(1)
	return ctx
}

func (*moveQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

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
	counter := &moveQueryCounter{}
	config := e.Pool.Config()
	config.ConnConfig.Tracer = counter
	pool, err := testdb.OwnPoolFromConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cards := newDealStatusService(pool, func() time.Time { return at })
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	read := func(deals []ids.UUID) int32 {
		t.Helper()
		counter.queries.Store(0)
		moves, err := cards.CurrentMoves(reader, deals)
		if err != nil || len(moves) != len(deals) {
			t.Fatalf("current candidate actions=%d/%d, %v", len(moves), len(deals), err)
		}
		return counter.queries.Load()
	}
	one, page := read(wanted[:1]), read(wanted)
	if one == 0 || page > one+2 {
		t.Fatalf("current advice grew queries per candidate: one=%d, forty=%d", one, page)
	}
	t.Logf("actual owner-query count: one=%d, forty=%d", one, page)
}
