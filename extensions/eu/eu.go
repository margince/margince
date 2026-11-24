// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package eu holds the jurisdiction packs of the 27 EU member states as one
// stable-tier extension, enabled by default in the vanilla tree. Each member
// state's messaging rules compile from policy/<code>/. That layer adds its
// national law to the layer every member state shares (policy/regional/eu.md).
// Retention floors sit in retention.go. Germany also carries its request
// deadline and the documents an employer signs. Core code never contains a
// jurisdiction string, so this unit is where the member states live.
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
		Description:   "EU member-state packs: the messaging rules and retention floors of all 27 member states, and the German request deadline and staff documents.",
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

// memberState is the pack of a member state other than Germany. Its floors
// come from memberStateFloors.
type memberState jurisdiction.Code

func (m memberState) Code() jurisdiction.Code { return jurisdiction.Code(m) }

func (m memberState) Retention() jurisdiction.Retention {
	return floors(memberStateFloors()[jurisdiction.Code(m)])
}
