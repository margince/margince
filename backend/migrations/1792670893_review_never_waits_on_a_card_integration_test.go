// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// A review still waiting on a card goes back to the rep, with one audit entry,
// and the column naming the card goes. The down migration then restores it.
func TestAReviewWaitingOnACardGoesBackToTheRep(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1792670894"), 2); err != nil {
		t.Fatalf("rolling the review migrations back: %v", err)
	}

	var waiting, untouched string
	if err := conn.QueryRow(ctx, `
		INSERT INTO communication_review (state, kind, reason_code, refusals, approval_id)
		VALUES ('awaiting_decision', 'single', 'no_grant', '[]', gen_random_uuid())
		RETURNING id`).Scan(&waiting); err != nil {
		t.Fatalf("seeding a review waiting on a card: %v", err)
	}
	if err := conn.QueryRow(ctx, `
		INSERT INTO communication_review (state, kind, reason_code, refusals)
		VALUES ('needs_repair', 'single', 'bad_address', '[]')
		RETURNING id`).Scan(&untouched); err != nil {
		t.Fatalf("seeding a review nobody routed: %v", err)
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("migrating back up: %v", err)
	}
	for id, want := range map[string]string{waiting: "needs_context", untouched: "needs_repair"} {
		var state string
		if err := conn.QueryRow(ctx, `SELECT state FROM communication_review WHERE id = $1`, id).Scan(&state); err != nil {
			t.Fatalf("reading review %s: %v", id, err)
		}
		if state != want {
			t.Errorf("review %s reads %s, want %s", id, state, want)
		}
	}
	var audits int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'communication_review' AND entity_id = ANY($1)`,
		[]string{waiting, untouched}).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Errorf("%d audit rows for the one review moved, want 1", audits)
	}
	if hasApprovalColumn(ctx, t, conn) {
		t.Error("communication_review.approval_id survived the migration")
	}

	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1792670894"), 2); err != nil {
		t.Fatalf("rolling back a second time: %v", err)
	}
	if !hasApprovalColumn(ctx, t, conn) {
		t.Error("the down migration did not restore communication_review.approval_id")
	}
}

func hasApprovalColumn(ctx context.Context, t *testing.T, conn *pgx.Conn) bool {
	t.Helper()
	var present bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = 'communication_review'
		   AND column_name = 'approval_id')`).Scan(&present); err != nil {
		t.Fatalf("reading the review columns: %v", err)
	}
	return present
}
