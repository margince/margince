// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A patch is one change. Its row, the audit row that says what changed and the
// event that tells consumers are written by one transaction.
func TestAContractPatchCommitsItsRowItsAuditAndItsEventTogether(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", nil)
	created, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
		CompanyID: ids.From[ids.CompanyKind](company), Title: "MSA 2026",
		ValueBasis: contracts.BasisTotal, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(created.Id)
	ctx := e.Admin()
	correlation, _ := principal.CorrelationID(ctx)
	retitled := "MSA 2026, amended"
	if _, err := e.Contracts.UpdateContract(ctx, ids.From[ids.ContractKind](id),
		crmcontracts.UpdateContractRequest{Title: &retitled}, nil, nil); err != nil {
		t.Fatalf("patching the title: %v", err)
	}

	var title, rowXact, auditXact, eventXact, before, after string
	var envelope []byte
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT c.title, c.xmin::text, a.xmin::text, a.before->>'title', a.after->>'title',
		       o.xmin::text, o.envelope
		  FROM contract c
		  JOIN audit_log a ON a.entity_type = 'contract' AND a.entity_id = c.id AND a.action = 'update'
		  JOIN event_outbox o ON o.envelope #>> '{trace,audit_log_id}' = a.id::text
		 WHERE c.id = $1`, id).Scan(
		&title, &rowXact, &auditXact, &before, &after, &eventXact, &envelope); err != nil {
		t.Fatalf("reading the patched row with its audit row and its event: %v", err)
	}
	if title != retitled {
		t.Errorf("the row holds %q, want %q", title, retitled)
	}
	if auditXact != rowXact || eventXact != rowXact {
		t.Errorf("the row, audit and event were written by transactions %s, %s and %s; they must be one",
			rowXact, auditXact, eventXact)
	}
	if before != "MSA 2026" || after != retitled {
		t.Errorf("the audit row says %q → %q, want %q → %q", before, after, "MSA 2026", retitled)
	}

	var env events.Envelope
	if err := json.Unmarshal(envelope, &env); err != nil {
		t.Fatalf("decoding the event: %v", err)
	}
	var payload crmcontracts.PublicEventContractUpdated
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("decoding the event's payload: %v", err)
	}
	if env.Type != "contract.updated" || env.Entity.ID != id || env.Trace.CorrelationID != correlation {
		t.Errorf("the event is %s on %s in request %s, want contract.updated on %s in request %s",
			env.Type, env.Entity.ID, env.Trace.CorrelationID, id, correlation)
	}
	if payload.ChangedFields["title"] != retitled || len(payload.ChangedFields) != 1 {
		t.Errorf("the event says %v changed, want only the title %q", payload.ChangedFields, retitled)
	}
}
