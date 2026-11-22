// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"reflect"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
		t.Run(scopeNames[scope], func(t *testing.T) {
			service := decisionsService(stubApprovals{rows: approvals}, stubDuplicates{pairs: pairs, open: len(pairs), ownerless: true}).
				WithTagSuggestions(&stubTagSuggestions{rows: tags})
			service.taskScope = scope
			service.taskOwner = ids.NewV7()
			rows, count, err := service.decisionsToDepth(pageReader(), needsYouPage)
			if err != nil {
				t.Fatal(err)
			}
			seen := tallyBySource(rows)
			if scope == TasksMine || scope == TasksOwnedBy {
				if len(rows) != needsYouPage || seen["approval"] != needsYouPage || count.items != needsYouPage || count.duplicates != 0 || count.tagSuggestions != nil {
					t.Fatalf("unassigned decisions consumed a personal page or count: sources=%v, count=%+v", seen, count)
				}
			} else if count.items != needsYouPage*3 || seen[sourceDuplicate] == 0 || seen[sourceTagSuggestion] == 0 {
				t.Fatalf("unassigned work disappeared from its wider queue: sources=%v, count=%+v", seen, count)
			}
		})
	}
}

// gatekit:fixture the label each scope prints in a failure message
var scopeNames = map[TaskScope]string{
	TasksMine: "mine", TasksOwnedBy: "named member", TasksVisible: "visible", TasksUnassigned: "unassigned",
}

// A suggestion is the company owner's to decide, so every scope reads it
// narrowed to the owners recordOwners names, list and count alike.
func TestSuggestionReadsReceiveTheScopeOwners(t *testing.T) {
	named, teammate := ids.NewV7(), ids.NewV7()
	reader := pageReader()
	actor, ok := principal.Actor(reader)
	if !ok {
		t.Fatal("the page reader carries no actor")
	}
	for _, tc := range []struct {
		name   string
		scope  TaskScope
		team   []ids.UUID
		owners *storekit.OwnerSet
	}{
		{"mine", TasksMine, nil, &storekit.OwnerSet{IDs: []ids.UUID{actor.UserID}}},
		{"named member", TasksOwnedBy, nil, &storekit.OwnerSet{IDs: []ids.UUID{named}}},
		{"unassigned", TasksUnassigned, nil, &storekit.OwnerSet{Unassigned: true}},
		{"team", TasksVisible, []ids.UUID{teammate}, &storekit.OwnerSet{IDs: []ids.UUID{teammate}, Unassigned: true}},
		{"all", TasksVisible, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suggestions := &stubSuggestions{rows: suggestionRows(2), open: 40}
			service := decisionsService(stubApprovals{}, stubDuplicates{}).WithDealSuggestions(suggestions)
			service.taskScope, service.taskOwner, service.noticeOwners = tc.scope, named, tc.team
			_, count, err := service.decisionsToDepth(reader, needsYouPage)
			if err != nil {
				t.Fatal(err)
			}
			for read, seen := range map[string]*ownerSetSeen{"list": suggestions.listed, "count": suggestions.counted} {
				if seen == nil || !reflect.DeepEqual(seen.owners, tc.owners) {
					t.Fatalf("the %s read was narrowed to %+v, want %+v", read, seen, tc.owners)
				}
			}
			if count.suggestions == nil || *count.suggestions != 40 {
				t.Fatalf("suggestion count = %v, want 40", count.suggestions)
			}
		})
	}

	t.Run("mine with no human behind it", func(t *testing.T) {
		suggestions := &stubSuggestions{rows: suggestionRows(2)}
		service := decisionsService(stubApprovals{}, stubDuplicates{}).WithDealSuggestions(suggestions)
		service.taskScope = TasksMine
		rows, count, err := service.decisionsToDepth(context.Background(), needsYouPage)
		if err != nil {
			t.Fatal(err)
		}
		if suggestions.listed != nil || suggestions.counted != nil || len(rows) != 0 || count.suggestions != nil {
			t.Fatalf("a seat with no human read suggestions: rows=%d, count=%+v", len(rows), count)
		}
	})
}

func TestPersonalDecisionsDoNotDependOnUnassignedProducerReads(t *testing.T) {
	unavailable := errors.New("unassigned source unavailable")
	for _, scope := range []TaskScope{TasksMine, TasksOwnedBy} {
		service := decisionsService(stubApprovals{rows: []crmcontracts.Approval{approval("A requested decision")}}, stubDuplicates{ownerless: true, err: unavailable, settleErr: unavailable}).
			WithDealSuggestions(&stubSuggestions{err: unavailable}).
			WithTagSuggestions(&stubTagSuggestions{err: unavailable})
		service.taskScope = scope
		rows, count, err := service.decisionsToDepth(pageReader(), needsYouPage)
		if err != nil || len(rows) != 1 || count.items != 1 {
			t.Fatalf("personal decision depended on unassigned reads: rows=%d, count=%+v, err=%v", len(rows), count, err)
		}
		// The owner's own suggestions are read on a personal page, so their
		// failure is named; the unassigned producers are never asked.
		if len(count.failed) != 1 || count.failed[0].Source != sourceDealSuggestion {
			t.Fatalf("sources unavailable = %+v, want only deal_suggestion", count.failed)
		}
	}
}

// Each scope reads the duplicate pairs narrowed to the owners it is for, list
// and count alike. The team view reads them all.
func TestPairReadsReceiveTheScopeOwners(t *testing.T) {
	named := ids.NewV7()
	reader := pageReader()
	actor, ok := principal.Actor(reader)
	if !ok {
		t.Fatal("the page reader carries no actor")
	}
	for _, tc := range []struct {
		name   string
		scope  TaskScope
		owners *storekit.OwnerSet
	}{
		{"mine", TasksMine, &storekit.OwnerSet{IDs: []ids.UUID{actor.UserID}}},
		{"named member", TasksOwnedBy, &storekit.OwnerSet{IDs: []ids.UUID{named}}},
		{"unassigned", TasksUnassigned, &storekit.OwnerSet{Unassigned: true}},
		{"team", TasksVisible, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var asked []*storekit.OwnerSet
			service := decisionsService(stubApprovals{}, stubDuplicates{owners: &asked})
			service.taskScope, service.taskOwner = tc.scope, named
			if _, _, err := service.decisionsToDepth(reader, needsYouPage); err != nil {
				t.Fatal(err)
			}
			if len(asked) != 2 {
				t.Fatalf("the pair store was read %d times, want a list and a count", len(asked))
			}
			for _, got := range asked {
				if !reflect.DeepEqual(got, tc.owners) {
					t.Fatalf("pairs were read narrowed to %+v, want %+v", got, tc.owners)
				}
			}
		})
	}
}
