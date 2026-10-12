// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

// Every task a screen names comes with what it does, the embeddings lane
// included, which is not a contract task but is billed and routed like one.
func TestEveryNamedTaskSaysWhatItDoes(t *testing.T) {
	for _, task := range append(AllTasks(), TaskEmbeddings) {
		name, summary := taskLabel(task)
		if name == "" || summary == "" {
			t.Errorf("%s: name %q, summary %q — a screen would show it with nothing behind it", task, name, summary)
		}
	}
	if name, _ := taskLabel(TaskEmbeddings); name == string(TaskEmbeddings) {
		t.Errorf("the embeddings lane is named by its key %q", name)
	}
}

func TestTheUsageLineCarriesTheTasksNameAndSummary(t *testing.T) {
	wire := wireAiUsage([]DayUsage{{Tasks: []TaskUsage{{Task: string(TaskEmbeddings), Tier: string(TierEmbedLane)}}}}, BudgetStatus{})
	line := wire.Days[0].Tasks[0]
	if line.TaskDisplayName == nil || *line.TaskDisplayName != "Search and retrieval" {
		t.Errorf("embeddings usage line named %v, want Search and retrieval", line.TaskDisplayName)
	}
	if line.TaskSummary == nil || *line.TaskSummary == "" {
		t.Error("the usage line carries no summary")
	}
}

// The call log names a task the way the usage lines do. An unnamed task
// arrives without a name, not with its key posing as one.
func TestTheCallLogNamesTheTaskLikeTheUsageLine(t *testing.T) {
	for _, task := range append(AllTasks(), TaskEmbeddings) {
		usage := wireAiUsage([]DayUsage{{Tasks: []TaskUsage{{Task: string(task)}}}}, BudgetStatus{})
		want := *usage.Days[0].Tasks[0].TaskDisplayName
		call := wireAiCallSummary(CallSummary{Task: string(task)})
		detail := wireAiCall(CallDetail{Task: string(task)})
		option := wireAiCallTaskOptions([]string{string(task)})[0]
		for wire, name := range map[string]*string{"row": call.TaskDisplayName, "drawer": detail.TaskDisplayName, "filter": option.DisplayName} {
			if name == nil || *name != want {
				t.Errorf("%s: the call log's %s named %v, usage line named %q", task, wire, name, want)
			}
		}
	}
	if unnamed := wireAiCallSummary(CallSummary{Task: "retired_task"}); unnamed.TaskDisplayName != nil {
		t.Errorf("an unnamed task carried the name %q", *unnamed.TaskDisplayName)
	}
	if option := wireAiCallTaskOptions([]string{"retired_task"})[0]; option.Task != "retired_task" || option.DisplayName != nil {
		t.Errorf("an unnamed task's filter option reads %+v, want its key and no name", option)
	}
}

// The filter's named options are the bare task list, one for one, in order.
func TestTheTaskOptionsFollowTheTaskList(t *testing.T) {
	tasks := []string{string(TaskEmbeddings), "retired_task", string(AllTasks()[0])}
	options := wireAiCallTaskOptions(tasks)
	if len(options) != len(tasks) {
		t.Fatalf("%d options for %d tasks", len(options), len(tasks))
	}
	for i, option := range options {
		if option.Task != tasks[i] {
			t.Errorf("option %d is %q, want %q", i, option.Task, tasks[i])
		}
	}
	if empty := wireAiCallTaskOptions(nil); empty == nil || len(empty) != 0 {
		t.Errorf("no tasks wires %v, want an empty list rather than null", empty)
	}
}
