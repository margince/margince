// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

func TestAViabilityAssessmentJudgesSilenceApartFromTheCloseDate(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, time.October, 29, 9, 0, 0, 0, berlin)
	daysAgo := func(n int) *time.Time { at := asOf.AddDate(0, 0, -n); return &at }
	closeIn94 := time.Date(2027, time.January, 31, 0, 0, 0, 0, time.UTC)
	supported := ViabilityInput{
		CreatedAt: asOf.AddDate(-1, 0, 0), LastCustomerProgressAt: daysAgo(10),
		ExpectedClose: &closeIn94, HasNextStep: true, MailCovered: true,
	}

	cases := map[string]struct {
		change  func(ViabilityInput) ViabilityInput
		outcome viability.Outcome
		reasons []viability.Reason
	}{
		"a customer who answered ten days ago supports the deal": {
			change: func(in ViabilityInput) ViabilityInput { return in }, outcome: viability.OutcomeEligible,
		},
		"196 days of silence needs review although the close date is 94 days ahead": {
			change:  func(in ViabilityInput) ViabilityInput { in.LastCustomerProgressAt = daysAgo(196); return in },
			outcome: viability.OutcomeReviewRequired,
			reasons: []viability.Reason{viability.ReasonCustomerSilent, viability.ReasonCloseDateUnsupported},
		},
		"exactly the threshold is not yet silence": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt = daysAgo(StalledThresholdDays)
				return in
			},
			outcome: viability.OutcomeEligible,
		},
		"one day past the threshold is silence": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt = daysAgo(StalledThresholdDays + 1)
				return in
			},
			outcome: viability.OutcomeReviewRequired,
			reasons: []viability.Reason{viability.ReasonCustomerSilent, viability.ReasonCloseDateUnsupported},
		},
		"a deal nobody ever heard from counts from its creation": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt, in.CreatedAt = nil, *daysAgo(90)
				return in
			},
			outcome: viability.OutcomeReviewRequired,
			reasons: []viability.Reason{viability.ReasonNeverContacted, viability.ReasonCloseDateUnsupported},
		},
		"an unread mailbox observes no silence": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt, in.MailCovered = daysAgo(196), false
				return in
			},
			outcome: viability.OutcomeUnknown, reasons: []viability.Reason{viability.ReasonCoverageUnknown},
		},
		"a wait the customer asked for defers the review": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt, in.CustomerWaitUntil = daysAgo(196), new(asOf.AddDate(0, 0, 14))
				return in
			},
			outcome: viability.OutcomeWaiting, reasons: []viability.Reason{viability.ReasonCustomerWait},
		},
		"a wait that ended defers nothing": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt, in.CustomerWaitUntil = daysAgo(196), daysAgo(1)
				return in
			},
			outcome: viability.OutcomeReviewRequired,
			reasons: []viability.Reason{viability.ReasonCustomerSilent, viability.ReasonCloseDateUnsupported},
		},
		"an accepted meeting still to come defers the review": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt, in.ConfirmedMeetingAt = daysAgo(196), new(asOf.AddDate(0, 0, 3))
				return in
			},
			outcome: viability.OutcomeWaiting, reasons: []viability.Reason{viability.ReasonMeetingConfirmed},
		},
		"a reply during an open review clears the silence and keeps the review": {
			change: func(in ViabilityInput) ViabilityInput {
				in.OpenReview, in.HasNextStep, in.CloseProvisional = true, false, true
				return in
			},
			outcome: viability.OutcomeReviewRequired,
			reasons: []viability.Reason{
				viability.ReasonRequalificationPending, viability.ReasonCloseDateUnsupported, viability.ReasonNoNextStep,
			},
		},
		"evidence dated in the future is not progress": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerProgressAt, in.CreatedAt = new(asOf.AddDate(0, 0, 1)), *daysAgo(196)
				return in
			},
			outcome: viability.OutcomeReviewRequired,
			reasons: []viability.Reason{viability.ReasonNeverContacted, viability.ReasonCloseDateUnsupported},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := AssessViability(c.change(supported), asOf, berlin)
			if got.Outcome != c.outcome || !slices.Equal(got.Reasons, c.reasons) {
				t.Fatalf("got %s %v, want %s %v", got.Outcome, got.Reasons, c.outcome, c.reasons)
			}
		})
	}
}

func TestADealTheCustomerAnswersStillNeedsADateAndANextStep(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, time.October, 29, 9, 0, 0, 0, berlin)
	daysAgo := func(n int) *time.Time { at := asOf.AddDate(0, 0, -n); return &at }
	closeIn94 := time.Date(2027, time.January, 31, 0, 0, 0, 0, time.UTC)
	answered := ViabilityInput{
		CreatedAt: asOf.AddDate(-1, 0, 0), LastCustomerProgressAt: daysAgo(7),
		ExpectedClose: &closeIn94, HasNextStep: true, MailCovered: true,
	}

	cases := map[string]struct {
		change  func(ViabilityInput) ViabilityInput
		outcome viability.Outcome
		reasons []viability.Reason
	}{
		"a provisional date leaves the deal out without a review": {
			change:  func(in ViabilityInput) ViabilityInput { in.CloseProvisional = true; return in },
			outcome: viability.OutcomeUnsupported, reasons: []viability.Reason{viability.ReasonCloseDateUnsupported},
		},
		"no close date leaves the deal out without a review": {
			change:  func(in ViabilityInput) ViabilityInput { in.ExpectedClose = nil; return in },
			outcome: viability.OutcomeUnsupported, reasons: []viability.Reason{viability.ReasonCloseDateUnsupported},
		},
		"no next step leaves the deal out without a review": {
			change:  func(in ViabilityInput) ViabilityInput { in.HasNextStep = false; return in },
			outcome: viability.OutcomeUnsupported, reasons: []viability.Reason{viability.ReasonNoNextStep},
		},
		"a wait is the next step, with a close date after it": {
			change: func(in ViabilityInput) ViabilityInput {
				in.HasNextStep, in.CustomerWaitUntil = false, new(asOf.AddDate(0, 0, 30))
				return in
			},
			outcome: viability.OutcomeWaiting, reasons: []viability.Reason{viability.ReasonCustomerWait},
		},
		"a close date before the wait ends cannot hold": {
			change: func(in ViabilityInput) ViabilityInput {
				in.CustomerWaitUntil = new(closeIn94.AddDate(0, 1, 0))
				return in
			},
			outcome: viability.OutcomeUnsupported, reasons: []viability.Reason{viability.ReasonCloseDateUnsupported},
		},
		"a refusal raises the review at once": {
			change:  func(in ViabilityInput) ViabilityInput { in.LastCustomerRefusalAt = daysAgo(2); return in },
			outcome: viability.OutcomeReviewRequired, reasons: []viability.Reason{viability.ReasonCustomerDeclined},
		},
		"a refusal answered by a later message raises nothing": {
			change:  func(in ViabilityInput) ViabilityInput { in.LastCustomerRefusalAt = daysAgo(20); return in },
			outcome: viability.OutcomeEligible,
		},
		"a refusal outranks a wait": {
			change: func(in ViabilityInput) ViabilityInput {
				in.LastCustomerRefusalAt, in.CustomerWaitUntil = daysAgo(2), new(asOf.AddDate(0, 0, 30))
				return in
			},
			outcome: viability.OutcomeReviewRequired, reasons: []viability.Reason{viability.ReasonCustomerDeclined},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := AssessViability(c.change(answered), asOf, berlin)
			if got.Outcome != c.outcome || !slices.Equal(got.Reasons, c.reasons) {
				t.Fatalf("got %s %v, want %s %v", got.Outcome, got.Reasons, c.outcome, c.reasons)
			}
		})
	}
}

func TestSilenceCountsCalendarDaysAcrossTheShortDay(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	wrote := time.Date(2026, time.January, 28, 0, 5, 0, 0, berlin)
	in := ViabilityInput{
		CreatedAt: wrote, LastCustomerProgressAt: &wrote, MailCovered: true, HasNextStep: true, ExpectedClose: new(wrote.AddDate(1, 0, 0)),
	}
	// 29 March 2026 is 23 hours long in Berlin; counting hours would call the
	// day after it one short.
	if got := AssessViability(in, time.Date(2026, time.March, 29, 0, 5, 0, 0, berlin), berlin); got.SilentDays != 60 ||
		got.Outcome != viability.OutcomeEligible {
		t.Fatalf("on 29 March: %d days, %s; want 60 days, eligible", got.SilentDays, got.Outcome)
	}
	if got := AssessViability(in, time.Date(2026, time.March, 30, 0, 5, 0, 0, berlin), berlin); got.SilentDays != 61 ||
		got.Outcome != viability.OutcomeReviewRequired {
		t.Fatalf("on 30 March: %d days, %s; want 61 days, review_required", got.SilentDays, got.Outcome)
	}
}

func TestAnEligibleAssessmentNamesTheDayItsSilenceCrossesTheThreshold(t *testing.T) {
	t.Parallel()
	zone := time.UTC
	asOf := time.Date(2026, time.October, 29, 12, 0, 0, 0, zone)
	wrote := time.Date(2026, time.October, 1, 15, 0, 0, 0, zone)
	got := AssessViability(ViabilityInput{
		CreatedAt: wrote.AddDate(0, -1, 0), LastCustomerProgressAt: &wrote, MailCovered: true, HasNextStep: true,
		ExpectedClose: new(wrote.AddDate(1, 0, 0)),
	}, asOf, zone)
	want := time.Date(2026, time.December, 1, 0, 0, 0, 0, zone)
	if got.Outcome != viability.OutcomeEligible || got.NextChange == nil || !got.NextChange.Equal(want) {
		t.Fatalf("got %s next change %v, want eligible until %v", got.Outcome, got.NextChange, want)
	}
	if after := AssessViability(ViabilityInput{
		CreatedAt: wrote.AddDate(0, -1, 0), LastCustomerProgressAt: &wrote, MailCovered: true, HasNextStep: true,
		ExpectedClose: new(wrote.AddDate(1, 0, 0)),
	}, want, zone); after.Outcome != viability.OutcomeReviewRequired {
		t.Fatalf("at the named day the deal reads %s, want review_required", after.Outcome)
	}
}
