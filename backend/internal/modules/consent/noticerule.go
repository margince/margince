// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "time"

// Which disclosure a contact is owed, decided from how it was obtained.
//
// The split is Art. 13 against Art. 14, and it turns on ONE question: did the
// data come from the subject themselves? If it did, the disclosure is owed at
// collection and the surface that collected it is what makes it. If it came
// from anywhere else — a referral, a directory, a bought list — the subject
// does not know we hold anything, and the controller has a month to say so.

// art14Months is the month Art. 14(3)(a) allows, counted from the acquisition
// rather than from when the row was written: an import landing today carrying
// last year's business card is already late, and dating the duty from the
// import would quietly restart a clock that has been running for a year.
//
// A CALENDAR month, added with AddDate, not a fixed span of hours. "One month"
// in the regulation follows the calendar, so 30*24h is short in a 31-day month
// and long in February — and a duration of hours also loses a day across a
// daylight-saving boundary, which is how a deadline drifts without anybody
// changing it.
const art14Months = 1

// NoticeDuty is what an acquisition kind obliges, and how it may be met.
type NoticeDuty struct {
	// Rule names the article. Zero value (empty) means no case is owed.
	Rule NoticeRule
	// State is where the case starts.
	State NoticeState
	// Routes are the ways this duty can be discharged. Empty means the product
	// has no way to send it, which the case says out loud rather than offering
	// an operator a button that fails.
	Routes []string
	// Months is how many calendar months after the acquisition the duty falls
	// due. Zero means it was already owed at collection.
	Months int
}

// noticeRouteReply — the disclosure rides the reply somebody is already going
// to send. Available only where a reply is actually expected.
const noticeRouteReply = "reply"

// noticeRouteRecordConfirmation — the confirm-details mail, which already
// exists as a controller-sent message and already names the installation.
const noticeRouteRecordConfirmation = "record_confirmation"

// noticeRoutePrivacyNotice — the notice mail itself, which tells the subject
// what is held and asks nothing. It is the route that reaches a contact who has
// asked us to stop: the disclosure duty survives that stop and the engine lets
// CategoryPrivacyNotice through, while the record confirmation's category is
// refused.
const noticeRoutePrivacyNotice = "privacy_notice"

// DutyFor answers what the installation owes a contact obtained this way.
//
// The four kinds that owe nothing all share one property: the subject handed us
// the data themselves, so Art. 13 was discharged by the surface that took it —
// the form said who we are, the meeting request was theirs, the contract has
// our name on it. A case for those would be a duty nobody owes, cluttering the
// queue that exists to show real ones.
//
// `not_required` is deliberately NOT written for them. The plan's own rule is
// that not_required belongs only where the disclosure is EVIDENCED, and a row
// asserting "already told, at collection" would be an assumption dressed as a
// record. No case at all is the honest answer: the acquisition row says how the
// contact arrived, and that is the evidence.
func DutyFor(acquisitionKind string) (NoticeDuty, bool) {
	switch acquisitionKind {
	// From the subject. Art. 13 was owed at collection, by the surface that
	// collected it, and no case is opened here.
	case "subject_initiated", "customer_contract",
		"requested_quote_or_meeting", "in_person_permission":
		return NoticeDuty{}, false

	// A form or event registration IS from the subject, but unlike the four
	// above it is a surface the installation controls and can be wrong about:
	// a booking widget embedded by a customer may disclose nothing. The case is
	// opened so somebody confirms the disclosure was actually made, and the
	// reply the booking already sends is where it can ride.
	case "event_or_form":
		return NoticeDuty{
			Rule: RuleArt13, State: NoticeOpen,
			Routes: []string{noticeRouteReply}, Months: 0,
		}, true

	// From somewhere else entirely. The subject does not know we hold anything.
	case "referral", "public_or_business_source", "purchased_or_imported":
		return strictNotice(), true

	// The door could not say. Named rather than left to the default so the
	// census in gates/noticedutycensus_test.go sees it decided — and it returns
	// the SAME expression, so the two cannot drift.
	case "unknown_legacy":
		return strictNotice(), true
	}
	// An unrecognised kind takes the strict reading too: this function must not
	// go quiet when the vocabulary grows past it. A duty wrongly raised costs
	// somebody a look; one wrongly skipped is invisible.
	return strictNotice(), true
}

// strictNotice is the Art. 14 duty owed where the data did not come from the
// subject, or where nobody can say whether it did.
func strictNotice() NoticeDuty {
	return NoticeDuty{
		Rule: RuleArt14, State: NoticeOpen,
		// BOTH routes, with the notice first. An Art. 14 duty can be
		// discharged by either mail, and an installation that has stopped
		// writing to this contact can only use the notice — so listing it
		// makes the duty dischargeable where it previously was not.
		Routes: []string{noticeRoutePrivacyNotice, noticeRouteRecordConfirmation},
		Months: art14Months,
	}
}

// AddMonths adds calendar months, clamping to the last day of the target month.
//
// time.AddDate does NOT mean "the same date next month": it normalizes an
// invalid date forward, so 31 January plus one month is 3 March — a statutory
// deadline three days past the end of February, in the direction that favours
// the controller. 30*24h was wrong the other way (short in a 31-day month, and
// a day adrift across a daylight-saving boundary), so neither the duration nor
// the raw AddDate is the answer.
//
// Clamping is what "one month" means when the day does not exist in the target
// month: 31 January plus one month is 28 February, or 29 in a leap year.
func AddMonths(from time.Time, months int) time.Time {
	if months == 0 {
		return from
	}
	year, month, day := from.Date()
	target := time.Date(year, month+time.Month(months), 1,
		from.Hour(), from.Minute(), from.Second(), from.Nanosecond(), from.Location())
	// The last day of the target month: day 0 of the month after it.
	last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, from.Location()).Day()
	if day > last {
		day = last
	}
	return time.Date(target.Year(), target.Month(), day,
		from.Hour(), from.Minute(), from.Second(), from.Nanosecond(), from.Location())
}
