// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestFocusDoesNotFillAQuietDayWithMaintenance(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:        rankInstant,
		NoticeCases: lane(item("future-privacy", "notice_case", withDue(rankInstant.Add(20*24*time.Hour)))),
		NeedsYou:    []crmcontracts.AttentionItem{item("hygiene", "approval", withKind("capture_counterparty"))},
		Planned:     []crmcontracts.AttentionItem{item("future-task", "task", withDue(rankInstant.Add(24*time.Hour)))},
	}
	out := (&Service{}).worklistFrom(context.Background(), day, "all", "", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)
	if out.Focus == nil || len(out.Focus.Items) != 0 || out.Focus.Total != 0 {
		t.Fatalf("maintenance filled focus: %+v", out.Focus)
	}
	if len(out.Queue) != 3 {
		t.Fatalf("maintenance disappeared from queue: %+v", out.Queue)
	}
}

func TestFocusSelectsBeforePaginationAndQueueFiltering(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:        rankInstant,
		Planned:     []crmcontracts.AttentionItem{item("due", "task", withDue(rankInstant.Add(-time.Hour)))},
		NoticeCases: lane(item("legal", "notice_case", withDue(rankInstant.Add(24*time.Hour)))),
	}
	for _, filter := range []string{"all", "tasks", "decisions"} {
		out := (&Service{}).worklistFrom(context.Background(), day, "all", filter, 1, waitingRead{}, leadRead{}, worklistCursor{}, nil)
		if out.Focus == nil || len(out.Focus.Items) != 2 || out.Focus.Items[0].Id != "legal" || out.Focus.Items[1].Id != "due" {
			t.Fatalf("%s page cut hid eligible work: %+v", filter, out.Focus)
		}
		if len(out.Queue) > 1 {
			t.Fatalf("queue ignored page limit: %d", len(out.Queue))
		}
	}
}

func TestFocusPreservesRankingAndReportsUrgentOverflowWithoutAPageCursor(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant}
	for i := range 8 {
		day.Planned = append(day.Planned, item(string(rune('a'+i)), "task", withDue(rankInstant.Add(-time.Duration(i+1)*time.Hour))))
	}
	out := (&Service{}).worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)
	if out.NextCursor != nil || out.Focus == nil || len(out.Focus.Items) != 6 || out.Focus.Total != 8 || out.Focus.UrgentRemaining != 2 {
		t.Fatalf("wrong focus overflow: %+v", out.Focus)
	}
	for i, row := range out.Focus.Items {
		if row.Id != out.Queue[i].Id {
			t.Fatalf("focus reranked %s against %s", row.Id, out.Queue[i].Id)
		}
	}
}

func TestFocusUsesOrdinaryRankingWhileTheQueueKeepsPins(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Planned: []crmcontracts.AttentionItem{
			item("later", "task", withDue(rankInstant.Add(-time.Hour))),
			item("earlier", "task", withDue(rankInstant.Add(-2*time.Hour))),
		},
		NoticeCases: lane(item("future", "notice_case", withDue(rankInstant.Add(30*24*time.Hour)))),
	}
	svc := &Service{pinned: map[RowRef]bool{
		{Source: "notice_case", RowID: "future"}: true,
		{Source: "task", RowID: "later"}:         true,
	}}
	for _, filter := range []string{"all", "decisions", "system"} {
		t.Run(filter, func(t *testing.T) {
			out := svc.worklistFrom(context.Background(), day, filter, "all", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)
			if out.Focus == nil || len(out.Focus.Items) != 2 || out.Focus.Items[0].Id != "earlier" {
				t.Fatalf("personal pins changed focus: %+v", out.Focus)
			}
			for _, row := range out.Focus.Items {
				if row.Level == levelPinned {
					t.Fatalf("focus retained pin priority: %+v", row)
				}
				for _, why := range row.Because {
					if why.Kind == "pinned" {
						t.Fatalf("focus retained a pin marker: %+v", row)
					}
				}
			}
			if filter == "all" && (len(out.Queue) != 3 || out.Queue[0].Level != levelPinned) {
				t.Fatalf("focus removal changed personal queue pins: %+v", out.Queue)
			}
		})
	}
}

func TestFocusDistinguishesMeetingPreparationFromSchedule(t *testing.T) {
	for _, test := range []struct {
		kind  string
		delay time.Duration
		want  bool
	}{
		{"prepared", time.Hour, false},
		{meetingKindUnprepared, time.Hour, true},
		{meetingKindUnprepared, 48 * time.Hour, false},
	} {
		row := classifyMeeting(item("meeting", "meeting", withKind(test.kind), withDue(rankInstant.Add(test.delay))), rankInstant)
		if got := focusEligible(row, rankInstant); got != test.want {
			t.Errorf("%s in %v: got %v", test.kind, test.delay, got)
		}
	}
}

func TestFocusKeepsPromisesDueLaterToday(t *testing.T) {
	// These two lanes already read through the installation's end of today.
	day := crmcontracts.Attention{AsOf: rankInstant, Commitments: lane(item("promise", "conversation_claim", withDue(rankInstant.Add(time.Hour))))}
	svc := &Service{planRows: []ranked{{item: crmcontracts.WorklistItem{Id: "plan", Source: sourceWeeklyCommitment, Level: levelPromise}, deadlineAt: rankInstant.Add(time.Hour)}}}
	out := svc.worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)
	if out.Focus == nil || len(out.Focus.Items) != 2 {
		t.Fatalf("today's promises disappeared: %+v", out.Focus)
	}
}

func TestFocusLeavesInformationalNoticesInUpdatesEvenWhenPinned(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant, Notices: lane(item("notice", "notice", withDetail("The deal changed stage")))}
	svc := &Service{}
	out := svc.worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)
	if len(out.Queue) != 1 || out.Focus == nil || len(out.Focus.Items) != 0 {
		t.Fatalf("informational notice filled focus: %+v", out.Focus)
	}
	svc.pinned = map[RowRef]bool{{Source: "notice", RowID: "notice"}: true}
	out = svc.worklistFrom(context.Background(), day, "all", "all", 25, waitingRead{}, leadRead{}, worklistCursor{}, nil)
	if len(out.Focus.Items) != 0 || out.Focus.UrgentRemaining != 0 {
		t.Fatalf("queue pin promoted an informational notice into focus: %+v", out.Focus)
	}
}

func TestFocusDoesNotSubtractAnEntireBatchFromUrgentObligations(t *testing.T) {
	urgent := candidate("urgent", levelPromise)
	routine := candidate("routine", levelRoutine)
	batch := candidate("group", levelPromise)
	batch.item.Source = "batch"
	batch.item.Batch = &crmcontracts.WorklistBatch{Count: 10}
	focus := focusOf([]ranked{batch}, []ranked{urgent, routine}, rankInstant, ids.UUID{})
	if focus.UrgentRemaining != 1 {
		t.Fatalf("batch count erased an urgent obligation: %+v", focus)
	}
}
