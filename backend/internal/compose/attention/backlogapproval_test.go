// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stagedNewestFirst is count approvals in the inbox's order, newest first.
func stagedNewestFirst(count int) []crmcontracts.Approval {
	rows := make([]crmcontracts.Approval, count)
	for i := range rows {
		rows[i] = approval("Send the follow-up")
		rows[i].CreatedAt = rankInstant.Add(-time.Duration(i) * time.Minute)
	}
	return rows
}

// When pairs share the lane's depth, the lane keeps only some approvals. The
// tail resumes right after the last one kept, so none is skipped or repeated.
func TestTheApprovalTailStartsAfterTheLastApprovalTheLaneKept(t *testing.T) {
	staged := stagedNewestFirst(5)
	pairs := []DuplicatePair{
		{ID: ids.NewV7(), EntityType: "contact", Confidence: 0.9},
		{ID: ids.NewV7(), EntityType: "contact", Confidence: 0.9},
		{ID: ids.NewV7(), EntityType: "contact", Confidence: 0.9},
	}
	svc := NewService(stubApprovals{rows: staged}, stubDuplicates{pairs: pairs, open: len(pairs)}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	lane, count, err := svc.decisionsToDepth(pageReader(), 4)
	if err != nil {
		t.Fatal(err)
	}

	if kept := shownFrom(lane, string(crmcontracts.WorklistItemSourceApproval)); kept != 2 {
		t.Fatalf("the lane kept %d approvals beside the pairs, want 2", kept)
	}
	want, err := approvals.PositionAfter(staged[1])
	if err != nil {
		t.Fatal(err)
	}
	if got := count.resumeAt[crmcontracts.WorklistItemSourceApproval]; got != want {
		t.Errorf("the approval tail starts at %q, want the position after the second approval", got)
	}
}

func TestALaneThatKeptNoApprovalStartsTheTailAtTheNewest(t *testing.T) {
	from, err := approvalTailStart(nil, stagedNewestFirst(3))
	if err != nil || from != "" {
		t.Errorf("tail start = %q, %v; want the beginning", from, err)
	}
}

// recordedApprovals is an approval backlog that records where each read began.
type recordedApprovals struct{ after []string }

func (r *recordedApprovals) ApprovalsAfter(_ context.Context, _ ApprovalQuery, after string) (
	[]Positioned[crmcontracts.Approval], bool, error,
) {
	r.after = append(r.after, after)
	return nil, false, nil
}

// A cut approval source in the Decisions view starts a tail at the lane's own
// resume point. The first tail read asks the backlog from there.
func TestACutApprovalSourceContinuesFromTheLanesResumePoint(t *testing.T) {
	staged := stagedNewestFirst(2)
	from, err := approvals.PositionAfter(staged[1])
	if err != nil {
		t.Fatal(err)
	}
	backlog := &recordedApprovals{}
	svc := (&Service{view: viewDecisions}).WithApprovalBacklog(backlog)
	resumeAt := map[crmcontracts.WorklistItemSource]string{crmcontracts.WorklistItemSourceApproval: from}
	reading := svc.readingBacklog(t.Context(), crmcontracts.Attention{AsOf: rankInstant},
		besideDay{until: rankInstant.Add(time.Hour), zone: time.UTC, resumeAt: resumeAt}, false)
	bounded := map[crmcontracts.WorklistItemSource]bool{crmcontracts.WorklistItemSourceApproval: true}

	rest := reading.startTail(tailPage{scope: scopeMine, day: crmcontracts.Attention{AsOf: rankInstant}, bounded: bounded, first: true})
	if rest == nil || rest.At[string(crmcontracts.WorklistItemSourceApproval)] != from {
		t.Fatalf("tail = %+v, want approvals resumed at the lane's position", rest)
	}
	_, after, _ := reading.backlogTail(pageReader(), tailPage{scope: scopeMine, day: crmcontracts.Attention{AsOf: rankInstant}}, 5, rest, nil)

	if len(backlog.after) != 1 || backlog.after[0] != from {
		t.Errorf("the tail read approvals from %q, want once from the lane's position", backlog.after)
	}
	if after != nil {
		t.Errorf("a backlog read to its end left a tail %+v, want none so the page mints no cursor", after)
	}
}
