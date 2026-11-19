// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two mail-capture hooks a unit may register are part of the published
// extension surface.
//
//margince:extension-surface

package extension

// MailView is what a mail hook may read about one captured mail or meeting.
// It carries no body and no attachment, and it gives the hook no way to read
// or write anything else.
type MailView struct {
	// Activity is the id of the mail.
	Activity string
	// Meeting is true for a calendar meeting and false for an email.
	Meeting bool
	// Subject is the mail's subject line.
	Subject string
	// Addresses are the participants' addresses, lower case, in order.
	Addresses []string
}

// MailParties is the contacts and companies core derived for a mail, as ids.
// There are at most two companies. A hook that returns more is ignored.
type MailParties struct {
	Contacts  []string
	Companies []string
	// Reason is one sentence saying why the hook changed the result. Core
	// records it. A hook that changes nothing leaves it empty.
	Reason string
}

// MailDeal is the deal rule's answer for a mail, as a hook sees it.
type MailDeal struct {
	// Outcome is "linked", "ambiguous" or "none".
	Outcome string
	// Deal is the deal id. It is set only when Outcome is "linked".
	Deal string
	// Candidates are the deals a question weighs. A hook may remove one from an
	// "ambiguous" answer and may not add one.
	Candidates []string
	// Reason is one sentence saying why the hook changed the answer. Core
	// records it. A hook that changes nothing leaves it empty.
	Reason string
}

// MailPartiesHook adjusts the contacts and companies core derived. It runs
// after core derived them and before the deal rule reads them.
type MailPartiesHook func(mail MailView, parties MailParties) (MailParties, error)

// MailDealHook adjusts the deal rule's answer. It runs at the end of the rule.
type MailDealHook func(mail MailView, deal MailDeal) (MailDeal, error)

// MailHooks are the behavior a unit registers in mail capture. Either may be nil.
//
// A hook is a plain function with no context and no Runtime. It reads and
// writes nothing, and the same input gives the same answer.
// Hooks run in unit-name order, each seeing the previous result. A failed or
// slow hook leaves it unchanged. A hook cannot change what a person decided.
// A filed mail never reaches it. A deal a person ruled out is dropped.
type MailHooks struct {
	Parties MailPartiesHook
	Deal    MailDealHook
}
