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

// undatedDay assembles a day whose later window read one task nobody dated,
// and whose Today read one undated task an automatic source owes now.
func undatedDay(t *testing.T) (crmcontracts.Attention, ids.UUID, ids.UUID) {
	t.Helper()
	backlog, owedNow := ids.NewV7(), ids.NewV7()
	tasks := &stubTasks{
		rows:     []Task{{ID: owedNow, Subject: "Reply to the request"}},
		upcoming: []Task{{ID: backlog, Subject: "Call back about the renewal"}},
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	day, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	return day, backlog, owedNow
}

func TestAnUndatedTaskOfTheLaterWindowHeadsTheNoDueDateGroup(t *testing.T) {
	day, backlog, owedNow := undatedDay(t)
	groups := map[string]*crmcontracts.AttentionItemDueGroup{}
	for _, item := range day.Planned {
		groups[item.Id] = item.DueGroup
	}
	if got := groups[backlog.String()]; got == nil || *got != crmcontracts.AttentionItemDueGroupNone {
		t.Errorf("the undated backlog task carries group %v, want none", got)
	}
	if got := groups[owedNow.String()]; got != nil {
		t.Errorf("the undated task owed now carries group %s, want none at all: it is Today's work", *got)
	}
}

func TestAnUndatedBacklogTaskIsUpcomingWorkAndNotToday(t *testing.T) {
	day, backlog, owedNow := undatedDay(t)
	today := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)
	if got := queueIDs(today); len(got) != 1 || got[0] != owedNow.String() {
		t.Errorf("Today holds %v, want only the task owed now", got)
	}
	upcoming := (&Service{}).worklistFrom(t.Context(), day, scopeAll, string(crmcontracts.WorklistFilterUpcoming), 25,
		waitingRead{}, worklistCursor{}, nil)
	if len(upcoming.Queue) != 1 || upcoming.Queue[0].Id != backlog.String() {
		t.Fatalf("Upcoming and backlog holds %v, want the undated backlog task", queueIDs(upcoming))
	}
	if group := upcoming.Queue[0].DueGroup; group == nil || *group != crmcontracts.WorklistItemDueGroupWorklistDueGroupNone {
		t.Errorf("the backlog row carries group %v on the queue, want none", group)
	}
}

// undatedTail answers every continuation with one task nobody dated.
type undatedTail struct {
	windowRecorder
	task ids.UUID
}

func (u *undatedTail) TasksAfter(context.Context, TaskWindow, string, int) ([]Positioned[Task], bool, error) {
	return []Positioned[Task]{{Row: Task{ID: u.task, Subject: "Send the price list"}}}, false, nil
}

func TestAnUndatedTaskBelowTheCapReadsAsTheBacklogItIsAboveIt(t *testing.T) {
	tail := &undatedTail{task: ids.NewV7()}
	svc := (&Service{}).WithTaskBacklog(tail)
	walk := tailWindow{AsOf: rankInstant, Until: rankInstant.Add(time.Hour), Zone: "UTC"}
	for filter, want := range map[string]*crmcontracts.WorklistItemDueGroup{
		string(crmcontracts.WorklistFilterUpcoming): new(crmcontracts.WorklistItemDueGroupWorklistDueGroupNone),
		string(crmcontracts.WorklistFilterTasks):    nil,
	} {
		batch, err := svc.moreTasks(t.Context(), tailPage{filter: filter}, walk, "", 5)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch.rows) != 1 {
			t.Fatalf("the %s tail drew %d rows, want 1", filter, len(batch.rows))
		}
		got := batch.rows[0].row.item.DueGroup
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("an undated task of the %s tail carries group %v, want %v", filter, got, want)
		}
	}
}
