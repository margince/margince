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
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func legacyRequestAuditCopies(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	contact := repairContact(t, e, "Erased Subject")
	request := ids.NewV7()
	e.WsExec(t, `INSERT INTO data_subject_request(id,kind,subject_ref,status,resolution,received_at,due_at,channel,receipt_reference)
 VALUES(@id,'access','erased+'||@id::uuid::text,'fulfilled','erased',now(),now()+interval '30 days','phone',@receipt)`, pgx.StrictNamedArgs{"id": request, "receipt": "RCPT-" + request.String()})
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		// These images predate the older eraser's domain-only case retirement.
		image := map[string]any{"subject_ref": contact.String(), "email": "private@example.test", "status": "fulfilled"}
		if _, err := storekit.AuditWithEvidence(e.Admin(), tx, "create", "data_subject_request", request, nil, image, nil); err != nil {
			return err
		}
		if _, err := storekit.AuditWithEvidence(e.Admin(), tx, "update", "data_subject_request", request, map[string]any{"status": "in_progress"}, image, nil); err != nil {
			return err
		}
		_, err := storekit.AuditWithEvidence(e.Admin(), tx, "erase", "contact", contact.UUID, nil, nil, map[string]any{"cause": "contact_erasure"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestLinkedAuditTaskCorrectsBothPhasesAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	request := legacyRequestAuditCopies(t, e)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	task := linkedAuditRedactionDataTask()
	for i := range task.Steps {
		task.Steps[i].Limit = 1
	}
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=@request AND concat(before::text,after::text,evidence::text) LIKE '%private@example.test%'`, pgx.StrictNamedArgs{"request": request}); n != 0 {
		t.Fatalf("kept %d personal request images", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=@request AND action='anonymize'`, pgx.StrictNamedArgs{"request": request}); n != 0 {
		t.Fatalf("added %d domain erasure boundaries", n)
	}
	for _, step := range task.Steps {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1000, RunID: ids.NewV7()})
			if err == nil && out.Changed != 0 {
				t.Errorf("replay changed %d entries", out.Changed)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLinkedAuditTaskRollsBackPayloadsAndStopsBeforeIdentityRemoval(t *testing.T) {
	e := integration.Setup(t)
	request := legacyRequestAuditCopies(t, e)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	task := linkedAuditRedactionDataTask()
	batch := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := batch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		if out.Changed == 0 {
			t.Error("the rollback batch did not change its fixture")
		}
		return out, errors.New("interrupt after linked payload correction")
	}
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=@request AND after->>'email'='private@example.test'`, pgx.StrictNamedArgs{"request": request}); n != 2 {
		t.Fatalf("rollback kept %d of two original images", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='audit_log' AND evidence->>'legacy_erased_record'='true'`); n != 0 {
		t.Fatalf("rollback left %d correction proofs", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := batch(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 1})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
}
