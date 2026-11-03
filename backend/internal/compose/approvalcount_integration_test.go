// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CountPending has two readers: the Home lane count and the digest. Past the
// engine's scan cap, both get the number a client counts over every page.
func TestHomeAndDigestApprovalCountsAreExactPastTheScanCap(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	svc := NewApprovalsService(e.DB())
	staged := approvals.PendingScanCap + 5
	for range staged {
		deal := e.SeedDeal(t, "Review date", pipeline, open, &e.AdminUser)
		proposal, err := json.Marshal(deals.CloseDateCorrection{DealID: ids.From[ids.DealKind](deal), ExpectedCloseDate: "2026-12-01", Basis: "Review the date"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: deals.CloseDateCorrectionKind, TargetType: "deal", TargetID: deal, ProposedChange: proposal, DiffHash: deal.String()}); err != nil {
			t.Fatal(err)
		}
	}
	status := string(crmcontracts.ApprovalStatusPending)
	in := approvals.ListInput{Status: &status, RequestedForReader: true, Limit: 50}
	paged := 0
	for range 20 {
		rows, page, err := svc.ListWire(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.Status == crmcontracts.ApprovalStatusPending {
				paged++
			}
		}
		if !page.HasMore {
			break
		}
		in.Cursor = page.NextCursor
	}
	if paged != staged {
		t.Fatalf("the inbox pages hold %d of the reader's %d proposals", paged, staged)
	}
	lane, err := (attentionApprovals{svc: svc}).CountPending(e.Admin(), attention.ApprovalQuery{Status: status, Limit: 9, RequestedForReader: true})
	if err != nil || lane != paged {
		t.Fatalf("Home lane count %d, pages hold %d: %v", lane, paged, err)
	}
	digest, err := newDigestReviewSource(e.Pool, svc)(e.Admin())
	if err != nil || digest.ApprovalsPending != paged {
		t.Fatalf("digest count %d, pages hold %d: %v", digest.ApprovalsPending, paged, err)
	}
}
