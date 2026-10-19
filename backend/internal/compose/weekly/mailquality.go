// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// MailUseful reports whether the frozen review has recorded results.
func MailUseful(review Review) bool {
	return !figuresOf(review, mailcopy.For("en")).beforeHistory()
}

// NarrativeCovered requires each figure source before a narrative is trusted.
func NarrativeCovered(review Review) bool {
	if review.NumericSummary == nil || review.NumericSummary.FigureCoverage == nil {
		return false
	}
	coverage := review.NumericSummary.FigureCoverage
	for _, source := range []*crmcontracts.WeeklyFigureCoverage{coverage.Tasks, coverage.Deals, coverage.Meetings, coverage.Leads, coverage.Commitments} {
		if source == nil || source.Status != "recorded" {
			return false
		}
	}
	return !review.NumericSummary.BookingsCoverage.Withheld && review.NumericSummary.BookingsCoverage.Status == "ok"
}
