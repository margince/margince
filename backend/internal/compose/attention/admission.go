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
// declares where that evidence comes from: an assignee or owner column, a read
// bound to the acting human, the deal's owner, the direct recipients of a
// mail, or a meeting's recorded host. A row whose source has no entry has no
// declared evidence, so it never reaches a page; a row claiming an answer its
// source may not give is demoted to unassigned, which is nobody's Mine. Both
// are logged, because either is a producer this table was not told about.
//
// EVERY contract source is listed; the test beside this requires the key set
// to equal the contract's enum, so a new producer fails there before it can
// enter anybody's day.
var sourceOwnerAnswers = map[crmcontracts.WorklistItemSource]ownerAnswers{
	// An assignee or owner column: named, or nobody when the column is empty.
	sourceTask:             answers(ownerNamed, ownerNobody),
	sourceLeadResponse:     answers(ownerNamed, ownerNobody),
	sourceNoticeCase:       answers(ownerNamed, ownerNobody),
	"dsr":                  answers(ownerNamed, ownerNobody),
	sourceWeeklyCommitment: answers(ownerNamed),
	// The contact's live owner; never the reader who was copied or wrote.
	"relationship_decay": answers(ownerNamed, ownerNobody),
	// A meeting's recorded host or attendee seat.
	sourceMeeting:        answers(ownerNamed, ownerNobody),
	sourceMeetingOutcome: answers(ownerNamed, ownerNobody),
	// The requested decider, or nobody when no one was asked.
	"approval":           answers(ownerNamed, ownerNobody),
	sourceDuplicate:      answers(ownerNamed, ownerNobody),
	sourceDealSuggestion: answers(ownerNamed, ownerNobody),
	sourceTagSuggestion:  answers(ownerNamed, ownerNobody),
	// The deal's owner, attached after classification.
	sourceBriefItem: answers(ownerFromTheDeal),
	sourceAtRisk:    answers(ownerFromTheDeal),
	// The owning record, the direct recipients, or withheld.
	sourceWaiting: answers(ownerNamed, ownerShared, ownerNobody, ownerUnreadable),
	// A promise read for its promiser; keepUnowned places it.
	sourceClaim: answers(ownerNobody),
	// Reads bound to the acting human inside the owning module.
	"failed_approval":      answers(ownerTheReader),
	"introduction_request": answers(ownerTheReader),
	"bounce":               answers(ownerTheReader),
	"undelivered":          answers(ownerTheReader),
	crmcontracts.WorklistItemSource(sourceNotice):         answers(ownerTheReader),
	crmcontracts.WorklistItemSource(sourceCaptureHealth):  answers(ownerTheReader),
	crmcontracts.WorklistItemSource(sourceDomainQuestion): answers(ownerTheReader),
	sourceAwaitingReply:   answers(ownerTheReader),
	sourceMeetingFollowUp: answers(ownerTheReader),
	// Product health nobody personally owes.
	"ai_work_health": answers(ownerNobody),
	"automation_run": answers(ownerNobody),
	// A fold carries its members' common answer, or none when they differ.
	"batch": answers(ownerUnstated, ownerNamed, ownerShared, ownerNobody, ownerTheReader, ownerFromTheDeal, ownerUnreadable),
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
// scope, counts, Focus and paging read them, so all of them see one answer.
func admitBySource(ctx context.Context, rows []ranked) []ranked {
	kept := rows[:0:0]
	for _, row := range rows {
		allowed, known := sourceOwnerAnswers[row.item.Source]
		if !known {
			slog.ErrorContext(ctx, "a worklist source has no admission policy; its rows create no work",
				"source", row.item.Source)
			continue
		}
		if !allowed[row.ownerRef.kind] {
			slog.ErrorContext(ctx, "a worklist row claimed an owner its source may not give; it is unassigned",
				"source", row.item.Source, "owner_kind", int(row.ownerRef.kind))
			row.ownerRef = unassigned()
		}
		kept = append(kept, row)
	}
	return kept
}
