// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package vn is the Vietnamese jurisdiction pack as a stable-tier extension:
// its directory under extensions/ IS the enablement. Core code never contains a
// jurisdiction string — this unit is where Vietnam lives.
//
// V2 declares the outbound-messaging rules Vietnamese law places on advertising
// email, and states THREE instruments rather than one.
//
// Decree 91/2020/ND-CP is where every obligation below comes from: the daily
// ceiling, the [QC] label, the advertiser identification, the acknowledged
// opt-out. That has not changed, and the engine applies exactly what it did at
// version 1.
//
// Law 91/2025/QH15 and Decree 356/2025/ND-CP both took effect on 1 January 2026
// and govern personal data protection. They are stated here because a
// Vietnamese recipient of advertising email is a data subject under them, so a
// decision taken from that date sits under all three — NOT because either one
// amends the 2020 decree. Whether they impose an outbound-messaging obligation
// this pack does not yet declare is an open question for a Vietnamese lawyer,
// and stating them is what makes that question askable from the record.
//
// The version moves because the instruments moved. A decision recording
// "vn version 2" can be read back against the law that was live when it was
// taken, which a version alone cannot answer.
//
// It declares NO retention class: the decree and the Vietnamese accounting law
// bind records this CRM does not hold, and a floor no record can carry would be
// documentation posing as enforcement.
package vn

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
)

// New returns the unit's declaration (the ADR-0120 §4 constructor contract the
// generated composition calls).
func New() extension.Extension {
	return extension.Extension{
		Name:          "vn",
		Version:       "1.0.0",
		Description:   "Vietnamese jurisdiction pack: the Decree 91/2020/ND-CP advertising-email rules — prior consent, the subject label, advertiser identity, the daily ceiling and the acknowledged opt-out.",
		Jurisdictions: []jurisdiction.Pack{pack{}},
		Messaging:     []messaging.Rules{messagingRules()},
		SubjectRights: []subjectrights.Rules{subjectRights()},
	}
}

type pack struct{}

func (pack) Code() jurisdiction.Code { return "vn" }

// Retention: none. The core engine reads a pack's classes as statutory FLOORS
// on records the product holds, and Vietnam's record-keeping duties fall on
// accounting books and invoices — neither of which lives in a CRM. Declaring a
// class the product cannot carry would put a floor in the composition that
// nothing is ever measured against, which reads as coverage and is not.
func (pack) Retention() jurisdiction.Retention { return retention{} }

type retention struct{}

func (retention) Classes() []jurisdiction.RetentionClass { return nil }

// subjectRights are the Decree 356/2025/ND-CP Art. 5 deadlines. Every request
// is owed a first answer in 2 working days. It is carried out in 10 days to
// correct (Art. 5.3), 20 to erase (Art. 5.4) and 15 to restrict (Art. 5.2),
// counted in calendar days. Access keeps the core month.
func subjectRights() subjectrights.Rules {
	return subjectrights.Rules{
		Jurisdiction: "vn",
		Version:      1,
		Instruments: []messaging.Instrument{
			{Name: "Decree 356/2025/ND-CP, Art. 5", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		Deadlines: map[subjectrights.RequestKind]subjectrights.Deadline{
			subjectrights.Rectify:     {Period: jurisdiction.Period{Days: 10}},
			subjectrights.Erasure:     {Period: jurisdiction.Period{Days: 20}},
			subjectrights.Restriction: {Period: jurisdiction.Period{Days: 15}},
		},
		ReplyWithinWorkingDays: 2,
		// Working days are Vietnamese days.
		UTCOffset: 7 * time.Hour,
	}
}
