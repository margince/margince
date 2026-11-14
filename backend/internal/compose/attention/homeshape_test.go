// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"reflect"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Home's shape answers the same day as the queue: the same Focus, summary,
// readings and counted candidates. Only the rows it carries differ.
func TestHomesShapeAnswersTheSameDayWithOnlyItsMeetings(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Planned: []crmcontracts.AttentionItem{
			item("overdue", "task", withDue(rankInstant.Add(-time.Hour))),
			item("later", "task", withDue(rankInstant.Add(3*time.Hour))),
		},
		NeedsYou: []crmcontracts.AttentionItem{item("approve", "approval")},
		Meetings: lane(
			item("standup", "meeting", withDue(rankInstant.Add(time.Hour))),
			item("review", "meeting", withDue(rankInstant.Add(2*time.Hour))),
		),
	}
	queue := (&Service{}).worklistFrom(context.Background(), day, "all", "", 1, waitingRead{}, worklistCursor{}, nil)
	home := (&Service{homeShape: true}).worklistFrom(context.Background(), day, "all", "", 1, waitingRead{}, worklistCursor{}, nil)

	if !reflect.DeepEqual(queue.Focus, home.Focus) {
		t.Errorf("Focus differs: queue %+v, home %+v", queue.Focus, home.Focus)
	}
	if !reflect.DeepEqual(queue.Summary, home.Summary) || !reflect.DeepEqual(queue.Readings, home.Readings) {
		t.Errorf("figures differ: queue %+v %+v, home %+v %+v", queue.Summary, queue.Readings, home.Summary, home.Readings)
	}
	considered := func(page crmcontracts.Worklist) map[crmcontracts.WorklistCountCategory]int {
		out := map[crmcontracts.WorklistCountCategory]int{}
		for _, count := range page.Counts {
			out[count.Category] = count.Considered
		}
		return out
	}
	if !reflect.DeepEqual(considered(queue), considered(home)) {
		t.Errorf("counted candidates differ: queue %v, home %v", considered(queue), considered(home))
	}
	if queue.NextCursor == nil {
		t.Fatal("the one-row queue page names no cursor, so the case does not tell the shapes apart")
	}
	if home.NextCursor != nil {
		t.Error("Home's shape names a cursor; it walks nothing")
	}
	if len(home.Queue) != 2 {
		t.Fatalf("Home's shape carries %d rows, want the day's 2 meetings", len(home.Queue))
	}
	for _, row := range home.Queue {
		if row.Source != sourceMeeting {
			t.Errorf("Home's shape carries a %s row; it draws only meetings beside Focus", row.Source)
		}
	}
}

func TestTheSoonestLeadDeadlineIsTheEarliestUnlessTheTasksWereCut(t *testing.T) {
	lead := func(id string, due time.Time) ranked {
		return ranked{item: crmcontracts.WorklistItem{
			Id: id, Source: sourceTask, Category: crmcontracts.WorklistItemCategoryTasks, DueAt: &due,
			Subject: &crmcontracts.AttentionSubject{Type: crmcontracts.AttentionSubjectTypeLead},
		}}
	}
	late, soon := lead("late", rankInstant.Add(9*time.Hour)), lead("soon", rankInstant.Add(time.Hour))
	got := soonestLeadDue([]ranked{late, soon}, nil, nil)
	if got == nil || !got.Equal(*soon.item.DueAt) {
		t.Errorf("the soonest of two leads is %v, want %v", got, soon.item.DueAt)
	}
	if got := soonestLeadDue([]ranked{late, soon}, map[crmcontracts.WorklistItemSource]bool{sourceTask: true}, nil); got != nil {
		t.Errorf("a task lane read to its bound names %v; an unseen lead could be sooner", got)
	}
	unread := []crmcontracts.WorklistSourceUnavailable{{Source: string(sourceTask)}}
	if got := soonestLeadDue([]ranked{late, soon}, nil, unread); got != nil {
		t.Errorf("an unread task lane names %v", got)
	}
}
