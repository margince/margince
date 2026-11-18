// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package de is the German jurisdiction pack (ADR-0042) as a stable-tier
// extension, the first first-party unit enabled by default in the vanilla
// tree. Its directory under extensions/ is the enablement. It ships the GoBD
// statutory retention classes, the outbound-messaging rules, the request
// deadline and the compliance documents an employer signs. The
// XRechnung/ZUGFeRD fiscal formats return to the seam when their work package
// lands. The CRA documents are the maker's duty and live in
// docs/compliance/cra. Core code never contains a jurisdiction string, so
// this unit is where Germany lives. The product ships no scraping-based enrichment; the
// legal position behind that is a whitepaper kept with the company's business
// material, not in this tree.
package de

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
)

// New returns the unit's declaration (the ADR-0120 §4 constructor
// contract the generated composition calls).
func New() extension.Extension {
	return extension.Extension{
		Name:          "de",
		Version:       "1.0.0",
		Description:   "German jurisdiction pack: the retention floors (§147 AO), the messaging rules (UWG §7, Art. 13), the request deadline and the staff documents. Registers no records, routes or jobs.",
		Jurisdictions: []jurisdiction.Pack{pack{}},
		Messaging:     []messaging.Rules{messagingRules()},
		SubjectRights: []subjectrights.Rules{subjectRights()},
		Documents:     documents(),
	}
}

// subjectRights states the GDPR month for every request kind. German law adds
// no shorter deadline, so a German installation gets what the core's fallback
// would give, under the citation that binds it here.
func subjectRights() subjectrights.Rules {
	month := subjectrights.Deadline{Period: jurisdiction.Period{Months: 1}}
	return subjectrights.Rules{
		Jurisdiction: "de",
		Version:      1,
		Instruments:  []messaging.Instrument{{Name: "Regulation (EU) 2016/679, Art. 12(3)", EffectiveFrom: time.Date(2018, time.May, 25, 0, 0, 0, 0, time.UTC)}},
		Deadlines: map[subjectrights.RequestKind]subjectrights.Deadline{
			subjectrights.Access: month, subjectrights.Rectify: month, subjectrights.Erasure: month,
		},
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
