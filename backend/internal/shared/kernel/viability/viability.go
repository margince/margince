// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package viability is the vocabulary of one question: is an open deal still
// supported by its customer, so that it may count toward the qualified forecast.
//
// deals answers the question, assurance raises and stores the review, and
// forecasting applies the answer to money. A module never imports a sibling, so
// the words the three share live here and none of them owns the others' copy.
package viability

// PolicyVersion names the assessment rule. A receipt stamped with another
// version was judged by a different rule, so a reader treats it as unknown.
const PolicyVersion = "v1"

// Outcome is what one assessment concluded about one deal.
type Outcome string

// The outcomes. Unknown is not a weaker eligible: it says the sources needed
// to judge the deal could not be read, and no qualified total may count it.
const (
	OutcomeEligible       Outcome = "eligible"
	OutcomeReviewRequired Outcome = "review_required"
	OutcomeWaiting        Outcome = "waiting"
	OutcomeUnknown        Outcome = "unknown"
)

// Outcomes lists every outcome, for the census that holds the receipt
// table's CHECK to this list.
func Outcomes() []Outcome {
	return []Outcome{OutcomeEligible, OutcomeReviewRequired, OutcomeWaiting, OutcomeUnknown}
}

// Reason is one cause behind an outcome. A reason is a code, never prose: the
// reader's own language renders it, and stored prose about a mailbox would
// outlive the protection on the mail it described.
type Reason string

// The reasons.
const (
	// ReasonCustomerSilent: the customer has said nothing for longer than the
	// threshold since they last wrote or met with us.
	ReasonCustomerSilent Reason = "customer_silent"
	// ReasonNeverContacted: the customer has said nothing since the deal was
	// created, for longer than the threshold.
	ReasonNeverContacted Reason = "never_contacted"
	// ReasonCloseDateUnsupported: the recorded close date has no customer
	// evidence behind it, or nobody confirmed it.
	ReasonCloseDateUnsupported Reason = "close_date_unsupported"
	// ReasonNoNextStep: nobody wrote down what happens next.
	ReasonNoNextStep Reason = "no_next_step"
	// ReasonRequalificationPending: a review is open, and stays open until a
	// person validates the next step and the date. A reply clears the silence
	// but not this.
	ReasonRequalificationPending Reason = "requalification_pending"
	// ReasonCustomerWait: the customer asked us to wait, until a stated day.
	ReasonCustomerWait Reason = "customer_wait"
	// ReasonMeetingConfirmed: the customer accepted a meeting still to come.
	ReasonMeetingConfirmed Reason = "meeting_confirmed"
	// ReasonCoverageUnknown: the mailbox could not be read, so silence cannot
	// be observed.
	ReasonCoverageUnknown Reason = "coverage_unknown"
)

// Qualification is how a deal's money is treated in a qualified forecast
// reading. It is derived on read from a receipt and an open review; nothing
// stores it on the deal.
type Qualification string

// The qualifications. NotApplied is the state while enforcement is off: the
// qualified readings are then unavailable, never the baseline under another name.
const (
	QualificationNotApplied Qualification = ""
	QualificationEligible   Qualification = "eligible"
	QualificationExcluded   Qualification = "excluded"
	QualificationUnknown    Qualification = "unknown"
)
