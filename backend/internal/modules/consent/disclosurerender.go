// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Turning a declared disclosure into words a message can carry.
//
// Every jurisdiction pack has declared its disclosures since it shipped, and
// nothing rendered one. gates/messagingruleapplied_test.go's register said so
// in its own words — "nothing renders a disclosure into a message body. The
// kinds are a closed set and the merge keeps them, but no consumer turns one
// into text" — which is a statement that the product claimed to follow rules it
// did not follow.
//
// It could not have, until now. A disclosure names the controller, their
// privacy contact and how to object, and none of those existed anywhere in this
// product: there was no record of who the installation IS. controllerparticulars.go
// is where an operator states that, and this is what reads it.

import (
	"context"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// DisclosureLine is one obligation, rendered or recorded as unmeetable.
type DisclosureLine struct {
	// Kind is the obligation the pack declared.
	Kind string
	// Text is what the message should carry. Empty exactly when the
	// installation has not stated the particular this obligation needs.
	Text string
}

// Missing reports that the obligation applies and the installation cannot meet
// it, because it has not said who it is.
//
// A SEPARATE ANSWER from "no obligation applies", and the difference is the
// whole value of this type. Returning nothing for both would let an
// installation that never configured itself look identical to one in a
// jurisdiction that demands nothing — and the first is a compliance gap
// somebody has to close, while the second is fine.
func (d DisclosureLine) Missing() bool { return d.Text == "" }

// DisclosureQuery names the message whose disclosures are asked about.
type DisclosureQuery struct {
	// Category decides which obligations apply: a pack may mark a disclosure
	// MarketingOnly.
	Category commsauthz.Category
	// Recipients are the addresses the one body goes to. Each one's country
	// adds the marketing obligations of their own pack.
	Recipients []string
	// Sender is the user the message goes out for; zero sends for the
	// installation's default legal entity.
	Sender ids.UUID
}

// Disclosures answers what this message must disclose, opening its own
// transaction. The sender is the acting principal's user.
//
// The send path composes a body without a transaction of its own to hand in —
// the unsubscribe linker beside it has the same shape — so this is the door the
// composition root wires, and DisclosuresFor below is the one a caller already
// holding a transaction uses.
//
// A scheduled fire DOES hold one while it prepares, so this opens a second. What
// it reads is committed record and settings state and an in-process pack
// registry, none of which the outer transaction writes.
func (s *Store) Disclosures(
	ctx context.Context, category commsauthz.Category, recipients []string,
) ([]DisclosureLine, error) {
	var out []DisclosureLine
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.DisclosuresFor(ctx, tx, DisclosureQuery{
			Category: category, Recipients: recipients, Sender: senderOf(ctx),
		})
		return err
	})
	return out, err
}

// DisclosuresFor answers what this message must disclose, and whether the
// installation can say it.
//
// Two selectors meet here. Who the controller is and who answers about the
// data are data protection, owed under the sending entity's pack. How to
// object and who advertises are marketing rules, owed under each recipient's
// pack. One body goes to all recipients, so it carries every recipient's.
//
// Answers an empty slice where no pack applies. That is not a claim that
// nothing is owed; it is the baseline, and the caller treats it the same way.
func (s *Store) DisclosuresFor(
	ctx context.Context, tx pgx.Tx, q DisclosureQuery,
) ([]DisclosureLine, error) {
	senderCountry, err := s.dataProtectionCountryTx(ctx, tx, q.Sender)
	if err != nil {
		return nil, err
	}
	var owed []messagingrules.Disclosure
	if senderRules, _, found := applicableRules(senderCountry); found {
		owed = appendOwed(owed, senderRules.Disclosures, true)
	}
	for _, address := range q.Recipients {
		country, err := recipientCountryOf(ctx, tx, connector.Recipient{Email: address})
		if err != nil {
			return nil, err
		}
		if rules, _, found := applicableRules(country.Code); found {
			owed = appendOwed(owed, rules.Disclosures, false)
		}
	}
	if len(owed) == 0 {
		return nil, nil
	}
	// StoredApplyTx, not StoredValTx: the particulars must reach whoever composes the
	// message, and a worker dispatching a scheduled send holds no settings
	// grant. See ControllerIdentity's own note on why that is safe here.
	particulars, err := config.StoredApplyTx(ctx, tx, config.Bound(), ControllerIdentity)
	if err != nil {
		return nil, err
	}
	// An UNKNOWN category is treated as not-marketing, which is the safe
	// direction: it carries the obligations binding every first contact and
	// leaves out the advertising-only ones. A send that names no category is
	// usually an older caller with a legacy purpose key, and putting an
	// objection route under what may be an invoice is the error that reads as
	// an invitation to stop receiving invoices.
	marketing := q.Category == commsauthz.CategoryMarketing
	out := make([]DisclosureLine, 0, len(owed))
	for _, d := range owed {
		// MarketingOnly is the pack's own narrowing. The objection route is
		// owed on advertising and not on an invoice, where it would read as an
		// unsubscribe line under a payment reminder.
		if d.MarketingOnly && !marketing {
			continue
		}
		out = append(out, DisclosureLine{
			Kind: string(d.Kind),
			Text: particularFor(d.Kind, particulars),
		})
	}
	return out, nil
}

// appendOwed adds the declared disclosures of one selector's pack: the
// data-protection kinds from the sender's, the rest from a recipient's. A kind
// already owed is not owed twice; a MarketingOnly copy yields to one that binds
// every message.
func appendOwed(owed, declared []messagingrules.Disclosure, dataProtection bool) []messagingrules.Disclosure {
	for _, d := range declared {
		if dataProtectionDisclosure(d.Kind) != dataProtection {
			continue
		}
		i := slices.IndexFunc(owed, func(o messagingrules.Disclosure) bool { return o.Kind == d.Kind })
		switch {
		case i < 0:
			owed = append(owed, d)
		case !d.MarketingOnly:
			owed[i].MarketingOnly = false
		}
	}
	return owed
}

// dataProtectionDisclosure reports whether a kind discharges a data-protection
// duty (who the controller is, who answers about the data) rather than a
// marketing one.
func dataProtectionDisclosure(kind messagingrules.DisclosureKind) bool {
	return kind == messagingrules.ControllerIdentity || kind == messagingrules.PrivacyContact
}

// particularFor answers which stated particular meets an obligation.
//
// TOTAL over the declared kinds, and an unrecognised one answers EMPTY rather
// than falling back to something plausible. A kind this function does not know
// is an obligation nobody has taught the product to meet, and reporting it as
// met with the wrong words would be worse than reporting it missing — the
// missing answer is visible and gets fixed, while the wrong words ship.
func particularFor(kind messagingrules.DisclosureKind, p ControllerParticulars) string {
	switch kind {
	case messagingrules.ControllerIdentity:
		// BOTH, because the kind's own definition is "the legal name and postal
		// address of the controller". A name alone does not meet it, and
		// reporting it met would tell an operator their configuration is
		// complete when a supervisory authority would say otherwise.
		return bothParticulars(p.LegalName, p.PostalAddress)
	case messagingrules.PrivacyContact:
		return strings.TrimSpace(p.PrivacyContact)
	case messagingrules.ObjectionRoute:
		return strings.TrimSpace(p.ObjectionRoute)
	case messagingrules.AdvertiserContact:
		// REACHABILITY, not identity, which is why it reads its own field. The
		// advertiser and the controller are the same party in this product —
		// an installation sends its own advertising and there is no surface for
		// sending somebody else's — but Decree 91/2020 asks how to reach them
		// directly, and a registered postal address is not a phone number.
		return strings.TrimSpace(p.AdvertiserContact)
	}
	return ""
}

// bothParticulars joins the parts, and answers EMPTY unless every one was
// stated.
//
// All-or-nothing, because an obligation naming two things is not met by one of
// them: messaging.ControllerIdentity is "the legal name and postal address of
// the controller", so a name with no address is an identity disclosure a
// supervisory authority would not accept. Rendering it anyway would report the
// obligation met and leave the operator with no signal that their setup is
// incomplete — the missing answer is the one that gets fixed.
func bothParticulars(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			return ""
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, "\n")
}
