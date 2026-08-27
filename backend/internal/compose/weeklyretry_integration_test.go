// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Whether a later tick looks at a rep again.
//
// It drives repsWithoutAReviewFor itself rather than a copy of its predicate.
// A test that restated the WHERE clause would pass against a production query
// that had stopped matching it — which is exactly what happened when this was
// first written the other way.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A review that landed un-narrated must stay reachable.
//
// The dispatcher ticks more than once inside a week, and a review can land
// without a sentence for reasons that pass: the role had no lane, the budget
// was spent, the provider was down. Selecting only reps with NO review would
// make every one of those permanent — the next tick finds the row and looks
// away, and nothing ever writes that week a sentence.
func TestAnUnnarratedWeekIsStillDueForNarration(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	week := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)

	// A review with no sentence: what a failed or absent narration leaves.
	reviewID := integration.SeedIDRow(t, owner, `
		INSERT INTO weekly_review (id, user_id, local_week_start, as_of)
		VALUES ($1, $2, $3, now())`, e.Rep1, week)

	due := repsDue(t, e, week)
	if !slices.Contains(due, e.Rep1) {
		t.Fatal("a rep whose review has no sentence is not selected again — a lane " +
			"that was down when the week closed could never write one")
	}

	// Once narrated they drop out: re-narrating would rewrite a sentence the
	// rep has already read.
	if _, err := owner.Exec(ctx,
		`UPDATE weekly_review SET narrative = 'Weber signed.', narrated_at = now() WHERE id = $1`,
		reviewID); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(repsDue(t, e, week), e.Rep1) {
		t.Error("a narrated review is still selected, so a later tick would rewrite the sentence")
	}
}

// repsDue runs the production candidate query.
func repsDue(t *testing.T, e *integration.Env, week time.Time) []ids.UUID {
	t.Helper()
	var due []ids.UUID
	err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool,
		func(tx pgx.Tx) error {
			var err error
			due, err = repsWithoutAReviewFor(context.Background(), tx, week)
			return err
		})
	if err != nil {
		t.Fatalf("reading who is due: %v", err)
	}
	return due
}
