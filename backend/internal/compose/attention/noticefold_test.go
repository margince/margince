// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDecisionAnnouncementsFoldBeforeHomeCountsAndWorklistPages(t *testing.T) {
	decision := approval("Send the agreed proposal")
	decisionID := ids.From[ids.ApprovalKind](ids.UUID(decision.Id))
	announcement := ids.NewV7()
	unrelated := ids.NewV7()
	svc := noticesLaneService(&stubNotices{rows: []UnreadNotice{
		{ID: announcement, ApprovalID: &decisionID, Kind: "approval_pending", Subject: "Review the proposal", CreatedAt: readInstant},
		{ID: unrelated, Kind: "automation", Subject: "A different update", CreatedAt: readInstant},
	}})
	svc.approvals = stubApprovals{rows: []crmcontracts.Approval{decision}}
	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	if day.Counts.NeedsYou != 1 || day.Counts.Notices == nil || *day.Counts.Notices != 1 {
		t.Fatalf("duplicate counts: %+v", day.Counts)
	}
	seen := map[string]bool{}
	cursor := ""
	for range 3 {
		page, err := svc.Worklist(pageReader(), "all", "all", ids.Nil, 1, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page.Queue {
			if seen[row.Id] || row.Id == announcement.String() {
				t.Fatalf("duplicate projection %s", row.Id)
			}
			seen[row.Id] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if !seen[decisionID.String()] || seen[unrelated.String()] || len(seen) != 1 {
		t.Fatalf("paged obligations: %v", seen)
	}
}

func TestAnnouncementWithoutAnInScopeDecisionRemainsReachable(t *testing.T) {
	id := ids.From[ids.ApprovalKind](ids.NewV7())
	svc := noticesLaneService(&stubNotices{rows: []UnreadNotice{{ID: ids.NewV7(), ApprovalID: &id, Kind: "approval_pending", Subject: "Review", CreatedAt: readInstant}}})
	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	if day.Notices == nil || len(*day.Notices) != 1 {
		t.Fatal("unloaded decision lost its announcement")
	}
}

func TestFoldedAnnouncementsPreserveTheNoticeReadBoundary(t *testing.T) {
	decision := approval("Review the proposal")
	decisionID := ids.From[ids.ApprovalKind](ids.UUID(decision.Id))
	rows := make([]UnreadNotice, doneCap)
	for i := range rows {
		rows[i] = UnreadNotice{ID: ids.NewV7(), ApprovalID: &decisionID, Kind: "approval_pending", CreatedAt: readInstant}
	}
	svc := noticesLaneService(&stubNotices{rows: rows})
	svc.approvals = stubApprovals{rows: []crmcontracts.Approval{decision}}
	page, err := svc.Worklist(pageReader(), "all", "all", ids.Nil, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Queue) != 1 || page.Readings.MoreAvailable {
		t.Fatalf("folded bounded notices lost coverage: %+v", page.Readings)
	}
	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	if day.NoticesMoreAvailable == nil || !*day.NoticesMoreAvailable || day.Notices == nil || len(*day.Notices) != 0 {
		t.Fatal("Home lost the coverage of its folded notices")
	}
}

type pagedApprovalFixture struct{ stubApprovals }

func (s pagedApprovalFixture) ListWire(_ context.Context, query ApprovalQuery) ([]crmcontracts.Approval, error) {
	return s.rows[:min(len(s.rows), query.Limit)], nil
}

func TestADecisionBeyondTheHomePageStillOwnsItsAnnouncement(t *testing.T) {
	decisions := make([]crmcontracts.Approval, needsYouPage+1)
	for i := range decisions {
		decisions[i] = approval("Review this customer's promise")
	}
	last := ids.From[ids.ApprovalKind](ids.UUID(decisions[len(decisions)-1].Id))
	svc := noticesLaneService(&stubNotices{rows: []UnreadNotice{{ID: ids.NewV7(), ApprovalID: &last, Kind: "approval_pending", CreatedAt: readInstant}}})
	svc.approvals = pagedApprovalFixture{stubApprovals{rows: decisions}}
	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	if day.Counts.NeedsYou != len(decisions) || len(day.NeedsYou) != needsYouPage {
		t.Fatal("fixture did not cross the decision page bound")
	}
	if day.Counts.Notices == nil || *day.Counts.Notices != 0 {
		t.Fatal("an unloaded decision was counted again as a notice")
	}
}

func TestBatchedDecisionsDoNotBringBackTheirAnnouncements(t *testing.T) {
	var decisions []crmcontracts.Approval
	var announcements []UnreadNotice
	for range batchFloor + 1 {
		decision := captureApproval("anna.weber@customer.example")
		id := ids.From[ids.ApprovalKind](ids.UUID(decision.Id))
		decisions = append(decisions, decision)
		announcements = append(announcements, UnreadNotice{
			ID: ids.NewV7(), ApprovalID: &id, Kind: "approval_pending",
			Subject: "Review the contact", CreatedAt: readInstant,
		})
	}
	svc := noticesLaneService(&stubNotices{rows: announcements})
	svc.approvals = stubApprovals{rows: decisions}
	page, err := svc.Worklist(pageReader(), "all", "all", ids.Nil, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Queue) != 1 || page.Queue[0].Batch == nil || page.Queue[0].Batch.Count != len(decisions) {
		t.Fatalf("decisions did not form their expected batch: %+v", page.Queue)
	}
	if page.NextCursor != nil || itemsOn(page.Queue) != len(decisions) {
		t.Fatal("announcements added a second page or inflated the batch count")
	}
}

func TestInformationCannotBecomeWorkThroughAnotherLane(t *testing.T) {
	t.Parallel()
	day := crmcontracts.Attention{NeedsYou: []crmcontracts.AttentionItem{
		{Source: sourceNotice, Kind: new("capture_backlog_stalled")},
		{Source: sourceDomainQuestion},
		{Source: crmcontracts.AttentionItemSource(sourceDecay)},
	}}
	if rows := classifyDay(day, rankInstant, dayMoney{}); len(rows) != 0 {
		t.Fatalf("information became personal obligations: %+v", rows)
	}
}
