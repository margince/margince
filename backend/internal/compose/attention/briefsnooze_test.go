// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASnoozedDealLeavesTheReadersRiskListAndCount(t *testing.T) {
	deal := ids.NewV7()
	risk := item(deal.String(), "deal_at_risk", withDeal(90_000_00), withDealSubject(deal))
	withDue(rankInstant)(&risk)
	day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: lane(risk)}
	service := &Service{briefSnoozed: map[ids.UUID]bool{deal: true}}
	before := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)
	after := service.worklistFrom(context.Background(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)
	if len(before.Queue) != 1 {
		t.Fatalf("risk fixture has %d rows", len(before.Queue))
	}
	for _, count := range after.Counts {
		if count.Considered != 0 {
			t.Fatalf("snoozed work still counts: %+v", count)
		}
	}
	if len(after.Queue) != 0 {
		t.Fatalf("snoozed deal still has %d rows", len(after.Queue))
	}
}

func TestASnoozedDealAlsoLeavesTheRiskTailAndItsDoneCandidates(t *testing.T) {
	deal := ids.NewV7()
	service := &Service{briefSnoozed: map[ids.UUID]bool{deal: true}}
	elsewhere, err := service.dealsElsewhere(context.Background(), rankInstant, []ids.UUID{deal}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !elsewhere[deal] {
		t.Fatal("the tail and Done can still offer the snoozed deal")
	}
	service.taskOwner = ids.NewV7()
	elsewhere, err = service.dealsElsewhere(context.Background(), rankInstant, []ids.UUID{deal}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if elsewhere[deal] {
		t.Fatal("a reader's snooze hid a named colleague's work")
	}
}
