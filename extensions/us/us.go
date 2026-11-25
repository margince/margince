// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package us holds the United States jurisdiction pack as one stable-tier
// extension, enabled by default in the vanilla tree. Its messaging rules
// compile from policy/us/. The United States is the first opt-out market:
// advertising needs no prior consent, and a refusal always stops it. Core code
// never contains a jurisdiction string, so this unit is where the country lives.
package us

import (
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

// New returns the unit's declaration, which the generated composition calls.
func New() extension.Extension {
	return extension.Extension{
		Name:    "us",
		Version: "1.0.0",
		Description: "United States pack: the outbound-messaging rules of the CAN-SPAM Act, " +
			"an opt-out market, and the federal tax-record retention floor.",
		Jurisdictions: packsFor(messagingRules()),
		Messaging:     messagingRules(),
	}
}

// packsFor answers one jurisdiction pack per compiled rule set, so the policy
// folders decide which countries the unit holds.
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
