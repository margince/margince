// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCurrentAdviceReadsEveryCandidateOnceForQueueAndFocus(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	item := func(id ids.UUID) crmcontracts.AttentionItem {
		return crmcontracts.AttentionItem{Subject: &crmcontracts.AttentionSubject{Type: subjectDeal, Id: openapi_types.UUID(id)}}
	}
	risk := []crmcontracts.AttentionItem{item(second), item(first)}
	day := crmcontracts.Attention{ThisMorning: []crmcontracts.AttentionItem{item(first)}, AtRisk: &risk}
	boundary := &stubDealMoves{moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
		first: cardMove("create_task", nil), second: cardMove("open_task", map[string]any{"activity_id": ids.NewV7()}),
	}}
	svc := (&Service{}).WithDealMoves(boundary)
	scoped, missing := svc.readingDealMoves(context.Background(), day)
	if missing != nil || !slices.Equal(boundary.asked, []ids.UUID{first, second}) {
		t.Fatalf("candidate advice = %v, missing = %+v", boundary.asked, missing)
	}
	for _, id := range []ids.UUID{second, first} {
		rows := []crmcontracts.WorklistItem{riskRow(id)}
		if err := scoped.nameTheStep(context.Background(), rows); err != nil || rows[0].Move == nil {
			t.Fatalf("page/focus lost prepared current advice: %+v, %v", rows, err)
		}
	}
	if boundary.calls != 1 || svc.currentDealMoves != nil {
		t.Fatalf("advice escaped its per-request batch: calls=%d", boundary.calls)
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
			id := ids.NewV7()
			day := crmcontracts.Attention{ThisMorning: []crmcontracts.AttentionItem{{
				Subject: &crmcontracts.AttentionSubject{Type: subjectDeal, Id: openapi_types.UUID(id)},
			}}}
			svc := (&Service{}).WithDealMoves(&stubDealMoves{err: refusal.err})
			scoped, missing := svc.readingDealMoves(context.Background(), day)
			if missing == nil || missing.Source != sourceDealAdvice || missing.ContributesRows == nil || *missing.ContributesRows {
				t.Fatalf("advice masquerades as a required row source: %+v", missing)
			}
			if (missing.Reason == "withheld") != refusal.withheld {
				t.Fatalf("advice failure lost its cause: %+v", missing)
			}
			unavailable := stampSources([]crmcontracts.WorklistSourceUnavailable{{Source: sourceAtRisk, Reason: "failed"}, *missing})
			if unavailable[0].Source == unavailable[1].Source || !*unavailable[0].ContributesRows || *unavailable[1].ContributesRows {
				t.Fatalf("side-read status changed the lane's coverage: %+v", unavailable)
			}
			rows := []crmcontracts.WorklistItem{riskRow(id)}
			if err := scoped.nameTheStep(context.Background(), rows); err != nil || len(rows) != 1 || rows[0].Move != nil {
				t.Fatalf("unavailable advice removed underlying work: %+v, %v", rows, err)
			}
		})
	}
}
