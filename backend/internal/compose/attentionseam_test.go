// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The receipt lane makes a claim about WHO acted, and the claim is the whole
// point of the lane: "done for you" says the system handled something so the
// reader did not have to. Telling somebody that about a decision they made
// themselves is not a cosmetic error — it misreports their own work back to
// them, and it inflates what the product looks like it is doing.
//
// The test is decided_by IS NULL, the convention the expiry sweep states in its
// own words ("decided_by stays NULL and the actor is the system: nobody
// decided"). Status alone cannot tell the two apart: a human approval and a
// system one are both `approved`.
func TestAReceiptIsSomethingNobodyWasAskedAbout(t *testing.T) {
	decidedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	since := decidedAt.Add(-time.Hour)
	human := openapi_types.UUID(ids.NewV7())

	byTheSystem := approvalRow("Moved the Acme close date to 27 Sep", decidedAt, nil)
	byTheReader := approvalRow("Sent the Weber follow-up", decidedAt, &human)

	receipts := receiptsWithin([]crmcontracts.Approval{byTheSystem, byTheReader}, since)

	if len(receipts) != 1 {
		t.Fatalf("the lane carries %d receipts, want only the system's one", len(receipts))
	}
	if receipts[0].Summary != "Moved the Acme close date to 27 Sep" {
		t.Errorf("the lane kept %q — a reader's own decision is not something that ran for them", receipts[0].Summary)
	}
}

func approvalRow(summary string, decidedAt time.Time, decidedBy *openapi_types.UUID) crmcontracts.Approval {
	return crmcontracts.Approval{
		Id:        openapi_types.UUID(ids.NewV7()),
		Kind:      "close_date_correction",
		Summary:   &summary,
		DecidedAt: &decidedAt,
		DecidedBy: decidedBy,
	}
}

// The receipts read reaches past the reader's own approvals.
//
// The filter that decides a receipt — decided_by IS NULL — runs AFTER the page
// is read, because the engine can only page by status. So the SIZE of that read
// decides whether an autonomous act is visible at all: a read the size of the
// lane is filled by the reader's own recent decisions, filters down to nothing,
// and the lane reports a quiet night while dozens of things ran.
//
// The bug was invisible while nothing was auto-applied: there was never an
// autonomous act to crowd out. The stub answers a page of the reader's own
// decisions with ONE autonomous act behind them, so a read that stops at the
// lane's width finds nothing.
func TestTheReceiptsReadIsWiderThanTheLaneItFills(t *testing.T) {
	decidedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	human := openapi_types.UUID(ids.NewV7())

	page := make([]crmcontracts.Approval, 0, doneLaneWidth+2)
	for i := 0; i < doneLaneWidth+1; i++ {
		page = append(page, approvalRow("A decision the reader made", decidedAt, &human))
	}
	page = append(page, approvalRow("Filed a message under Riverty", decidedAt, nil))

	engine := &stubApprovalPage{rows: page}
	receipts, err := recentReceipts(decidedAt.Add(-time.Hour), doneLaneWidth, engine.list)
	if err != nil {
		t.Fatalf("reading the receipts: %v", err)
	}
	if len(receipts) != 1 {
		t.Fatalf("the lane carries %d receipts; the read stopped at %d rows and never reached the autonomous act",
			len(receipts), engine.asked)
	}
}

// doneLaneWidth is what the day's surface shows in its receipt lane.
const doneLaneWidth = 8

// stubApprovalPage answers exactly the number of rows it is asked for, which is
// the whole subject: a seam that asks for the lane's width gets a page of human
// decisions and filters it to nothing.
type stubApprovalPage struct {
	rows  []crmcontracts.Approval
	asked int
}

func (s *stubApprovalPage) list(limit int) ([]crmcontracts.Approval, error) {
	s.asked = limit
	if limit > len(s.rows) {
		limit = len(s.rows)
	}
	return s.rows[:limit], nil
}
