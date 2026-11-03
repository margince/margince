// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The plans behind the Worklist's two mail scans. Each predicate here runs once
// per candidate message. One that no index can answer reads a whole table per
// message, and a result test cannot see that.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// explainWithoutSeqScans plans statement on the seeded mailbox plus extra, with
// sequential scans priced out. A scan left in the plan means no index serves it.
func explainWithoutSeqScans(t *testing.T, extra, statement string, args ...any) planNode {
	t.Helper()
	return explainOnMailbox(t, extra, "SET LOCAL enable_seqscan = off", "EXPLAIN (FORMAT JSON) ", statement, args...)
}

// explainOnMailbox runs explain over statement on the seeded mailbox plus
// extra, after the planner setting given.
func explainOnMailbox(t *testing.T, extra, setting, explain, statement string, args ...any) planNode {
	t.Helper()
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded mailbox: %v", err)
		}
	}()
	seedMailbox(ctx, t, tx, busyMailbox)
	if extra != "" {
		if _, err := tx.Exec(ctx, extra); err != nil {
			t.Fatalf("seeding %s: %v", extra, err)
		}
	}
	if _, err := tx.Exec(ctx, setting); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := tx.QueryRow(ctx, explain+statement, args...).Scan(&raw); err != nil {
		t.Fatalf("planning %s: %v", statement, err)
	}
	return decodePlan(t, raw)
}

// decodePlan reads the one plan an EXPLAIN (FORMAT JSON) answered.
func decodePlan(t *testing.T, raw string) planNode {
	t.Helper()
	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	return plans[0].Plan
}

func TestTheSalesArmFindsAWorkingLeadByIndexAlone(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(),
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"company": {Read: true}, "relationship": {Read: true},
				"lead": {Read: true}, "deal": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	arms, err := customerArms(ctx, arg, liveRecord(openDealPredicate, "d"), liveRecord(workingLeadPredicate, "ld"))
	if err != nil {
		t.Fatal(err)
	}
	// Many working leads, so walking the working-lead index costs more than one
	// index lookup per column.
	plan := explainWithoutSeqScans(t, `
		INSERT INTO lead (full_name, status, source, captured_by)
		  SELECT 'Lead '||g, 'new', 'manual', 'human:seed' FROM generate_series(1, 5000) g;
		ANALYZE lead`,
		`SELECT 1 FROM activity_link sales WHERE `+arms, args...)
	var leadReads, promotedLookups int
	plan.walk(func(n planNode) {
		if planCopySuffix.ReplaceAllString(n.Alias, "") == "ld" {
			leadReads++
		}
		// The bitmap child carries the index name; the heap scan above it the alias.
		if n.IndexName == "idx_lead_promoted_contact" && n.IndexCond != "" {
			promotedLookups++
		}
	})
	if leadReads == 0 {
		t.Fatal("the plan reads no lead under the alias ld: the lead arm was not rendered, so this test checks nothing")
	}
	if promotedLookups == 0 {
		t.Errorf("the lead arm never looks up idx_lead_promoted_contact: the promoted-contact alternative is filtered row by row over every working lead")
	}
}

func TestTheWaitingHorizonReadsAHeldRequestByItsID(t *testing.T) {
	plan := explainWithoutSeqScans(t, "", `SELECT a.id FROM activity a
		WHERE a.direction = 'inbound' AND a.archived_at IS NULL
		  AND `+withinWaitingHorizonSQL("$1::timestamptz", "90"), clocktest.Now(t))
	var heldByID, youngByDate int
	plan.walk(func(n planNode) {
		if n.NodeType != "Bitmap Index Scan" {
			return
		}
		if n.IndexName == "activity_pkey" && strings.Contains(n.IndexCond, "id = ANY") {
			heldByID++
		}
		if strings.Contains(n.IndexCond, "occurred_at >=") {
			youngByDate++
		}
	})
	if heldByID == 0 {
		t.Errorf("no arm of the horizon reads activity by its primary key: the held-request arm is tested on every inbound message ever captured, not looked up from the reminders that hold one")
	}
	if youngByDate == 0 {
		t.Errorf("no arm of the horizon reads a date range from an index: the scan reads every inbound message, not the ones inside the horizon")
	}
}

// Old mail never reaches a per-message subquery: the horizon is a cheap
// top-level test, and Postgres runs it first. The probe stands in for the
// subqueries a full waiting scan carries.
func TestTheWaitingHorizonRunsBeforeAnyPerMessageSubquery(t *testing.T) {
	at := clocktest.Now(t)
	plan := explainOnMailbox(t, "", "SET LOCAL enable_bitmapscan = off; SET LOCAL max_parallel_workers_per_gather = 0",
		"EXPLAIN (ANALYZE, FORMAT JSON) ", `SELECT a.id FROM activity a
		WHERE a.direction = 'inbound' AND a.archived_at IS NULL
		  AND (SELECT count(*) FROM activity_participant probe WHERE probe.activity_id = a.id) >= 0
		  AND `+withinWaitingHorizonSQL("$1::timestamptz", "30"), at)
	var inbound, kept, probed float64
	plan.walk(func(n planNode) {
		switch {
		case n.Alias == "a" && n.RelationName == "activity":
			inbound, kept = n.ActualRows+n.RemovedByFilter, n.ActualRows
		case n.Alias == "probe":
			probed = n.ActualLoops
		}
	})
	if inbound == 0 || kept == 0 || kept >= inbound {
		t.Fatalf("the scan read %v inbound messages and kept %v: the mailbox does not straddle the horizon, so this test checks nothing", inbound, kept)
	}
	if probed > kept {
		t.Errorf("the per-message probe ran %v times for %v messages inside the horizon: mail past the horizon is judged by every subquery before its age", probed, kept)
	}
}
