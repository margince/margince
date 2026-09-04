// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package de is the German jurisdiction pack (ADR-0042) as a stable-tier
// extension — the ADR-0069 migration pilot: the first first-party unit
// shipping enabled-by-default in the vanilla tree (its directory under
// extensions/ IS the enablement). V1 ships the GoBD statutory retention
// classes; further obligations (the XRechnung/ZUGFeRD fiscal formats,
// the CRA conformity regime) return to the seam when their work packages
// land. Core code never contains a jurisdiction string — this unit is
// where Germany lives. The product ships no scraping-based enrichment; the
// legal position behind that is a whitepaper kept with the company's business
// material, not in this tree.
package de

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// New returns the unit's declaration (the ADR-0069 §4 constructor
// contract the generated composition calls).
func New() extension.Extension {
	return extension.Extension{
		Name:          "de",
		Version:       "1.0.0",
		Description:   "German jurisdiction pack: the statutory retention floors (§147 AO) and the outbound-messaging rules (UWG §7, Art. 13) the core engines apply. Registers no records, routes or jobs.",
		Jurisdictions: []jurisdiction.Pack{pack{}},
		Messaging:     []messaging.Rules{messagingRules()},
	}
}

type pack struct{}

func (pack) Code() jurisdiction.Code { return "de" }

// Retention: the GoBD/AO statutory floors for the record classes the
// product can hold (§147 AO as amended 2025: booking records 8 years;
// commercial correspondence 6), each ANCHORED at calendar-year end —
// §147(4) AO starts every period at the end of the calendar year the
// record occurred in, so a January Handelsbrief keeps almost seven
// calendar years. The core engine treats these as FLOORS — a workspace
// policy may keep longer, never destroy earlier. The spans are calendar
// years (ISO 8601 periods), never day counts. Bücher/Abschlüsse (10 yr)
// are deliberately absent: a CRM holds no books or annual accounts, and
// a floor no record can carry would be documentation posing as
// enforcement.
func (pack) Retention() jurisdiction.Retention { return retention{} }

type retention struct{}

func (retention) Classes() []jurisdiction.RetentionClass {
	return []jurisdiction.RetentionClass{
		{Name: jurisdiction.CommercialCorrespondence, Keep: jurisdiction.Period{Years: 6}, Anchor: jurisdiction.AnchorCalendarYearEnd},
		{Name: jurisdiction.AccountingRecords, Keep: jurisdiction.Period{Years: 8}, Anchor: jurisdiction.AnchorCalendarYearEnd},
	}
}

// messagingRules is what German law requires of an outbound message, stated as
// data the core engine applies. Nothing here decides a send.
//
// THE EXISTING-CUSTOMER EXCEPTION (UWG §7(3)) is the only route to advertising
// without a recorded consent, and it carries all four of its statutory
// conditions rather than a subset. §7(3) permits email advertising to an
// existing customer only where the address was obtained IN CONNECTION WITH A
// SALE, the customer was told at collection that they may object at any time
// free of charge beyond transmission cost, the advertising is for the seller's
// OWN SIMILAR goods, and no objection stands. Declaring the exception without
// one of these would be an exception the engine applies while checking less
// than the statute asks — which is worse than having none, because it looks
// lawful.
//
// Similarity is checked PER MESSAGE. A customer who bought one product has not
// opened the door to everything the seller sells, and an exception evaluated
// once per person rather than once per message is the shape that turns one
// purchase into a permanent mailing list.
//
// THE WINDOWS are the core defaults, restated here so this pack says what it
// applies rather than inheriting silently. Twelve months is how long an inbound
// message keeps making an unprompted follow-up a follow-up; six months is how
// long a live deal does. Neither bounds a same-thread reply — the subject wrote
// to us and did not withdraw.
//
// NO PREFIX, NO CAP, NO ACKNOWLEDGEMENT. German law requires none of the three:
// no marking on the subject line of a commercial email beyond the general ban on
// disguising the commercial nature, no statutory frequency ceiling, and no
// confirmation owed for an opt-out. The zero values say so, and saying so is the
// point — a reader comparing this pack with another can see which obligations
// Germany does not impose rather than wondering whether somebody forgot.
func messagingRules() messaging.Rules {
	return messaging.Rules{
		Jurisdiction: "de",
		Version:      1,
		// Twelve and six months as calendar-ish durations. Hours because the
		// type is a time.Duration and a month is not one; the engine compares
		// against a recorded timestamp, so the small drift against calendar
		// months is on the permissive side of a window that bounds an
		// unprompted follow-up rather than a refusal.
		ReplyWindow:        365 * 24 * time.Hour,
		DealFollowUpWindow: 182 * 24 * time.Hour,
		MarketingExceptions: []messaging.MarketingException{{
			Kind:                         messaging.ExistingCustomer,
			RequiresSaleEvidence:         true,
			RequiresCollectionTimeOptOut: true,
			RequiresSimilarity:           true,
			RequiresNoObjection:          true,
		}},
		// Art. 13 GDPR at first contact: who is writing, who answers about the
		// data, and how to say stop. The objection route binds advertising
		// specifically — §7(3) requires it at every use, not only the first.
		Disclosures: []messaging.Disclosure{
			{Kind: messaging.ControllerIdentity},
			{Kind: messaging.PrivacyContact},
			{Kind: messaging.ObjectionRoute, MarketingOnly: true},
		},
	}
}
