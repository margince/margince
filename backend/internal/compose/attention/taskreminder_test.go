// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func remindedTaskRow() crmcontracts.AttentionItem {
	holder := ids.NewV7()
	item := taskRow(&holder)
	item.ReminderDue = new(true)
	return item
}

func TestADueReminderSortsAboveAWaitingCustomerAndSaysWhy(t *testing.T) {
	t.Parallel()

	reminded := classifyTask(remindedTaskRow(), rankInstant)
	rows := []ranked{candidate("waiting", levelWaiting), reminded}
	sortByRank(rows)

	if rows[0].item.Id != reminded.item.Id {
		t.Fatalf("the reminded task sorts below a waiting customer: %s first", rows[0].item.Id)
	}
	if got := rows[0].item.Because[0].Kind; got != crmcontracts.WorklistReasonKindReminderDue {
		t.Errorf("the reminded task leads with the reason %q, want reminder_due", got)
	}
	if got := compare(rows[0], rows[1]).Comparator; got != crmcontracts.WorklistComparisonComparatorReminder {
		t.Errorf("the row above the waiting customer names %q, want reminder", got)
	}
	if bandOfRow(rows[0]) != bandNow {
		t.Errorf("the reminded task sits under %q, want the first band", bandOfRow(rows[0]))
	}
}

// A reminder moves where the row sorts and leaves what it means: a task due
// next week is still agreed work, not a promise breaking.
func TestADueReminderIsCountedAtWhatTheTaskIs(t *testing.T) {
	t.Parallel()

	reminded := classifyTask(remindedTaskRow(), rankInstant)
	plain := classifyTask(taskRow(new(ids.NewV7())), rankInstant)

	if semanticLevelOf(reminded) != plain.item.Level {
		t.Errorf("the reminded task means level %d, want the plain task's %d",
			semanticLevelOf(reminded), plain.item.Level)
	}
	if got := summarize([]ranked{reminded}, materialBar{}); got.Urgent != 0 {
		t.Errorf("a reminder on next week's task counted %d urgent", got.Urgent)
	}
	if hasReasonKind(reminded.item.Because, "pinned") {
		t.Error("a reminded row says the reader pinned it, which offers an unpin it cannot honour")
	}
}

func TestAPinnedDueReminderSaysBoth(t *testing.T) {
	t.Parallel()

	reminded := classifyTask(remindedTaskRow(), rankInstant)
	rows := applyPins([]ranked{reminded}, map[RowRef]bool{refOf(reminded): true})

	if !hasReasonKind(rows[0].item.Because, "pinned") || !hasReasonKind(rows[0].item.Because, "reminder_due") {
		t.Errorf("a pinned reminder states %v, want both reasons", rows[0].item.Because)
	}
	if semanticLevelOf(rows[0]) != levelAgreed {
		t.Errorf("pinning a reminded task moved its meaning to level %d", semanticLevelOf(rows[0]))
	}
}

func TestADueReminderReachesHomeFocusWhileItsDueDateIsNextWeek(t *testing.T) {
	t.Parallel()

	reminded := classifyTask(remindedTaskRow(), rankInstant)
	if !focusEligible(reminded, rankInstant) {
		t.Error("a due reminder on next week's task is kept off Home")
	}
	if focusEligible(classifyTask(taskRow(new(ids.NewV7())), rankInstant), rankInstant) {
		t.Error("next week's task reached Home with no reminder, so the case above proves nothing")
	}
}

func TestThePlannedLaneListsAReminderFirstAndOnce(t *testing.T) {
	due := rankInstant.Add(-time.Hour)
	nextWeek := rankInstant.AddDate(0, 0, 7)
	remindedID := ids.NewV7()
	tasks := &stubTasks{
		rows:      []Task{{ID: ids.NewV7(), Subject: "Overdue", DueAt: &due}},
		upcoming:  []Task{{ID: remindedID, Subject: "Reminded", DueAt: &nextWeek}},
		reminders: []Task{{ID: remindedID, Subject: "Reminded", DueAt: &nextWeek, ReminderDue: true}},
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	out, err := svc.forReader().Assemble(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Planned) != 2 {
		t.Fatalf("the planned lane lists %d rows, want the overdue task and the reminder once each", len(out.Planned))
	}
	if first := out.Planned[0]; first.Id != remindedID.String() || first.ReminderDue == nil || !*first.ReminderDue {
		t.Errorf("the planned lane leads with %s, want the reminded task marked as due", first.Id)
	}
	if tasks.remindersLimit != plannedCap {
		t.Errorf("the reminder read is bounded at %d, want %d", tasks.remindersLimit, plannedCap)
	}
}

// Only the reader's own queue is reminded. The lane feed reads every visible
// task, so it never asks for reminders.
func TestOnlyTheReadersOwnQueueAsksForReminders(t *testing.T) {
	tasks := &stubTasks{}
	svc := NewService(stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{}, stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	if _, err := svc.Assemble(pageReader()); err != nil {
		t.Fatal(err)
	}
	if tasks.remindersAsked {
		t.Error("a read of every visible task asked for the reader's reminders")
	}
}
