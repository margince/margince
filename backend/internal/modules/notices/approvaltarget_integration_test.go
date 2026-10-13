// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A notice asking for a decision opens the decision, whatever record it was
// written about; any other notice keeps its record.
func TestTheCentreOpensTheDecisionAnApprovalNoticeAsksFor(t *testing.T) {
	e := setupNotices(t)
	approval := ids.NewV7()
	asking, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: KindApprovalPending, Subject: "Is boris@customer.example a contact worth keeping?",
		Target: Target{Type: "activity", ID: ids.NewV7()}, DedupeKey: ApprovalNoticeKey(approval),
	})
	if err != nil {
		t.Fatalf("seeding the approval notice: %v", err)
	}
	deal := ids.NewV7()
	telling, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "automation", Subject: "A deal you own changed stage",
		Target: Target{Type: "deal", ID: deal}, DedupeKey: "stage_change_notify:" + ids.NewV7().String(),
	})
	if err != nil {
		t.Fatalf("seeding the record notice: %v", err)
	}

	page, err := e.store.ListFor(e.asUser(e.recipient), 10, "")
	if err != nil {
		t.Fatalf("ListFor: %v", err)
	}
	targets := map[ids.UUID]Target{}
	for _, item := range page.Items {
		targets[item.ID] = item.Target
	}
	if got := targets[asking]; got != (Target{Type: TargetApproval, ID: approval}) {
		t.Errorf("the approval notice opens %+v, want the decision %v", got, approval)
	}
	if got := targets[telling]; got != (Target{Type: "deal", ID: deal}) {
		t.Errorf("the record notice opens %+v, want its deal %v", got, deal)
	}
}
