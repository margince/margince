// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailcopy is what this installation's outbound mail says, in the
// language the installation speaks.
//
// Every message the product sent was hard-coded English while the screens were
// translated three ways. For the two transactional messages that is a small
// thing — a reader reading a password link already knows what they asked for.
// The weekly retrospective is not: it arrives unasked every Monday, it is the
// product talking to a rep about their own week, and a German-speaking rep read
// their Home panel in German and then got an English summary of the same
// numbers.
//
// ONE catalog for every sender rather than a table beside each one. Three
// senders each inventing their own is how one product comes to have three
// voices, and it is the duplication the issue that raised this named.
//
// THE WEEKLY'S LABELS ARE THE SCREEN'S. "Tasks delivered" in the mail is the
// same phrase the weekly panel draws, so they are not translated twice: the
// strings here are the frontend catalog's, and a gate compares them. Two
// translations of one label is how the mail comes to say something the panel
// does not, in a message whose whole subject is numbers the reader can also see
// on screen.
//
// Held by: TestEveryMailLabelMatchesTheScreenThatShowsIt
// (backend/gates/mailcopy_test.go)
package mailcopy

import (
	"slices"
	"strings"
	"time"
)

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

// DateLayout is how a date is written in this installation's mail, in every
// language.
//
// ISO, and that is deliberate: `2 January 2006` puts an English month name in
// the middle of a German sentence — the half-translated message this catalog
// exists to stop — and a numeric order like 06/01 is read as 6 January by half
// the world. Go's layouts name months in English and nothing here translates
// one, so a formatted date is the one part of a mail a catalog cannot fix.
//
// It lives beside the copy rather than beside each sender. The weekly, the
// morning brief and the confirm link all write dates a recipient reads, and
// three constants would be three chances to decide this differently.
const DateLayout = time.DateOnly

// Languages is every language this build carries copy for, in a stable order.
//
// DERIVED from the catalog rather than restated. The set is already written
// three times — the constants above, the map catalog.go builds, and
// textlang.Shipped — and a fourth hand-written list would be the one that goes
// stale: a language added to the catalog but forgotten here would publish two
// wordings where three are needed and pin two hashes where three are, with
// nothing failing to say so.
//
// Sorted so a caller writing one row per language writes them the same way on
// every boot. Map iteration is random, and a bootstrap that inserted three rows
// in a different order each time would be harder to read in an audit log than
// it needs to be.
//
// Held by: TestTheMailCatalogSpeaksEveryLanguageTheContractAdmits
// (backend/gates/mailcopy_test.go), which fails when the contract admits a
// language the catalog has no copy for — so a set this returns short is a set
// that gate has already refused.
func Languages() []Language {
	out := make([]Language, 0, len(catalog))
	for language := range catalog {
		out = append(out, language)
	}
	slices.Sort(out)
	return out
}

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

	// The password reset a colleague asked for.
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
	WeeklySubject        string
	WeeklyHeading        string
	WeeklyTasksDelivered string
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

	// The two links the installation sends as ITSELF rather than on a rep's
	// behalf: the confirm-details link and the double-opt-in link.
	//
	// These are the hardest copy in the catalog to get wrong safely. Both go to
	// somebody who did not ask for them and may not remember the company, so a
	// bare "confirm your details" reads exactly like a phishing mail; and both
	// are EVIDENCE — the consent proof records which version a contact was
	// shown, so what these say is what an installation will one day have to
	// stand behind. A translation that softens "we will not write to you about
	// it" into a pleasantry changes what was promised, not just how it reads.
	ConfirmRecordSubject  string
	ConfirmRecordBody     string
	ConfirmConsentSubject string
	ConfirmConsentBody    string
	// The PRIVACY NOTICE, which asks for nothing.
	//
	// It is a separate message from the record confirmation beside it because
	// the two do different jobs and only one of them is owed. Art. 14 requires
	// telling somebody we hold their data; it requires no answer from them. The
	// record confirmation discharges that duty and ALSO asks whether they want
	// to hear from us — a marketing question riding a legal obligation, which
	// is the arrangement a supervisory authority reads as consent obtained
	// under pressure.
	//
	// It also reaches contacts the other one cannot. A contact who asked us to
	// stop is still owed their disclosure, and only the privacy-notice category
	// survives that stop (consent/authorizesuppression.go's
	// survivesARestriction). Before this template existed the duty was owed and
	// undeliverable.
	NoticeSubject string
	NoticeBody    string
	// ConfirmMarketingAsk is the QUESTION ON THE PAGE, not in the mail — the
	// sentence beside the yes/no a subject actually answers.
	//
	// It lives in this catalog rather than in the frontend's because it is the
	// proposition a consent is given to, and a proof row that quoted a string
	// the client sent would be evidence the client wrote. Published through the
	// same text-version machinery as the mail wording, so the grant can name
	// the row the controller published.
	//
	// ConfirmMarketingYes and ConfirmMarketingNo are the two answers, here for
	// the same reason: what a subject chose is part of what they were asked.
	ConfirmMarketingAsk string
	ConfirmMarketingYes string
	ConfirmMarketingNo  string
	// ConfirmSubscriptionAsk is the OTHER question, and the two are not
	// interchangeable. A record-confirmation link asks the generic marketing
	// question above; a dedicated subscription link names the purpose it was
	// minted for — "confirm that you want to receive {purpose}" — which is a
	// narrower and more specific proposition.
	//
	// Binding either door to the other's sentence would record somebody
	// agreeing to something they were not asked, which is the defect the whole
	// published-question change exists to end.
	//
	// {purpose} is substituted with the purpose's own label at render time.
	ConfirmSubscriptionAsk     string
	ConfirmSubscriptionConfirm string
	// ConfirmPersonal says the link is the reader's alone. ConfirmExpiry is
	// APPENDED to it when the link has a date, with the date as %s.
	//
	// Two strings rather than one sentence with a substitution: the shipped
	// English replaced a phrase inside its own body text, which only works
	// while every language spells that sentence the same way.
	ConfirmPersonal string
	ConfirmExpiry   string
	// The closing line, different for the two messages: a record confirmation
	// asks nothing of its reader, while an unanswered opt-in withholds the
	// permission until they answer.
	ConfirmRecordIgnore  string
	ConfirmConsentIgnore string
	// The privacy notice's closing line. It asks for nothing, so it says so:
	// a reader who does nothing has lost nothing, which is what makes this a
	// notice rather than a request.
	NoticeIgnore string

	// The opt-out acknowledgement, which Decree 91/2020/ND-CP Art. 16 owes a
	// Vietnamese recipient who refuses further advertising: a confirmation
	// that their refusal was received, within twenty-four hours.
	//
	// IT CARRIES NO LINK and no advertising of its own. This is the one message
	// the product sends to somebody who has just told it to stop, so anything
	// in it beyond "we heard you" would be the thing they asked not to receive
	// — and a link asking them to do something more would read as a message
	// that did not take the first answer.
	OptOutAckSubject string
	OptOutAckBody    string
	OptOutAckIgnore  string

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
