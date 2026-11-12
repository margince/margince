// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// A verdict's write holds every cited activity before it checks the evidence.
// A calendar move of one waits for the write's commit, so the check and the
// recorded moments read the same row.
func TestAVerdictHoldsItsCitedActivitiesFromTheCheckToTheCommit(t *testing.T) {
	e := setupReviewEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	meeting := seedTranscribedMeeting(t, e.configEnv, dealID, "")
	var moving error
	_, err := e.store.ApplyVerdict(e.productCtx(), VerdictInput{
		DealID: dealID, Owner: e.admin,
		Changes: []VerdictChange{{
			Correction: VerdictAmountCorrection, AmountMinor: 99_000,
			Evidence: map[string]any{CitedActivitiesEvidenceKey: []string{meeting.String()}},
		}},
		AsOwner: func(context.Context, pgx.Tx) (context.Context, error) { return e.productCtx(), nil },
		Allowed: func(context.Context, pgx.Tx, string) (bool, error) { return true, nil },
		// A second connection takes the lock a calendar move takes, without waiting.
		EvidenceHolds: func(context.Context, pgx.Tx) (bool, error) {
			_, moving = e.owner.Exec(t.Context(), `SELECT 1 FROM activity WHERE id = $1 FOR UPDATE NOWAIT`, meeting)
			return false, nil
		},
		Record: func(context.Context, pgx.Tx, []VerdictChange) error { return nil },
	})
	if err != nil {
		t.Fatalf("applying the verdict: %v", err)
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](moving); !ok || pgErr.Code != "55P03" {
		t.Errorf("a move of the cited meeting during the check answered %v, want lock_not_available", moving)
	}
}
