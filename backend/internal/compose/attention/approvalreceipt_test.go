// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SQL picks the system's decisions by decided_by_system. What is left here is
// the lane's horizon: a receipt older than the window was news on a morning
// the reader has already seen.
func TestAReceiptOlderThanTheWindowIsNotTodaysNews(t *testing.T) {
	since := time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC)
	thisMorning := approvalRow("Moved the Acme close date to 27 Sep", since.Add(time.Hour))
	lastWeek := approvalRow("Filed a message under Riverty", since.AddDate(0, 0, -7))

	receipts := ReceiptsWithin([]crmcontracts.Approval{thisMorning, lastWeek}, since)

	if len(receipts) != 1 {
		t.Fatalf("the lane carries %d receipts, want only the one inside the window", len(receipts))
	}
	if receipts[0].Summary != "Moved the Acme close date to 27 Sep" {
		t.Errorf("the lane kept %q, want this morning's act", receipts[0].Summary)
	}
}

// A row with no decision time cannot be placed against the window. A receipt
// the reader cannot date is not a receipt.
func TestAnUndatedDecisionIsNotAReceipt(t *testing.T) {
	since := time.Date(2026, 8, 25, 7, 0, 0, 0, time.UTC)
	undated := crmcontracts.Approval{
		Id: openapi_types.UUID(ids.NewV7()), Kind: "close_date_correction",
	}

	if receipts := ReceiptsWithin([]crmcontracts.Approval{undated}, since); len(receipts) != 0 {
		t.Fatalf("the lane carries %d receipts, want none: nothing dates this row", len(receipts))
	}
}

func approvalRow(summary string, decidedAt time.Time) crmcontracts.Approval {
	return crmcontracts.Approval{
		Id:        openapi_types.UUID(ids.NewV7()),
		Kind:      "close_date_correction",
		Summary:   &summary,
		DecidedAt: &decidedAt,
	}
}

// The receipts read asks the store for the lane's width and no more. A wider
// read narrowed afterwards could fill a page with rows the lane then drops.
func TestTheReceiptsReadAsksForTheLaneItFills(t *testing.T) {
	decidedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	page := make([]crmcontracts.Approval, 0, doneLaneWidth)
	for range doneLaneWidth {
		page = append(page, approvalRow("Filed a message under Riverty", decidedAt))
	}

	engine := &stubApprovalPage{rows: page}
	receipts, _, err := RecentReceipts(decidedAt.Add(-time.Hour), doneLaneWidth, engine.list)
	if err != nil {
		t.Fatalf("reading the receipts: %v", err)
	}
	if engine.asked != doneLaneWidth {
		t.Errorf("the seam asked for %d rows to fill a lane of %d", engine.asked, doneLaneWidth)
	}
	if len(receipts) != doneLaneWidth {
		t.Fatalf("the lane carries %d receipts, want the %d the store answered", len(receipts), doneLaneWidth)
	}
}

// doneLaneWidth is what the day's surface shows in its receipt lane.
const doneLaneWidth = 8

// stubApprovalPage answers as many rows as it is asked for, so a test sees the
// width the read requested.
type stubApprovalPage struct {
	rows  []crmcontracts.Approval
	asked int
}

func (s *stubApprovalPage) list(_ string, limit int) ([]crmcontracts.Approval, storekit.Page, error) {
	s.asked = limit
	if limit > len(s.rows) {
		limit = len(s.rows)
	}
	return s.rows[:limit], storekit.Page{}, nil
}

// clampingInbox answers as the approvals inbox does: a page wider than it
// serves comes back at its default width, with a cursor to the rest.
type clampingInbox struct{ rows []crmcontracts.Approval }

func (c clampingInbox) list(cursor string, width int) ([]crmcontracts.Approval, storekit.Page, error) {
	if width > approvalPageWidth {
		width = 50
	}
	start := 0
	if cursor != "" {
		start = len(cursor)
	}
	end := min(start+width, len(c.rows))
	page := storekit.Page{HasMore: end < len(c.rows)}
	if page.HasMore {
		page.NextCursor = strings.Repeat("x", end)
	}
	return c.rows[start:end], page, nil
}

func TestTheReceiptsReadPagesPastTheInboxPageWidth(t *testing.T) {
	decidedAt := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	rows := make([]crmcontracts.Approval, 0, 450)
	for range 450 {
		rows = append(rows, approvalRow("Moved a deal", decidedAt))
	}
	receipts, more, err := RecentReceipts(decidedAt.Add(-time.Hour), 400, clampingInbox{rows: rows}.list)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 400 || !more {
		t.Fatalf("read %d receipts (more=%t), want 400 and more", len(receipts), more)
	}
	receipts, more, err = RecentReceipts(decidedAt.Add(-time.Hour), 1000, clampingInbox{rows: rows}.list)
	if err != nil || len(receipts) != 450 || more {
		t.Fatalf("read %d receipts (more=%t, err=%v), want all 450 and no more", len(receipts), more, err)
	}
}
