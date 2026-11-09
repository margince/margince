// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestDailyReceiptsCarryTheAppliedDealChange(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	seats := createDailySeats(t, e)
	seats.Reps = seats.Reps[:1]
	rep := seats.Reps[0]
	stages := apptest.DiscoverSeededPipeline(t, e)
	for range 6 {
		mustCall(t, e, "POST", "/v1/deals", AnyMap{
			"name": "Receipt fixture", "source": "manual", "owner_id": rep.UserID,
			"pipeline_id": stages.PipelineID, "stage_id": stages.Open,
		}, 201, nil)
	}
	made := seedDailyApprovals(t, e, seats, 0.001)
	makeDailyBriefs(t, e, seats)
	made = applyDailyApprovals(t, e, made)
	if made.Applied != 2 || made.Pending != 2 || made.Expired != 2 {
		t.Fatalf("receipt counts = %+v, want two of each", made)
	}
	var changed, audited, consumed int
	err := e.Owner.QueryRow(t.Context(), `SELECT
		count(*) FILTER (WHERE d.expected_close_date::text = a.proposed_change->>'expected_close_date'),
		count(*) FILTER (WHERE EXISTS (SELECT 1 FROM audit_log l WHERE l.entity_id = d.id
			AND l.action = 'update' AND l.after ? 'expected_close_date' AND l.on_behalf_of = d.owner_id)),
		count(*) FILTER (WHERE a.consumed_at IS NOT NULL AND a.effect_ran_at IS NOT NULL)
		FROM approval a JOIN deal d ON d.id = a.target_entity_id
		WHERE a.kind = 'close_date_correction' AND a.status = 'approved'`).Scan(&changed, &audited, &consumed)
	if err != nil {
		t.Fatal(err)
	}
	if changed != made.Applied || audited != made.Applied || consumed != made.Applied {
		t.Fatalf("%d receipts have %d changed deals, %d audit trails, %d consumed effects", made.Applied, changed, audited, consumed)
	}
	for _, path := range []string{"/v1/worklist/handled", "/v1/magic?limit=100"} {
		var rows struct {
			Receipts []json.RawMessage `json:"receipts"`
			Done     []json.RawMessage `json:"done"`
		}
		mustCall(t, rep.env(e), "GET", path, nil, 200, &rows)
		if len(rows.Receipts)+len(rows.Done) == 0 {
			t.Fatalf("%s has no handled work", path)
		}
	}
}
