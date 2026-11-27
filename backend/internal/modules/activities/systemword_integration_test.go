// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// seedSystemTask writes one open task on the lead under captured_by system,
// with source and source_system as given. older keeps the source as written
// before the channel check, the way a row waits for the correction task.
func (e *resolveEnv) seedSystemTask(t *testing.T, source string, system *string, older bool) ids.UUID {
	t.Helper()
	if older {
		e.exec(t, `ALTER TABLE activity DROP CONSTRAINT activity_source_check`)
		e.exec(t, `ALTER TABLE activity DISABLE TRIGGER activity_source_channel`)
	}
	id := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, subject, occurred_at, due_at, source, source_system, captured_by)
		VALUES ($1, 'task', 'Follow up with the new lead', now(), now() + interval '1 day', $2, $3, 'system:lead-sla')`,
		id, source, system)
	if older {
		e.exec(t, `ALTER TABLE activity ENABLE TRIGGER activity_source_channel`)
		e.exec(t, `ALTER TABLE activity ADD CONSTRAINT activity_source_check CHECK (`+provenance.ListedRecordSourceSQL("source")+`) NOT VALID`)
	}
	e.exec(t, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, id, e.lead)
	return id
}

func (e *resolveEnv) scalar(t *testing.T, sql string, args ...any) string {
	t.Helper()
	var got string
	if err := e.owner.QueryRow(context.Background(), sql, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

// The system's word becomes the automation channel. The follow-up still
// completes the system's work on either side of the correction, and still
// leaves a commitment's task to its own lifecycle.
func TestALeadLeavingThePoolCompletesTheSystemsWorkBeforeAndAfterTheCorrection(t *testing.T) {
	e := setupResolve(t)
	store := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	sla, commitment := provenance.LeadSLATaskSource, provenance.CommitmentTaskSource
	work := map[string]ids.UUID{
		"an older system task":          e.seedSystemTask(t, systemSource, nil, true),
		"an older escalation":           e.seedSystemTask(t, systemSource, &sla, true),
		"a system task written now":     e.seedSystemTask(t, systemSource, nil, false),
		"an escalation written now":     e.seedSystemTask(t, systemSource, &sla, false),
		"an engine task on its channel": e.seedSystemTask(t, provenance.RecordSourceAutomation, nil, false),
	}
	kept := e.seedSystemTask(t, commitment, &commitment, false)
	if got := e.scalar(t, `SELECT source || '/' || coalesce(source_system, '') FROM activity WHERE id = $1`, work["a system task written now"]); got != "automation/" {
		t.Fatalf("a system task written now reads %q, want automation with no source_system", got)
	}

	fire(e.systemCtx(), t, handlerFor(t, store, "lead.disqualified"), workflow.Event{
		ID: ids.NewV7(), Type: "lead.disqualified", OccurredAt: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
		Entity: datasource.EntityRef{Type: "lead", ID: e.lead},
	})
	for name, task := range work {
		if !e.isDone(t, task) {
			t.Errorf("%s is still open after the lead left the pool", name)
		}
	}
	if e.isDone(t, kept) {
		t.Error("the commitment's task was completed as the system's own work")
	}
}
