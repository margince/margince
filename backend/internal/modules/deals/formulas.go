// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The stalled-deal rule: a deterministic boolean over last_activity_at, stable
// under a fixed clock. "The customer asked us to wait" and "a meeting is booked
// ahead" suppress it. The Go predicate stamps the wire flag and the SQL clause
// filters lists. TestDealsByStageStalledFilterAgreesWithIsStalled (compose
// package, integration lane) keeps the two from drifting.

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// StalledThresholdDays is the §8 tunable: open deals idle longer than
// this are stalled unless a wait suppresses it. It is the answer to "has this
// deal stalled" — a status the whole product agrees on.
const StalledThresholdDays = 60

// QuietThresholdDays is a shorter idle window for surfaces that want to notice
// a deal going quiet BEFORE it meets the stalled bar — a morning queue nudging
// a rep at three weeks rather than reporting a two-month-old fact.
//
// It is a second THRESHOLD, never a second predicate: IsQuietFor and QuietSQL
// below take the window as an argument, so the rule itself does not fork. A
// surface asking at this window is asking the same question with a different
// patience, which is why the copy beside it must name the window it used —
// "quiet 19 days" and "stalled" are different claims about the same deal.
const QuietThresholdDays = 19

// IsStalled evaluates §8.1 at one instant. Idle is an absolute-duration
// comparison on UTC instants, never a calendar-day count — stable under
// a fixed test clock, identical across zones.
// meetingAhead is MeetingAheadSQL's answer for the deal, read in the same
// statement as lastActivityAt.
func IsStalled(status string, createdAt time.Time, lastActivityAt, waitUntil *time.Time, meetingAhead bool, now time.Time) bool {
	return IsQuietFor(StalledThresholdDays, status, createdAt, lastActivityAt, waitUntil, meetingAhead, now)
}

// IsQuietFor is IsStalled with the idle window named by the caller. Every rule
// bar the number is identical — a closed deal is never quiet, an explicit
// deferral suppresses it, and idle is measured the same way.
//
// Held by: TestIsQuietForIsTheSameRuleAtADifferentWindow
// (backend/internal/modules/deals/formulas_test.go), which asserts the two
// answer identically across the idle range rather than trusting the delegation.
func IsQuietFor(days int, status string, createdAt time.Time, lastActivityAt, waitUntil *time.Time, meetingAhead bool, now time.Time) bool {
	if DealStatus(status) != DealOpen {
		return false // closed deals never stall
	}
	if meetingAhead {
		return false // the next contact is already booked
	}
	if now.Sub(idlebase.Since(createdAt, lastActivityAt)) <= time.Duration(days)*24*time.Hour {
		return false
	}
	if waitUntil != nil && now.Before(*waitUntil) {
		return false // respecting an explicit deferral
	}
	return true
}

// StalledSQL is the list-filter spelling of IsStalled (true branch);
// callers negate it for stalled=false. Takes the query's table alias —
// deal_read.go's own list query reads the unaliased `deal` table (alias
// ""), but a caller that JOINs a table sharing a column name this
// expression touches (compose/report.go's deals-by-stage joins `stage`,
// which also has created_at) MUST qualify every column or the reference
// is ambiguous SQL, not merely wrong. One spelling, parameterized, rather
// than a second copy that only agrees by accident.
func StalledSQL(alias string) string {
	return QuietSQL(alias, StalledThresholdDays)
}

// QuietSQL is StalledSQL with the idle window named by the caller — the SQL
// half of IsQuietFor, and the same one-spelling rule. The window is an int
// formatted into an interval literal rather than a bound parameter because
// this expression is composed into larger statements whose placeholders are
// numbered by their own callers; an int is not attacker-reachable.
func QuietSQL(alias string, days int) string {
	return fmt.Sprintf(`(%[1]sstatus = 'open'
		AND %[3]s < now() - interval '%[2]d days'
		AND (%[1]swait_until IS NULL OR %[1]swait_until <= now())
		AND NOT %[4]s)`,
		columnPrefix(alias), days, idlebase.SQL(alias), MeetingAheadSQL(alias))
}

// MeetingAheadSQL is true when the deal has a booked meeting its stored clock
// has not reached. That is a meeting still to come, or one that started before
// the refold job moved the clock to it. It reads the clock's own filters. The
// audience is spelled as in last_activity_of_deal, so a meeting only some
// colleagues may see changes no number everyone reads.
func MeetingAheadSQL(alias string) string {
	deal := "deal"
	if alias != "" {
		deal = alias
	}
	return `EXISTS (SELECT 1 FROM activity_link ahead_link
		JOIN activity ahead ON ahead.id = ahead_link.activity_id
		WHERE ahead_link.deal_id = ` + deal + `.id AND ahead.archived_at IS NULL
		  AND ahead.kind = 'meeting' AND ` + relstrength.NotCalledOffSQL("ahead") +
		auth.OriginIsEngagement("ahead") + ` AND ahead.audience = 'workspace'
		  AND ahead.occurred_at > coalesce(` + deal + `.last_activity_at, '-infinity'))`
}

// LastActivityDaySQL says the activity the deal's stored clock names is a day
// entry, so days since it count from its own date. The clock holds that row's
// occurred_at, which is how the row is found.
func LastActivityDaySQL(deal string) string {
	return `EXISTS (SELECT 1 FROM activity_link day_link
		JOIN activity day_row ON day_row.id = day_link.activity_id
		WHERE day_link.deal_id = ` + deal + `.id AND day_row.archived_at IS NULL
		  AND day_row.occurred_at = ` + deal + `.last_activity_at AND day_row.occurred_precision = 'day')`
}

// PartnerSourcedSQL is the list-filter spelling of "this deal is
// partner-sourced" (true branch); callers negate it for
// partner_sourced=false. Attribution presence, not a value match — same
// alias-qualification reason as StalledSQL: a caller that joins another
// table sharing a column name this expression touches must qualify it or
// the reference is ambiguous SQL.
func PartnerSourcedSQL(alias string) string {
	return columnPrefix(alias) + "partner_company_id IS NOT NULL"
}

// columnPrefix renders a query's table alias as a column prefix — "" for
// deal_read.go's own unaliased query, "t." for a caller that names it.
func columnPrefix(alias string) string {
	if alias == "" {
		return ""
	}
	return alias + "."
}
