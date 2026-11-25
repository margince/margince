// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Two won deals of one opportunity count as two wins until they are merged. The
// review counts archived wins on purpose, so the merge must take the duplicate
// out of the count and leave the survivor's.
func TestTwoWonDuplicatesMergedCountAsOneWin(t *testing.T) {
	e := setupWeekly(t)
	company := e.SeedCompany(t, "Twin Wins GmbH", nil)
	first := ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Twin win", company))
	secondID := e.WsScalar(t, `INSERT INTO deal (name, owner_id, pipeline_id, stage_id, company_id, source, captured_by)
		SELECT 'Twin win copy', owner_id, pipeline_id, stage_id, company_id, source, captured_by FROM deal WHERE id = $1
		RETURNING id::text`, first)
	second, err := ids.Parse(secondID)
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE deal SET owner_id = $1, status = 'won', closed_at = now() WHERE id IN ($2, $3)`, e.Rep1, first, second)

	start, end := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	wins := func() int {
		var n int
		err := database.WithWorkspaceTx(e.repCtx, e.Pool, func(tx pgx.Tx) error {
			counts, err := countWeek(e.repCtx, tx, e.Rep1, start, end)
			n = counts.DealsWon
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := wins(); got != 2 {
		t.Fatalf("before the merge the week counts %d wins, want 2: the test premise is broken", got)
	}
	if _, err := e.Deals.MergeDeal(e.Admin(), ids.From[ids.DealKind](second), first, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := wins(); got != 1 {
		t.Errorf("after the merge the week counts %d wins, want 1", got)
	}
}
