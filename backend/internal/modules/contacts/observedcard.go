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

// applyObservedCard writes everything a card states, and reports which fields
// moved so the caller can audit them.
//
// The same writer the signature pass uses, deliberately: a card and a signature
// are one kind of input — the contact describing themselves — and two writers
// would let the ARRIVAL ROUTE decide whether a stale number gets corrected.
//
// Emails are the exception and stay additive. An address the card omits is not
// an address retired, and no automatic path may take away a way to reach
// somebody; the card's own addresses are added on the create path and left
// alone here.
func applyObservedCard(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, c observedCard) ([]string, error) {
	var applied []string
	for _, f := range cardFields(c.Entry) {
		outcome, err := applyObservedField(ctx, tx, contactID, observedField{
			Field: f.name, Value: f.value, Evidence: c.Evidence, SourceRef: c.SourceRef,
			Source: c.Source, CapturedBy: c.CapturedBy, ObservedAt: c.ObservedAt,
		})
		if err != nil {
			return nil, err
		}
		if outcome == observedApplied || outcome == observedConfirmed {
			applied = append(applied, f.name)
		}
	}
	// Every number the card lists, as one statement: each gets its own
	// evidence line, and a number replaces only the older one of its own
	// country and type.
	numbers := make([]observedNumber, 0, len(c.Entry.Phones))
	for _, phone := range c.Entry.Phones {
		numbers = append(numbers, observedNumber{Phone: phone.Value, PhoneType: phone.Kind, Evidence: c.Evidence})
	}
	landed, err := applyObservedNumbers(ctx, tx, contactID, observedNumbers{
		Numbers: numbers, SourceRef: c.SourceRef, Source: c.Source,
		CapturedBy: c.CapturedBy, ObservedAt: c.ObservedAt,
	})
	if err != nil {
		return nil, err
	}
	if len(landed) > 0 {
		applied = append(applied, fieldPhone)
	}
	return applied, nil
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
	if u := strings.TrimSpace(entry.URL); u != "" {
		if isLinkedinURL(u) {
			add(fieldLinkedin, u)
		} else {
			add(fieldWebsite, u)
		}
	}
	return out
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
