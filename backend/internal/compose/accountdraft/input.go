// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

// What one draft is written from, in A132's grounding order: the caller's
// intent, then the recipient and how we stand with them, then the deal and
// what we last committed to, then the recent conversation, then the dossier.
//
// The order is the prompt's reading order, not a preference: an instruction
// the caller typed outranks a record, a named recipient outranks the account
// in general, and what we PROMISED outranks what we merely discussed.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Input is the account, narrowed to the one recipient and one deal this draft
// is about. It is a projection of the caller's own 360 — nothing here
// re-queries, so anything absent is absent because that caller may not see it.
type Input struct {
	// Intent is the caller's own steering ("shorter", "ask for Tuesday"). The
	// one field they typed, and the one field not fenced.
	Intent string `json:"intent,omitempty"`

	// RewriteOf is the contact surface's field, for the same reason.
	RewriteOf string `json:"rewrite_of,omitempty"`

	// Envelope is the correspondence this draft is written into: its language,
	// how long it has been silent, the current time and who is signing it.
	// Server-derived, never read out of the counterparty's own text.
	Envelope draftfloor.Envelope `json:"envelope"`

	// CompanyID never reaches the model; it names the account for Grounding.
	CompanyID string `json:"-"`
	Company   string `json:"company"`
	Industry  string `json:"industry,omitempty"`
	// Description is the one line a contact wrote about what this company does
	// (core 0203). Short, human, and the fastest way for a draft to sound like
	// it knows who it is writing to.
	Description string `json:"description,omitempty"`

	Recipient RecipientIn `json:"recipient"`
	// Deal is the opportunity the message is about, when the caller named one.
	Deal *DealIn `json:"deal,omitempty"`
	// Project is the body of work the message is about, when the caller named
	// one. The view it is folded from is already scoped to it, so Recent and
	// Commitment below describe this project's correspondence, not another's.
	Project *ProjectIn `json:"project,omitempty"`
	// Commitment is the soonest thing one side said they would do. It outranks
	// the conversation below it: a promise is a reason to write, where a
	// message is only context for one.
	Commitment *TaskIn `json:"commitment,omitempty"`
	Recent     []ActIn `json:"recent,omitempty"`
	// Dossier is what the company IS, from its own recorded facts — as opposed
	// to everything above, which is how it stands with us.
	Dossier []string `json:"dossier,omitempty"`
}

// RecipientIn is who the draft is addressed to and how we stand with them.
type RecipientIn struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// FirstName is what a familiar greeting uses. Split here rather than in
	// the prompt: a model asked to shorten a name will shorten "Dr. Anne-Marie
	// Weiß-Konrad" differently on every call.
	FirstName string `json:"first_name"`
	// LastName is what a FORMAL greeting uses. The two registers take
	// different names, and a formal opening built from a first name is wrong
	// in every language that has the distinction — so the prompt is given both
	// and told which is which rather than left to guess one from the other.
	LastName string `json:"last_name,omitempty"`
	Title    string `json:"title,omitempty"`
	Email    string `json:"email,omitempty"`
	// Bucket is the relationship's own reading (strong/moderate/weak/none),
	// which tells the writer how familiar to be. Never a score: a number would
	// invite the prose to quote it.
	Bucket string `json:"relationship,omitempty"`
	// LastInteraction is RFC3339 UTC, empty when we have never exchanged a
	// message with this contact. Empty is the honest state and reads as "first
	// contact", not as "long ago".
	LastInteraction string `json:"last_interaction,omitempty"`
}

// DealIn is the opportunity the message is about.
type DealIn struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stage string `json:"stage,omitempty"`
	// AmountMinor never reaches the model: MarshalJSON renders `amount` from
	// it, in the currency's own scale. It carried minor units under a key that
	// said `amount_minor` to this file and nothing at all to the model, so a
	// 180,000 EUR deal read as eighteen million — the same defect the account
	// brief had (#591), and the more consequential half of it, because this
	// prompt writes an outbound message to the customer.
	AmountMinor int64  `json:"-"`
	Currency    string `json:"currency,omitempty"`
}

// MarshalJSON writes the amount as the figure a contact would say — "180000.00"
// for 18000000 EUR, "18000000" for the same integer in JPY — rather than the
// minor-unit integer the column holds.
//
// Derived at the moment of writing rather than stored beside the integer, so
// the two can never disagree, and scaled by the ISO-4217 table rather than by
// /100, which understates every zero-decimal currency a hundredfold. An amount
// with no currency is not shown at all: a figure without its code is a number
// whose scale the reader has to guess, which is the defect rather than a lesser
// form of it. A zero amount IS shown — a deal deliberately priced at nothing is
// not the same as one nobody has priced.
func (d DealIn) MarshalJSON() ([]byte, error) {
	type wire DealIn // no methods, so no recursion back into this one
	amount := ""
	if d.Currency != "" {
		amount = values.MajorUnits(d.AmountMinor, d.Currency)
	}
	return json.Marshal(struct {
		wire
		Amount string `json:"amount,omitempty"`
	}{wire: wire(d), Amount: amount})
}

// ProjectIn is the body of work the message is about. Its id never reaches the
// model: a project is not a record a reason may cite, and an id the grounding
// filter refuses only invites a citation it drops.
type ProjectIn struct {
	ID   string `json:"-"`
	Name string `json:"name"`
	// Key is the handle a human writes in a subject line, when the project
	// has one.
	Key   string `json:"key,omitempty"`
	Phase string `json:"phase"`
	// TargetEnd is the date the work is meant to finish, YYYY-MM-DD, empty
	// when nobody set one.
	TargetEnd string `json:"target_end,omitempty"`
	// OpenCommitments counts the open tasks in this project's scope — the
	// same rows Commitment's head is taken from — so the draft can say "two
	// things are still open" without naming ones it was not shown.
	OpenCommitments int `json:"open_commitments"`
}

// TaskIn is one open commitment.
type TaskIn struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Due  string `json:"due,omitempty"`
}

// ActIn is one recent exchange.
type ActIn struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Subject string `json:"subject,omitempty"`
	At      string `json:"at"`
	// Inbound says who spoke. A draft that answers what THEY said reads very
	// differently from one that follows up on what we said, and the direction
	// is the only thing that tells them apart.
	Inbound bool `json:"inbound"`
	// Snippet is the opening of what this message says.
	//
	// A subject line says a message happened; the words say what it was about.
	// Without them a draft can only gesture at a conversation it never read, and
	// on a thread whose every subject is the same string — a support thread, an
	// onboarding sequence, anything answered in place — subjects alone carry no
	// information at all, so the model fills the gap with plausible invention.
	//
	// It is what the message SAYS, not a claim about who wrote it. An activity
	// reaches an account through activity_link and the employment walk, both of
	// which record what a message concerns rather than who authored it, and the
	// 360 carries no participants — so authorship is not knowable here.
	Snippet string `json:"snippet,omitempty"`
}

// ActivityKind lets draftcore.ThreadMail tell an email from a task or a call.
func (a ActIn) ActivityKind() string { return a.Kind }

// ActivitySubject is the subject draftcore.ThreadMail answers.
func (a ActIn) ActivitySubject() string { return a.Subject }

// draftInputActivities bounds how much of the conversation the draft reads.
// A follow-up is about the last exchange, not the relationship's history; a
// longer window costs prefill and buys older news.
const draftInputActivities = 6

// draftInputSnippetRunes bounds how much of each message the draft reads.
//
// Enough for the opening of a real business email — a greeting, the reason for
// writing, and the ask. Past that an email is detail, which a new message asks
// about rather than repeats, and every rune of it is prompt cost on every draft.
const draftInputSnippetRunes = 400

// String is the debug rendering, never the prompt payload. The prompt sends
// JSON so the model reads a structure rather than prose it might imitate.
func (in Input) String() string {
	return fmt.Sprintf("accountdraft{company:%q to:%q deal:%v}",
		in.Company, in.Recipient.Name, in.Deal != nil)
}

// Threaded is always false here, and that is what this surface IS: it opens a
// new conversation, so no subject it writes can be a reply to anything. The
// method exists so the shared check reads the same shape from every surface.
func (Input) Threaded() bool { return false }

// ThreadSubject is false: this surface opens a conversation.
func (Input) ThreadSubject() (string, bool) { return "", false }

// Booked is false: an account draft folds no meeting, so nothing here can
// support a day. A surface that gains one answers from it rather than here.
func (Input) Booked() bool { return false }
