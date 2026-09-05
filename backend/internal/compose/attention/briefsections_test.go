// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which part of the morning a row belongs to.
//
// Every case here is about the ORDER of the rules, because that is where the
// mapping is a product decision rather than a lookup: a lead owed a reply is
// somebody waiting, and a deal whose next step is a meeting brief is a
// conversation to prepare.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sectioned(over crmcontracts.WorklistItem) crmcontracts.WorklistItemBriefSection {
	if over.Actions == nil {
		over.Actions = []crmcontracts.WorklistItemActions{}
	}
	return BriefSectionOf(over)
}

func TestACustomerWaitingIsAnsweredFirst(t *testing.T) {
	got := sectioned(crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceCustomerWaiting,
		Category: crmcontracts.WorklistItemCategoryCustomerWaiting,
	})

	if got != crmcontracts.BriefSectionRespondNow {
		t.Errorf("section = %q, want respond_now", got)
	}
}

// THE ordering case for leads. A lead is pipeline until its reply clock is
// running, and then somebody is waiting on us.
func TestALeadOwedAReplyIsSomebodyWaitingNotPipelineToBuild(t *testing.T) {
	got := sectioned(crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceLeadResponse,
		Category: crmcontracts.WorklistItemCategoryLeads,
		Because: []crmcontracts.WorklistReason{
			{Kind: crmcontracts.WorklistReasonKindResponseOverdue},
		},
	})

	if got != crmcontracts.BriefSectionRespondNow {
		t.Errorf("section = %q, want respond_now for a lead past its reply clock", got)
	}
}

func TestALeadWithNoReplyClockIsPipelineToBuild(t *testing.T) {
	got := sectioned(crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceLeadResponse,
		Category: crmcontracts.WorklistItemCategoryLeads,
	})

	if got != crmcontracts.BriefSectionBuildPipeline {
		t.Errorf("section = %q, want build_pipeline", got)
	}
}

// A message the customer never received. They are waiting without knowing it,
// and nothing else about the day surfaces that.
func TestABouncedMessageIsSomebodyWaiting(t *testing.T) {
	for _, source := range []crmcontracts.WorklistItemSource{
		crmcontracts.WorklistItemSourceBounce,
		crmcontracts.WorklistItemSourceUndelivered,
	} {
		got := sectioned(crmcontracts.WorklistItem{
			Source: source, Category: crmcontracts.WorklistItemCategorySystem,
		})
		if got != crmcontracts.BriefSectionRespondNow {
			t.Errorf("%s: section = %q, want respond_now", source, got)
		}
	}
}

// THE ordering case for deals: a deal row whose next step is the meeting brief
// is preparation, whatever lane raised it.
func TestADealWhoseNextStepIsAMeetingBriefIsPreparation(t *testing.T) {
	got := sectioned(crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceDealAtRisk,
		Category: crmcontracts.WorklistItemCategoryDealsAtRisk,
		Move: &crmcontracts.WorklistMove{
			Action: crmcontracts.WorklistMoveActionOpenMeetingBrief,
		},
	})

	if got != crmcontracts.BriefSectionPrepareConversations {
		t.Errorf("section = %q, want prepare_conversations", got)
	}
}

func TestAMeetingIsAConversationToPrepare(t *testing.T) {
	got := sectioned(crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceMeeting,
		Category: crmcontracts.WorklistItemCategoryMeetings,
	})

	if got != crmcontracts.BriefSectionPrepareConversations {
		t.Errorf("section = %q, want prepare_conversations", got)
	}
}

func TestADecisionAndASystemFailureAreBothReviewAndRepair(t *testing.T) {
	for _, category := range []crmcontracts.WorklistItemCategory{
		crmcontracts.WorklistItemCategoryDecisions,
		crmcontracts.WorklistItemCategorySystem,
	} {
		got := sectioned(crmcontracts.WorklistItem{
			Source: crmcontracts.WorklistItemSourceApproval, Category: category,
		})
		if got != crmcontracts.BriefSectionReviewAndRepair {
			t.Errorf("%s: section = %q, want review_and_repair", category, got)
		}
	}
}

// A relationship going quiet has no lead and no deal at risk, and reconnecting
// is exactly how pipeline gets built.
func TestARelationshipGoingQuietIsPipelineToBuild(t *testing.T) {
	got := sectioned(crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceRelationshipDecay,
		Category: crmcontracts.WorklistItemCategoryDealsAtRisk,
	})

	if got != crmcontracts.BriefSectionBuildPipeline {
		t.Errorf("section = %q, want build_pipeline", got)
	}
}

func TestADriftingDealIsRevenueToMove(t *testing.T) {
	got := sectioned(crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceDealAtRisk,
		Category: crmcontracts.WorklistItemCategoryDealsAtRisk,
	})

	if got != crmcontracts.BriefSectionMoveRevenue {
		t.Errorf("section = %q, want move_revenue", got)
	}
}

// A category this build cannot place carries NO section rather than an empty one.
//
// sectionOfCategory returns "" for a category nobody has placed, which is what
// makes the census gate fail loudly instead of drawing the row under whatever
// the fall-through happened to be. That empty string must not reach the wire:
// the field is an enum, and a client reading "" has a value its own generated
// types do not carry.
//
// The gate is what keeps this arm unreachable in a shipped build. This is what
// keeps a build where it IS reachable from serving a value no client can read.
func TestAnUnplaceableRowCarriesNoSectionRatherThanAnEmptyOne(t *testing.T) {
	unplaced := ranked{item: crmcontracts.WorklistItem{
		Id:       ids.NewV7().String(),
		Source:   crmcontracts.WorklistItemSourceTask,
		Category: "renewals",
		Actions:  []crmcontracts.WorklistItemActions{},
	}}

	drawn := renderInOrder([]ranked{unplaced}, ids.UUID{})

	if drawn[0].BriefSection != nil {
		t.Errorf("brief_section = %q reached the wire for a category this build does not place — "+
			"a client reading it has an enum value its generated types do not carry",
			*drawn[0].BriefSection)
	}
}

// And a row this build DOES place still carries its section, so the guard above
// cannot be satisfied by drawing nothing at all.
func TestAPlaceableRowStillCarriesItsSection(t *testing.T) {
	placed := ranked{item: crmcontracts.WorklistItem{
		Id:       ids.NewV7().String(),
		Source:   crmcontracts.WorklistItemSourceDealAtRisk,
		Category: crmcontracts.WorklistItemCategoryDealsAtRisk,
		Actions:  []crmcontracts.WorklistItemActions{},
	}}

	drawn := renderInOrder([]ranked{placed}, ids.UUID{})

	if drawn[0].BriefSection == nil {
		t.Fatal("a placeable row carries no section — the guard is drawing nothing at all")
	}
	if *drawn[0].BriefSection != crmcontracts.BriefSectionMoveRevenue {
		t.Errorf("section = %q, want move_revenue", *drawn[0].BriefSection)
	}
}
