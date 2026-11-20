// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package apac holds the jurisdiction packs of Asia-Pacific countries as one
// stable-tier extension, enabled by default in the vanilla tree. Each
// country's messaging rules compile from policy/<code>/, which lists that
// country's own layer only. The region shares no law to put in a common layer.
// Retention floors sit in retention.go. Vietnam also carries its request
// deadlines. Core code never
// contains a jurisdiction string, so this unit is where these countries live.
package apac

import (
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
)

// New returns the unit's declaration, which the generated composition calls
// (ADR-0120).
func New() extension.Extension {
	return extension.Extension{
		Name:    "apac",
		Version: "1.0.0",
		Description: "Asia-Pacific packs: the outbound-messaging rules and retention floors of 14 countries " +
			"from Australia to Vietnam, and the Vietnamese request deadlines.",
		Jurisdictions: packsFor(messagingRules()),
		Messaging:     messagingRules(),
		SubjectRights: []subjectrights.Rules{vietnameseSubjectRights()},
		Documents:     vietnameseDocuments(),
	}
}

// packsFor answers one jurisdiction pack per compiled rule set, so the policy
// folders decide which countries the unit holds. A unit may declare messaging rules
// only for a code it carries a pack for.
func packsFor(rules []messaging.Rules) []jurisdiction.Pack {
	out := make([]jurisdiction.Pack, 0, len(rules))
	for _, r := range rules {
		out = append(out, country(r.Jurisdiction))
	}
	return out
}

// country is the pack of one country. Its floors come from countryFloors.
type country jurisdiction.Code

func (c country) Code() jurisdiction.Code { return jurisdiction.Code(c) }

func (c country) Retention() jurisdiction.Retention {
	return floors(countryFloors()[jurisdiction.Code(c)])
}
