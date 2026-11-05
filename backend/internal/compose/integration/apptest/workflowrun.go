// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WorkflowRun is one firing of the assign_lead_owner automation, as the
// engine records it. Planned and Applied ride the workflow.Action encoding;
// Detail is the jsonb payload in the automation module's rundetail.go shape.
type WorkflowRun struct {
	Automation       string
	Status           string
	Trigger          ids.UUID
	Planned, Applied *string
	Detail           []byte
	At               time.Time
}

// SeedWorkflowRun records run linked to its automation as the engine links it:
// handler = catalog key, idempotency key suffixed with "@<automation id>". The
// key names the run, not a lead, so it never dedupes against the engine's own.
// No route writes a run; only the engine does, as it handles an event.
func SeedWorkflowRun(t *testing.T, e *AppEnv, run WorkflowRun) string {
	t.Helper()
	runID := ids.NewV7().String()
	planned := "[]"
	if run.Planned != nil {
		planned = *run.Planned
	}
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO workflow_run (id, handler, idempotency_key, trigger_event, planned, applied, status, detail, created_at)
		VALUES ($1, 'assign_lead_owner', $2, $3, $4, $5, $6, $7, $8)`,
		runID, fmt.Sprintf("assign_lead_owner:%s@%s", runID, run.Automation),
		run.Trigger, planned, run.Applied, run.Status, run.Detail, run.At); err != nil {
		t.Fatalf("seeding workflow_run: %v", err)
	}
	return runID
}
