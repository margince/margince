// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package apac holds the jurisdiction packs of Asia-Pacific countries as one
// stable-tier extension, enabled by default in the vanilla tree. Each
// country's messaging rules compile from policy/<code>/, which lists that
// country's own layer and nothing else: the region shares no law to put in a
// common layer. Vietnam also carries its request deadlines. Core code never
// contains a jurisdiction string, so this unit is where these countries live.
package apac

import (
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
)

// New returns the unit's declaration (the ADR-0120 §4 constructor
// contract the generated composition calls).
func New() extension.Extension {
	return extension.Extension{
		Name:    "apac",
		Version: "1.0.0",
		Description: "Asia-Pacific packs: the outbound-messaging rules of 14 countries from Australia to Vietnam, " +
			"and the Vietnamese request deadlines.",
		Jurisdictions: packsFor(messagingRules()),
		Messaging:     messagingRules(),
		SubjectRights: []subjectrights.Rules{vietnameseSubjectRights()},
	}
}

// packsFor answers one jurisdiction pack per compiled rule set, so the policy
// folders are the one list of countries. A unit may declare messaging rules
// only for a code it carries a pack for.
func packsFor(rules []messaging.Rules) []jurisdiction.Pack {
	out := make([]jurisdiction.Pack, 0, len(rules))
	for _, r := range rules {
		out = append(out, country(r.Jurisdiction))
	}
	return out
}

// country is the pack of a country with no retention floor here. Its
// record-keeping duties bind books and invoices, which a CRM does not hold,
// and a floor no record can carry would read as coverage.
type country jurisdiction.Code

func (c country) Code() jurisdiction.Code { return jurisdiction.Code(c) }

func (country) Retention() jurisdiction.Retention { return noFloors{} }

type noFloors struct{}

func (noFloors) Classes() []jurisdiction.RetentionClass { return nil }
