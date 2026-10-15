// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

func TestEveryEventOfAnImportedWonDealNamesTheRunSoNoAutomationOrCommissionStarts(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, won, _ := seededStages(t, e)
	run, _ := importDeals(t, e, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@marked.example,Jo\n",
		"deals.csv":    fmt.Sprintf("ref,name,pipeline,stage,close_date,contact_ref\nD-1,Marked win,%s,%s,2025-06-30,P-1\nD-2,Marked open,%s,%s,2099-01-01,\n", pipeline, won, pipeline, open),
	}, append(bundleParts()[:1], dealImportParts("ref,name,pipeline,stage,close_date,contact_ref")...))
	deal := dealNamed(t, e, "Marked win")
	rows, err := e.Owner.Query(context.Background(), `
		SELECT envelope FROM event_outbox WHERE envelope #>> '{entity,id}' = $1 ORDER BY id`, deal.ID)
	if err != nil {
		t.Fatalf("reading the deal's events: %v", err)
	}
	defer rows.Close()
	types := map[string]bool{}
	for rows.Next() {
		var env events.Envelope
		if err := rows.Scan(&env); err != nil {
			t.Fatalf("reading an event: %v", err)
		}
		types[env.Type] = true
		if !events.IsImported(env) {
			t.Errorf("%s of the imported deal does not name run %s: %s", env.Type, run.ID, env.Payload)
		}
		for _, group := range []string{"cg:workflows", "cg:commissions", "cg:stage-evidence"} {
			if !events.SkipsImported(group) {
				t.Errorf("%s does not skip imported events", group)
			}
		}
	}
	for _, want := range []string{"deal.created", "deal.stage_changed", "deal.updated"} {
		if !types[want] {
			t.Errorf("no %s event was staged for the imported deal (saw %v)", want, types)
		}
	}
}

func TestTheArchiveAnUndoWritesOnAnImportedDealNamesTheRunToo(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	run, _ := importDeals(t, e, map[string]string{
		"deals.csv": fmt.Sprintf("ref,name,pipeline,stage,close_date\nD-1,Undone open,%s,%s,2099-01-01\n", pipeline, open),
	}, dealImportParts("ref,name,pipeline,stage,close_date"))
	deal := dealNamed(t, e, "Undone open")
	var undone importRunDTO
	if status := importCall(t, e, run.ID, "undo", nil, &undone); status != http.StatusAccepted {
		t.Fatalf("undo -> %d", status)
	}
	var payload string
	err := e.Owner.QueryRow(context.Background(), `
		SELECT envelope -> 'payload' FROM event_outbox
		 WHERE envelope #>> '{entity,id}' = $1 AND envelope ->> 'type' = 'deal.archived'`, deal.ID).Scan(&payload)
	if err != nil {
		t.Fatalf("reading the deal.archived event of the undo: %v", err)
	}
	env := events.Envelope{Type: "deal.archived", Payload: []byte(payload)}
	if !events.IsImported(env) {
		t.Errorf("deal.archived written by the undo of run %s does not name it: %s", run.ID, payload)
	}
}
