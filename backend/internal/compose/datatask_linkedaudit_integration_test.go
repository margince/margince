// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
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
	task := linkedAuditRedactionDataTask(e.Pool)
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
	task := linkedAuditRedactionDataTask(e.Pool)
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

func legacyWeeklyCopies(t *testing.T, e *integration.Env, workingText string) ids.UUID {
	t.Helper()
	plan, commitment := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO weekly_plan(id,owner_id,local_week_start,captured_by) VALUES(@id,@owner,date '2026-11-02','system:legacy') ON CONFLICT(owner_id,local_week_start) DO NOTHING`, pgx.StrictNamedArgs{"id": plan, "owner": e.AdminUser})
	e.WsExec(t, `INSERT INTO weekly_plan_commitment(id,plan_id,label,help_requested,due_on,position,captured_by) SELECT @id,p.id,'(erased: the contact this named exercised erasure)',@working,date '2026-11-06',(SELECT count(*) FROM weekly_plan_commitment c WHERE c.plan_id=p.id),'system:legacy' FROM weekly_plan p WHERE p.owner_id=@owner AND p.local_week_start=date '2026-11-02'`, pgx.StrictNamedArgs{"id": commitment, "owner": e.AdminUser, "working": workingText})
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		audit, err := storekit.AuditWithEvidence(e.Admin(), tx, "create", "weekly_plan_commitment", commitment, nil, map[string]any{"label": "Private Contact", "help_requested": "private@example.test"}, nil)
		if err != nil {
			return err
		}
		_, err = tx.Exec(e.Admin(), `INSERT INTO event_outbox(stream,envelope) VALUES('gw:events:crm:weekly_plan_commitment',jsonb_build_object('entity',jsonb_build_object('type','weekly_plan_commitment','id',@record::uuid::text),'trace',jsonb_build_object('audit_log_id',@audit::uuid::text),'payload',jsonb_build_object('name','Private Contact')))`, pgx.StrictNamedArgs{"record": commitment, "audit": audit})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return commitment
}

func TestMarkedWeeklyTaskClearsOldCopiesKeepsLaterWorkAndReplays(t *testing.T) {
	e := integration.Setup(t)
	cleared := legacyWeeklyCopies(t, e, "")
	changed := legacyWeeklyCopies(t, e, "New authorized work")
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	task := markedAuditRedactionDataTask()
	for i := range task.Steps {
		task.Steps[i].Limit = 1
	}
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND concat(before::text,after::text,evidence::text) LIKE '%Private Contact%'`, cleared); n != 0 {
		t.Fatalf("kept %d old personal images", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=$1::uuid::text`, cleared); n != 0 {
		t.Fatalf("kept %d old events", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND after->>'label'='Private Contact'`, changed); n != 1 {
		t.Fatalf("later working text lost its history: %d", n)
	}
	for _, step := range task.Steps {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1000, RunID: ids.NewV7()})
			if err == nil && out.Changed != 0 {
				t.Fatalf("replay changed %d rows", out.Changed)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMarkedWeeklyTaskKeepsCopiesWhileAnySubjectIsHeld(t *testing.T) {
	e := integration.Setup(t)
	commitment := legacyWeeklyCopies(t, e, "")
	contact := e.SeedContact(t, "Held Contact", nil)
	e.WsExec(t, `UPDATE contact SET legal_hold=true WHERE id=$1`, contact)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, e, markedAuditRedactionDataTask())
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND after->>'label'='Private Contact'`, commitment); n != 1 {
		t.Fatalf("held marker history was removed: %d", n)
	}
}

func TestMarkedWeeklyCorrectionRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	record := legacyWeeklyCopies(t, e, "")
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	task := markedAuditRedactionDataTask()
	for i := range task.Steps {
		if task.Steps[i].Name != "marked_weekly_images" {
			continue
		}
		batch := task.Steps[i].Batch
		task.Steps[i].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			out, err := batch(ctx, tx, in)
			if err != nil {
				return out, err
			}
			if out.Changed != 1 {
				t.Errorf("rollback batch changed %d records", out.Changed)
			}
			return out, errors.New("interrupt after marker correction")
		}
	}
	runMachineAddressTask(ctx, t, e, task)
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND after->>'label'='Private Contact'`, record); n != 1 {
		t.Fatalf("rollback lost its old payload: %d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=$1::uuid::text`, record); n != 1 {
		t.Fatalf("rollback lost its old event: %d", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND action='erase'`, record); n != 0 {
		t.Fatalf("rollback kept %d erasure proofs", n)
	}
}

func TestRetiredRequestMarkerCleanupKeepsDisputesAndLaterAnswers(t *testing.T) {
	e := integration.Setup(t)
	cleared, disputed, later := legacyRequestAuditCopies(t, e), legacyRequestAuditCopies(t, e), legacyRequestAuditCopies(t, e)
	e.WsExec(t, `UPDATE data_subject_request SET dispute_open=true WHERE id=$1`, disputed)
	e.WsExec(t, `UPDATE data_subject_request SET resolution='Later authorized answer' WHERE id=$1`, later)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	runMachineAddressTask(ctx, t, e, markedAuditRedactionDataTask())
	for _, id := range []ids.UUID{cleared, disputed, later} {
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=$1 AND after->>'email'='private@example.test'`, id); n != 2 {
			t.Fatalf("dispute must defer all marker copies, got %d", n)
		}
	}
	e.WsExec(t, `UPDATE data_subject_request SET dispute_open=false WHERE id=$1`, disputed)
	if err := NewRetentionServiceFor(e.DB(), nil, slog.Default()).EvaluateInstallation(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []ids.UUID{cleared, disputed} {
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=$1 AND after->>'email'='private@example.test'`, id); n != 0 {
			t.Fatalf("retry kept %d old request images", n)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=$1 AND after->>'email'='private@example.test'`, later); n != 2 {
		t.Fatalf("later answer lost history: %d", n)
	}
}
