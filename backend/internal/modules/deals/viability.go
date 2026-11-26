// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/idlebase"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// ViabilityInput is what one assessment of one open deal reads. Every field is
// evidence compose assembled; none is a judgement another rule already made.
type ViabilityInput struct {
	CreatedAt time.Time
	// LastCustomerProgressAt is when the customer last wrote to us or met with
	// us. Our own mail, a field edit or a task never moves it.
	LastCustomerProgressAt *time.Time
	// LastCustomerProgressDateOnly says that progress names only a date, so the
	// silence after it counts from that date.
	LastCustomerProgressDateOnly bool
	// LastCustomerRefusalAt is when the customer last wrote to refuse. A refusal
	// is not progress; it only counts while nothing they sent since moved the deal.
	LastCustomerRefusalAt *time.Time
	// CustomerWaitUntil is the instant the customer's wait ends (deals.PauseEndsSQL).
	CustomerWaitUntil *time.Time
	// ConfirmedMeetingAt is the next meeting still to come that a customer
	// guest accepted. A booked meeting nobody outside accepted is not one.
	ConfirmedMeetingAt *time.Time
	// EarlierAnswerUntil is when an answer somebody gave the retired
	// buyer-silent check runs out. Until then the deal is not asked again.
	EarlierAnswerUntil *time.Time
	ExpectedClose      *time.Time
	CloseProvisional   bool
	HasNextStep        bool
	// MailCovered is true when the mail this deal's silence is read from was
	// read on time: the mailboxes of the owner and of every user whose mailbox
	// captured mail on the deal, or every mailbox when there is no such user. Without it no silence is observed, because "nothing arrived"
	// would mean "nothing was read".
	MailCovered bool
	// OpenReview is true while a viability review on this deal is open. Only
	// somebody's validated answer closes it; a reply alone does not.
	OpenReview bool
}

// Viability is the assessment. The outcome is stored on the receipt.
type Viability struct {
	Outcome    viability.Outcome
	Reasons    []viability.Reason
	SilentDays int
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
	// A date-only entry is past once its date has begun in zone. Far east of UTC
	// its stored noon is then still ahead of asOf.
	progress := in.LastCustomerProgressAt
	if progress != nil && !occurredday.Begun(occurredday.Moment{At: *progress, DateOnly: in.LastCustomerProgressDateOnly}, asOf, zone) {
		progress = nil
	}
	anchor := occurredday.Moment{At: idlebase.Since(in.CreatedAt, progress), DateOnly: progress != nil && in.LastCustomerProgressDateOnly}
	silentDays := occurredday.DaysSince(anchor, asOf, zone)
	silent := silentDays > StalledThresholdDays

	out := Viability{SilentDays: silentDays}
	deferral := activeDeferral(in, asOf)

	switch {
	case declined(in.LastCustomerRefusalAt, progress, asOf):
		out.Reasons = append(out.Reasons, viability.ReasonCustomerDeclined)
	case silent && deferral == "":
		out.Reasons = append(out.Reasons, silenceReason(progress))
	}
	if in.OpenReview {
		out.Reasons = append(out.Reasons, viability.ReasonRequalificationPending)
	}
	support := supportReasons(in, silent && deferral == "", deferral, asOf, zone)
	if len(out.Reasons) > 0 {
		out.Outcome = viability.OutcomeReviewRequired
		out.Reasons = append(out.Reasons, support...)
		return out
	}
	if len(support) > 0 {
		out.Outcome = viability.OutcomeUnsupported
		out.Reasons = support
		return out
	}
	if deferral != "" {
		out.Outcome = viability.OutcomeWaiting
		out.Reasons = []viability.Reason{deferral}
		return out
	}
	out.Outcome = viability.OutcomeEligible
	return out
}

// declined reports a refusal nothing has answered since: a later message or
// meeting that moved the deal outranks it.
func declined(refusal, progress *time.Time, asOf time.Time) bool {
	if refusal == nil || refusal.After(asOf) {
		return false
	}
	return progress == nil || refusal.After(*progress)
}

// activeDeferral is the supported reason to wait, if one is still running.
func activeDeferral(in ViabilityInput, asOf time.Time) viability.Reason {
	switch {
	case in.CustomerWaitUntil != nil && asOf.Before(*in.CustomerWaitUntil):
		return viability.ReasonCustomerWait
	case in.ConfirmedMeetingAt != nil && asOf.Before(*in.ConfirmedMeetingAt):
		return viability.ReasonMeetingConfirmed
	case in.EarlierAnswerUntil != nil && asOf.Before(*in.EarlierAnswerUntil):
		return viability.ReasonEarlierAnswer
	}
	return ""
}

func silenceReason(progress *time.Time) viability.Reason {
	if progress == nil {
		return viability.ReasonNeverContacted
	}
	return viability.ReasonCustomerSilent
}

// supportReasons names what a deal lacks to count, judged on every deal. An
// unset provisional flag is not a confirmation: a date the customer has been
// silent about is unsupported whatever the flag says. A wait or a confirmed
// meeting is itself the next step, and a close date before the wait ends
// cannot hold.
func supportReasons(in ViabilityInput, silent bool, deferral viability.Reason, asOf time.Time, zone *time.Location) []viability.Reason {
	var out []viability.Reason
	switch {
	case in.ExpectedClose == nil || in.CloseProvisional || silent || CloseIsOverdue(*in.ExpectedClose, asOf, zone):
		out = append(out, viability.ReasonCloseDateUnsupported)
	case deferral == viability.ReasonCustomerWait && in.ExpectedClose.Before(elapsed.CalendarDay(*in.CustomerWaitUntil, zone)):
		out = append(out, viability.ReasonCloseDateUnsupported)
	}
	if !in.HasNextStep && deferral == "" {
		out = append(out, viability.ReasonNoNextStep)
	}
	return out
}
