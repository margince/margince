// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who a deep read acts as. A read a colleague asked for writes its findings as
// that colleague's agent, under their live grants. A read Margince started itself
// writes them as system:deep_read. Either way the dossier row is kept by
// system:deep_read, so a read the colleague may no longer finish still ends.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestADeepReadAColleagueAskedForWritesAsTheirAgentAndKeepsItsRowAsTheSystem(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), acmeDeepBrain())
	read, args := startDeepRead(t, e, company)

	if err := worker.run(context.Background(), args); err != nil {
		t.Fatalf("run: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'company' AND entity_id = $1
		AND actor_type = 'agent' AND actor_id = 'agent:deepread' AND on_behalf_of = $2`, company, e.Rep1); n == 0 {
		t.Error("no finding was written by agent:deepread for Rep1")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE detail->>'occurrence_key' = $1
		AND actor_id = 'system:deep_read' AND on_behalf_of = $2`, read.ID.String(), e.Rep1); n == 0 {
		t.Error("no change to the dossier row was kept by system:deep_read for Rep1")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM system_log WHERE detail->>'occurrence_key' = $1
		AND actor_type = 'agent'`, read.ID.String()); n != 0 {
		t.Errorf("%d dossier changes were made by the agent, want 0: the row is the system's", n)
	}
}

func TestADeepReadForARequesterWithoutTheGrantFailsAndWritesNoFinding(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), acmeDeepBrain())
	read, args := startDeepRead(t, e, company)
	e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, e.Rep1)

	if err := worker.run(context.Background(), args); err == nil {
		t.Fatal("a read for a requester with no grant on the company succeeded")
	}
	if status := e.WsScalar(t, `SELECT status FROM site_read WHERE id = $1`, read.ID); status != "failed" {
		t.Fatalf("dossier is %q, want failed and never left running", status)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_fact WHERE company_id = $1`, company); n != 0 {
		t.Errorf("%d facts written for a requester who may not change the company, want 0", n)
	}
}

func TestADeepReadMarginceStartedWritesAsTheSystem(t *testing.T) {
	e := integration.Setup(t)
	company := insertCompany(t, e, e.Rep1, "acme.example", "")
	e.WsExec(t, `INSERT INTO capture_auto_enrich_state (company_id) VALUES ($1)`, company)
	worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), acmeDeepBrain())
	read, args := startDeepRead(t, e, company)
	e.WsExec(t, `UPDATE site_read SET requested_by = $1 WHERE id = $2`, systemAutoEnrichActor, read.ID)

	if err := worker.run(context.Background(), args); err != nil {
		t.Fatalf("run: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM company_fact WHERE company_id = $1 AND captured_by = 'system:deep_read'`, company); n == 0 {
		t.Error("the automatic read's facts are not captured by system:deep_read")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND actor_type = 'agent'`, company); n != 0 {
		t.Errorf("%d writes by an agent for a read nobody asked for, want 0", n)
	}
}
