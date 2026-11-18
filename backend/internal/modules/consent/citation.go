// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which law a finding or refusal rests on, for the recipient it is about.
//
// A citation is shown to a rep beside the engine's own sentence and never
// changes a verdict. One code can rest on different law in two markets, so the
// lookup asks the recipient's own pack. That pack may narrow a citation to a
// sender established in its country, or outside it.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// citationOutcome says whether a citation is shown, and why not when it is not.
type citationOutcome string

const (
	// citationShown is a citation the rep may be shown.
	citationShown citationOutcome = "shown"
	// citationNotDeclared is no pack for the recipient, or none citing this
	// code for this sender.
	citationNotDeclared citationOutcome = "not_declared"
	// citationNotYetReviewed is a pack whose review is dated after the lookup.
	// A review that has not happened yet approves nothing.
	citationNotYetReviewed citationOutcome = "not_yet_reviewed"
	// citationReviewOverdue is a pack whose review fell due. The pack still
	// applies; only its citations are withheld.
	citationReviewOverdue citationOutcome = "review_overdue"
	// citationLawExpired is an instrument whose ExpiresFrom has passed.
	citationLawExpired citationOutcome = "law_expired"
	// citationNotInForce is an instrument whose EffectiveFrom is still ahead.
	citationNotInForce citationOutcome = "not_in_force"
)

// citationContext is what a citation is resolved against: the recipient the
// finding is about, the country of the legal entity sending, and the instant.
type citationContext struct {
	Recipient RecipientCountry
	Sender    jurisdiction.Code
	At        time.Time
}

// citationFor returns the citation of the recipient's pack for code, with its
// provision and source filled from the instrument, or the reason there is none.
// An unreviewed pack has none to return: Register refuses one that cites.
func citationFor(code string, where citationContext) (messagingrules.Citation, citationOutcome) {
	rules, _, found := applicableRules(where.Recipient.Code)
	if !found {
		return messagingrules.Citation{}, citationNotDeclared
	}
	for _, declared := range rules.Citations {
		if declared.Code != code || !senderTerritoryHolds(declared.Territory, rules.Jurisdiction, where.Sender) {
			continue
		}
		if where.At.Before(rules.Provenance.ReviewedAt) {
			return messagingrules.Citation{}, citationNotYetReviewed
		}
		if due := rules.Provenance.ReviewDue; !due.IsZero() && deadline.Passed(&due, where.At) {
			return messagingrules.Citation{}, citationReviewOverdue
		}
		// Register validated the pack, so this fails only for a citation that
		// names no single instrument, and such a citation names no law to show.
		resolved, err := rules.ResolveCitation(declared)
		if err != nil {
			return messagingrules.Citation{}, citationNotDeclared
		}
		for _, instrument := range rules.Instruments {
			if instrument.Name != resolved.Instrument {
				continue
			}
			if !instrument.ExpiresFrom.IsZero() && !where.At.Before(instrument.ExpiresFrom) {
				return messagingrules.Citation{}, citationLawExpired
			}
			if where.At.Before(instrument.EffectiveFrom) {
				return messagingrules.Citation{}, citationNotInForce
			}
		}
		return resolved, citationShown
	}
	return messagingrules.Citation{}, citationNotDeclared
}

// citationsFor resolves every code the recipient's pack cites, for the entity
// the message goes out for, and keeps the ones shown. A pack that cites
// nothing returns before the sender is read.
func (g *Gate) citationsFor(ctx context.Context, tx pgx.Tx, country RecipientCountry, entity ids.UUID) (map[string]commsauthz.Provision, error) {
	codes := citedCodes(country)
	if len(codes) == 0 {
		return nil, nil //nolint:nilnil // no citation is the answer for a pack that cites nothing
	}
	sender, err := g.store.senderCountryOf(ctx, tx, entity)
	if err != nil {
		return nil, err
	}
	return provisionsShown(codes, citationContext{Recipient: country, Sender: sender, At: g.store.now()}), nil
}

// citedCodes are the codes the recipient's own pack declares a citation for.
func citedCodes(country RecipientCountry) []string {
	rules, _, found := applicableRules(country.Code)
	if !found {
		return nil
	}
	codes := make([]string, 0, len(rules.Citations))
	for _, declared := range rules.Citations {
		codes = append(codes, declared.Code)
	}
	return codes
}

// provisionsShown keeps each code whose citation is shown at where. An
// overdue, unreviewed or expired one is left out, and the surface then shows
// its facts without a citation.
func provisionsShown(codes []string, where citationContext) map[string]commsauthz.Provision {
	out := make(map[string]commsauthz.Provision, len(codes))
	for _, code := range codes {
		if shown := shownProvision(code, where); shown.Cited() {
			out[code] = shown
		}
	}
	return out
}

// shownProvision is citationFor as a surface shows it: the provision when the
// citation is shown, and the zero value for every other outcome.
func shownProvision(code string, where citationContext) commsauthz.Provision {
	cited, outcome := citationFor(code, where)
	if outcome != citationShown {
		return commsauthz.Provision{}
	}
	return commsauthz.Provision{Instrument: cited.Instrument, Provision: cited.Provision, SourceURL: cited.SourceURL}
}

// senderTerritoryHolds reports whether a citation's territory covers the
// sender. An unknown sender is elsewhere, the same test the foreign-advertiser
// finding applies, so a citation of that finding is shown wherever it fires.
func senderTerritoryHolds(territory messagingrules.Territory, pack, sender jurisdiction.Code) bool {
	switch territory {
	case messagingrules.AnySender:
		return true
	case messagingrules.SenderHere:
		return sender == pack
	case messagingrules.SenderElsewhere:
		return sender != pack
	}
	return false
}
