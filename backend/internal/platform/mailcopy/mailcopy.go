// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailcopy is what this installation's outbound mail says, in the
// language the installation speaks.
//
// Every message the product sent was hard-coded English while the screens were
// translated three ways. For the two transactional messages that is a small
// thing — a person reading a password link already knows what they asked for.
// The weekly retrospective is not: it arrives unasked every Monday, it is the
// product talking to a rep about their own week, and a German-speaking rep read
// their Home panel in German and then got an English summary of the same
// numbers.
//
// ONE catalog for every sender rather than a table beside each one. Three
// senders each inventing their own is how one product comes to have three
// voices, and it is the duplication the issue that raised this named.
//
// THE WEEKLY'S LABELS ARE THE SCREEN'S. "Promised, delivered" in the mail is the
// same phrase the weekly panel draws, so they are not translated twice: the
// strings here are the frontend catalog's, and a gate compares them. Two
// translations of one label is how the mail comes to say something the panel
// does not, in a message whose whole subject is numbers the reader can also see
// on screen.
//
// Held by: TestEveryMailLabelMatchesTheScreenThatShowsIt
// (backend/gates/mailcopy_test.go)
package mailcopy

import "strings"

// Language is a base language this installation can send in. The set is the
// contract's `base_language` enum, and English is what an installation that
// names none is treated as speaking.
type Language string

// The languages this build has copy for, which the gate holds equal to the
// contract's own base_language enum.
const (
	English    Language = "en"
	German     Language = "de"
	Vietnamese Language = "vi"
)

// Fallback is the language a message is written in when the installation names
// one this build has no copy for.
//
// It is a FALLBACK rather than a refusal because the alternative is sending
// nothing: a password link that does not arrive is worse for its reader than an
// English one, and a weekly summary is worth reading in the wrong language. A
// build that adds a language to the contract and not to this catalog is caught
// by the gate, not by a rep's mailbox.
const Fallback = English

// For is the copy one installation's mail is written in.
func For(language string) Copy {
	if words, known := catalog[Language(language)]; known {
		return words
	}
	return catalog[Fallback]
}

// Known reports whether this build carries copy for a language.
//
// Separate from For because For ANSWERS for every language — that is what makes
// it safe to call — and a caller asking "is this one carried?" cannot tell a
// real entry from the fallback by looking at what came back. Comparing a field
// against English would call a language missing the moment one of its labels
// legitimately matched English's.
func Known(language string) bool {
	_, known := catalog[Language(language)]
	return known
}

// Copy holds the strings the senders need, in one language.
//
// A field left out of an entry below is the empty string, which for a subject
// is a message a mail client files as blank. A keyed struct literal does not
// require every field, so the compiler will not say so — and neither would a
// map. What says so is
// TestTheMailCatalogSpeaksEveryLanguageTheContractAdmits, which walks every
// field of every language: that is what stops a language reaching a mailbox
// half-written.
type Copy struct {
	// The unsubscribe footer under an outgoing message. A German mail with
	// an English footer is the product speaking two languages in one
	// message, and the footer is the half a recipient reads when they want
	// it to stop.
	UnsubscribeLabel       string
	ManagePreferencesLabel string

	// The password reset a person asked for.
	ResetSubject string
	ResetIntro   string
	ResetAction  string
	ResetIgnore  string

	// The invitation an administrator sent on somebody's behalf.
	InviteSubject string
	InviteIntro   string
	InviteAction  string
	InviteIgnore  string

	// The Monday retrospective. WeeklySubject and WeeklyHeading both name the
	// week, because the subject is what tells two of these apart in a list and
	// the heading is what a reader sees once the message is open.
	WeeklySubject  string
	WeeklyHeading  string
	WeeklyPromised string
	// WeeklyOfDue is the panel's own "{done} of {due}" template, with the two
	// counts as %d in the order the language puts them.
	WeeklyOfDue     string
	WeeklyDealsWon  string
	WeeklyDealsLost string
	WeeklyMoved     string
	WeeklyDecided   string
	WeeklyYes       string
	WeeklyNo        string
	WeeklyQueue     string
	WeeklyActed     string
	WeeklyDismissed string
	WeeklyCarried   string
	WeeklyWhatMoved string
	WeeklyAndMore   string
	WeeklyFullWeek  string
	// The question the message closes on, in the panel's own words. The weekly
	// mail reported the week and asked nothing, so it read as a receipt for
	// work already done — while the screen it links to opens with a panel
	// inviting the reader to plan the next one.
	WeeklyPlanAhead string
	// The three outcomes a deal line reports, lower-case because they are read
	// inside a sentence rather than as a label.
	WeeklyOutcomeWon   string
	WeeklyOutcomeLost  string
	WeeklyOutcomeMoved string

	// The morning brief. Shorter than the weekly on purpose: it arrives every
	// working day, so it names the top of the queue and links to the rest
	// rather than restating a day a reader is about to open anyway.
	MorningSubject string
	MorningHeading string
	// MorningTop heads the ranked lines. MorningAndMore is the "%d more"
	// tail when the queue runs past the cap.
	MorningTop     string
	MorningAndMore string
	// MorningQuiet is the whole body when the queue is empty and the rep asked
	// to hear about quiet days anyway. Nothing else is sent in that case: a
	// heading over an empty list reads as a message that failed to render.
	MorningQuiet   string
	MorningOpenDay string
}

// OneLine collapses any run of line breaks and other control separators into
// single spaces, so a stored string cannot forge structure in a message.
//
// EVERY rendered string goes through this, not only the ones that look
// dangerous. A mail body is a line-oriented format a human reads as
// authoritative, so any value reaching it that can hold a newline can write a
// line that looks like ours — a fake count, a fake heading, a "From:" that
// reads as a header. Deal labels, stage names and a model's own sentence are
// each typed or generated somewhere that does not reject a newline, and asking
// every caller to remember is how one of them comes not to.
//
// Here rather than in one sender, because there are two now: the weekly
// retrospective and the morning brief. A second copy of this is a second thing
// to remember to fix.
//
// mailer.Send refuses line breaks in the recipient and the subject, which are
// the header fields. The body is the sender's to keep honest.
func OneLine(text string) string {
	return strings.Join(strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\v' || r == '\f' ||
			r == '\u0085' || r == '\u2028' || r == '\u2029'
	}), " ")
}
