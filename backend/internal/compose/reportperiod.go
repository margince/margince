// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The period bucket vocabulary (reporting REPORT-PARAM-4..7) and win-loss
// (REPORT-KEY-8 / REPORT-VOCAB-1), its first consumer. They live together
// because they ship and change together: the bucket has exactly one caller
// today, and a shared vocabulary with one caller is a file two readers have to
// open to answer one question. A second key adopting periods is what splits
// them.
//
// Separate from report.go because that file sits at the package's file-length
// cap, which reportcatalog.go's header already records as deliberate.

import (
	"maps"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	// colClosedAt is win-loss's period anchor: the instant a deal was actually
	// won or lost. The anchor is the KEY's to declare, never the caller's to
	// choose (REPORT-PARAM-7) — a caller who could pick it could produce two
	// reports that disagree and both look right. It is NOT NULL for every row
	// this report reads, guaranteed by the deal_closed_at CHECK.
	colClosedAt = "t.closed_at"
	colSource   = "t.source"

	// fieldDaysToClose is named because it is spelled in the measure map and
	// in the default aggregates, and a name written twice can come to be
	// written two ways — which makes an aggregate reference a measure that
	// does not exist.
	fieldDaysToClose = "days_to_close"

	fieldSource        = "source"
	fieldPeriodYear    = "period_year"
	fieldPeriodQuarter = "period_quarter"
	fieldPeriodMonth   = "period_month"

	// The three grains' to_char patterns (REPORT-PARAM-5). Q is Postgres's
	// quarter-of-year; the "-Q" is a quoted literal inside the pattern, not a
	// second field. Each renders a value that is BOTH what a reader sees and
	// what sorts: '2025-Q4' < '2026-Q1' lexically as well as chronologically,
	// so a period report needs no separate sort key.
	patternYear    = "YYYY"
	patternQuarter = `YYYY"-Q"Q`
	patternMonth   = "YYYY-MM"

	// A fiscal year renders through patternYear above — the SHIFT is what makes
	// it fiscal, not the pattern — and a month keeps its calendar spelling under
	// every fiscal start, since March is March. Only the quarter needs a
	// spelling of its own: bare Q, because the year half is built separately
	// and the two are concatenated.
	patternFiscalQuarter = "Q"
)

// daysToCloseExpr is how long a deal took: whole days from the day it was
// created to the day it closed.
//
// Both columns sit on the deal, so this needs no join and no history.
// closed_at is NOT NULL for every row this report reads, and created_at is
// NOT NULL on every deal there is, so the measure is never absent.
//
// DATES subtracted, not instants. A deal created at 23:50 and closed at 00:10
// the next night took two calendar days by any reader's reckoning and 0.01 by
// the clock's; every other duration on this surface — stage age, the slipped
// rule — already counts the days a person would count.
//
// Both days are read on the INSTALLATION's reporting clock, which is what the
// period buckets beside them use. Cast in the session's zone instead, the same
// pair of instants is two days in UTC and one in Asia/Ho_Chi_Minh — so the
// duration would move with the deployment region while the period bucket on
// the very same row did not, and a row could disagree with itself.
//
// Zero is a real answer: a deal created and closed the same day closed
// same-day, which is a fact about the sale rather than a missing figure.
//
// A NEGATIVE span is not an answer, and answers NULL. Nothing in the schema
// orders closed_at after created_at — the product's own write path sets
// closed_at to now() and never takes it from a caller, so the pair can only
// invert through an import or a hand-written UPDATE. Left in, one backdated
// row drags a median toward a duration nobody experienced, and a reader has no
// way to see it happen. Absent, the count beside it still says how many deals
// there were, which is the same bargain the sample floor makes.
var daysToCloseExpr = "(CASE WHEN " + zonedAnchor(colClosedAt) + "::date >= " +
	zonedAnchor("t.created_at") + "::date THEN " +
	zonedAnchor(colClosedAt) + "::date - " + zonedAnchor("t.created_at") + "::date END)"

// periodBucketExpr renders one grain over one anchor.
//
// The value is TEXT, and that is the load-bearing choice rather than a
// formatting preference. A derivation handle carries a group key out through a
// URL query string and binds it back as an equality predicate against this very
// expression, so a bucket value has to survive that round trip exactly. A
// timestamp does not: it leaves as a driver-native instant and returns as
// whatever the transport's default rendering produced, which Postgres is under
// no obligation to parse back. Text makes the round trip total instead of
// incidental, which is what keeps "Explain This Number" working on the reports
// most likely to be doubted.
//
// The bucket is cut in the installation's reporting zone (REPORT-PARAM-6,
// data-semantics §2 r4) — the same zone the forecast's "today" uses, and for
// the same reason: a year that begins at midnight in Berlin is not a year that
// begins at midnight in UTC, and a total that moves with the deployment region
// is the fast-but-wrong number reporting exists to refuse. The zone arrives as
// a bind parameter through reportZoneToken, so this stays a static expression
// with no bind position of its own to name.
func periodBucketExpr(anchor, pattern string) string {
	return "to_char(" + zonedAnchor(anchor) + ", '" + pattern + "')"
}

// zonedAnchor is the anchor instant read on the installation's reporting clock
// — the inner half every bucket expression below is built from.
func zonedAnchor(anchor string) string {
	return "timezone(" + reportZoneToken + ", " + anchor + ")"
}

// fiscalAnchor shifts the zoned anchor back so the fiscal year's first month
// lands on January, which is what lets the ordinary calendar patterns read a
// fiscal year and a fiscal quarter off it.
//
// A shift rather than arithmetic on the month number, because the two are not
// the same at a year boundary: a fiscal year starting in April means 31 March
// 2026 belongs to the year that began in April 2025, and only moving the
// instant carries the year back with the month. `make_interval` also takes the
// month count as a value, so the setting stays a bind parameter rather than
// something formatted into the statement.
func fiscalAnchor(anchor string) string {
	return zonedAnchor(anchor) +
		" - make_interval(months => " + reportFiscalStartMonthToken + " - 1)"
}

// fiscalYearLabel renders the year a bucket falls in, in the form that says
// what the year actually is:
//
//	January start → '2026'      — a fiscal year that IS the calendar year
//	April start   → 'FY2025/26' — a fiscal year that spans two of them
//
// The condition is the point, not a convenience. Every installation that
// existed before this setting did runs on the January default, so the branch it
// takes produces the byte-identical label it produced before — no report moves,
// and no saved view starts asking for a span nobody chose. Spelling a calendar
// year 'FY2026/27' would also just be false: it does not span 2027.
//
// The spanning form is spanningYearLabel below, which records why it names
// both years.
// Its second half is read off the shifted anchor plus a year rather than by
// adding one to the first, so Postgres does the arithmetic and the century
// rolls over on its own — 'FY2099/00' comes out right with nobody having
// thought about it.
func fiscalYearLabel(anchor string) string {
	return calendarOrFiscal(
		periodBucketExpr(anchor, patternYear),
		spanningYearLabel(anchor),
	)
}

// spanningYearLabel is the 'FY2025/26' half, without the calendar branch — so
// the quarter label can build on it rather than nesting one CASE inside
// another's ELSE.
//
// It names BOTH calendar years the fiscal year spans rather than one. "FY2026"
// alone is a real convention in two incompatible directions — the UK names a
// fiscal year for the year it starts in, Australia and Japan for the year it
// ends — so either short form reads as the other twelve months to half the
// readers, and this product is deployed in Europe and Vietnam at once. The
// label is also effectively permanent (see periodBucketExpr on the derivation
// round trip), which makes five extra characters the cheaper side of the trade.
//
// It still sorts as text, which is the property the calendar patterns are
// chosen for and the one a longer label could have broken silently. The leading
// four-digit year carries the ordering and the "/YY" is decorative, so
// 'FY2099/00-Q1' sorts after 'FY2026/27-Q4' rather than before it.
func spanningYearLabel(anchor string) string {
	shifted := fiscalAnchor(anchor)
	return "'FY' || to_char(" + shifted + ", '" + patternYear + "')" +
		" || '/' || to_char(" + shifted + " + interval '1 year', 'YY')"
}

// calendarOrFiscal picks between the two spellings on the one condition that
// separates them: a fiscal year starting in January IS the calendar year, and
// every installation predating this setting runs on that default.
func calendarOrFiscal(calendar, fiscal string) string {
	return "CASE WHEN " + reportFiscalStartMonthToken + " = 1" +
		" THEN " + calendar + " ELSE " + fiscal + " END"
}

// fiscalQuarterLabel renders the quarter WITHIN the fiscal year, so an
// April-starting installation reads April–June as Q1 rather than as the
// calendar's Q2:
//
//	January start → '2026-Q1'
//	April start   → 'FY2025/26-Q4'
//
// Same condition and same reason as fiscalYearLabel: the January branch is what
// every installation produced before this existed.
func fiscalQuarterLabel(anchor string) string {
	return calendarOrFiscal(
		periodBucketExpr(anchor, patternQuarter),
		spanningYearLabel(anchor)+
			" || '-Q' || to_char("+fiscalAnchor(anchor)+", '"+patternFiscalQuarter+"')",
	)
}

// periodDimensions is the three grains over one anchor, keyed by their wire
// names (REPORT-PARAM-4). The set is closed: a caller asking for a grain
// outside it is refused by the ordinary out-of-vocabulary path, never widened.
//
// The year and quarter are FISCAL, cut from the installation's configured
// start month; the month is not, because a month is the same month under every
// fiscal start. On the default January start both take their calendar branch
// and render exactly what they rendered before this setting existed, which is
// what keeps every existing report and every saved view where it was.
func periodDimensions(anchor string) map[string]string {
	return map[string]string{
		fieldPeriodYear:    fiscalYearLabel(anchor),
		fieldPeriodQuarter: fiscalQuarterLabel(anchor),
		fieldPeriodMonth:   periodBucketExpr(anchor, patternMonth),
	}
}

// withProjectFilter adds the project_id filter a deal-based report carries:
// every deal report answers "for this project" the same way.
func withProjectFilter(filters map[string]string) map[string]string {
	filters[fieldProjectID] = colProjectID
	return filters
}

// winLossSpec is REPORT-KEY-8's vocabulary, pinned upstream as REPORT-VOCAB-1.
//
// The grain is one row per deal, so a deal counts once. The base set is closed
// deals ONLY: an open deal has not been won or lost, so it is absent from this
// report rather than a zero inside it.
//
// Win RATE is deliberately not a measure. It is a ratio across two groups, so
// it is read off the won and lost rows rather than computed inside a single
// one — the same reason the stage-conversion key leaves conversion rates out of
// V1. A measure that silently answered a per-group ratio would reconcile
// against nothing a drill-through could show.
func winLossSpec() reportSpec {
	dimensions := map[string]string{
		fieldStatus:         colStatus,
		fieldSource:         colSource,
		fieldOwnerID:        colOwnerID,
		fieldPipelineID:     colPipelineID,
		fieldOrganizationID: colOrganizationID,
		fieldCurrency:       colCurrency,
	}
	maps.Copy(dimensions, periodDimensions(colClosedAt))

	return reportSpec{
		entity:    datasource.EntityDeal,
		table:     tableDeal,
		baseWhere: "t.archived_at IS NULL AND t.status IN ('won','lost')",
		basePlain: "live (unarchived) deals that have been won or lost, bucketed by when they closed " +
			"in the installation's reporting timezone (an open deal is absent from this report, not a zero in it)",
		dimensions: dimensions,
		// Money in both denominations, and how long the deal took.
		//
		// The NATIVE pair keeps its place because this spec's default grouping
		// includes currency, so a sum under one currency row is well defined —
		// the same reason the forecast and deals-by-stage keep theirs.
		//
		// The BASE pair is what makes a period comparable with the one before
		// it. "Did we win more this quarter" is a question about the business,
		// not about each currency it trades in, and a caller who groups by
		// period alone can only ask it of converted money.
		//
		// days_to_close is the measure this report existed without: it knew
		// how much closed and never how long it took, so a team getting slower
		// at the same revenue looked identical to one that was not.
		measures: map[string]string{
			fieldAmountMinor:     colAmountMinor,
			fieldAmountBaseMinor: pipelineBaseValueExpr,
			fieldDaysToClose:     daysToCloseExpr,
		},
		// Every dimension also filters (REPORT-VOCAB-1): the period grains
		// included, so "won deals in 2026" is one call rather than a group-by
		// the caller then has to sift.
		//
		// Cloned rather than aliased — the two vocabularies are equal today and
		// nothing about them has to stay equal, so sharing one map would make a
		// later edit to either silently change both.
		filters:      withProjectFilter(maps.Clone(dimensions)),
		filterScopes: projectFilterScope,
		// The company a won or lost deal points at is row-scoped and masked on
		// a normal deal read, so grouping by it carries the same obligation the
		// partner dimension does on deals-by-stage.
		referenceScopes: map[string]string{colOrganizationID: tableOrganization},
		defaultBy:       moneyDefaultBy(fieldStatus),
		// The count and the native sum keep their names and their places: they
		// are what every caller of this report already reads, and renaming or
		// reordering a default column breaks them for no gain.
		//
		// Median and p75 days are ADDED rather than substituted. How long a
		// deal takes is the other half of how a team is doing, and a report
		// that knew only the money could not tell a team getting slower at the
		// same revenue from one that was not.
		//
		// Both percentiles together, for the reason stage-age gives: one
		// without the other invites the reading that the middle deal is the
		// whole story, and the gap between them is what says whether the tail
		// is long. Below the engine's sample floor they come back NULL, and the
		// count beside them says why.
		defaultAggs: []reportAggregate{
			{Fn: aggFnCount, As: aliasDeals},
			{Fn: aggFnSum, Field: fieldAmountMinor, As: "amount_minor_sum"},
			{Fn: aggFnMedian, Field: fieldDaysToClose, As: "median_days_to_close"},
			{Fn: aggFnP75, Field: fieldDaysToClose, As: "p75_days_to_close"},
		},
	}
}
