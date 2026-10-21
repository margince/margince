// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Historical rows give the native sweep an observed population, not a fallback.
func (e *closeDateEnv) seedObservedPace(t *testing.T) {
	t.Helper()
	for range deals.CloseDateMinHistory {
		id := ids.NewV7()
		if _, err := e.owner.Exec(context.Background(), `INSERT INTO deal (id,name,pipeline_id,stage_id,status,closed_at,source,captured_by) VALUES ($1,'Observed won deal',$2,$3,'won',now(),'manual','human:x')`, id, e.pipeline, e.late); err != nil {
			t.Fatal(err)
		}
		if _, err := e.owner.Exec(context.Background(), `INSERT INTO deal_stage_history(deal_id,to_stage_id,changed_by,changed_at) VALUES ($1,$2,'human:x',now()-interval '28 days'),($1,$3,'human:x',now()-interval '14 days')`, id, e.early, e.late); err != nil {
			t.Fatal(err)
		}
	}
}

func TestThinHistoryLeavesDatesAndCorrectionLedgerUntouched(t *testing.T) {
	for _, remaining := range []int{0, deals.CloseDateMinHistory - 1} {
		t.Run(strconv.Itoa(remaining), func(t *testing.T) {
			e := setupCloseDate(t)
			if _, err := e.owner.Exec(context.Background(), `UPDATE deal SET archived_at=now() WHERE id IN (SELECT id FROM deal WHERE pipeline_id=$1 AND status='won' ORDER BY id OFFSET $2)`, e.pipeline, remaining); err != nil {
				t.Fatal(err)
			}
			for _, date := range []*int{nil, new(-12)} {
				id := e.seedSweepDeal(t, "No observed estimate", e.early, nil, date, 3)
				before := e.readSwept(t, id)
				if err := e.sweep(); err != nil {
					t.Fatal(err)
				}
				after := e.readSwept(t, id)
				if after.provisional || (before.expectedClose == nil) != (after.expectedClose == nil) || (before.expectedClose != nil && !before.expectedClose.Equal(*after.expectedClose)) {
					t.Fatalf("thin history changed date: before=%+v after=%+v", before, after)
				}
				var changes int
				if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type='deal' AND entity_id=$1 AND evidence ? 'close_date_correction'`, id).Scan(&changes); err != nil {
					t.Fatal(err)
				}
				if changes != 0 {
					t.Fatalf("thin history wrote %d corrections", changes)
				}
			}
		})
	}
}

func TestObservedCloseDateCarriesItsActualSampleAndPace(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Observed estimate", e.early, nil, new(-12), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	got := e.readSwept(t, id)
	if !got.provisional {
		t.Fatal("observed history cannot confirm a buyer's date")
	}
	var samples, stages int
	var median float64
	if err := e.owner.QueryRow(context.Background(), `SELECT (evidence->>'close_date_sample_count')::int,(evidence->>'close_date_median_stage_days')::float8,(evidence->>'close_date_remaining_stages')::int FROM audit_log WHERE entity_type='deal' AND entity_id=$1 AND evidence ? 'close_date_correction'`, id).Scan(&samples, &median, &stages); err != nil {
		t.Fatal(err)
	}
	if samples != deals.CloseDateMinHistory || median != 14 || stages != 2 {
		t.Fatalf("invented pace: samples=%d median=%v stages=%d", samples, median, stages)
	}
}
