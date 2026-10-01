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
