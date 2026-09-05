// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which part of the morning a row belongs to.
//
// A LABEL, NEVER AN ORDER, and that is the whole rule this file exists to hold.
// The queue arrives ranked by one policy — the levels, then the tie-breaks in
// ranksteps.go — and a section says nothing about where a row sits. A client may
// draw the label and may group runs of consecutive rows that share it. It may
// not partition the page by section and concatenate the parts: that is a second
// ranking, and the two disagree the first time a `respond_now` row ranks below a
// `move_revenue` one, which is ordinary and correct.
//
// So this is computed on the SERVER, once, and travels as presentation metadata.
// The alternative — a browser deriving it from category and source — is the same
// mapping in a second place, in a language where nothing can check it, and the
// day it drifts the page groups a row under a heading the readings counted
// elsewhere.
//
// EXHAUSTIVE over WorklistItemCategory, and held that way: every category
// reaches a section, because a row drawn with no section at all is a row a
// grouping client cannot place. backend/gates/briefsectioncensus_test.go derives
// the category list from the generated contract rather than keeping a second
// copy of it, so a category added to crm.yaml fails here until somebody decides
// which part of the morning it belongs to.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// BriefSectionOf answers which part of the morning this row belongs to.
//
// Read in this order, because the questions are not independent. A lead that is
// overdue for a reply is somebody waiting, not pipeline to build; a deal row
// whose move is a meeting brief is a conversation to prepare, not revenue to
// move. The FIRST matching rule wins, and the order below is the product
// decision — the general category answer is last, after the specific ones.
func BriefSectionOf(item crmcontracts.WorklistItem) crmcontracts.WorklistItemBriefSection {
	switch {
	case respondsNow(item):
		return crmcontracts.BriefSectionRespondNow
	case preparesAConversation(item):
		return crmcontracts.BriefSectionPrepareConversations
	case repairsSomething(item):
		return crmcontracts.BriefSectionReviewAndRepair
	case sectionBuildsPipeline(item):
		return crmcontracts.BriefSectionBuildPipeline
	default:
		return crmcontracts.BriefSectionMoveRevenue
	}
}

// respondsNow: somebody is waiting for an answer from this rep.
//
// A bounce and an undelivered message belong here too, and they are the arm a
// reader would not guess: the customer never received what we sent, so they are
// waiting without knowing it, and nothing else about the day will surface that.
func respondsNow(item crmcontracts.WorklistItem) bool {
	switch item.Source {
	case crmcontracts.WorklistItemSourceBounce, crmcontracts.WorklistItemSourceUndelivered:
		return true
	}
	if item.Category == crmcontracts.WorklistItemCategoryCustomerWaiting {
		return true
	}
	// A lead is pipeline UNTIL its reply clock is running, and then it is
	// somebody waiting. The reasons are what carry that, because the lane
	// already computed it and a second reading here would be a second answer.
	return item.Category == crmcontracts.WorklistItemCategoryLeads && owedAReply(item)
}

// owedAReply reads the response clock the lead lane already stamped.
func owedAReply(item crmcontracts.WorklistItem) bool {
	for _, because := range item.Because {
		switch because.Kind {
		case crmcontracts.WorklistReasonKindResponseOverdue,
			crmcontracts.WorklistReasonKindResponseDueSoon:
			return true
		}
	}
	return false
}

// preparesAConversation: a meeting is coming and somebody has to walk in ready.
//
// The deal arm matters as much as the meeting one: a deal row whose next step is
// "open the meeting brief" is preparation, whatever lane raised it.
func preparesAConversation(item crmcontracts.WorklistItem) bool {
	if item.Source == crmcontracts.WorklistItemSourceMeeting || item.Category == crmcontracts.WorklistItemCategoryMeetings {
		return true
	}
	return item.Move != nil && item.Move.Action == crmcontracts.WorklistMoveActionOpenMeetingBrief
}

// repairsSomething: a judgement to make or a source to restore, rather than
// customer work to do.
func repairsSomething(item crmcontracts.WorklistItem) bool {
	switch item.Category {
	case crmcontracts.WorklistItemCategoryDecisions, crmcontracts.WorklistItemCategorySystem:
		return true
	}
	return false
}

// sectionBuildsPipeline: work that creates future revenue rather than
// protecting booked revenue.
//
// The BAND is asked first, because it is the server's own answer to "what is the
// reader being asked to do today" and it already carries rows this section wants
// that no subject test would find. bands.go's buildsPipeline answers the
// narrower question — is the row filed under a lead — and is reused rather than
// restated: two spellings of "this builds pipeline" would drift, and the band
// this file reads is itself derived from that function.
//
// A relationship going quiet is the third arm and belongs to neither: there is
// no lead yet and the band is about momentum, but reconnecting is exactly how
// pipeline gets built.
func sectionBuildsPipeline(item crmcontracts.WorklistItem) bool {
	if item.Band != nil && *item.Band == crmcontracts.BuildPipeline {
		return true
	}
	if item.Source == crmcontracts.WorklistItemSourceRelationshipDecay {
		return true
	}
	return buildsPipeline(item) || item.Category == crmcontracts.WorklistItemCategoryLeads
}
