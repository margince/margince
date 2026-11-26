// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The briefing lane the Worklist reads writes nothing: an expired snooze is
// judged at read and the open is recorded by the request afterwards. A pool
// whose every transaction is read-only refuses any write the read still made.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// readOnlyPool is the test pool with every transaction opened read-only.
func readOnlyPool(t *testing.T, pool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	cfg := pool.Config()
	cfg.ConnConfig = cfg.ConnConfig.Copy()
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readOnly, err := testdb.OwnPoolFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(readOnly.Close)
	return readOnly
}

func TestTheBriefingLaneReadsAnExpiredSnoozeInAReadOnlyTransaction(t *testing.T) {
	b := setupBriefingLane(t)
	seedBriefingLaneDeals(t, b)
	run, err := b.engine.SnapshotRun(b.repCtx, b.now)
	if err != nil {
		t.Fatal(err)
	}
	timed, replied := run.Items[0], run.Items[1]
	setDown := b.now.Add(time.Minute)
	until := b.now.Add(time.Hour)
	if _, err := b.engine.MarkSnoozed(b.repCtx, timed.ID, values.ReopenOnTime, &until, nil, setDown, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := b.engine.MarkSnoozed(b.repCtx, replied.ID, values.ReopenOnReply, nil, nil, setDown, nil); err != nil {
		t.Fatal(err)
	}
	owner := integration.OwnerConn(t)
	reply := integration.SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, direction)
		VALUES ($1, 'email', 'they wrote back', $2, 'manual', 'human:x', 'inbound')`, setDown.Add(10*time.Minute))
	integration.LinkActivity(t, owner, reply, "deal", replied.DealID)
	b.now = until.Add(time.Minute)
	readOnly := b.reader
	readOnly.engine = briefs.NewBriefEngine(readOnlyPool(t, b.Pool), contacts.NewStore(InstallationDB(b.Pool)))

	got, ran, _, err := readOnly.Queue(b.repCtx, b.now)
	if err != nil {
		t.Fatalf("the briefing lane failed in a read-only transaction, so its read still writes: %v", err)
	}
	want, _, _, err := b.reader.Queue(b.repCtx, b.now)
	if err != nil {
		t.Fatal(err)
	}
	if !ran || !slices.Equal(got, want) {
		t.Fatalf("the read-only lane answered %+v (ran=%v), the read-write lane %+v", got, ran, want)
	}
	for _, item := range []briefs.BriefRunItem{timed, replied} {
		if !slices.ContainsFunc(got, func(entry attention.BriefEntry) bool { return entry.ID == item.ID && !entry.Snoozed }) {
			t.Errorf("the snooze on %s ended but the item is not back in the lane", item.ID)
		}
		if state := b.WsScalar(t, `SELECT state FROM brief_item WHERE id = $1`, item.ID); state != "snoozed" {
			t.Errorf("the read rewrote the ended snooze on %s to %q; it must judge the row, not change it", item.ID, state)
		}
	}
	if opens := b.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'brief.opened'`); opens != 0 {
		t.Errorf("a read no request showed recorded %d open(s), want none", opens)
	}
}
