// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What an AI-work incident group CALLS the thing that broke.
//
// The row folds several failures of one task into one line, so the words on it
// are the only thing saying which task. It used to send none, because the
// obvious candidate — the task key — is generated enum vocabulary, and a rep
// reading "site_triage failed 8 times" learns nothing they can act on.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func troubledRun(kind string) TroubledRun {
	return TroubledRun{
		ID:         ids.NewV7(),
		Kind:       kind,
		State:      "failed",
		OccurredAt: time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC),
	}
}

func TestAnAIWorkGroupNamesTheTaskInWords(t *testing.T) {
	t.Parallel()

	item := aiWorkItem(troubledRun(string(ai.TaskSiteTriage)))

	if item.CauseLabel == nil {
		t.Fatal("the group carries no label, so it reads as the generic phrase and " +
			"never says which task broke")
	}
	if got, want := *item.CauseLabel, ai.DisplayName(ai.TaskSiteTriage); got != want {
		t.Errorf("the group is called %q, want %q — the label is the task catalog's "+
			"own name, so the two cannot drift", got, want)
	}
	if item.CauseRef == nil || *item.CauseRef == *item.CauseLabel {
		t.Error("the label and the identity are the same string: one groups the " +
			"failures and the other is read, and they are not the same job")
	}
}

// A kind this build does not know — a row an older binary wrote, or a task
// retired from the contract — goes unlabelled rather than being named after
// the key it was handed. That is the behaviour every kind had before, so the
// row is no worse than it was.
func TestAnUnknownTaskGoesUnlabelledRatherThanShowingItsKey(t *testing.T) {
	t.Parallel()

	item := aiWorkItem(troubledRun("a_task_this_build_never_shipped"))

	if item.CauseLabel != nil {
		t.Errorf("an unknown task was labelled %q — the only name available for it "+
			"is the key, which is what this lane exists not to show", *item.CauseLabel)
	}
	if item.CauseRef == nil {
		t.Error("an unknown task lost its identity too, so its failures stop grouping")
	}
}
