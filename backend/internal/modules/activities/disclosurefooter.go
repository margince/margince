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

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// DisclosureLine is one obligation and the words that meet it. Mirrors
// consent.DisclosureLine across the module boundary: activities may not import
// consent, and the composition root injects the resolver.
//
// Held by: TestTheDisclosureLineAgreesAcrossTheSeam
// (backend/gates/disclosureseam_test.go)
type DisclosureLine struct {
	MissingLanguage string
	MissingMeans    []string
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
func unmeetable(lines []DisclosureLine) []commsauthz.FooterFinding {
	var out []commsauthz.FooterFinding
	for _, line := range lines {
		if line.Missing() {
			out = append(out, commsauthz.FooterFinding{Kind: line.Kind, Language: line.MissingLanguage, MissingMeans: line.MissingMeans})
		}
	}
	return out
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
	// DisclosuresFor answers one line per obligation the category carries for
	// these recipients, whose countries pick the marketing obligations. The
	// sending legal entity picks the data-protection ones and names the
	// controller; zero is the one the sender works for now. language is the
	// message's, which picks the stated translation of each particular. An
	// empty answer means nothing is owed here. countries are the ones the
	// sender stated for this message, which come before the record's.
	DisclosuresFor(ctx context.Context, category string, recipients []string, countries []commsauthz.CountryOverride, entity ids.UUID, language string) ([]DisclosureLine, error)
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

// appendDisclosures omits unavailable text; the assessment records those gaps.
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
func (s *Store) disclosuresFor(ctx context.Context, category string, recipients []string, countries []commsauthz.CountryOverride, entity ids.UUID, language string) ([]DisclosureLine, error) {
	if s.disclosures == nil {
		return nil, nil
	}
	return s.disclosures.DisclosuresFor(ctx, category, recipients, countries, entity, language)
}
