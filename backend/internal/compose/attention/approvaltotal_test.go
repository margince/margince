// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvalsByScope answers the count for the scope it is asked about, so a test
// can tell the reader's own total from a named owner's.
type approvalsByScope struct {
	rows      []crmcontracts.Approval
	forReader int
	forOwners map[ids.UUID]int
}

func (s approvalsByScope) ListWire(context.Context, ApprovalQuery) ([]crmcontracts.Approval, error) {
	return s.rows, nil
}

func (s approvalsByScope) CountPending(_ context.Context, in ApprovalQuery) (int, error) {
	if in.RequestedForReader {
		return s.forReader, nil
	}
	if len(in.RequestedForOwners) == 1 {
		return s.forOwners[in.RequestedForOwners[0]], nil
	}
	return 0, fmt.Errorf("unexpected approval scope %+v", in)
}

func (s approvalsByScope) PendingInScope(context.Context, ApprovalQuery, []ids.ApprovalID) (map[ids.ApprovalID]bool, error) {
	return map[ids.ApprovalID]bool{}, nil
}

func approvalItems(count int, kind string) []crmcontracts.AttentionItem {
	rows := make([]crmcontracts.AttentionItem, 0, count)
	for i := range count {
		rows = append(rows, item(fmt.Sprintf("approval-%d", i), "approval", withKind(kind), decidable()))
	}
	return rows
}

func TestTheDecisionsCountIsTheExactApprovalTotalPastTheReadDepth(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: approvalItems(batchScanDepth, "send_email")}
	day.Counts.ApprovalsOpen = new(batchScanDepth + 140)

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

	count := countFor(t, got, "decisions")
	if count.Total == nil || *count.Total != batchScanDepth+140 {
		t.Fatalf("decisions total = %v, want the engine's exact %d", count.Total, batchScanDepth+140)
	}
	if !count.MoreAvailable {
		t.Error("the page read 200 of 340 approvals and claimed it considered them all")
	}
}

func TestApprovalsBesideManyPairsAreNotAFloor(t *testing.T) {
	pairs := make([]crmcontracts.AttentionItem, 0, 150)
	for i := range 150 {
		pairs = append(pairs, item(fmt.Sprintf("pair-%d", i), sourceDuplicate, decidable()))
	}
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: append(pairs, approvalItems(50, "capture_counterparty")...),
	}
	day.Counts.DuplicatesOpen = new(150)
	day.Counts.ApprovalsOpen = new(50)

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

	groups := 0
	for _, card := range got.Queue {
		if card.Batch == nil {
			continue
		}
		groups++
		if card.Batch.AtLeast != nil && *card.Batch.AtLeast {
			t.Errorf("group %q of %d is marked as a floor, and every member was read", card.Batch.Key, card.Batch.Count)
		}
	}
	if groups != 2 {
		t.Fatalf("the fixture folded into %d groups, want the pairs and the approvals", groups)
	}
	count := countFor(t, got, "decisions")
	if count.MoreAvailable || count.Total == nil || *count.Total != 200 {
		t.Errorf("decisions = %+v, want an exact 200", count)
	}
}

func TestANamedOwnersQueueCountsTheirApprovalsNotTheReaders(t *testing.T) {
	rep := ids.NewV7()
	repID := openapi_types.UUID(rep)
	rows := make([]crmcontracts.Approval, 0, batchScanDepth)
	for range batchScanDepth {
		row := approval("Send the rep's follow-up")
		row.RequestedDeciderId = &repID
		rows = append(rows, row)
	}
	stub := approvalsByScope{rows: rows, forReader: 999, forOwners: map[ids.UUID]int{rep: 260}}
	svc := NewService(stub, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).forOwner(rep)

	day, _, err := svc.countingDecisions().assembleDay(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	got := svc.worklistFrom(pageReader(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

	count := countFor(t, got, "decisions")
	if count.Total == nil || *count.Total != 260 {
		t.Fatalf("the rep's decisions total = %v, want the rep's 260 and never the reader's 999", count.Total)
	}
}

func TestTheTeamScopeStatesNoApprovalTotal(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant}
	day.Counts.ApprovalsOpen = new(260)

	team := (&Service{}).exactSources(day, scopeTeam)
	if _, stated := team[crmcontracts.WorklistItemSourceApproval]; stated {
		t.Error("the team scope offered the lane's approval count, and its narrowing admits a different population")
	}
	for _, scope := range []string{scopeAll, scopeMine, scopeUnassigned} {
		if got := (&Service{}).exactSources(day, scope); got[crmcontracts.WorklistItemSourceApproval] != 260 {
			t.Errorf("scope %s dropped the approval count its narrowing matches: %v", scope, got)
		}
	}
	if owned := (&Service{taskOwner: ids.NewV7()}).exactSources(day, scopeTeam); len(owned) != 1 {
		t.Errorf("a named owner under the team word lost the owner's count: %v", owned)
	}
}

func TestOnlyTheGroupWhoseSourceRanShortIsAFloor(t *testing.T) {
	pairs := make([]crmcontracts.AttentionItem, 0, 150)
	for i := range 150 {
		pairs = append(pairs, item(fmt.Sprintf("pair-%d", i), sourceDuplicate, decidable()))
	}
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: append(pairs, approvalItems(50, "capture_counterparty")...),
	}
	day.Counts.DuplicatesOpen = new(400)
	day.Counts.ApprovalsOpen = new(50)

	got := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

	floors := map[crmcontracts.WorklistBatchKey]bool{}
	for _, card := range got.Queue {
		if card.Batch != nil {
			floors[card.Batch.Key] = card.Batch.AtLeast != nil && *card.Batch.AtLeast
		}
	}
	if !floors["duplicates"] {
		t.Error("150 of 400 pairs drew as an exact group")
	}
	if floors["uncertain_contact"] {
		t.Error("the approvals group took the pairs' floor, and all 50 approvals were read")
	}
	if total := countFor(t, got, "decisions").Total; total != nil {
		t.Errorf("decisions stated a total of %d over pairs it never finished reading", *total)
	}
}
