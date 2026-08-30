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
	// The three outcomes a deal line reports, lower-case because they are read
	// inside a sentence rather than as a label.
	WeeklyOutcomeWon   string
	WeeklyOutcomeLost  string
	WeeklyOutcomeMoved string
}
