// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func orphanSignatureNumber(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	contact := seedEnrichContact(t, e, address, "Bob Contact\n"+germany)
	applySignature(t, e, contact, onlyMailOf(t, e, contact), phoneField(germany))
	// The legacy retraction removed evidence but left the writer's phone row.
	e.WsExec(t, `DELETE FROM contact_profile_field WHERE contact_id = @contact`, pgx.StrictNamedArgs{"contact": contact})
	if got := liveNumbers(t, e, contact); len(got) != 1 {
		t.Fatalf("orphan fixture has %v, want the signature number", got)
	}
	return contact
}

func TestTheSignaturePhoneTaskRestoresOnlyASupportedPredecessor(t *testing.T) {
	for _, originalSource := range []string{"capture_enrich", "manual"} {
		t.Run(originalSource, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			contact := seedEnrichContact(t, e, "changed@phone.example", "Bob Contact\n"+germany)
			originalMail := onlyMailOf(t, e, contact)
			applySignature(t, e, contact, originalMail, phoneField(germany))
			e.WsExec(t, `UPDATE contact_phone SET source = @source WHERE contact_id = @id`,
				pgx.StrictNamedArgs{"source": originalSource, "id": contact})
			seedLaterMail(t, e, contact, "changed@phone.example", "Bob Contact\n"+germanyNew, "1 hour")
			applySignature(t, e, contact, otherMailOf(t, e, contact, originalMail), phoneField(germanyNew))
			if got := liveNumbers(t, e, contact); !slices.Equal(got, []string{"+491715550109"}) {
				t.Fatalf("the fixture did not replace the old number: %v", got)
			}
			e.WsExec(t, `DELETE FROM contact_profile_field WHERE contact_id = @id`, pgx.StrictNamedArgs{"id": contact})
			task := signaturePhonesDataTask()
			task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
			runMachineAddressTask(ctx, t, e, task)
			var want []string
			if originalSource == "manual" {
				want = []string{"+491755550101"}
			}
			if got := liveNumbers(t, e, contact); !slices.Equal(got, want) {
				t.Fatalf("repaired numbers = %v, want %v", got, want)
			}
		})
	}
}

func TestTheSignaturePhoneTaskRetractsOrphansAndKeepsSupportedAndHumanNumbers(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	orphan := orphanSignatureNumber(t, e, "orphan@phone.example")
	human := orphanSignatureNumber(t, e, "human@phone.example")
	e.WsExec(t, `UPDATE contact_phone SET source = 'manual' WHERE contact_id = @contact`, pgx.StrictNamedArgs{"contact": human})
	supported := seedEnrichContact(t, e, "supported@phone.example", "Bob Contact\n"+germany)
	applySignature(t, e, supported, onlyMailOf(t, e, supported), phoneField(germany))
	corrected := orphanSignatureNumber(t, e, "corrected@phone.example")
	if err := ai.NewFeedbackStore(InstallationDB(e.Pool)).Record(e.Admin(), ai.RecordInput{
		SubjectType: "contact", SubjectID: corrected, ClaimKind: ai.ClaimProfileField,
		ClaimPath: ai.ProfileFieldClaimPath("phone"), Verdict: ai.VerdictCorrected,
		CorrectedValue: new("+49305550199"),
	}); err != nil {
		t.Fatal(err)
	}
	task := signaturePhonesDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	runMachineAddressTask(ctx, t, e, task)
	if got := liveNumbers(t, e, orphan); len(got) != 0 {
		t.Fatalf("unsupported number survived: %v", got)
	}
	for name, contact := range map[string]ids.UUID{"human": human, "supported": supported, "corrected": corrected} {
		if got := liveNumbers(t, e, contact); len(got) != 1 {
			t.Errorf("%s number removed: %v", name, got)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = @id AND evidence->>'reason' = @reason`,
		pgx.StrictNamedArgs{"id": orphan, "reason": "signature number has no supporting evidence"}); n != 1 {
		t.Fatalf("retraction audit rows = %d, want one", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id = (o.envelope->'trace'->>'audit_log_id')::uuid
		WHERE a.entity_id = @id AND a.evidence->>'reason' = @reason`,
		pgx.StrictNamedArgs{"id": orphan, "reason": "signature number has no supporting evidence"}); n != 1 {
		t.Fatalf("retraction events = %d, want one", n)
	}
	step := task.Steps[0]
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || !out.Done) {
			t.Errorf("replayed repair = %+v, want no changes and done", out)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheSignaturePhoneTaskRollsBackWithItsProgressAndRejectsBadCursors(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	orphan := orphanSignatureNumber(t, e, "rollback@phone.example")
	task := signaturePhonesDataTask()
	batch := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := batch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after phone retraction")
	}
	runMachineAddressTask(ctx, t, e, task)
	if got := liveNumbers(t, e, orphan); len(got) != 1 {
		t.Fatalf("phone retraction escaped rollback: %v", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE evidence->>'reason' = 'signature number has no supporting evidence'`); n != 0 {
		t.Errorf("%d retraction audits escaped rollback", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := batch(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: []byte(`"not-a-uuid"`)})
		return err
	}); err == nil {
		t.Error("a malformed cursor was accepted")
	}
}
