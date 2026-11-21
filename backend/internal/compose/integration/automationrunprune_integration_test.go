// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The nightly pass removes an old run that changed nothing. It keeps every run
// something still reads: one with an effect, a failed one, a parked one, a
// young one and a lead rule's.
func TestTheNightlyPassRemovesOldRunsThatChangedNothing(t *testing.T) {
	e := Setup(t)
	run := func(handler, status, applied string, days int) ids.UUID {
		id := ids.NewV7()
		e.WsExec(t, `INSERT INTO workflow_run (id, handler, idempotency_key, trigger_event, planned, status, applied, created_at)
			VALUES ($1::uuid, $2, $1::uuid::text, $1::uuid, '[]', $3, $4::jsonb, now() - make_interval(days => $5))`,
			id, handler, status, applied, days)
		return id
	}
	const effect = `[{"Kind":"create_task"}]`
	gone := map[string]ids.UUID{
		"a skipped run":                 run("stale_deal_nudge", "skipped", "null", 31),
		"a blocked run":                 run("stale_deal_nudge", "blocked", "null", 31),
		"an applied run with no effect": run("stale_deal_nudge", "applied", "[]", 31),
	}
	kept := map[string]ids.UUID{
		"an applied run with an effect":   run("stale_deal_nudge", "applied", effect, 31),
		"a failed run":                    run("stale_deal_nudge", "failed", "null", 31),
		"a run waiting on an approval":    run("stale_deal_nudge", "requires_approval", effect, 31),
		"a skipped run inside its window": run("stale_deal_nudge", "skipped", "null", 29),
		"a lead rule's blocked run":       run("route_lead", "blocked", "null", 31),
	}

	if err := compose.NewRetentionServiceFor(e.DB(), nil, slog.Default()).EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	stored := func(id ids.UUID) bool {
		return e.WsCount(t, `SELECT count(*) FROM workflow_run WHERE id = $1`, id) == 1
	}
	for name, id := range gone {
		if stored(id) {
			t.Errorf("the pass kept %s past its window", name)
		}
	}
	for name, id := range kept {
		if !stored(id) {
			t.Errorf("the pass removed %s", name)
		}
	}
}
