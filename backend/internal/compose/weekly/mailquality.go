// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// MailUseful reports whether the frozen review has recorded results.
func MailUseful(review Review) bool {
	f := figuresOf(review, mailcopy.For("en"))
	c := review.Counts
	measured := func(source *crmcontracts.WeeklyFigureCoverage) bool {
		return source == nil || source.Status != crmcontracts.WeeklyFigureCoverageStatusNotRecorded
	}
	tasks := c.TasksDue+c.TasksDone+c.TasksCarriedOver > 0 || (c.TasksCompleted != nil && *c.TasksCompleted > 0)
	deals := c.DealsLost+c.DealsMoved > 0 || len(review.Deals) > 0 || (c.DealsWon > 0 && f.readableBookings())
	return (tasks && measured(f.set.Tasks)) || (deals && measured(f.set.Deals)) || (c.CommitmentsKept+c.CommitmentsDue > 0 && measured(f.set.Commitments)) || (c.LeadsAnsweredInTarget+c.LeadsRouted+c.LeadsBreached > 0 && measured(f.set.Leads)) || (c.MeetingsWithNextStep+c.MeetingsHeld > 0 && measured(f.set.Meetings))
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

// TeamMailUseful requires outcomes or a concrete exception, rather than roster counts.
func TeamMailUseful(review TeamReview) bool {
	c := review.Counts
	if review.RepsUnread > 0 || c.DealsWon+c.DealsLost+c.DealsMoved+c.LeadsRouted+c.LeadsAnsweredInTarget+c.LeadsBreached+c.MeetingsHeld+c.MeetingsWithNextStep+c.CommitmentsKept > 0 || c.CommitmentsDue > 0 {
		return true
	}
	for _, member := range review.Reps {
		if member.HelpRequested > 0 {
			return true
		}
	}
	attention := TeamMailAttention(review)
	return len(attention.Exceptions) > 0 || len(attention.StaleDeals) > 0
}

// TeamMailSummary states coverage without assigning a performance grade.
func TeamMailSummary(review TeamReview, words mailcopy.Copy) string {
	return fmt.Sprintf(words.MailMembersCounted, review.Counts.RepsCounted, review.Counts.RepsCounted+review.RepsUnread)
}

// TeamMailFacts uses the same named metrics and source coverage as personal mail.
func TeamMailFacts(review TeamReview, words mailcopy.Copy) []emailview.Fact {
	c := review.Counts
	return mailFacts(Review{NumericSummary: review.NumericSummary, Money: review.Money, Counts: Counts{DealsWon: c.DealsWon, DealsLost: c.DealsLost, DealsMoved: c.DealsMoved, LeadsRouted: c.LeadsRouted, LeadsAnsweredInTarget: c.LeadsAnsweredInTarget, LeadsBreached: c.LeadsBreached, MeetingsHeld: c.MeetingsHeld, MeetingsWithNextStep: c.MeetingsWithNextStep, CommitmentsDue: c.CommitmentsDue, CommitmentsKept: c.CommitmentsKept}}, words)
}

// TeamMailFocus names a frozen pattern in the recipient's language without rewriting it.
func TeamMailFocus(member TeamRep, language string) string {
	phrase, known := focusWords[member.FocusKind]
	if !known || member.FocusKind == FocusStrongWeek || member.FocusKind == FocusQuietWeek || member.FocusKind == FocusHelpRequested {
		return ""
	}
	return teamCopyFor(language).Say(phrase)
}

// TeamMailAttention excludes unnameable items from both usefulness and presentation.
func TeamMailAttention(review TeamReview) TeamAttentionItems {
	out := TeamAttentionItems{Read: review.Attention.Read}
	for _, finding := range review.Attention.Exceptions {
		if finding.Subject != nil && finding.Subject.Label != nil && strings.TrimSpace(*finding.Subject.Label) != "" {
			out.Exceptions = append(out.Exceptions, finding)
		}
	}
	for _, deal := range review.Attention.StaleDeals {
		if strings.TrimSpace(deal.DealName) != "" {
			out.StaleDeals = append(out.StaleDeals, deal)
		}
	}
	return out
}
