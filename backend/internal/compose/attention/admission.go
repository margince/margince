// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Which owner answers each source may give, and so whose personal work its
// rows can become.
//
// Personal automatic work needs an evidenced responsible seat. Each source
// declares where that evidence comes from. It may be an assignee or owner
// column, a read bound to the acting human, or the deal's owner. It may also be
// the direct recipients of a mail, or a meeting's recorded host.
//
// Some rows never reach a page: one whose source has no entry, and one that
// claims an answer its source may not give. Neither reaches Mine, Unassigned
// or a team view.
// The lane it came from is reported failed, so the page does not claim to be
// complete, and the fault is logged.
//
// Every contract source is listed. The test beside this requires the key set
// to equal the contract's enum, so a new producer fails there first.
var sourceOwnerAnswers = map[crmcontracts.WorklistItemSource]ownerAnswers{
	// An assignee or owner column: named, or nobody when the column is empty.
	crmcontracts.WorklistItemSourceTask:             answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceLeadResponse:     answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceNoticeCase:       answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceDsr:              answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceWeeklyCommitment: answers(ownerNamed),
	// The contact's live owner; never the reader who was copied or wrote.
	crmcontracts.WorklistItemSourceRelationshipDecay: answers(ownerNamed, ownerNobody),
	// A meeting's recorded host or attendee seat.
	crmcontracts.WorklistItemSourceMeeting:        answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceMeetingOutcome: answers(ownerNamed, ownerNobody),
	// The requested decider, or nobody when no one was asked.
	crmcontracts.WorklistItemSourceApproval:        answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceDedupeCandidate: answers(ownerNamed, ownerNobody),
	// The member owner the page was raised for, or nobody when no member has one.
	crmcontracts.WorklistItemSourceContactCase:    answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceDealSuggestion: answers(ownerNamed, ownerNobody),
	crmcontracts.WorklistItemSourceTagSuggestion:  answers(ownerNamed, ownerNobody),
	// The deal's owner, attached after classification.
	crmcontracts.WorklistItemSourceBriefItem:  answers(ownerFromTheDeal),
	crmcontracts.WorklistItemSourceDealAtRisk: answers(ownerFromTheDeal),
	// The sole addressed seat or the owning record, nobody, or withheld.
	crmcontracts.WorklistItemSourceCustomerWaiting: answers(ownerNamed, ownerNobody, ownerUnreadable),
	// A promise read for its promiser; keepUnowned places it.
	crmcontracts.WorklistItemSourceConversationClaim: answers(ownerNobody),
	// Reads bound to the acting human inside the owning module.
	crmcontracts.WorklistItemSourceFailedApproval:      answers(ownerTheReader),
	crmcontracts.WorklistItemSourceIntroductionRequest: answers(ownerTheReader),
	crmcontracts.WorklistItemSourceBounce:              answers(ownerTheReader),
	crmcontracts.WorklistItemSourceUndelivered:         answers(ownerTheReader),
	crmcontracts.WorklistItemSourceNotice:              answers(ownerTheReader),
	crmcontracts.WorklistItemSourceCaptureHealth:       answers(ownerTheReader),
	crmcontracts.WorklistItemSourceDomainQuestion:      answers(ownerTheReader),
	crmcontracts.WorklistItemSourceAwaitingReply:       answers(ownerTheReader),
	crmcontracts.WorklistItemSourceMeetingFollowUp:     answers(ownerTheReader),
	// Product health nobody personally owes.
	crmcontracts.WorklistItemSourceAiWorkHealth:  answers(ownerNobody),
	crmcontracts.WorklistItemSourceAutomationRun: answers(ownerNobody),
	// A fold carries its members' common answer, or none when they differ.
	crmcontracts.WorklistItemSourceBatch: answers(ownerUnstated, ownerNamed, ownerNobody, ownerTheReader, ownerFromTheDeal, ownerUnreadable),
}

// ownerAnswers is the set of owner kinds one source may state.
type ownerAnswers map[ownerKind]bool

func answers(kinds ...ownerKind) ownerAnswers {
	out := make(ownerAnswers, len(kinds))
	for _, kind := range kinds {
		out[kind] = true
	}
	return out
}

// admitBySource applies sourceOwnerAnswers to every classified row, before
// scope, counts, Focus and paging read them, so all of them see one answer. It
// returns the kept rows and one failed-source entry per lane that lost a row.
func admitBySource(ctx context.Context, rows []ranked) ([]ranked, []crmcontracts.WorklistSourceUnavailable) {
	kept := rows[:0:0]
	var refused []crmcontracts.WorklistSourceUnavailable
	failed := map[crmcontracts.WorklistItemSource]bool{}
	for _, row := range rows {
		allowed, known := sourceOwnerAnswers[row.item.Source]
		if known && allowed[row.ownerRef.kind] {
			kept = append(kept, row)
			continue
		}
		slog.ErrorContext(ctx, "a worklist row has no admission for its owner; it creates no work",
			"source", row.item.Source, "lane", row.lane, "owner_kind", int(row.ownerRef.kind), "policy_known", known)
		lane := reportedLane(row)
		if !failed[lane] {
			failed[lane] = true
			refused = append(refused, crmcontracts.WorklistSourceUnavailable{Source: string(lane), Reason: "failed"})
		}
	}
	return kept, refused
}

// reportedLane names the lane a refused row is reported under. That is the
// lane that read it, or its own source for a row read outside the day's lanes.
func reportedLane(row ranked) crmcontracts.WorklistItemSource {
	if row.lane != "" {
		return row.lane
	}
	return row.item.Source
}
