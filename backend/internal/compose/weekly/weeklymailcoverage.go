// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What the weekly message says about a figure its source never measured.
//
// The review freezes each figure family's coverage, and the Home panel reads
// it in frontend/src/screens/brief.numeric.ts. The two cannot share code across
// the wire, so this file mirrors that one rule for rule; the labels themselves
// are the panel's, held equal by backend/gates/mailcopy_test.go.

import (
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// mailFigures reads the frozen coverage once for the rows that print it. set is
// never nil: a review without coverage holds an empty one.
type mailFigures struct {
	set   *crmcontracts.WeeklyFigureCoverageSet
	zone  *time.Location
	words mailcopy.Copy
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
	// The zone was valid when the engine froze it; a build without that zone's
	// data writes the date in UTC and says so rather than shifting it silently.
	if zone, err := time.LoadLocation(review.NumericSummary.Timezone); err == nil {
		out.zone = zone
	}
	return out
}

// value prints one figure as the panel does: its count where measured, the
// count qualified where the week was measured in part, and no count at all
// where the source had not begun, because that zero is not a measurement.
func (f mailFigures) value(coverage *crmcontracts.WeeklyFigureCoverage, count string) string {
	if coverage == nil {
		return count
	}
	since := coverage.RecordedSince
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
