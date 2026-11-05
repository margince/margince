// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Reading a business card into the shared vocabulary, and applying it.
//
// Split from observedcontact.go, which holds the RULE a dated statement obeys.
// This holds what a CARD specifically states and how its properties map onto
// that vocabulary — the questions a signature never raises, like which of a
// card's URLs is a profile and which is a company site.

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// observedCard is one business card, as a dated statement by the contact on it.
type observedCard struct {
	Entry      VCardEntry
	Evidence   string
	SourceRef  string
	Source     string
	CapturedBy string
	// ObservedAt is when the card states it was last revised (its REV), or zero
	// where it states none this reader trusts — the import then dates it from
	// its own clock.
	ObservedAt time.Time
}

// applyObservedCard writes everything a card states, and reports what it did
// with each field so the caller can audit the moves and report them.
//
// The same writer the signature pass uses, deliberately: a card and a signature
// are one kind of input — the contact describing themselves — and two writers
// would let the ARRIVAL ROUTE decide whether a stale number gets corrected.
//
// Emails are the exception and stay additive. An address the card omits is not
// an address retired, and no automatic path may take away a way to reach
// somebody; the card's own addresses are added on the create path and left
// alone here.
//
// A field in corrected stands, as it does against a signature, and is not
// written at all.
func applyObservedCard(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, c observedCard, corrected map[string]bool) (cardStatement, error) {
	stated := cardFields(c.Entry)
	st := cardStatement{corrected: corrected}
	for _, f := range stated {
		if corrected[f.name] {
			st.fields = append(st.fields, statedField{cardField: f, outcome: observedSkipped})
			continue
		}
		outcome, err := applyObservedField(ctx, tx, contactID, observedField{
			Field: f.name, Value: f.value, Evidence: c.Evidence, SourceRef: c.SourceRef,
			Source: c.Source, CapturedBy: c.CapturedBy, ObservedAt: c.ObservedAt,
		})
		if err != nil {
			return cardStatement{}, err
		}
		st.fields = append(st.fields, statedField{cardField: f, outcome: outcome})
	}
	if corrected[fieldPhone] {
		return st, nil
	}
	// Every number the card lists, as one statement: each gets its own
	// evidence line, and a number replaces only the older one of its own
	// country and type.
	numbers := make([]observedNumber, 0, len(c.Entry.Phones))
	for _, phone := range c.Entry.Phones {
		numbers = append(numbers, observedNumber{Phone: phone.Value, PhoneType: phone.Kind, Evidence: c.Evidence})
	}
	var err error
	st.numbers, err = applyObservedNumberList(ctx, tx, contactID, observedNumbers{
		Numbers: numbers, SourceRef: c.SourceRef, Source: c.Source,
		CapturedBy: c.CapturedBy, ObservedAt: c.ObservedAt,
	})
	if err != nil {
		return cardStatement{}, err
	}
	return st, nil
}

// cardStatement is what applying one card did, field by field.
type cardStatement struct {
	fields []statedField
	// numbers holds each parsed number with its outcome; nil when the card
	// lists none this reader can parse, or when a human corrected the phone.
	numbers   []listedNumber
	corrected map[string]bool
}

// statedField is one single-answer field the card stated, and its outcome.
type statedField struct {
	cardField
	outcome observedOutcome
}

// moved names the fields whose record now carries the card's value, for the
// audit image.
func (st cardStatement) moved() []string {
	var out []string
	for _, f := range st.fields {
		if f.outcome.landed() {
			out = append(out, f.name)
		}
	}
	for _, n := range st.numbers {
		if n.outcome == observedApplied || n.outcome == observedReplaced {
			return append(out, fieldPhone)
		}
	}
	return out
}

// cardField is one of the card's single-answer fields, named in the shared
// vocabulary.
type cardField struct{ name, value string }

// cardFields reads a card into that vocabulary, dropping what it does not
// state.
//
// TITLE fills `title` and NOT `role`. They are different claims — a title is
// what somebody is called, a role is what they do in a deal — and contact360
// attaches role evidence to the employment role, so a card saying "VP Finance"
// would make an unrelated role like "Decision maker" display with that as its
// evidence.
//
// URL is split rather than filed as linkedin, which is what the old import did
// to every card: a company website landed under a contact's LinkedIn profile and
// the page then showed it as one. Host, not guesswork.
func cardFields(entry VCardEntry) []cardField {
	out := make([]cardField, 0, 5)
	add := func(name, value string) {
		if v := strings.TrimSpace(value); v != "" {
			out = append(out, cardField{name: name, value: v})
		}
	}
	add(fieldTitle, entry.Title)
	add(fieldCompanyName, entry.Company)
	add(fieldAddress, entry.Address)
	if url, ok := cardURLField(entry); ok {
		out = append(out, url)
	}
	return out
}

// cardURLField files the card's URL as a LinkedIn profile or a website.
func cardURLField(entry VCardEntry) (cardField, bool) {
	u := strings.TrimSpace(entry.URL)
	switch {
	case u == "":
		return cardField{}, false
	case isLinkedinURL(u):
		return cardField{name: fieldLinkedin, value: u}, true
	default:
		return cardField{name: fieldWebsite, value: u}, true
	}
}

// isLinkedinURL reports whether a URL names a LinkedIn profile, by host rather
// than by substring: a website whose path merely mentions linkedin is not one.
//
// The host comes from the NORMALIZER rather than from a parse of its own, so
// this classifier and the value that would be stored agree about what the host
// is. Deciding on the raw string admitted `javascript://linkedin.com/x` as a
// profile and left the scheme for the storing writer to refuse two calls
// later — a value classified LinkedIn and then not stored as one, which the
// vCard import and the card reader both act on before that second call.
// Hostname() rather than Host, so an explicit port does not turn a profile
// into a website.
func isLinkedinURL(raw string) bool {
	normalized, err := NormalizeLinkedInURL(raw)
	if err != nil {
		return false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return false
	}
	return onHost(strings.ToLower(parsed.Hostname()), LinkedInSlotHosts())
}
