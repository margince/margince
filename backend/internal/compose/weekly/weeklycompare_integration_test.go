// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// The comparison half of the retrospective, over real migrated Postgres.
//
// What these defend is not arithmetic — a unit test could check a subtraction.
// It is that the figures are FROZEN and that an absent one stays absent: a week
// whose numbers move when you reopen it is not a retrospective, and a money
// figure that reads zero when it means "we could not work it out" is a lie a
// reader has no way to detect.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// capturedBy is the principal string an activity or lead records its writer as.
//
// A meeting has no owner column — activity_task_fields reserves assignee_id for
// tasks — so this IS how a meeting is attributed to the rep who held it, and
// the counts read it the same way.
func capturedBy(user ids.UUID) string { return "human:" + user.String() }

// lastWeek is an instant inside the week weekClock's review covers.
func lastWeek(t *testing.T, at time.Time) time.Time {
	t.Helper()
	owner := integration.OwnerConn(t)
	var thisWeek time.Time
	if err := owner.QueryRow(context.Background(),
		`SELECT date_trunc('week', $1::timestamptz)::date`, at).Scan(&thisWeek); err != nil {
		t.Fatal(err)
	}
	// Wednesday of the closed week: inside it whatever the installation zone.
	return thisWeek.AddDate(0, 0, -5)
}

// A lead nobody routed still has a first-response target — it runs from when
// the lead arrived (leadsla.go COALESCEs the two). A count keyed on routed_at
// alone would drop exactly the leads most likely to have been missed.
func TestAnUnroutedLeadStillCountsInTheWeekItArrived(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	inWeek := lastWeek(t, weekClock)

	// Routed and unrouted, both this rep's, both inside the closed week.
	for _, routed := range []bool{true, false} {
		id := ids.NewV7()
		var routedAt any
		if routed {
			routedAt = inWeek
		}
		if _, err := owner.Exec(ctx, `
			INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, created_at, routed_at)
			VALUES ($1, 'A Lead', 'new', 'manual', $2, $3, $4, $5)`,
			id, capturedBy(e.Rep1), e.Rep1, inWeek, routedAt); err != nil {
			t.Fatal(err)
		}
	}

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}

	if review.Counts.LeadsRouted != 2 {
		t.Errorf("the week counted %d leads, want 2 — an unrouted lead arrived in it too",
			review.Counts.LeadsRouted)
	}
}

// A meeting is judged by whether it HAPPENED.
//
// A booking that was cancelled or no-showed is not a conversation the week can
// be credited with, and counting it would tell a rep they met four customers
// when they met one.
func TestOnlyMeetingsThatWereHeldCountTowardTheWeek(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	inWeek := lastWeek(t, weekClock)

	for _, status := range []string{"held", "booked", "no_show", "canceled"} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
			VALUES ($1, 'meeting', $2, $3, $4, 'manual', $5)`,
			ids.NewV7(), "A meeting ("+status+")", inWeek, status, capturedBy(e.Rep1)); err != nil {
			t.Fatal(err)
		}
	}

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}

	if review.Counts.MeetingsHeld != 1 {
		t.Errorf("the week counted %d meetings held, want 1 — only one of the four happened",
			review.Counts.MeetingsHeld)
	}
}

// The week a review is measured against is the rep's PREVIOUS review, whenever
// it was — not "seven days ago".
//
// A rep with a gap in their history has a prior week that is not last week, and
// date arithmetic would find nothing and report every count as new.
func TestThePriorWeekIsTheLastOneWritten(t *testing.T) {
	e := setupWeekly(t)

	// Three weeks back, then the closed one: a gap in between.
	older, _, err := e.engine.AssembleFor(e.repCtx, weekClock.AddDate(0, 0, -21))
	if err != nil {
		t.Fatal(err)
	}
	recent, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}

	if recent.PriorReviewID == nil {
		t.Fatal("the later week names no prior review, though an earlier one exists")
	}
	if *recent.PriorReviewID != older.ID {
		t.Errorf("prior = %v, want the older review %v — it is the last one WRITTEN, not last week",
			*recent.PriorReviewID, older.ID)
	}

	// And a reader gets that week's own frozen figures beside this one's.
	read, err := e.engine.LatestReview(e.repCtx, &recent.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if read.Prior == nil {
		t.Fatal("the review was served with no prior week to compare against")
	}
	if !read.Prior.LocalWeekStart.Equal(older.LocalWeekStart) {
		t.Errorf("the comparison names the week of %s, want %s",
			read.Prior.LocalWeekStart.Format(time.DateOnly),
			older.LocalWeekStart.Format(time.DateOnly))
	}
}

// A rep's first week has nothing to be compared against, and says so.
func TestAFirstWeekNamesNoPriorReview(t *testing.T) {
	e := setupWeekly(t)

	review, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}

	if review.PriorReviewID != nil {
		t.Errorf("a first review named a prior week (%v) — there is none",
			*review.PriorReviewID)
	}
}

// The frozen row does not move when the records it counted do.
//
// The guarantee the whole table exists for, asserted over the NEW columns: a
// deal deleted, a lead answered late, a meeting cancelled after the fact must
// all leave last week exactly as it was written.
func TestTheNewCountsAreFrozenAgainstLaterEdits(t *testing.T) {
	e := setupWeekly(t)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	inWeek := lastWeek(t, weekClock)

	meeting := ids.NewV7()
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, source, captured_by)
		VALUES ($1, 'meeting', 'The meeting', $2, 'held', 'manual', $3)`,
		meeting, inWeek, capturedBy(e.Rep1)); err != nil {
		t.Fatal(err)
	}
	lead := ids.NewV7()
	if _, err := owner.Exec(ctx, `
		INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, created_at, routed_at)
		VALUES ($1, 'A Lead', 'new', 'manual', $2, $3, $4, $4)`,
		lead, capturedBy(e.Rep1), e.Rep1, inWeek); err != nil {
		t.Fatal(err)
	}

	written, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatal(err)
	}
	if written.Counts.MeetingsHeld != 1 || written.Counts.LeadsRouted != 1 {
		t.Fatalf("the week was written as %d meetings and %d leads, want 1 and 1 — "+
			"without both this test proves nothing",
			written.Counts.MeetingsHeld, written.Counts.LeadsRouted)
	}

	// Now undo the week: cancel the meeting, delete the lead.
	if _, err := owner.Exec(ctx,
		`UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`, meeting); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `DELETE FROM lead WHERE id = $1`, lead); err != nil {
		t.Fatal(err)
	}

	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatal(err)
	}
	if read.Counts.MeetingsHeld != 1 {
		t.Errorf("meetings_held read back as %d after the meeting was cancelled, want the frozen 1",
			read.Counts.MeetingsHeld)
	}
	if read.Counts.LeadsRouted != 1 {
		t.Errorf("leads_routed read back as %d after the lead was deleted, want the frozen 1",
			read.Counts.LeadsRouted)
	}
}

// The delta is a subtraction between two frozen rows, and nothing stores it.
//
// The claim on Review.PriorReviewID, held here: a stored delta is a third copy
// of a fact the two rows already carry, and the three drift the first time one
// row is rewritten. This asserts the absence — no column anywhere on
// weekly_review holds a difference — so a later author who adds one has to
// come here and argue with this test rather than quietly introducing the drift.
func TestNoDeltaIsStoredBesideTheTwoFrozenRows(t *testing.T) {
	owner := integration.OwnerConn(t)
	ctx := context.Background()

	rows, err := owner.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		 WHERE table_name = 'weekly_review'
		   AND (column_name LIKE '%delta%' OR column_name LIKE '%change%'
		        OR column_name LIKE '%_vs_%' OR column_name LIKE '%diff%')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var stored []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(stored) > 0 {
		t.Errorf("weekly_review stores %v — a delta is a third copy of what the two "+
			"frozen rows already say, and the three drift when one row is rewritten. "+
			"Compute it from prior_review_id instead", stored)
	}
}
