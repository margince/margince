// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A mail the provider sent without a timestamp lands no later than the
// database's now(). A replay keeps the time it landed with.
func TestAnUndatedCaptureKeepsItsTimeOnReplay(t *testing.T) {
	t.Parallel()
	ctx, db := ownDomainWorkspace(t)
	ws, _ := principal.WorkspaceID(ctx)
	sink := capture.NewSink(db)
	record := mailRecord("undated-replay", "buyer@buyer.example", "buyer@buyer.example", "rep@seller.example")
	landed := func() (ids.UUID, time.Time, time.Time) {
		t.Helper()
		ref, err := sink.Upsert(mailSinkContext(context.Background(), ws), record)
		if err != nil {
			t.Fatalf("capturing the undated mail: %v", err)
		}
		var occurred, now time.Time
		if err := db.Tx(principal.WithWorkspaceID(context.Background(), ws), func(tx pgx.Tx) error {
			return tx.QueryRow(context.Background(), `SELECT occurred_at, now() FROM activity WHERE id = $1`, ref.ID).Scan(&occurred, &now)
		}); err != nil {
			t.Fatal(err)
		}
		return ref.ID, occurred, now
	}
	first, at, now := landed()
	if at.After(now) {
		t.Fatalf("the undated mail landed at %v, after the database's now() %v", at, now)
	}
	if again, replayedAt, _ := landed(); again != first || !replayedAt.Equal(at) {
		t.Fatalf("the replay is %v at %v, want %v at %v", again, replayedAt, first, at)
	}
}
