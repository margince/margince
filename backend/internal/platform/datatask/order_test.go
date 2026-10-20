// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func testTask(id string) Task {
	return Task{ID: id, Version: 1, Title: "Correct records", Steps: []Step{
		{
			Name: "first", Limit: 4, MinLimit: 1,
			Audit: AuditPolicy{PerRecord: true}, Estimate: func(context.Context, Querier, int64) (int64, bool, error) { return 0, false, nil },
			Batch: func(context.Context, pgx.Tx, BatchIn) (BatchOut, error) { return BatchOut{Done: true}, nil },
		},
		{
			Name: "second", Limit: 4, MinLimit: 1, Audit: AuditPolicy{PerRecord: true},
			Estimate: func(context.Context, Querier, int64) (int64, bool, error) { return 0, false, nil },
			Batch:    func(context.Context, pgx.Tx, BatchIn) (BatchOut, error) { return BatchOut{Done: true}, nil },
		},
	}}
}

func TestCatalogOrdersAndOwnsItsRegistration(t *testing.T) {
	first, second := testTask("core/100_first"), testTask("core/200_second")
	first.Requires = []string{"core/99"}
	catalog, err := Catalog([]Task{second, first})
	if err != nil {
		t.Fatal(err)
	}
	first.Steps[0].Name = "changed"
	first.Requires[0] = "core/98"
	if catalog[0].ID != "core/100_first" || catalog[0].Steps[0].Name != "first" || catalog[0].Requires[0] != "core/99" {
		t.Fatalf("registration changed: %+v", catalog)
	}
	for _, mutate := range []func(*Task){
		func(t *Task) { t.ID = "extension/100_bad" }, func(t *Task) { t.Version = 0 }, func(t *Task) { t.Requires = []string{"100"} },
		func(t *Task) { t.Steps[0].Audit = AuditPolicy{} }, func(t *Task) { t.Steps[0].Audit.Reason = "both" },
		func(t *Task) { t.Steps[0].MinLimit = 5 }, func(t *Task) { t.Steps[0].Batch = nil },
		func(t *Task) { t.Steps[1].Name = t.Steps[0].Name },
	} {
		task := testTask("core/100_valid")
		mutate(&task)
		if _, err := Catalog([]Task{task}); err == nil {
			t.Fatalf("invalid registration accepted: %+v", task)
		}
	}
	if _, err := Catalog([]Task{second, second}); err == nil {
		t.Fatal("duplicate task accepted")
	}
}

func TestQueueStopsAtFirstUnfinishedStep(t *testing.T) {
	tasks := []Task{testTask("core/100_first"), testTask("core/200_second")}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	firstKey := progressKey(tasks[0].ID, "first")
	for _, status := range []string{Pending, Running, Paused, Failed, Done} {
		t.Run(status, func(t *testing.T) {
			stored := map[string]Progress{firstKey: {TaskVersion: 1, Status: status}}
			got, blocked := nextStep(tasks, stored, now)
			if got == nil || got.task.ID != tasks[0].ID {
				t.Fatal("later task escaped earlier correction")
			}
			if status == Done {
				if got.step.Name != "second" || blocked != "waiting_for_worker" {
					t.Fatalf("next = %+v, %s", got, blocked)
				}
				return
			}
			if got.step.Name != "first" {
				t.Fatal("later step escaped")
			}
			want := ""
			if status == Paused || status == Failed {
				want = status
			}
			if blocked != want {
				t.Fatalf("block=%s want %s", blocked, want)
			}
		})
	}
	stored := map[string]Progress{firstKey: {TaskVersion: 2, Status: Done}}
	if _, block := nextStep(tasks, stored, now); block != "newer_release_required" {
		t.Fatalf("old binary got %s", block)
	}
	stored[firstKey] = Progress{TaskVersion: 0, Status: Done}
	if got, block := nextStep(tasks, stored, now); got == nil || got.step.Name != "first" || block != "" {
		t.Fatal("changed code did not restart")
	}
	next := now.Add(time.Minute)
	stored[firstKey] = Progress{TaskVersion: 1, Status: Running, NextAttemptAt: &next}
	if _, block := nextStep(tasks, stored, now); block != "retry_wait" {
		t.Fatalf("retry prematurely selected: %s", block)
	}
}

func TestBatchBoundRequiresScannedCursorProgress(t *testing.T) {
	in := Progress{BatchLimit: 4, Cursor: []byte(`"a"`)}
	valid := []BatchOut{{Done: true}, {Cursor: []byte(`"b"`), Scanned: 4, Changed: 0}, {Cursor: []byte(`"b"`), Scanned: 2, Changed: 2, Done: true}}
	for _, out := range valid {
		if err := validateBatch(in, out); err != nil {
			t.Fatal(err)
		}
	}
	invalid := []BatchOut{{}, {Scanned: 1}, {Scanned: 5, Cursor: []byte(`"b"`)}, {Scanned: 1, Changed: 2, Done: true}, {Scanned: 1, Cursor: []byte(`"a"`)}, {Scanned: 1, Cursor: []byte(`bad`)}}
	for _, out := range invalid {
		if err := validateBatch(in, out); err == nil {
			t.Fatalf("invalid batch accepted: %+v", out)
		}
	}
}

func TestTaskActorKeepsTheTaskIdentity(t *testing.T) {
	actor, ok := principal.Actor(Acting(t.Context(), "core/100_first"))
	if !ok || actor.Type != principal.PrincipalSystem || actor.ID != "system:datatask:core/100_first" {
		t.Fatalf("task actor=%+v, present=%v", actor, ok)
	}
}

func TestUnactivatedManualWorkDoesNotHoldAutomaticCorrections(t *testing.T) {
	manual, automatic := testTask("core/100_manual"), testTask("core/200_automatic")
	manual.Manual, manual.ReviewVersion = true, "rules-1"
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, status := range []string{Pending, Paused, Failed} {
		stored := map[string]Progress{progressKey(manual.ID, "first"): {TaskVersion: 1, Status: status}}
		got, _ := nextStep([]Task{manual, automatic}, stored, now)
		if got == nil || got.task.ID != automatic.ID {
			t.Fatalf("unactivated %s held automatic work", status)
		}
	}
	stored := map[string]Progress{progressKey(manual.ID, "first"): {TaskVersion: 1, Status: Pending, Activation: &Activation{ReviewRequest: ReviewRequest{RuleVersion: "rules-0"}}}}
	if got, _ := nextStep([]Task{manual, automatic}, stored, now); got == nil || got.task.ID != automatic.ID {
		t.Fatal("changed rules blocked automatic work")
	}
}

func TestChangedManualRulesRequireAnotherReview(t *testing.T) {
	task := testTask("core/100_manual")
	task.Manual, task.ReviewVersion = true, "rules-2"
	key := progressKey(task.ID, "first")
	for _, revision := range []struct {
		task int
		rule string
	}{{1, "rules-1"}, {2, "rules-2"}} {
		stored := map[string]Progress{key: {TaskVersion: revision.task, Status: Running, Activation: &Activation{ReviewRequest: ReviewRequest{RuleVersion: revision.rule}}}}
		if got, _ := nextStep([]Task{task}, stored, time.Time{}); got != nil {
			t.Fatal("changed review continued")
		}
	}
}
