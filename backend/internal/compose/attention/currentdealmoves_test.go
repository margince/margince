// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCurrentAdviceReadsThePageDealsOnceForQueueAndFocus(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	boundary := &stubDealMoves{moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
		first: cardMove("create_task", nil), second: cardMove("open_task", map[string]any{"activity_id": ids.NewV7()}),
	}}
	svc := (&Service{}).WithDealMoves(boundary)
	page := crmcontracts.Worklist{
		Queue: []crmcontracts.WorklistItem{riskRow(first), riskRow(second)},
		Focus: &crmcontracts.WorklistFocus{Items: []crmcontracts.WorklistItem{riskRow(first)}},
	}
	if err := svc.nameWorklistRows(context.Background(), &page, nil); err != nil {
		t.Fatalf("naming: %v", err)
	}
	if boundary.calls != 1 || !slices.Equal(boundary.asked, []ids.UUID{first, second}) {
		t.Fatalf("advice asked = %v over %d calls, want the page's two deals in one", boundary.asked, boundary.calls)
	}
	for _, row := range append(page.Queue, page.Focus.Items...) {
		if row.Move == nil {
			t.Fatalf("a page row lost its current advice: %+v", row)
		}
	}
	if len(page.SourcesUnavailable) != 0 || svc.currentDealMoves != nil {
		t.Fatalf("advice escaped its per-request batch: %+v", page.SourcesUnavailable)
	}
}

func TestUnavailableAdviceKeepsTheRiskLaneCompletenessSeparate(t *testing.T) {
	for _, refusal := range []struct {
		name     string
		err      error
		withheld bool
	}{
		{"withheld", apperrors.ErrPermissionDenied, true},
		{"failed", errors.New("fixture database unavailable"), false},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			svc := (&Service{}).WithDealMoves(&stubDealMoves{err: refusal.err})
			page := crmcontracts.Worklist{
				Queue:              []crmcontracts.WorklistItem{riskRow(ids.NewV7())},
				SourcesUnavailable: []crmcontracts.WorklistSourceUnavailable{{Source: sourceAtRisk, Reason: "failed"}},
			}
			page.SourcesUnavailable = svc.stampSources(page.SourcesUnavailable)
			if err := svc.nameWorklistRows(context.Background(), &page, nil); err != nil {
				t.Fatalf("unavailable advice failed the page: %v", err)
			}
			if len(page.Queue) != 1 || page.Queue[0].Move != nil {
				t.Fatalf("unavailable advice changed the underlying work: %+v", page.Queue)
			}
			unavailable := page.SourcesUnavailable
			if len(unavailable) != 2 || unavailable[1].Source != sourceDealAdvice {
				t.Fatalf("advice gap not named on the page: %+v", unavailable)
			}
			if (unavailable[1].Reason == "withheld") != refusal.withheld {
				t.Fatalf("advice failure lost its cause: %+v", unavailable[1])
			}
			if !*unavailable[0].ContributesRows || *unavailable[1].ContributesRows {
				t.Fatalf("side-read status changed the lane's coverage: %+v", unavailable)
			}
		})
	}
}
