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
	// The specific rules first, because they cut ACROSS the categories: a lead
	// owed a reply is somebody waiting rather than pipeline to build, and a deal
	// whose next step is the meeting brief is a conversation to prepare whatever
	// lane raised it. Each is a product decision and none of them is derivable
	// from the category alone.
	switch {
	case respondsNow(item):
		return crmcontracts.BriefSectionRespondNow
	case preparesAConversation(item):
		return crmcontracts.BriefSectionPrepareConversations
	case sectionBuildsPipeline(item):
		return crmcontracts.BriefSectionBuildPipeline
	}
	return sectionOfCategory(item.Category)
}

// sectionOfCategory places a row that no specific rule claimed, by its category
// alone.
//
// EVERY CATEGORY IS NAMED and there is no default, which is the whole point of
// this function existing separately. With a `default` arm the placement was
// total but not considered: a category nobody had thought about fell through to
// `move_revenue` and was drawn under a heading that happened to be plausible,
// and backend/gates/briefsectioncensus_test.go passed because it could only ask
// whether a section came back at all.
//
// Here an eighth category returns the empty string, and that gate fails on it.
// The gate's question is unchanged; what changed is that this function can now
// answer it wrongly, which is what makes asking it worth anything.
func sectionOfCategory(category crmcontracts.WorklistItemCategory) crmcontracts.WorklistItemBriefSection {
	switch category {
	case crmcontracts.WorklistItemCategoryCustomerWaiting:
		return crmcontracts.BriefSectionRespondNow
	case crmcontracts.WorklistItemCategoryLeads:
		return crmcontracts.BriefSectionBuildPipeline
	case crmcontracts.WorklistItemCategoryMeetings:
		return crmcontracts.BriefSectionPrepareConversations
	case crmcontracts.WorklistItemCategoryDecisions, crmcontracts.WorklistItemCategorySystem:
		return crmcontracts.BriefSectionReviewAndRepair
	case crmcontracts.WorklistItemCategoryDealsAtRisk, crmcontracts.WorklistItemCategoryTasks:
		// The two that are revenue to move: a deal drifting, and the work
		// somebody agreed to do about one.
		return crmcontracts.BriefSectionMoveRevenue
	default:
		// A category this build does not place. Empty rather than a guess, so
		// the census gate says so out loud.
		return ""
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

// sectionBuildsPipeline: work that creates future revenue rather than
// protecting booked revenue.
//
// bands.go's buildsPipeline is CALLED rather than restated — two spellings of
// "this builds pipeline" would drift — and the band is deliberately NOT read
// here, though an earlier version of this function read it first. That version
// was dead code with a paragraph of justification on top: bandOfRow reaches
// bandBuildPipeline only through this same buildsPipeline, so the band arm could
// never be true where the call below is false. Deleting it changed no test,
// which is how it was found.
//
// The two arms it does have are the ones buildsPipeline does not answer. A lead
// row is pipeline by its category even where its subject resolves elsewhere. And
// a relationship going quiet has no lead and no deal at risk, yet reconnecting is
// exactly how pipeline gets built.
func sectionBuildsPipeline(item crmcontracts.WorklistItem) bool {
	if item.Source == crmcontracts.WorklistItemSourceRelationshipDecay {
		return true
	}
	return buildsPipeline(item) || item.Category == crmcontracts.WorklistItemCategoryLeads
}
