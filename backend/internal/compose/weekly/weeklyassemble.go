// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// Measuring one rep's week.
//
// Every count here is written ONCE, when the week closes, and read back
// unchanged forever after. A retrospective that recomputes on read answers
// differently depending on when you open it — the deal you closed on Friday
// gets reclassified in March because somebody edited its stage — and a record
// of a past week that does that is not a record.
//
// Each read runs under the acting rep's own principal and row scope. The job
// that drives this binds that principal per rep; nothing here widens it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealLineCap bounds how many deal lines one week records.
//
// A retrospective is read in a few minutes; a hundred lines is an export, not
// a review. The counts stay complete either way — the cap bounds what is
// LISTED, never what is counted, so a busy week reads "12 deals moved" beside
// the most recent of them rather than a truncated number.
const dealLineCap = 20

// WeekStartOf is the Monday of the local week containing now, in the
// installation's reporting zone.
//
// Monday because the review is about a working week and the product's own
// language says so. Derived through briefs.LocalDayAt rather than a second
// spelling of the zone lookup: two answers to "what day is it here" is how a
// Sunday-night job files its work under the wrong week.
func WeekStartOf(ctx context.Context, tx pgx.Tx, now time.Time) (time.Time, error) {
	day, _, err := briefs.LocalDayAt(ctx, tx, now)
	if err != nil {
		return time.Time{}, err
	}
	// Go's Weekday has Sunday at 0; the offset back to Monday is 6 for Sunday
	// and weekday-1 otherwise.
	back := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -back), nil
}

// AssembleFor measures the week that just closed for the acting rep and writes
// it, unless they already have one for that week.
//
// created=false means the week already had a review. That is the constraint
// doing its job rather than a failure: the dispatcher ticks more than once
// inside a week so that a worker which was down still backfills.
func (e *Engine) AssembleFor(ctx context.Context, now time.Time) (Review, bool, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return Review{}, false, err
	}
	userID, err := reviewUser(ctx)
	if err != nil {
		return Review{}, false, err
	}

	var review Review
	var created bool
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		// The week under review is the one that just CLOSED, not the one in
		// progress: a retrospective of a week still being lived would be
		// rewritten every day it ran.
		thisWeek, err := WeekStartOf(ctx, tx, now)
		if err != nil {
			return err
		}
		weekStart := thisWeek.AddDate(0, 0, -7)

		// The window is the LOCAL week, resolved to real instants.
		//
		// WeekStartOf returns a calendar date carried as midnight UTC — the
		// same shape brief_run.local_day uses, which is right for a date
		// column and wrong for a range. Comparing timestamptz against it
		// measures a week offset by the installation's UTC offset, and in a
		// DST zone a fixed 168 hours rather than the week people lived.
		start, end, err := localWeekWindow(ctx, tx, weekStart)
		if err != nil {
			return err
		}

		// LearningsState is set here, not left as Go's zero value: the column
		// defaults to not_run, and a returned review whose state was "" would
		// disagree with the same review read back a moment later.
		review = Review{
			UserID: userID, LocalWeekStart: weekStart, AsOf: now.UTC(),
			LearningsState: LearningsNotRun,
		}
		if err := e.measureWeek(ctx, tx, &review, now, start, end); err != nil {
			return err
		}

		id, wrote, err := insertReview(ctx, tx, review)
		if err != nil {
			return err
		}
		created = wrote
		if !wrote {
			// Somebody already wrote this week. Read theirs rather than
			// reporting a failure — the rep gets one review either way.
			return nil
		}
		review.ID = id
		if err := insertDealLines(ctx, tx, id, review.Deals); err != nil {
			return err
		}
		// How well the week went, frozen in the same transaction as what
		// happened in it. Both blocks may be absent — a rep with no leads and
		// no deals gets a scorecard that says so, which is what lets a reader
		// tell an empty week from an unmeasured one.
		card, err := scoreWeek(ctx, tx, userID, start, end)
		if err != nil {
			return err
		}
		if err := insertScorecard(ctx, tx, id, card); err != nil {
			return err
		}
		review.Scorecard = &card
		// Where the week was landing, frozen into the same transaction as the
		// counts. Split across two, a review could exist with no outlook and no
		// way to tell that from an installation that forecasts nothing.
		//
		// Only on the branch that WROTE the review: the loser of the insert
		// race returned above, and freezing an outlook onto somebody else's
		// review row would give it two.
		if e.forecast == nil {
			// No forecast composed. The review stands without one.
			return nil
		}
		outlooks, movements, drivers, err := e.forecast.CloseWeek(ctx, tx, start, end)
		if err != nil {
			return err
		}
		if err := writeOutlook(ctx, tx, id, outlooks, movements, drivers); err != nil {
			return err
		}
		// Carried on the returned review as well as written. The weekly mail is
		// built from THIS value rather than from a re-read, so a review that
		// froze its landing and did not carry it would send a rep an email with
		// no outlook in it.
		review.Outlook = outlooks
		return nil
	})
	if err != nil {
		return Review{}, false, err
	}
	if !created {
		existing, err := e.LatestReview(ctx, &review.LocalWeekStart)
		return existing, false, err
	}
	return review, true, nil
}

// measureWeek fills in everything the review REPORTS: the tallies, the money,
// the plan's outcome, the deals and the week it is compared against. Every one
// is a read — nothing here writes the review, which is why it can run before the
// insert decides whether this rep's week is already taken.
func (e *Engine) measureWeek(
	ctx context.Context, tx pgx.Tx, review *Review, now, start, end time.Time,
) error {
	userID := review.UserID
	var err error
	if review.Counts, err = countWeek(ctx, tx, userID, start, end); err != nil {
		return err
	}
	// Leads and meetings read separately from the tallies above: different
	// tables, different scope clauses, and each dated by a rule that takes
	// a paragraph to justify. They fold onto the same Counts because a
	// reader of a week wants one set of figures.
	c := &review.Counts
	c.LeadsRouted, c.LeadsAnsweredInTarget, c.LeadsBreached, err = countWeekLeads(ctx, tx, userID, start, end)
	if err != nil {
		return err
	}
	c.MeetingsHeld, c.MeetingsWithNextStep, err = countWeekMeetings(ctx, tx, userID, start, end)
	if err != nil {
		return err
	}
	if review.Money, err = countWeekMoney(ctx, tx, userID, start, end); err != nil {
		return err
	}
	// The plan's outcome, settled once and then frozen alongside the rest.
	//
	// Settled BEFORE the counts are written, so the review records what the
	// week actually came to rather than what was still open when the job
	// happened to run. CloseWeek is idempotent, so the dispatcher's extra
	// ticks inside a week do not re-settle a commitment the rep completed
	// after the first pass.
	if e.plan != nil {
		if c.CommitmentsDue, c.CommitmentsKept, err = e.plan.CloseWeek(ctx, now); err != nil {
			return err
		}
	}
	if review.Deals, err = readWeekDeals(ctx, tx, userID, start, end); err != nil {
		return err
	}
	// The week this one is measured against: the rep's most recent EARLIER
	// review, whenever it was.
	//
	// Their previous review rather than "last week" by arithmetic. A rep
	// with a gap — a leave, a worker outage — has a prior week that is not
	// seven days back, and looking for one would find nothing and report
	// every count as new.
	review.PriorReviewID, err = priorReview(ctx, tx, userID, review.LocalWeekStart)
	return err
}

// localWeekWindow turns a local week's calendar start into the two instants
// that bound it, in the installation's own zone.
//
// It asks Postgres rather than doing the arithmetic in Go so the zone lookup
// and the conversion are one answer: a DST week is 167 or 169 hours, and
// adding 7*24h to a start would measure an hour of the wrong week twice a year.
func localWeekWindow(ctx context.Context, tx pgx.Tx, weekStart time.Time) (time.Time, time.Time, error) {
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	var start, end time.Time
	if err := tx.QueryRow(ctx, `
		SELECT ($1::date)::timestamp AT TIME ZONE $2,
		       ($1::date + 7)::timestamp AT TIME ZONE $2`, weekStart, zone).
		Scan(&start, &end); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("weekly: bounding the local week: %w", err)
	}
	return start, end, nil
}
