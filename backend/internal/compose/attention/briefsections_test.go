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

// EVERY category reaches a section. A row drawn with no section is a row a
// grouping client cannot place, and the default arm is what guarantees it —
// this is the test that would fail if somebody made the mapping partial.
func TestEveryWorklistCategoryHasABriefSection(t *testing.T) {
	for _, category := range []crmcontracts.WorklistItemCategory{
		crmcontracts.WorklistItemCategoryCustomerWaiting,
		crmcontracts.WorklistItemCategoryLeads,
		crmcontracts.WorklistItemCategoryDealsAtRisk,
		crmcontracts.WorklistItemCategoryMeetings,
		crmcontracts.WorklistItemCategoryTasks,
		crmcontracts.WorklistItemCategoryDecisions,
		crmcontracts.WorklistItemCategorySystem,
	} {
		got := sectioned(crmcontracts.WorklistItem{Category: category})
		if got == "" {
			t.Errorf("category %q reaches no section", category)
		}
	}
}
