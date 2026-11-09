// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const leadObject = "lead"

// MayReadLeads reports whether the caller may read lead figures.
func MayReadLeads(ctx context.Context) bool {
	return auth.Allows(ctx, leadObject, principal.ActionRead)
}

// A frozen summary can quote the lead figures, so removing its counts must
// also remove prose that cannot be separated into authorized facts.
func withholdReviewLeads(review *Review) {
	review.LeadsWithheld = true
	review.NumericSummary = withoutLeadCoverage(review.NumericSummary)
	review.Counts.LeadsRouted, review.Counts.LeadsAnsweredInTarget, review.Counts.LeadsBreached = 0, 0, 0
	if review.Scorecard != nil {
		card := *review.Scorecard
		card.Lead = nil
		review.Scorecard = &card
	}
	review.Narrative, review.NarratedAt = "", nil
	review.LearningsState, review.Learnings = LearningsNotRun, nil
}

func withholdTeamLeads(review *TeamReview) {
	review.LeadsWithheld = true
	review.NumericSummary = withoutLeadCoverage(review.NumericSummary)
	review.Counts.LeadsRouted, review.Counts.LeadsAnsweredInTarget, review.Counts.LeadsBreached = 0, 0, 0
	said := teamCopyFor(review.Language)
	for i := range review.Reps {
		rep := &review.Reps[i]
		rep.LeadsBreached = 0
		removeTeamPattern(rep, FocusLeadsBreached, said)
	}
}

func removeTeamPattern(rep *TeamRep, kind string, said teamCopy) {
	rep.Patterns = slices.DeleteFunc(slices.Clone(rep.Patterns), func(pattern Pattern) bool { return pattern.Kind == kind })
	if len(rep.Patterns) == 0 {
		rep.Patterns = []Pattern{{Kind: FocusQuietWeek, Label: said.Say(patternWords.Quiet)}}
	}
	rep.FocusKind, rep.FocusLabel = rep.Patterns[0].Kind, rep.Patterns[0].Label
}

func withoutLeadCoverage(summary *crmcontracts.WeeklyNumericSummary) *crmcontracts.WeeklyNumericSummary {
	if summary == nil || summary.FigureCoverage == nil {
		return summary
	}
	redacted := *summary
	coverage := *redacted.FigureCoverage
	coverage.Leads = nil
	redacted.FigureCoverage = &coverage
	return &redacted
}
