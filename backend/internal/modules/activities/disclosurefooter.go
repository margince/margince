// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Putting the disclosures a jurisdiction demands into the message that owes
// them.
//
// The packs have declared them since they shipped, consent can now turn each
// one into the words that meet it, and nothing put those words in a body —
// gates/messagingruleapplied_test.go's register says exactly that, and names
// this file's own caller as the step that was missing.
//
// It is a SEPARATE footer from the unsubscribe one beside it, and runs before
// that one's category gate. An unsubscribe surface belongs to advertising; a
// controller identity belongs to a first contact whatever it is about, so a
// disclosure that rode the unsubscribe path would appear on marketing alone
// and be absent from the correspondence Art. 13 actually covers.

import (
	"context"
	"strings"
)

// DisclosureLine is one obligation and the words that meet it. Mirrors
// consent.DisclosureLine across the module boundary: activities may not import
// consent, and the composition root injects the resolver.
//
// Held by: TestTheDisclosureLineAgreesAcrossTheSeam
// (backend/gates/disclosureseam_test.go)
type DisclosureLine struct {
	// Kind is the obligation a pack declared.
	Kind string
	// Text is what the message must carry, empty when the installation has not
	// stated the particular this obligation needs.
	Text string
}

// Missing reports that this obligation applies and the installation cannot
// meet it.
//
// A SEPARATE ANSWER from "no obligation applies", and the resolver's own type
// carries the same distinction. Folding the two together is what lets a send
// that owes a controller identity nobody configured look identical to a send in
// a jurisdiction that demands nothing — the first is a gap somebody has to
// close, the second is fine.
func (d DisclosureLine) Missing() bool { return strings.TrimSpace(d.Text) == "" }

// unmeetable answers which obligations apply and cannot be met.
func unmeetable(lines []DisclosureLine) []string {
	var out []string
	for _, line := range lines {
		if line.Missing() {
			out = append(out, line.Kind)
		}
	}
	return out
}

// UndisclosableError refuses a send whose jurisdiction demands a line the
// installation has never stated.
//
// THE SEND FAILS rather than shipping short, and that is the whole point of
// separating "missing" from "not owed". Leaving the line out of the body is
// correct — a message reading "unknown" where its controller's name belongs
// discloses nothing — but leaving it out AND sending anyway hands a recipient a
// message that is missing something the law required, with nobody told. The
// operator can answer this in seconds by stating the particular; a regulator
// reading the sent message cannot.
//
// It maps to 422 with the fix in the message, like the shared-token refusal
// above it: the caller states the particulars and re-issues the send.
type UndisclosableError struct{ Kinds []string }

func (e *UndisclosableError) Error() string {
	return "this message owes a disclosure the workspace has not stated (" +
		strings.Join(e.Kinds, ", ") +
		") — state the controller particulars in privacy settings, then send again"
}

// FieldFault names the settings the operator has to fill in.
func (e *UndisclosableError) FieldFault() (field, code, message string) {
	return fieldBody, "undisclosable_obligation", e.Error()
}

// DisclosureResolver answers what a message of this category must disclose.
//
// Declared here by the consumer and implemented by consent, injected in
// compose/ like every other cross-module edge: activities composes the body and
// must not know what a jurisdiction pack is.
//
// A nil resolver means no disclosures, which is what every send did before this
// existed. That keeps the dozen test stores and the MCP tool surface working
// without teaching each one about compliance packs — and a marketing send still
// passes the consent gate, so a missing footer is a wiring gap rather than a
// permission bypass.
type DisclosureResolver interface {
	// DisclosuresFor answers one line per obligation the category carries.
	// An empty answer means nothing is owed here.
	DisclosuresFor(ctx context.Context, category string) ([]DisclosureLine, error)
}

// WithDisclosures wires the resolver onto the send path.
func (s *Store) WithDisclosures(r DisclosureResolver) *Store {
	clone := *s
	clone.disclosures = r
	return &clone
}

// WithDisclosures is the handler-level wiring, matching WithUnsubscribe beside
// it: the MCP tool surface enters the send path without passing a handler, so
// both doors exist.
func (h Handlers) WithDisclosures(r DisclosureResolver) Handlers {
	h.store = h.store.WithDisclosures(r)
	return h
}

// appendDisclosures puts the obligations this message owes at the bottom of it.
//
// It renders only what the installation can actually say. A line reported
// MISSING is left out of the body rather than printed as a gap: a message
// carrying the word "unknown" where its controller's name belongs discloses
// nothing and looks like a defect to the recipient.
//
// THE GAP IS NOT SWALLOWED HERE. Leaving it out of the body is a rendering
// decision and nothing else; deliverability asks unmeetable() the same question
// and refuses the send, because a message that quietly ships without a line its
// jurisdiction demands is the failure this whole file exists to close.
//
// Returns the body UNCHANGED when nothing is owed, so a send with no applicable
// pack is byte-identical to what it was before this existed.
func appendDisclosures(body string, lines []DisclosureLine) string {
	stated := make([]string, 0, len(lines))
	for _, line := range lines {
		if !line.Missing() {
			stated = append(stated, strings.TrimSpace(line.Text))
		}
	}
	if len(stated) == 0 {
		return body
	}
	return body + "\n\n--\n" + strings.Join(stated, "\n\n")
}

// htmlDisclosures renders the same obligations as markup.
//
// The plain-text and markup alternatives of one message must disclose the same
// things: which one a recipient reads is their mail client's decision, and a
// message whose HTML omitted its controller identity would be compliant only
// for readers whose client fell back to text. sendcore.go says the same about
// the signature and the unsubscribe footer beside this.
//
// Escaped, because these are words an operator typed into a settings form and
// they are about to become markup.
func htmlDisclosures(lines []DisclosureLine) string {
	stated := make([]string, 0, len(lines))
	for _, line := range lines {
		if !line.Missing() {
			stated = append(stated, "<p>"+htmlLines(strings.TrimSpace(line.Text))+"</p>")
		}
	}
	if len(stated) == 0 {
		return ""
	}
	return "<hr>\n" + strings.Join(stated, "\n")
}

// disclosuresFor asks the resolver, and answers nothing when none is wired.
//
// AN EMPTY CATEGORY IS STILL ASKED ABOUT, and that is deliberate. A caller that
// names only the deprecated consent key — an MCP tool, a stored scheduled send
// — carries no category, and skipping those would mean the oldest senders in
// the product quietly owe disclosures no message of theirs carries. The
// resolver owns the vocabulary and answers what an unrecognised category owes,
// which for the packs that exist is the obligations binding every first
// contact.
func (s *Store) disclosuresFor(ctx context.Context, category string) ([]DisclosureLine, error) {
	if s.disclosures == nil {
		return nil, nil
	}
	return s.disclosures.DisclosuresFor(ctx, category)
}
