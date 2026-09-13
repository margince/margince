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

	for _, tc := range []struct {
		patch string
		due   bool
	}{
		{`narrative = 'Recorded work.', narrated_at = now()`, true},
		{`learnings_state = 'insufficient_evidence', learned_at = now()`, true},
		{`mail_attempted_at = now()`, false},
	} {
		if _, err := owner.Exec(ctx, `UPDATE weekly_review SET `+tc.patch+` WHERE id=$1`, reviewID); err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(repsDue(t, e, week), e.Rep1); got != tc.due {
			t.Fatalf("after %s due=%v want %v", tc.patch, got, tc.due)
		}
	}

}

// repsDue runs the production candidate query.
func repsDue(t *testing.T, e *integration.Env, week time.Time) []ids.UUID {
	t.Helper()
	var due []ids.UUID
	err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool,
		func(tx pgx.Tx) error {
			var err error
			w := &weeklyGenerateWorker{narrator: &weeklyMeasurementProbe{}, learner: &weeklyMeasurementProbe{}, mail: WeeklyMailConfig{Mailer: &countingMailer{}}}
			due, err = w.repsWithoutAReviewFor(context.Background(), tx, week)
			return err
		})
	if err != nil {
		t.Fatalf("reading who is due: %v", err)
	}
	return due
}

func TestAnInstallationWithoutOptionalLanesOnlyMeasuresOnce(t *testing.T) {
	e := integration.Setup(t)
	w := teamSnapshotWorker(e)
	seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
	if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
		t.Fatal(err)
	}
	due, err := w.repsDueTheirReview(e.Admin(), e.WS, teamJobClock)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		if slices.Contains(due, user) {
			t.Fatal("unconfigured services keep a measured member due")
		}
	}
	w.learner = &weeklyMeasurementProbe{}
	due, err = w.repsDueTheirReview(e.Admin(), e.WS, teamJobClock)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		if !slices.Contains(due, user) {
			t.Fatal("newly configured observations did not become reachable")
		}
	}
}
