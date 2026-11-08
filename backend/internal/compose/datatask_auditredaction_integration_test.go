// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func legacyAuditErasure(t *testing.T, e *integration.Env) (ids.ContactID, ids.UUID) {
	t.Helper()
	contact := repairContact(t, e, "Legacy Personal Name")
	if _, err := e.Contacts.UpdateContact(e.Admin(), contact, contacts.UpdateContactInput{FullName: new("Erased Subject")}); err != nil {
		t.Fatal(err)
	}
	var certificate ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		// Earlier erasures wrote this certificate without physically removing images.
		certificate, err = storekit.AuditWithEvidence(e.Admin(), tx, "erase", "contact", contact.UUID, nil, nil,
			map[string]any{"emails_suppressed": 0, "reason": "test", "rows": map[string]int64{}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return contact, certificate
}

func TestHistoricalAuditTaskRemovesEarlierPayloadsAndPreservesLaterWork(t *testing.T) {
	e := integration.Setup(t)
	contact, certificate := legacyAuditErasure(t, e)
	if _, err := e.Contacts.UpdateContact(e.Admin(), contact, contacts.UpdateContactInput{FullName: new("Later Authorized Name")}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	task := auditRedactionDataTask()
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log history, audit_log certificate
	 WHERE certificate.id=@certificate AND history.entity_id=@contact
	 AND history.entity_type='contact' AND history.action NOT IN ('erase','anonymize')
	 AND (history.occurred_at,history.id)<=(certificate.occurred_at,certificate.id)
	 AND (history.before IS NOT NULL OR history.after IS NOT NULL)`,
		pgx.StrictNamedArgs{"certificate": certificate, "contact": contact}); n != 0 {
		t.Fatalf("%d earlier personal images remain", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='contact' AND entity_id=$1
	 AND after->>'full_name'='Later Authorized Name'`, contact); n != 1 {
		t.Fatalf("later authorized audit images=%d, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=$1
	 AND envelope::text LIKE '%Later Authorized Name%'`, contact.String()); n != 1 {
		t.Fatalf("later authorized events=%d, want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='contact' AND entity_id=$1 AND action='erase'`, contact); n != 1 {
		t.Fatalf("correction changed the subject's erasure boundary: %d certificates", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 1000, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("replay changed %d certificates", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalAuditTaskRollsBackAndRejectsMalformedCursors(t *testing.T) {
	e := integration.Setup(t)
	contact, _ := legacyAuditErasure(t, e)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	task := auditRedactionDataTask()
	batch := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := batch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		if out.Changed == 0 {
			t.Error("the rollback fixture did not redact its legacy history")
		}
		return out, errors.New("interrupt after audit redaction")
	}
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1
	 AND after->>'full_name'='Legacy Personal Name'`, contact); n != 1 {
		t.Fatalf("rollback lost %d original images, want 1 still present", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 2})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
}

func TestHistoricalAuditTaskKeepsCurrentLegalHolds(t *testing.T) {
	e := integration.Setup(t)
	contact, _ := legacyAuditErasure(t, e)
	e.WsExec(t, `UPDATE contact SET legal_hold=true WHERE id=$1`, contact)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, e, auditRedactionDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1
	 AND after->>'full_name'='Legacy Personal Name'`, contact); n != 1 {
		t.Fatalf("held original images=%d, want 1", n)
	}
}
