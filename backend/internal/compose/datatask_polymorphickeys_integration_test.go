// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/lifecyclereview"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// keyedTables are the tables the task keys, each with its trigger and marker.
var keyedTables = []struct{ table, trigger, marker string }{
	{"signal", "trg_signal_entity_keys", "entity_unkeyed"},
	{"weekly_plan_commitment", "trg_weekly_plan_commitment_linked_keys", "linked_record_unkeyed"},
	{"user_record_view", "trg_user_record_view_entity_keys", "entity_unkeyed"},
}

// writeBeforeTheKeys runs write as the tables stood before the keys: no trigger
// fills them, and a new row carries the marker.
func writeBeforeTheKeys(t *testing.T, owner *pgx.Conn, write func(pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		for _, k := range keyedTables {
			if _, err := tx.Exec(ctx, `ALTER TABLE `+k.table+` DISABLE TRIGGER `+k.trigger+`,
				ALTER COLUMN `+k.marker+` SET DEFAULT true`); err != nil {
				return err
			}
		}
		if err := write(tx); err != nil {
			return err
		}
		for _, k := range keyedTables {
			if _, err := tx.Exec(ctx, `ALTER TABLE `+k.table+` ENABLE TRIGGER `+k.trigger+`,
				ALTER COLUMN `+k.marker+` SET DEFAULT false`); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("writing rows as they stood before the keys: %v", err)
	}
}

func TestThePolymorphicKeysTaskKeysLiveRowsAndEndsTheRest(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	owner := integration.OwnerConn(t)
	contact := e.SeedContact(t, "Ines Vogel", nil)
	company := e.SeedCompany(t, "Vogel Maschinen GmbH", nil)
	gone := ids.NewV7()
	now := clocktest.Now(t)

	writeBeforeTheKeys(t, owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO signal (kind, entity_type, entity_id, summary, source, captured_by)
			VALUES ('risk', 'contact', $1, 'Live', 'test', 'human:seed'),
			       ('risk', 'deal', $2, 'Orphan', 'test', 'human:seed')`, contact, gone); err != nil {
			return err
		}
		if err := seedContractEndedSignal(ctx, tx, company); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			WITH plan AS (
			  INSERT INTO weekly_plan (owner_id, local_week_start, captured_by)
			  VALUES ($1, date '2026-10-19', 'human:seed') RETURNING id)
			INSERT INTO weekly_plan_commitment (plan_id, label, linked_record_type, linked_record_id, captured_by)
			SELECT plan.id, v.label, v.kind, v.record, 'human:seed'
			  FROM plan, (VALUES ('Live', 'contact', $2::uuid), ('Orphan', 'deal', $3::uuid)) v(label, kind, record)`,
			e.AdminUser, contact, gone); err != nil {
			return err
		}
		if _, err := company360.RecordVisit(ctx, tx, ids.From[ids.UserKind](e.AdminUser), "company", company, now); err != nil {
			return err
		}
		_, err := company360.RecordVisit(ctx, tx, ids.From[ids.UserKind](e.AdminUser), "contact", gone, now)
		return err
	})

	stamp := func() string {
		return e.WsScalar(t, `SELECT concat_ws(',', version, updated_at) FROM signal WHERE entity_id = $1`, contact)
	}
	stampBefore := stamp()
	shown := readLifecycleFinding(ctx, t, e, company)
	if !lifecyclereview.Actionable(shown) {
		t.Fatalf("the seeded account-ending signal is not actionable (%s), so the pin check below proves nothing",
			lifecyclereview.BlockingReason(shown))
	}

	task := polymorphicKeysDataTask()
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 1, 1
	}
	runMachineAddressTask(ctx, t, e, task)

	for _, check := range []struct {
		name, query string
		record      ids.UUID
		want        string
	}{
		{"the live signal", `SELECT concat_ws(',', entity_unkeyed, entity_contact_id)
			FROM signal WHERE entity_id = $1`, contact, "f," + contact.String()},
		{"the orphan signal", `SELECT count(*)::text FROM signal WHERE entity_id = $1`, gone, "0"},
		{"the live commitment", `SELECT concat_ws(',', linked_record_unkeyed, linked_contact_id)
			FROM weekly_plan_commitment WHERE linked_record_id = $1`, contact, "f," + contact.String()},
		{"the orphan commitment", `SELECT concat_ws(',', linked_record_unkeyed, linked_record_type, linked_record_id)
			FROM weekly_plan_commitment c JOIN weekly_plan p ON p.id = c.plan_id
			WHERE p.owner_id = $1 AND c.label = 'Orphan'`, e.AdminUser, "f"},
		{"the live view", `SELECT concat_ws(',', entity_unkeyed, entity_company_id)
			FROM user_record_view WHERE entity_id = $1`, company, "f," + company.String()},
		{"the orphan view", `SELECT count(*)::text FROM user_record_view WHERE entity_id = $1`, gone, "0"},
	} {
		if got := e.WsScalar(t, check.query, check.record); got != check.want {
			t.Errorf("%s reads %q after the task, want %q", check.name, got, check.want)
		}
	}

	if got := stamp(); got != stampBefore {
		t.Errorf("keying moved the signal's version and updated_at from %q to %q", stampBefore, got)
	}
	if current := readLifecycleFinding(ctx, t, e, company); !lifecyclereview.Matches(current, &shown) {
		t.Errorf("keying made a pinned lifecycle approval stale: shown %+v, now %+v", shown, current)
	}

	for _, step := range task.Steps {
		var replay datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			replay, err = step.Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if replay.Changed != 0 {
			t.Errorf("a second run of %s changed %d rows, want none", step.Name, replay.Changed)
		}
	}

	// The keys the task filled are the ones a delete now follows.
	e.WsExec(t, `DELETE FROM contact WHERE id = $1`, contact)
	e.WsExec(t, `DELETE FROM company WHERE id = $1`, company)
	if left := e.WsCount(t, `SELECT count(*) FROM signal WHERE entity_id = $1`, contact); left != 0 {
		t.Errorf("%d signal(s) still name the deleted contact", left)
	}
	if got := e.WsScalar(t, `SELECT concat_ws(',', linked_record_type, linked_record_id, label)
		FROM weekly_plan_commitment WHERE label = 'Live'`); got != "Live" {
		t.Errorf("the commitment on the deleted contact reads %q, want its label with no link", got)
	}
	if left := e.WsCount(t, `SELECT count(*) FROM user_record_view WHERE entity_id = $1`, company); left != 0 {
		t.Errorf("%d mark(s) still name the deleted company", left)
	}
}

// seedContractEndedSignal writes an account-ending signal on the company whose
// evidence quotes an activity, the shape a lifecycle approval pins.
func seedContractEndedSignal(ctx context.Context, tx pgx.Tx, company ids.UUID) error {
	const body = "We will not renew the contract after March."
	var activity ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO activity (kind, subject, body, occurred_at, source, captured_by)
		VALUES ('note', 'Renewal', $1, now(), 'manual', 'test') RETURNING id`, body).Scan(&activity); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(values.CollapseSpace(body)))
	evidence := fmt.Sprintf(`[{"snippet": %q, "source_type": "activity", "source_id": %q, "lifecycle_scope": "company",
		"source_quote": {"start": 0, "end": %d, "sha256": %q}}]`, body, activity.String(), len(body), hex.EncodeToString(digest[:]))
	_, err := tx.Exec(ctx, `
		INSERT INTO signal (kind, entity_type, entity_id, resolved_company_id, summary, evidence, source, captured_by)
		VALUES ('contract_ended', 'company', $1, $1, 'Contract ends', $2::jsonb, 'test', 'human:seed')`,
		company, evidence)
	return err
}

// readLifecycleFinding is what a lifecycle review reads and pins of the
// company's account-ending signal.
func readLifecycleFinding(ctx context.Context, t *testing.T, e *integration.Env, company ids.UUID) lifecyclereview.Finding {
	t.Helper()
	var finding lifecyclereview.Finding
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var signal ids.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM signal WHERE kind = 'contract_ended' AND entity_id = $1`,
			company).Scan(&signal); err != nil {
			return err
		}
		var err error
		finding, err = lifecyclereview.Read(ctx, tx, ids.From[ids.CompanyKind](company), signal)
		return err
	}); err != nil {
		t.Fatalf("reading the lifecycle finding: %v", err)
	}
	return finding
}

// RecordVisit keys the mark it writes, so a delete reaches a mark made today.
func TestAVisitMarkGoesWithItsRecord(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Brandt Logistik AG", nil)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := company360.RecordVisit(e.Admin(), tx, ids.From[ids.UserKind](e.AdminUser), "company", company, clocktest.Now(t))
		return err
	}); err != nil {
		t.Fatalf("recording the visit: %v", err)
	}
	if got := e.WsScalar(t, `SELECT entity_company_id::text FROM user_record_view WHERE entity_id = $1`,
		company); got != company.String() {
		t.Fatalf("the mark's company key = %q, want the company the visit named", got)
	}
	e.WsExec(t, `DELETE FROM company WHERE id = $1`, company)
	if left := e.WsCount(t, `SELECT count(*) FROM user_record_view WHERE entity_id = $1`, company); left != 0 {
		t.Errorf("%d mark(s) still name the deleted company", left)
	}
}
