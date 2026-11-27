// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestSourceJournalWriteCosts measures the real transaction, audit and outbox path.
// It deliberately asserts rows rather than a wall-clock ceiling on a shared laptop.
func TestSourceJournalWriteCosts(t *testing.T) {
	o := seedOlder(t)
	id := o.e.SeedDeal(t, "Ordinary journal cost", o.pipeline, o.stage, nil)
	ctx := o.e.Admin()
	measureSourceWrite(t, "ordinary edit", 100, func(i int) {
		_, err := o.e.Deals.UpdateDeal(ctx, ids.From[ids.DealKind](id), deals.UpdateDealInput{Name: new(fmt.Sprintf("Ordinary journal cost %d", i))})
		if err != nil {
			t.Fatal(err)
		}
	})
	integration.SeedBeforeSourceCheck(t, o.owner, "deal", `INSERT INTO deal(id,name,status,pipeline_id,stage_id,source,captured_by)
 SELECT CASE WHEN n=1 THEN $1 ELSE public.uuidv7() END,'Journal correction '||n,'open',$2,$3,'seed','human:old'
 FROM generate_series(1,500) AS n`, o.pipeline, o.stage)
	system := systemCtx(o.e)
	measureSourceWrite(t, "full correction page", 1, func(_ int) {
		err := database.WithWorkspaceTx(system, o.e.Pool, func(tx pgx.Tx) error {
			moved, err := deals.CorrectDealSourcesTx(system, tx, ids.Nil, 500, ids.NewV7())
			if err == nil && len(moved) != 500 {
				t.Fatalf("corrected %d rows, want500", len(moved))
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	if count := o.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='deal' AND after->>'source'='api' AND before->>'source'='seed'`); count != 500 {
		t.Fatalf("correction lost or duplicated audits: %d", count)
	}
	if count := o.e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id=(o.envelope->'trace'->>'audit_log_id')::uuid WHERE a.before->>'source'='seed' AND o.envelope->>'type'='deal.updated'`); count != 500 {
		t.Fatalf("correction lost or duplicated events: %d", count)
	}
}

func measureSourceWrite(t *testing.T, label string, runs int, write func(int)) {
	t.Helper()
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	for i := range runs {
		write(i)
	}
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	t.Logf("%s: runs=%d ns/op=%d bytes/op=%d allocations/op=%d", label, runs, elapsed.Nanoseconds()/int64(runs), (after.TotalAlloc-before.TotalAlloc)/uint64(runs), (after.Mallocs-before.Mallocs)/uint64(runs))
}
