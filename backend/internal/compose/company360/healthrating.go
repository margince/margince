// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// rateHealthDimensions turns the parts readHealth gathered into the named
// dimensions the card draws (PO-AC-N-10..12).
//
// TWO of the three, deliberately. Relationship and commercial are readable from
// this assembly; PAYMENT is not — the finance mirror is another module's, and a
// module never imports a sibling. The surface composes it from the finance read
// it already makes, which is also why `overall` is computed there rather than
// here: a verdict that ignored payment would be the exact "strong relationship
// hides a payment problem" failure the worst-of rule exists to prevent.
//
// A dimension that cannot be read is ABSENT rather than rated. Absence is a
// fact about the reading; a rating is a claim about the account.
func rateHealthDimensions(
	health *crmcontracts.Company360Health,
	strip *crmcontracts.Company360StateStrip,
	touch relstrength.InTouch,
) {
	// An account nobody has ever reached is not "at risk" — it is unstarted,
	// and rating it would put a verdict on a relationship that has not begun.
	if health.ActiveContacts != nil && *health.ActiveContacts > 0 {
		single := health.SingleThreaded != nil && *health.SingleThreaded
		health.Relationship = rateRelationship(touch, *health.ActiveContacts, single)
	}
	// Null commercial means the caller has no deal grant, which is a fact about
	// the reader rather than about the account.
	if strip != nil && strip.Commercial != nil {
		health.Commercial = rateCommercial(strip.Commercial.OpenCount, strip.Commercial.StalledCount)
	}
}

// rateRelationship: are we in touch, by the one rule relstrength holds for
// every reading. A meeting counts as much as a message from them: an account
// we sat with last week is not one that has stopped talking to us.
func rateRelationship(touch relstrength.InTouch, active int, single bool) *crmcontracts.HealthDimension {
	switch {
	case touch.Basis == relstrength.InTouchNever:
		return dimension(crmcontracts.HealthDimensionRatingAtRisk, crmcontracts.HealthDimensionReasonCodeNeverWritten,
			crmcontracts.HealthDimensionReasonParams{}, "They have never written to you, and you have never met them.")
	case touch.Basis == relstrength.InTouchQuiet:
		return dimension(crmcontracts.HealthDimensionRatingAtRisk, crmcontracts.HealthDimensionReasonCodeQuiet,
			crmcontracts.HealthDimensionReasonParams{Days: &touch.Days},
			fmt.Sprintf("No reply and no meeting for %d days.", touch.Days))
	case touch.Basis == relstrength.InTouchBooked:
		// The instant, not a day: which day it falls on depends on the record's
		// zone, which the client holds and this read does not.
		return dimension(crmcontracts.HealthDimensionRatingGood, crmcontracts.HealthDimensionReasonCodeMeetingBooked,
			crmcontracts.HealthDimensionReasonParams{At: touch.BookedAt}, "A meeting with them is booked.")
	case touch.Basis == relstrength.InTouchMet:
		// The meeting is named even on a single-threaded account: it is what
		// keeps the rating off at risk, and SingleThreaded still says the rest.
		rating := crmcontracts.HealthDimensionRatingStrong
		if single {
			rating = crmcontracts.HealthDimensionRatingGood
		}
		return dimension(rating, crmcontracts.HealthDimensionReasonCodeLastMet,
			crmcontracts.HealthDimensionReasonParams{Days: &touch.Days},
			fmt.Sprintf("Last met them %d days ago.", touch.Days))
	case single:
		return dimension(crmcontracts.HealthDimensionRatingGood, crmcontracts.HealthDimensionReasonCodeSingleThreaded,
			crmcontracts.HealthDimensionReasonParams{}, "In contact, but one contact carries the whole account.")
	default:
		return dimension(crmcontracts.HealthDimensionRatingStrong, crmcontracts.HealthDimensionReasonCodeSeveralContacts,
			crmcontracts.HealthDimensionReasonParams{Count: &active},
			fmt.Sprintf("%d contacts here are in touch with you.", active))
	}
}

// rateCommercial: is work moving?
//
// An account with NOTHING open is not rated at all. "No open deal" is not a
// risk — a customer under contract who is not being sold to right now is in
// the ordinary state of a customer — and rating it at risk put every such
// account under a red verdict it had done nothing to earn, then dragged the
// overall standing down with it through the worst-of rule.
func rateCommercial(open, stalled int) *crmcontracts.HealthDimension {
	switch {
	case open == 0:
		return nil
	case stalled >= open:
		return dimension(crmcontracts.HealthDimensionRatingAtRisk, crmcontracts.HealthDimensionReasonCodeDealsAllStalled,
			crmcontracts.HealthDimensionReasonParams{Count: &open},
			fmt.Sprintf("All %d open deals have stalled.", open))
	case stalled > 0:
		return dimension(crmcontracts.HealthDimensionRatingGood, crmcontracts.HealthDimensionReasonCodeDealsSomeStalled,
			crmcontracts.HealthDimensionReasonParams{Count: &stalled, Total: &open},
			fmt.Sprintf("%d of %d open deals have stalled.", stalled, open))
	default:
		return dimension(crmcontracts.HealthDimensionRatingStrong, crmcontracts.HealthDimensionReasonCodeDealsNoneStalled,
			crmcontracts.HealthDimensionReasonParams{Count: &open},
			fmt.Sprintf("%d open deals, none stalled.", open))
	}
}

// dimension carries the English sentence for clients that do not know the
// code, and the code with its values for those that say it in the reader's
// language.
func dimension(
	rating crmcontracts.HealthDimensionRating, code crmcontracts.HealthDimensionReasonCode,
	params crmcontracts.HealthDimensionReasonParams, reason string,
) *crmcontracts.HealthDimension {
	return &crmcontracts.HealthDimension{Rating: rating, Reason: reason, ReasonCode: &code, ReasonParams: &params}
}

// meetingsAround reads the account's last meeting already held and its next
// one booked ahead, in one round trip of two LIMIT-1 arms.
//
// The held arm reads relstrength.MeetingHeldSQL and the booked arm
// relstrength.MeetingBookedSQL, so a canceled meeting, a no-show and a booking
// whose start passed unconfirmed count in neither. Nil is "no meeting on
// record", a fact about the reading rather than a claim that none happened:
// the caller may hold no scope over the activity that would prove otherwise.
func (a *assembly) meetingsAround() (last, next *time.Time, err error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos := arg(a.companyID.UUID)
	nowPos := arg(a.now)
	activityScope, err := auth.ActivityDiscoverClause(a.ctx, "a", arg)
	if err != nil {
		return nil, nil, err
	}
	if activityScope == "" {
		activityScope = scopeAll
	}
	// The body of work, on the same terms as every other activity read on this
	// page: a page narrowed to one project must not rate its health from
	// another project's meeting.
	where := fmt.Sprintf(`a.kind = 'meeting' AND a.archived_at IS NULL AND %s AND %s`,
		activityScope, activities.CompanyLinkedActivityExists(companyPos)) + a.opts.projectScope(arg)
	now := fmt.Sprintf("$%d", nowPos)
	rows, err := a.tx.Query(a.ctx, fmt.Sprintf(`
		(SELECT 'last' AS side, a.occurred_at FROM activity a
		  WHERE %[1]s AND %[2]s AND a.occurred_at <= %[4]s
		  ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1)
		UNION ALL
		(SELECT 'next', a.occurred_at FROM activity a
		  WHERE %[1]s AND %[3]s
		  ORDER BY a.occurred_at, a.id LIMIT 1)`,
		where, relstrength.MeetingHeldSQL("a", now), relstrength.MeetingBookedSQL("a", now), now), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("read the account's meetings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var side string
		var at time.Time
		if err := rows.Scan(&side, &at); err != nil {
			return nil, nil, err
		}
		if side == "last" {
			last = &at
			continue
		}
		next = &at
	}
	return last, next, rows.Err()
}
