// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubSuggestions struct {
	rows     []crmcontracts.DealSuggestion
	open     int
	err      error
	countErr error
}

func (s *stubSuggestions) OpenSuggestions(_ context.Context, limit int) ([]crmcontracts.DealSuggestion, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.rows[:min(limit, len(s.rows))], nil
}

func (s *stubSuggestions) CountOpen(context.Context) (int, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	if s.open > 0 {
		return s.open, nil
	}
	return len(s.rows), nil
}

func suggestionRow(hint crmcontracts.DealSuggestionNameHint) crmcontracts.DealSuggestion {
	return crmcontracts.DealSuggestion{
		Id: openapi_types.UUID(ids.NewV7()), Kind: "open_deal", State: "open", NameHint: hint,
		CompanyId: openapi_types.UUID(ids.NewV7()), CompanyName: "Acme", Confidence: 0.8,
		CreatedAt: readInstant,
	}
}

func suggestionRows(n int) []crmcontracts.DealSuggestion {
	rows := make([]crmcontracts.DealSuggestion, 0, n)
	for range n {
		rows = append(rows, suggestionRow("meeting_held"))
	}
	return rows
}

// decisionsService is a feed with only the decision producers bound.
func decisionsService(approvals stubApprovals, duplicates stubDuplicates) *Service {
	return NewService(approvals, duplicates, &stubTasks{}, stubReceipts{}, stubBriefing{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
}

func tallyBySource(lane []crmcontracts.AttentionItem) map[string]int {
	seen := map[string]int{}
	for _, item := range lane {
		seen[string(item.Source)]++
	}
	return seen
}

func TestAFloodOfSuggestionsBuriesNeitherDecisionsNorPairs(t *testing.T) {
	svc := decisionsService(
		stubApprovals{rows: []crmcontracts.Approval{approval("a staged send"), approval("another")}},
		stubDuplicates{pairs: []DuplicatePair{
			{ID: ids.NewV7(), EntityType: "contact", Confidence: 0.9, LeftID: ids.NewV7(), RightID: ids.NewV7()},
		}, open: 1},
	).WithDealSuggestions(&stubSuggestions{rows: suggestionRows(40)})

	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	seen := tallyBySource(day.NeedsYou)
	if len(day.NeedsYou) != needsYouPage || seen["approval"] != 2 || seen[sourceDuplicate] != 1 {
		t.Fatalf("the lane holds %v in %d rows, want both approvals and the pair among a full page", seen, len(day.NeedsYou))
	}
	if day.Counts.NeedsYou != 43 || day.Counts.DealSuggestionsOpen == nil || *day.Counts.DealSuggestionsOpen != 40 {
		t.Fatalf("counts = %d needing you, %v suggestions; want 43 and 40", day.Counts.NeedsYou, day.Counts.DealSuggestionsOpen)
	}
	if !boundedSources(day)[sourceDealSuggestion] {
		t.Fatal("forty suggestions shown as a page of fewer did not report the lane truncated")
	}
}

func TestASuggestionRowOffersItsVerbsAndNamesTheCompany(t *testing.T) {
	row := suggestionRow("proposal_sent")
	item := suggestionItem(row)
	if item.Subject == nil || item.Subject.Type != subjectCompany || item.Subject.Id != row.CompanyId {
		t.Fatalf("subject = %+v, want the suggestion's company", item.Subject)
	}
	if item.Title != nil {
		t.Fatalf("title = %q, want none: the client words the row, so it repeats no evidence text", *item.Title)
	}
	if len(item.Actions) != 3 || item.Actions[0] != "decide" || item.Actions[1] != actionDismiss {
		t.Fatalf("actions = %v, want decide, dismiss and open", item.Actions)
	}
}

func TestAReaderWhoMayNotReadSuggestionsKeepsTheRestOfTheLane(t *testing.T) {
	svc := decisionsService(stubApprovals{rows: []crmcontracts.Approval{approval("a staged send")}}, stubDuplicates{}).
		WithDealSuggestions(&stubSuggestions{err: apperrors.ErrPermissionDenied})

	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(day.NeedsYou) != 1 || day.Counts.DealSuggestionsOpen != nil {
		t.Fatalf("lane = %d rows, suggestions count %v; want the approval and no suggestion count", len(day.NeedsYou), day.Counts.DealSuggestionsOpen)
	}
	if _, asked := boundedSources(day)[sourceDealSuggestion]; asked {
		t.Fatal("a source the reader may not read reported a truncation verdict")
	}
}

func TestABrokenSuggestionReadIsNamedFailedAndTheRestOfTheDayStillLoads(t *testing.T) {
	stalled := errors.New("canceling statement due to statement timeout")
	for name, suggestions := range map[string]*stubSuggestions{
		"the list fails": {err: stalled},
		"the list answers and then the count fails": {rows: suggestionRows(3), countErr: stalled},
	} {
		t.Run(name, func(t *testing.T) {
			svc := decisionsService(stubApprovals{rows: []crmcontracts.Approval{approval("a staged send")}}, stubDuplicates{}).
				WithDealSuggestions(suggestions)

			day, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25, "")
			if err != nil {
				t.Fatalf("a stalled suggestion read failed the whole worklist: %v", err)
			}
			if seen := tallyByWorklistSource(day.Queue); len(day.Queue) != 1 || seen[sourceDealSuggestion] != 0 {
				t.Fatalf("the queue carries %v, want only the staged approval: a failed read shows no suggestions", seen)
			}
			named := false
			for _, missing := range day.SourcesUnavailable {
				if missing.Source == sourceDealSuggestion && missing.Reason == crmcontracts.WorklistSourceUnavailableReasonFailed {
					named = true
				}
			}
			if !named {
				t.Fatalf("sources_unavailable = %+v, want deal_suggestion named as failed", day.SourcesUnavailable)
			}

			lanes, err := svc.Assemble(pageReader())
			if err != nil {
				t.Fatalf("a stalled suggestion read failed the lane feed: %v", err)
			}
			if lanes.Counts.DealSuggestionsOpen != nil || tallyBySource(lanes.NeedsYou)[sourceDealSuggestion] != 0 {
				t.Fatalf("lane = %v, suggestion count %v; want neither: a read that failed must not print a confident zero",
					tallyBySource(lanes.NeedsYou), lanes.Counts.DealSuggestionsOpen)
			}
		})
	}
}

func tallyByWorklistSource(queue []crmcontracts.WorklistItem) map[string]int {
	seen := map[string]int{}
	for _, item := range queue {
		seen[string(item.Source)]++
	}
	return seen
}
