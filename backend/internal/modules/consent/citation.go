// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which law a finding or refusal rests on, for the recipient it is about.
//
// A citation is shown to a rep beside the engine's own sentence and never
// changes a verdict. One code can rest on different law in two markets, so the
// lookup asks the recipient's own pack first. That pack may narrow a citation
// to a sender established in its country, or outside it. For a finding about
// the lawful basis, the sending entity's own pack is asked too: its
// data-protection law binds the sender wherever the recipient is.

import (
	"context"
	"slices"
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
	// citationReviewOverdue is a reviewed pack whose next review fell due. The
	// pack still applies; only its citations are withheld. A draft has no
	// review to fall due, and cites.
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
func citationFor(code string, where citationContext) (messagingrules.Citation, citationOutcome) {
	return packCitation(where.Recipient.Code, code, where)
}

// packCitation is the citation one pack declares for code, for this sender.
func packCitation(pack jurisdiction.Code, code string, where citationContext) (messagingrules.Citation, citationOutcome) {
	rules, _, found := applicableRules(pack)
	if !found {
		return messagingrules.Citation{}, citationNotDeclared
	}
	for _, declared := range rules.Citations {
		if declared.Code != code || !senderTerritoryHolds(declared.Territory, rules.Jurisdiction, where.Sender) {
			continue
		}
		if reviewOverdue(rules.Provenance, where.At) {
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

// senderCitesFinding reports whether the sending entity's own pack is asked
// for code: a finding about the lawful basis or the controller, which its
// data-protection law settles. Its marketing rules never follow a message
// abroad, so a marketing finding cites the recipient's pack alone.
func senderCitesFinding(code string) bool {
	switch code {
	case commsauthz.ReasonNoEvidence, commsauthz.ReasonDealNotRequested, commsauthz.ReasonLawfulBasisUnconfirmed,
		commsauthz.ReasonRestricted, commsauthz.ReasonConsentWithdrawn:
		return true
	}
	return dataProtectionDisclosure(messagingrules.DisclosureKind(code))
}

// reviewOverdue reports whether a reviewed pack's next review fell due by at.
// A draft carries no review stamp, so it has no due date to miss.
func reviewOverdue(p messagingrules.Provenance, at time.Time) bool {
	if p.ReviewedAt.IsZero() || p.ReviewDue.IsZero() {
		return false
	}
	return deadline.Passed(&p.ReviewDue, at)
}

// citationsFor resolves every code the packs binding this recipient cite, for
// the entity the message goes out for, and keeps the ones shown. Packs that
// cite nothing return before the sender is read.
func (g *Gate) citationsFor(ctx context.Context, tx pgx.Tx, country RecipientCountry, entity ids.UUID) (map[string][]commsauthz.Provision, error) {
	if len(citedCodes(country.dataProtectionCodes()...)) == 0 {
		return nil, nil //nolint:nilnil // no citation is the answer for packs that cite nothing
	}
	sender := country.Sender
	if sender == "" {
		var err error
		if sender, err = g.store.senderCountryOf(ctx, tx, entity); err != nil {
			return nil, err
		}
	}
	return provisionsShown(citationContext{Recipient: country, Sender: sender, At: g.store.now()}), nil
}

// citedCodes are the codes these packs declare a citation for.
func citedCodes(packs ...jurisdiction.Code) []string {
	var codes []string
	for _, pack := range packs {
		rules, _, found := applicableRules(pack)
		if !found {
			continue
		}
		for _, declared := range rules.Citations {
			if !slices.Contains(codes, declared.Code) {
				codes = append(codes, declared.Code)
			}
		}
	}
	return codes
}

// provisionsShown keeps, for each cited code, the citations shown at where:
// the recipient's pack's, then the sender's for a finding it settles. An
// overdue, expired or not yet binding one is left out, and the surface then
// shows its facts without it.
func provisionsShown(where citationContext) map[string][]commsauthz.Provision {
	packs := []jurisdiction.Code{where.Recipient.Code}
	if where.Sender != "" && where.Sender != where.Recipient.Code {
		packs = append(packs, where.Sender)
	}
	out := map[string][]commsauthz.Provision{}
	for _, code := range citedCodes(packs...) {
		for i, pack := range packs {
			if i > 0 && !senderCitesFinding(code) {
				continue
			}
			if shown := shownProvision(pack, code, where); shown.Cited() && !slices.Contains(out[code], shown) {
				out[code] = append(out[code], shown)
			}
		}
	}
	return out
}

// shownProvision is packCitation as a surface shows it: the provision when the
// citation is shown, and the zero value for every other outcome.
func shownProvision(pack jurisdiction.Code, code string, where citationContext) commsauthz.Provision {
	cited, outcome := packCitation(pack, code, where)
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
