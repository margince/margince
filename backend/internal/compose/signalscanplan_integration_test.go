// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
)

// keyedOnTheOuterThread is an index condition taking its key from the outer
// row. The planner names it by the column under the aggregate.
var keyedOnTheOuterThread = regexp.MustCompile(`^\(thread_key = \w+\.thread_key\)$`)

// The mail reader's queue reaches each thread's scan row by a probe of that
// thread's key. The planner estimates the conversation aggregate at a few rows.
// Under that estimate a plain join read the whole scan table once per thread.
//
// Sequential scans are off so a test-sized table cannot hide the shape. The
// probe must still be keyed on the outer thread rather than a whole-index walk.
func TestTheMailReaderQueueProbesEachThreadsScanRowByKey(t *testing.T) {
	e := integration.Setup(t)
	now := clocktest.Now(t)
	var raw []byte
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `EXPLAIN (FORMAT JSON) `+dueThreadsQuery,
			now.Add(-extractSettleHours*time.Hour), 200, extractRefusalCap, now.Add(-extractParkFor)).Scan(&raw)
	}); err != nil {
		t.Fatalf("planning the queue: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // Postgres's EXPLAIN key, not ours to rename
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	var probes int
	plans[0].Plan.walkPlan(func(n planNode) {
		if n.Alias == "scan" && n.IndexName == "signal_thread_scan_pkey" && keyedOnTheOuterThread.MatchString(n.IndexCond) {
			probes++
		}
	})
	if probes != 1 {
		t.Errorf("the scan table is not probed once by each thread's key (%d keyed probes):\n%s", probes, raw)
	}
}
