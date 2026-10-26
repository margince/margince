// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestTheRealTaskWriterKeepsManualAgentAndRegisteredWork(t *testing.T) {
	e := setupSend(t)
	human := e.as(principal.RowScopeAll)
	agent := e.asAgent(principal.RowScopeAll)
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "system:task-fixture")
	assignee := ids.From[ids.UserKind](e.rep)
	store := e.store(nil)
	asOf := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	expected := map[ids.UUID]bool{}
	for _, tc := range []struct {
		name, origin string
		ctx          context.Context
		source       *string
		want         bool
	}{
		{name: "manual human task", ctx: human, want: true},
		{name: "requested agent task", ctx: agent, origin: OriginAgent, want: true},
		{name: "imported task", ctx: human, source: new(provenance.ReservedSourceSystemPrefix + "csv"), want: true},
		{name: "registered automatic task", ctx: system, origin: OriginSystemRemediation, source: new(provenance.LeadSLATaskSource), want: true},
		{name: "unregistered automatic task", ctx: system, origin: OriginSystemRemediation, source: new("unknown-bot")},
	} {
		row, created, err := store.LogActivity(tc.ctx, LogActivityInput{
			Kind: "task", Subject: &tc.name, Origin: tc.origin, SourceSystem: tc.source,
			OccurredAt: &asOf, AssigneeID: &assignee, Source: "manual",
		})
		if err != nil || !created {
			t.Fatalf("%s: created=%v, err=%v", tc.name, created, err)
		}
		if row.AssigneeId == nil || ids.UUID(*row.AssigneeId) != e.rep {
			t.Fatalf("%s lost its explicit assignee", tc.name)
		}
		expected[ids.UUID(row.Id)] = tc.want
	}
	rows, truncated, err := store.ListOpenTasks(human, ListOpenTasksInput{AsOf: asOf, AssigneeID: &e.rep})
	if err != nil || truncated {
		t.Fatalf("reading current tasks: truncated=%v, err=%v", truncated, err)
	}
	seen := map[ids.UUID]bool{}
	for _, row := range rows {
		seen[row.ID] = true
	}
	for id, want := range expected {
		if seen[id] != want {
			t.Errorf("task %s visible=%v, want %v", id, seen[id], want)
		}
	}
}
