// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestUnassignedDecisionFloodCannotConsumeAPersonalDecisionPage(t *testing.T) {
	approvals := make([]crmcontracts.Approval, needsYouPage)
	pairs := make([]DuplicatePair, needsYouPage)
	tags := make([]crmcontracts.TagSuggestion, needsYouPage)
	for i := range needsYouPage {
		approvals[i] = approval("A requested decision")
		pairs[i] = DuplicatePair{ID: ids.NewV7(), EntityType: "contact", LeftID: ids.NewV7(), RightID: ids.NewV7()}
		tags[i] = tagSuggestionRow(crmcontracts.TagSuggestionEntityTypeCompany)
	}
	for _, scope := range []TaskScope{TasksMine, TasksOwnedBy, TasksVisible, TasksUnassigned} {
		t.Run(map[TaskScope]string{TasksMine: "mine", TasksOwnedBy: "named member", TasksVisible: "visible", TasksUnassigned: "unassigned"}[scope], func(t *testing.T) {
			service := decisionsService(stubApprovals{rows: approvals}, stubDuplicates{pairs: pairs, open: len(pairs)}).
				WithDealSuggestions(&stubSuggestions{rows: suggestionRows(needsYouPage)}).
				WithTagSuggestions(&stubTagSuggestions{rows: tags})
			service.taskScope = scope
			service.taskOwner = ids.NewV7()
			rows, count, err := service.decisionsToDepth(pageReader(), needsYouPage)
			if err != nil {
				t.Fatal(err)
			}
			seen := tallyBySource(rows)
			if scope == TasksMine || scope == TasksOwnedBy {
				if len(rows) != needsYouPage || seen["approval"] != needsYouPage || count.items != needsYouPage || count.duplicates != 0 || count.suggestions != nil || count.tagSuggestions != nil {
					t.Fatalf("unassigned decisions consumed a personal page or count: sources=%v, count=%+v", seen, count)
				}
			} else if count.items != needsYouPage*4 || seen[sourceDuplicate] == 0 || seen[sourceDealSuggestion] == 0 || seen[sourceTagSuggestion] == 0 {
				t.Fatalf("unassigned work disappeared from its wider queue: sources=%v, count=%+v", seen, count)
			}
		})
	}
}

func TestPersonalDecisionsDoNotDependOnUnassignedProducerReads(t *testing.T) {
	unavailable := errors.New("unassigned source unavailable")
	for _, scope := range []TaskScope{TasksMine, TasksOwnedBy} {
		service := decisionsService(stubApprovals{rows: []crmcontracts.Approval{approval("A requested decision")}}, stubDuplicates{err: unavailable, settleErr: unavailable}).
			WithDealSuggestions(&stubSuggestions{err: unavailable}).
			WithTagSuggestions(&stubTagSuggestions{err: unavailable})
		service.taskScope = scope
		rows, count, err := service.decisionsToDepth(pageReader(), needsYouPage)
		if err != nil || len(rows) != 1 || count.items != 1 || len(count.failed) != 0 {
			t.Fatalf("personal decision depended on unassigned reads: rows=%d, count=%+v, err=%v", len(rows), count, err)
		}
	}
}
