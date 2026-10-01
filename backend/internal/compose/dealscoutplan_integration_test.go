// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
)

// The scout's signal branch checks, for every citation of every open signal,
// that the cited activity is still live. That lookup must reach activity by its
// primary key: comparing text scans the whole table once per citation, so its
// cost grows with the table instead of with the citations.
//
// Sequential scans are switched off so the plan cannot pick one for being cheap
// on a test-sized table: with them off, a lookup that CAN use the key does, and
// one that cannot is left with the scan this test then refuses.
func TestTheScoutReachesACitedActivityByItsPrimaryKey(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC()
	// The planner prices a lookup from the table's statistics, and an empty
	// table has none: it would pick whatever is cheapest on zero rows.
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO activity (kind, subject, source, captured_by, occurred_at)
		SELECT 'email', 'plan seed ' || g, 'manual', 'human:test', now() - (g || ' minutes')::interval
		  FROM generate_series(1, 5000) g`); err != nil {
		t.Fatalf("seeding activities: %v", err)
	}
	if _, err := owner.Exec(context.Background(), `ANALYZE activity`); err != nil {
		t.Fatalf("analyzing activities: %v", err)
	}
	query, args := dealScoutSQL(now.AddDate(0, 0, -90), now, 20)

	var plan []string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `EXPLAIN `+query, args...)
		if err != nil {
			return err
		}
		plan, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatalf("planning the scout's read: %v", err)
	}

	text := strings.Join(plan, "\n")
	if !strings.Contains(text, "Index Scan using activity_pkey on activity ca") {
		t.Errorf("the citation check does not look the cited activity up by primary key:\n%s", text)
	}
	if strings.Contains(text, "Seq Scan on activity ca") {
		t.Errorf("the citation check scans the activity table once per citation:\n%s", text)
	}
}
