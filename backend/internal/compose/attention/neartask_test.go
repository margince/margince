// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A deal at risk with a task about it due within nearTaskDays is one row: the
// task, carrying the deal's grounds. A task due later, or undated, leaves both.
func TestANearTaskStandsInForItsDealAtRisk(t *testing.T) {
	cases := []struct {
		name   string
		due    *time.Time
		folded bool
	}{
		{"overdue", new(rankInstant.AddDate(0, 0, -3)), true},
		{"due in three days", new(rankInstant.AddDate(0, 0, 3)), true},
		{"due just inside the window", new(nearTaskCutoff(rankInstant).Add(-time.Minute)), true},
		{"due at the window's end", new(nearTaskCutoff(rankInstant)), false},
		{"due in eight days", new(rankInstant.AddDate(0, 0, 8)), false},
		{"undated", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deal := ids.NewV7()
			onDeal := func(i *crmcontracts.AttentionItem) {
				i.Subject = &crmcontracts.AttentionSubject{Type: subjectDeal, Id: openapi_types.UUID(deal)}
			}
			risk := []crmcontracts.AttentionItem{item(deal.String(), "deal_at_risk", onDeal)}
			task := item(ids.NewV7().String(), "task", onDeal, func(i *crmcontracts.AttentionItem) { i.DueAt = tc.due })
			day := crmcontracts.Attention{AsOf: rankInstant, Planned: []crmcontracts.AttentionItem{task}, AtRisk: &risk}

			got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, waitingRead{}, worklistCursor{}, nil)

			sources := map[crmcontracts.WorklistItemSource]int{}
			for _, row := range got.Queue {
				sources[row.Source]++
			}
			if sources[sourceTask] != 1 {
				t.Fatalf("task rows = %d, want 1", sources[sourceTask])
			}
			if folded := sources[sourceAtRisk] == 0; folded != tc.folded {
				t.Fatalf("at-risk rows = %d, folded = %v, want folded = %v", sources[sourceAtRisk], folded, tc.folded)
			}
		})
	}
}

// The task that absorbs a deal's at-risk row keeps what that row would have
// said, so the reader still learns the deal is drifting.
func TestTheNearTaskCarriesTheDealsGrounds(t *testing.T) {
	deal := ids.NewV7()
	subject := &crmcontracts.AttentionSubject{Type: subjectDeal, Id: openapi_types.UUID(deal)}
	risk := ranked{item: crmcontracts.WorklistItem{
		Id: deal.String(), Source: sourceAtRisk, Subject: subject,
		Because: []crmcontracts.WorklistReason{reason(crmcontracts.WorklistReasonKindQuietDays, nil)},
	}}
	due := rankInstant.AddDate(0, 0, 2)
	task := classifyTask(crmcontracts.AttentionItem{Id: "t", Source: "task", Subject: subject, DueAt: &due}, rankInstant)

	rows := dropDealsAlreadyInHand([]ranked{risk, task}, rankInstant)

	if len(rows) != 1 || rows[0].item.Source != sourceTask {
		t.Fatalf("rows = %+v, want the task alone", rows)
	}
	for _, because := range rows[0].item.Because {
		if because.Kind == crmcontracts.WorklistReasonKindQuietDays {
			return
		}
	}
	t.Fatalf("the task row lost the deal's ground: %+v", rows[0].item.Because)
}
