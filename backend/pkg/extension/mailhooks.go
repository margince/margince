// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two mail-capture hooks a unit may register are part of the published
// extension surface.
//
//margince:extension-surface

package extension

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// MailView is what a mail hook may read about one captured mail or meeting.
// It carries no attachment. Its body is cut to the first characters, and its
// Lookup reads the unit's own data and nothing else.
type MailView struct {
	// Activity is the id of the mail.
	Activity string
	// Meeting is true for a calendar meeting and false for an email.
	Meeting bool
	// Subject is the mail's subject line.
	Subject string
	// Addresses are the participants' addresses, lower case, in order.
	Addresses []string
	// Body is the start of the text, at most 8000 characters.
	Body string
	// Lookup reads one row of the unit's own tables, for example the customer
	// a reference in the subject belongs to. It cannot write, and it ends after
	// 250 milliseconds. Pass the statement and its arguments as for Tx.QueryRow.
	Lookup func(sql string, args ...any) Row
}

// MailParties is the contacts and companies core derived for a mail, as ids.
// There are at most two companies. A hook that returns more is ignored.
type MailParties struct {
	Contacts  []string
	Companies []string
	// Reason is one of the unit's declared MailHooks.Reasons. A hook that
	// changes the result must name one, or core ignores the change.
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
	// Reason is one of the unit's declared MailHooks.Reasons. A hook that
	// changes the answer must name one, or core ignores the change.
	Reason string
}

// MailPartiesHook adjusts the contacts and companies core derived. It runs
// after core derived them and before the deal rule reads them.
type MailPartiesHook func(ctx context.Context, mail MailView, parties MailParties) (MailParties, error)

// MailDealHook adjusts the deal rule's answer. It runs at the end of the rule.
type MailDealHook func(ctx context.Context, mail MailView, deal MailDeal) (MailDeal, error)

// MailHooks are the behavior a unit registers in mail capture. Either may be nil.
//
// A hook writes nothing and reads only its MailView, and the same input gives
// the same answer. Its context ends when the hook's time limit passes, so a
// hook that waits should watch it. Hooks run in unit-name order, each seeing the
// previous result.
//
// A hook that fails, panics or runs over its limit decides nothing. Core keeps
// any link the mail already has and asks again later. It does not fall back to
// its own answer. A hook cannot change what a colleague decided. A filed mail
// never reaches it. A deal a colleague ruled out is dropped.
//
// Every change a hook makes is shown in Settings > Capture under one of its
// Reasons. A change that names no declared reason is ignored.
type MailHooks struct {
	Parties MailPartiesHook
	Deal    MailDealHook
	// Reasons are the keys the unit's answers may give, lower snake case. The
	// unit ships a label for each in its own translations, under
	// ext<Unit>.captureReason.<key>.
	Reasons []string
}

// MailReasonLimit is how many reasons one unit may declare.
const MailReasonLimit = 16

var mailReasonGrammar = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// Validate holds the declared reasons to the key grammar. A key is lower snake
// case of at most 40 characters and unique, and a unit with a hook declares
// at least one.
func (h MailHooks) Validate() error {
	if (h.Parties != nil || h.Deal != nil) && len(h.Reasons) == 0 {
		return errors.New("a mail hook needs at least one declared reason, or every change it makes is ignored")
	}
	if len(h.Reasons) > MailReasonLimit {
		return fmt.Errorf("a unit may declare at most %d mail hook reasons, not %d", MailReasonLimit, len(h.Reasons))
	}
	seen := map[string]bool{}
	for _, key := range h.Reasons {
		if !mailReasonGrammar.MatchString(key) {
			return fmt.Errorf("mail hook reason %q is not a key (lower snake case, at most 40 characters)", key)
		}
		if seen[key] {
			return fmt.Errorf("mail hook reason %q is declared twice", key)
		}
		seen[key] = true
	}
	return nil
}
