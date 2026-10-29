// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// ViabilityInput is what one assessment of one open deal reads. Every field is
// evidence compose assembled; none is a judgement another rule already made.
type ViabilityInput struct {
	CreatedAt time.Time
	// LastCustomerProgressAt is when the customer last wrote to us or met with
	// us. Our own mail, a field edit or a task never moves it.
	LastCustomerProgressAt *time.Time
	// CustomerWaitUntil is the day the customer asked us to wait until.
	CustomerWaitUntil *time.Time
	// ConfirmedMeetingAt is the next meeting still to come that a customer
	// guest accepted. A booked meeting nobody outside accepted is not one.
	ConfirmedMeetingAt *time.Time
	ExpectedClose      *time.Time
	CloseProvisional   bool
	HasNextStep        bool
	// MailCovered is true when every mailbox was read on time. Without it no
	// silence is observed, because "nothing arrived" would mean "nothing was read".
	MailCovered bool
	// OpenReview is true while a viability review on this deal is open. Only
	// somebody's validated answer closes it; a reply alone does not.
	OpenReview bool
}

// Viability is the assessment. Outcome, the reasons and the next time the
// answer can change by the clock alone are stored on the receipt.
type Viability struct {
	Outcome    viability.Outcome
	Reasons    []viability.Reason
	SilentDays int
	// NextChange is when the answer changes if no evidence arrives: the day the
	// silence crosses the threshold, or the day a deferral ends. Nil when only
	// new evidence can change it.
	NextChange *time.Time
}

// AssessViability judges one open deal at asOf, counting days in the
// installation's zone.
//
// Customer silence is judged apart from the close date. A close date 94 days
// ahead does not make a deal the customer stopped answering 196 days ago
// supported, and the close-date check never asked that question.
func AssessViability(in ViabilityInput, asOf time.Time, zone *time.Location) Viability {
	if !in.MailCovered {
		return Viability{Outcome: viability.OutcomeUnknown, Reasons: []viability.Reason{viability.ReasonCoverageUnknown}}
	}
	progress := in.LastCustomerProgressAt
	if progress != nil && progress.After(asOf) {
		progress = nil
	}
	anchor := idlebase.Since(in.CreatedAt, progress)
	silentDays := elapsed.Days(anchor, asOf, zone)
	silent := silentDays > StalledThresholdDays

	out := Viability{SilentDays: silentDays}
	deferral := activeDeferral(in, asOf)
	if !silent {
		crossing := thresholdCrossing(anchor, zone)
		out.NextChange = &crossing
	}

	if silent && deferral == "" {
		out.Reasons = append(out.Reasons, silenceReason(progress))
	}
	if in.OpenReview {
		out.Reasons = append(out.Reasons, viability.ReasonRequalificationPending)
	}
	if len(out.Reasons) > 0 {
		out.Outcome = viability.OutcomeReviewRequired
		out.Reasons = append(out.Reasons, dateAndStepReasons(in, silent, asOf, zone)...)
		return out
	}
	if deferral != "" {
		out.Outcome = viability.OutcomeWaiting
		out.Reasons = []viability.Reason{deferral}
		out.NextChange = earliest(out.NextChange, deferralEnd(in, deferral))
		return out
	}
	out.Outcome = viability.OutcomeEligible
	return out
}

// activeDeferral is the supported reason to wait, if one is still running.
func activeDeferral(in ViabilityInput, asOf time.Time) viability.Reason {
	switch {
	case in.CustomerWaitUntil != nil && asOf.Before(*in.CustomerWaitUntil):
		return viability.ReasonCustomerWait
	case in.ConfirmedMeetingAt != nil && asOf.Before(*in.ConfirmedMeetingAt):
		return viability.ReasonMeetingConfirmed
	}
	return ""
}

func deferralEnd(in ViabilityInput, deferral viability.Reason) *time.Time {
	if deferral == viability.ReasonCustomerWait {
		return in.CustomerWaitUntil
	}
	return in.ConfirmedMeetingAt
}

func silenceReason(progress *time.Time) viability.Reason {
	if progress == nil {
		return viability.ReasonNeverContacted
	}
	return viability.ReasonCustomerSilent
}

// dateAndStepReasons names what somebody must validate before support returns.
// An unset provisional flag is not a confirmation: a date the customer has
// been silent about is unsupported whatever the flag says.
func dateAndStepReasons(in ViabilityInput, silent bool, asOf time.Time, zone *time.Location) []viability.Reason {
	var out []viability.Reason
	if in.ExpectedClose == nil || in.CloseProvisional || silent || CloseIsOverdue(*in.ExpectedClose, asOf, zone) {
		out = append(out, viability.ReasonCloseDateUnsupported)
	}
	if !in.HasNextStep {
		out = append(out, viability.ReasonNoNextStep)
	}
	return out
}

// thresholdCrossing is the first local midnight at which the silence counts as
// more than StalledThresholdDays days.
func thresholdCrossing(anchor time.Time, zone *time.Location) time.Time {
	local := anchor.In(zone)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	return day.AddDate(0, 0, StalledThresholdDays+1)
}

func earliest(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil || a.Before(*b) {
		return a
	}
	return b
}
