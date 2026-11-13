// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTaskIntentSurvivesAnUnrelatedEditAndClearsWithTheDeadline(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	raw := &crmcontracts.IntentTime{WallClock: "2027-10-31T02:30", Timezone: "Europe/Berlin"}
	task, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Follow up"), DueHasTime: true, DueIntent: raw})
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2027, 10, 31, 0, 30, 0, 0, time.UTC)
	if task.DueAt == nil || !task.DueAt.Equal(expected) || task.DueIntent == nil || task.DueIntent.Timezone != raw.Timezone || task.RemindIntent == nil {
		t.Fatalf("stored schedule = %+v", task)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(task.Id))
	changed, err := store.UpdateActivity(ctx, id, UpdateActivityInput{Subject: new("Different words")})
	if err != nil {
		t.Fatal(err)
	}
	if changed.DueIntent == nil || *changed.DueIntent != *task.DueIntent || changed.RemindIntent == nil || *changed.RemindIntent != *task.RemindIntent {
		t.Fatal("editing words changed the schedule intent")
	}
	moved := expected.Add(time.Hour)
	legacy, err := store.UpdateActivity(ctx, id, UpdateActivityInput{DueAt: &moved, DueHasTime: new(false)})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.DueIntent != nil || legacy.DueAt == nil || !legacy.DueAt.Equal(moved) {
		t.Fatalf("legacy move retained a different wall clock: %+v", legacy)
	}
	if _, err := store.UpdateActivity(ctx, id, UpdateActivityInput{DueIntent: raw, DueHasTime: new(true)}); err != nil {
		t.Fatal(err)
	}
	cleared, err := store.UpdateActivity(ctx, id, UpdateActivityInput{Clear: []string{"due_at"}})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.DueAt != nil || cleared.DueIntent != nil || cleared.RemindAt != nil || cleared.RemindIntent != nil {
		t.Fatal("clearing the deadline kept its clock or following reminder")
	}
}
