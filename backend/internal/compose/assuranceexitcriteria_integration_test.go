// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// What counts a criterion as met, asked of the statement rather than of the
// rule. The rule reads one number; this is where that number is decided, and
// the four cases differ only in the evidence attached.
func TestACriterionCountsAsMetOnlyOnUnrefutedEvidenceSayingSo(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	// One stage and one criterion for every case. The count is per deal, so
	// each case brings its own deal and its own evidence against this same
	// criterion. A criterion per case would put every case's criteria in every
	// deal's count.
	pipeline, open, _ := integration.DealFixture(t, e)
	criterion := integration.SeedIDRow(t, owner, `
		INSERT INTO stage_exit_criterion (id, stage_id, key, label, kind, "position")
		VALUES ($1, $2, 'buyer_confirmed', 'The buyer confirmed', 'buyer_confirmed', 0)`,
		open)

	for _, tc := range []struct {
		name string
		// evidence is the row attached to the criterion, or none when empty.
		met       bool
		refuted   bool
		attach    bool
		wantUnmet int
	}{
		{name: "no evidence at all", attach: false, wantUnmet: 1},
		{name: "evidence saying not met", attach: true, met: false, wantUnmet: 1},
		{name: "evidence saying met", attach: true, met: true, wantUnmet: 0},
		{name: "evidence saying met, since refuted", attach: true, met: true, refuted: true, wantUnmet: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deal := integration.SeedIDRow(t, owner, `
				INSERT INTO deal (id, pipeline_id, stage_id, name, owner_id, status,
				                  source, captured_by, forecast_category)
				VALUES ($1, $2, $3, 'Exit criteria subject', $4, 'open', 'manual', 'test', 'commit')`,
				pipeline, open, e.Rep1)
			if tc.attach {
				// Both halves of the refutation or neither: the table refuses
				// a moment without an author.
				if _, err := owner.Exec(ctx, `
					INSERT INTO deal_stage_evidence (deal_id, criterion_id, source_type, source_id,
					                                 author_side, commitment, met, observed_at,
					                                 extracted_by, refuted_at, refuted_by)
					VALUES ($1, $2, 'activity', $3, 'buyer', 'agreed', $4, now(), 'test',
					        CASE WHEN $5 THEN now() ELSE NULL END,
					        CASE WHEN $5 THEN $6::uuid ELSE NULL END)`,
					deal, criterion, ids.NewV7(), tc.met, tc.refuted, e.Rep1); err != nil {
					t.Fatalf("seeding evidence: %v", err)
				}
			}

			var got int
			for _, s := range subjectsFor(t, e) {
				if s.DealID == deal.String() {
					got = s.UnmetExitCriteria
				}
			}

			if got != tc.wantUnmet {
				t.Errorf("unmet = %d, want %d", got, tc.wantUnmet)
			}
		})
	}
}
