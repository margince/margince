// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// What a decision ECHOES, which is a different question from who may make one.
//
// authority.go answers "may this seat decide this staging" — grants, row rules,
// the seats a kind narrows to. This file answers "and when they do, what else
// hears about it": the domain event a decided approval publishes for kinds
// whose lifecycle the event catalog tracks beyond approval.decided itself.
//
// Split out because the two grew into one 500-line file and only one of them is
// about authority. A reader asking either question was reading past the other.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// decidedEcho builds the approved/rejected payload a kind's decision
// echoes, given the decided approval's own id and the deciding human's
// user id — the fixed shape every decided-echo carries today.
type decidedEcho struct {
	approved, rejected func(approvalID, decidedBy openapi_types.UUID) events.Payload
}

// kindDecidedEvents names the domain event a decision echoes for kinds
// whose lifecycle the event catalog tracks beyond approval.decided.
var kindDecidedEvents = map[string]decidedEcho{
	"coldstart": {
		approved: func(approvalID, decidedBy openapi_types.UUID) events.Payload {
			return crmcontracts.PublicEventColdstartAccepted{ApprovalId: approvalID, DecidedBy: decidedBy}
		},
		rejected: func(approvalID, decidedBy openapi_types.UUID) events.Payload {
			return crmcontracts.PublicEventColdstartRejected{ApprovalId: approvalID, DecidedBy: decidedBy}
		},
	},
}

// The target types this package names in more than one place. They are the
// `target_entity_type` vocabulary the staged rows carry, and this package spells
// each in several places — the visibility probe's classification, the
// decision-grant map, and the version-table whitelist. One spelling, because a
// typo makes a target undecidable in the first and unpinnable in the last, and
// neither failure announces itself.
const (
	targetOffer        = "offer"
	targetProduct      = "product"
	targetRelationship = "relationship"
	targetSavedView    = "saved_view"
	targetSignal       = "signal"
	targetTag          = "tag"
	// The workspace-shared config rows an object grant governs, each named by
	// both the existence probe and the version-table whitelist.
	targetOfferTemplate       = "offer_template"
	targetWebhookSubscription = "webhook_subscription"
	// The effective-dated rate sheets. Both are named twice — the probe
	// classification and the decision-grant map — and both are workspace-scoped
	// config with no row of their own until a proposal is accepted.
	targetFxRate      = "fx_rate"
	targetAIModelRate = "ai_model_rate"
	// The row-scoped record tables. Named as a SET rather than one at a time: this
	// package spells them in the probe classification, the decision-grant map and
	// the version-table whitelist, and a typo makes a target undecidable in the
	// first and unpinnable in the last without announcing either.
	tablePerson       = "person"
	tableOrganization = "organization"
	tableDeal         = "deal"
	tableLead         = "lead"
	tableProject      = "project"
	tableList         = "list"
)
