// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What the weekly message says about a figure its source never measured, or
// could not read.
//
// The review freezes each family's coverage and the bookings coverage behind
// Won. The Home panel reads both in brief.numeric.ts. Code cannot cross the
// wire, so this file mirrors figureState for the figures the mail prints. The
// labels are the panel's, held by the mailcopy gate.

import (
	"slices"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// mailFigures reads the frozen coverage once for the rows that print it. set is
// never nil: a review without coverage holds an empty one.
type mailFigures struct {
	set *crmcontracts.WeeklyFigureCoverageSet
	// bookings is the coverage Won was measured under; nil on a legacy review.
	bookings *crmcontracts.ReportingCoverage
	zone     *time.Location
	words    mailcopy.Copy
}

// figuresOf reads a review frozen before figures carried coverage as fully
// measured, which is how that review rendered before coverage existed.
func figuresOf(review Review, words mailcopy.Copy) mailFigures {
	out := mailFigures{set: &crmcontracts.WeeklyFigureCoverageSet{}, words: words}
	if review.NumericSummary == nil {
		return out
	}
	if review.NumericSummary.FigureCoverage != nil {
		out.set = review.NumericSummary.FigureCoverage
	}
	out.bookings = &review.NumericSummary.BookingsCoverage
	// A frozen zone name this tz database does not know prints the date in UTC
	// and says so, rather than shifting it.
	if zone, err := time.LoadLocation(review.NumericSummary.Timezone); err == nil {
		out.zone = zone
	}
	return out
}

// value prints one figure as the panel does. A measured figure keeps its count
// and a partial one qualifies it. A figure whose source had not begun prints no
// count, because that zero is not a measurement.
func (f mailFigures) value(coverage *crmcontracts.WeeklyFigureCoverage, count string) string {
	if coverage == nil {
		return count
	}
	since := coverage.RecordedSince
	// The engine's Reason is English prose and the mail speaks the installation's
	// language. An undated figure takes the catalog's words where the panel shows Reason.
	switch coverage.Status {
	case crmcontracts.WeeklyFigureCoverageStatusNotRecorded:
		if since == nil {
			return f.words.WeeklyNotRecorded + " · " + f.words.WeeklyNoRecords
		}
		return f.words.WeeklyNotRecorded + " · " + strings.ReplaceAll(f.words.WeeklyRecordedFrom, "{date}", f.date(*since))
	case crmcontracts.WeeklyFigureCoverageStatusPartial:
		if since == nil {
			return strings.ReplaceAll(f.words.WeeklyPartialValue, "{value}", count)
		}
		return count + " · " + strings.ReplaceAll(f.words.WeeklyPartialFrom, "{date}", f.date(*since))
	}
	return count
}

// won prints the Won count under the bookings coverage it was measured with,
// before the deals family qualifies the row. A read that could not see the
// bookings names no count; a partial one marks the count itself, having no
// start date to give.
func (f mailFigures) won(count string) string {
	if f.bookings == nil {
		return count
	}
	switch {
	case f.bookings.Withheld || !slices.Contains(readableBookings, f.bookings.Status):
		return f.words.WeeklyUnavailable
	case f.bookings.Status == crmcontracts.ReportingStatusPartial &&
		(f.set.Deals == nil || f.set.Deals.Status != crmcontracts.WeeklyFigureCoverageStatusPartial):
		return strings.ReplaceAll(f.words.WeeklyPartialValue, "{value}", count)
	}
	return count
}

// readableBookings are the statuses the panel counts as a reading of Won.
var readableBookings = []crmcontracts.ReportingStatus{
	crmcontracts.ReportingStatusOk, crmcontracts.ReportingStatusNoData, crmcontracts.ReportingStatusPartial,
}

// beforeHistory is the panel's beforeRecordedHistory: every family the review
// prints stated not_recorded. A family with no statement counts as measured,
// so a review that could not state one is never called a week before history.
func (f mailFigures) beforeHistory() bool {
	for _, family := range []*crmcontracts.WeeklyFigureCoverage{
		f.set.Deals, f.set.Tasks, f.set.Meetings, f.set.Leads, f.set.Commitments,
	} {
		if family == nil || family.Status != crmcontracts.WeeklyFigureCoverageStatusNotRecorded {
			return false
		}
	}
	return true
}

func (f mailFigures) date(at time.Time) string {
	if f.zone == nil {
		return at.UTC().Format(mailcopy.DateLayout) + " UTC"
	}
	return at.In(f.zone).Format(mailcopy.DateLayout)
}
