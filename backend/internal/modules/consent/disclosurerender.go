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
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
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

// Disclosures answers what a message of this category must disclose, opening
// its own transaction.
//
// The send path composes a body without a transaction of its own to hand in —
// the unsubscribe linker beside it has the same shape — so this is the door the
// composition root wires, and DisclosuresFor below is the one a caller already
// holding a transaction uses.
//
// A scheduled fire DOES hold one while it prepares, so this opens a second. What
// it reads is a settings row and an in-process pack registry, neither of which
// the outer transaction writes — so the nested read sees committed state, which
// is what it wants.
func (s *Store) Disclosures(
	ctx context.Context, category commsauthz.Category,
) ([]DisclosureLine, error) {
	var out []DisclosureLine
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.DisclosuresFor(ctx, tx, category)
		return err
	})
	return out, err
}

// DisclosuresFor answers what this message must disclose, and whether the
// installation can say it.
//
// The CATEGORY decides which obligations apply: a pack may mark a disclosure
// MarketingOnly, which §7(3) UWG does for the objection route — it binds every
// advertising message rather than only the first contact. A transactional
// message carries the identity and the privacy contact and not that one.
//
// Answers an empty slice where no pack applies. That is not a claim that
// nothing is owed; it is the same answer applicableRules gives for an
// installation whose country is unknown, and the caller treats it the same way.
func (s *Store) DisclosuresFor(
	ctx context.Context, tx pgx.Tx, category commsauthz.Category,
) ([]DisclosureLine, error) {
	rules, found, err := s.applicableRules(ctx, tx)
	if err != nil || !found {
		return nil, err
	}
	if len(rules.Disclosures) == 0 {
		return nil, nil
	}
	// ApplyTx, not GetTx: the particulars must reach whoever composes the
	// message, and a worker dispatching a scheduled send holds no settings
	// grant. See ControllerIdentity's own note on why that is safe here.
	particulars, err := settings.ApplyTx(ctx, tx, ControllerIdentity)
	if err != nil {
		return nil, err
	}
	// An UNKNOWN category is treated as not-marketing, which is the safe
	// direction: it carries the obligations binding every first contact and
	// leaves out the advertising-only ones. A send that names no category is
	// usually an older caller with a legacy purpose key, and putting an
	// objection route under what may be an invoice is the error that reads as
	// an invitation to stop receiving invoices.
	marketing := category == commsauthz.CategoryMarketing
	out := make([]DisclosureLine, 0, len(rules.Disclosures))
	for _, d := range rules.Disclosures {
		// MarketingOnly is the pack's own narrowing and is honoured rather
		// than folded away: an installation in Germany owes the objection
		// route on advertising and not on an invoice, and rendering it on both
		// would put an unsubscribe line under a payment reminder.
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
