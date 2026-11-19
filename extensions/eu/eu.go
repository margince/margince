// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package eu holds the jurisdiction packs of the 27 EU member states as one
// stable-tier extension, enabled by default in the vanilla tree. Each member
// state's messaging rules compile from policy/<code>/, which layers its
// national law on the ePrivacy layer every member state shares
// (policy/regional/eu.md). Germany also carries its retention floors, its
// request deadline and the documents an employer signs. Core code never
// contains a jurisdiction string, so this unit is where the member states live.
package eu

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
		Name:          "eu",
		Version:       "1.0.0",
		Description:   "EU member-state packs: the messaging rules of all 27 member states on the shared ePrivacy layer, and the German retention floors (§147 AO), request deadline and staff documents.",
		Jurisdictions: packsFor(messagingRules()),
		Messaging:     messagingRules(),
		SubjectRights: []subjectrights.Rules{germanSubjectRights()},
		Documents:     documents(),
	}
}

// packsFor answers one jurisdiction pack per compiled rule set, so the policy
// folders decide which member states the unit holds. A unit may declare messaging
// rules only for a code it carries a pack for.
func packsFor(rules []messaging.Rules) []jurisdiction.Pack {
	out := make([]jurisdiction.Pack, 0, len(rules))
	for _, r := range rules {
		if r.Jurisdiction == (germany{}).Code() {
			out = append(out, germany{})
			continue
		}
		out = append(out, memberState(r.Jurisdiction))
	}
	return out
}

// memberState is the pack of a member state with no retention floor here. Its
// record-keeping duties bind books and invoices, which a CRM does not hold, and
// a floor no record can carry would read as coverage.
type memberState jurisdiction.Code

func (m memberState) Code() jurisdiction.Code { return jurisdiction.Code(m) }

func (memberState) Retention() jurisdiction.Retention { return noFloors{} }

type noFloors struct{}

func (noFloors) Classes() []jurisdiction.RetentionClass { return nil }
