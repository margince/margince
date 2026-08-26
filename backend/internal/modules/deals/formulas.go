// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The stalled-deal rule (formulas-and-rules §8): a deterministic,
// fixed-clock-stable boolean over last_activity_at with the "customer
// asked us to wait" suppression. Two spellings exist by necessity —
// the Go predicate stamps the wire flag, the SQL clause filters lists
// server-side — and TestDealsByStageStalledFilterAgreesWithIsStalled
// (compose package, integration lane) keeps them from drifting.

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
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
func IsStalled(status string, createdAt time.Time, lastActivityAt, waitUntil *time.Time, now time.Time) bool {
	return IsQuietFor(StalledThresholdDays, status, createdAt, lastActivityAt, waitUntil, now)
}

// IsQuietFor is IsStalled with the idle window named by the caller. Every rule
// bar the number is identical — a closed deal is never quiet, an explicit
// deferral suppresses it, and idle is measured the same way.
//
// Held by: TestIsQuietForIsTheSameRuleAtADifferentWindow
// (backend/internal/modules/deals/formulas_test.go), which asserts the two
// answer identically across the idle range rather than trusting the delegation.
func IsQuietFor(days int, status string, createdAt time.Time, lastActivityAt, waitUntil *time.Time, now time.Time) bool {
	if DealStatus(status) != DealOpen {
		return false // closed deals never stall
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
		AND (%[1]swait_until IS NULL OR %[1]swait_until <= now()))`,
		columnPrefix(alias), days, idlebase.SQL(alias))
}

// PartnerSourcedSQL is the list-filter spelling of "this deal is
// partner-sourced" (true branch); callers negate it for
// partner_sourced=false. Attribution presence, not a value match — same
// alias-qualification reason as StalledSQL: a caller that joins another
// table sharing a column name this expression touches must qualify it or
// the reference is ambiguous SQL.
func PartnerSourcedSQL(alias string) string {
	return columnPrefix(alias) + "partner_org_id IS NOT NULL"
}

// columnPrefix renders a query's table alias as a column prefix — "" for
// deal_read.go's own unaliased query, "t." for a caller that names it.
func columnPrefix(alias string) string {
	if alias == "" {
		return ""
	}
	return alias + "."
}
