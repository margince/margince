// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identifierspellings_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/identifierspellings"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func legacyColumn(t *testing.T, e *integration.Env, table, column string) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(t.Context(), "ALTER TABLE "+pgx.Identifier{table}.Sanitize()+" DROP CONSTRAINT IF EXISTS "+pgx.Identifier{table + "_" + column + "_one_spelling"}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}

func stepFor(t *testing.T, table string) datatask.Step {
	t.Helper()
	for _, step := range identifierspellings.DataTask().Steps {
		if step.Name == "fold_"+table {
			return step
		}
	}
	t.Fatalf("missing step for %s", table)
	return datatask.Step{}
}

func TestIdentifierSpellingRepairBoundsPagesAndPreservesAtomicHistory(t *testing.T) {
	e := integration.Setup(t)
	legacyColumn(t, e, "contact_email", "email")
	contact := e.SeedContact(t, "Address owner", nil)
	raw := "  re\u200bp@example.test\u00a0"
	row := ids.NewV7()
	e.WsExec(t, `INSERT INTO contact_email(id,contact_id,email,source,captured_by) VALUES($1,$2,$3,'manual','human:test')`, row, contact, raw)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	step := stepFor(t, "contact_email")
	before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, contact)
	abort := errors.New("abort the page")
	err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		count, more, err := step.Estimate(ctx, tx, 0)
		if err != nil {
			return err
		}
		if count != 0 || !more {
			t.Fatalf("unbounded estimate: %d %v", count, more)
		}
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Scanned != 1 || out.Changed != 1 || out.Action != nil {
			t.Fatalf("page: %+v", out)
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT email FROM contact_email WHERE id=$1`, row); got != raw {
		t.Fatalf("rollback left %q", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, contact); got != before {
		t.Fatal("rollback left audit")
	}
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
		first, err := step.Batch(ctx, tx, in)
		if err != nil {
			return err
		}
		if first.Changed != 1 {
			t.Fatalf("changed %d", first.Changed)
		}
		in.Cursor = first.Cursor
		last, err := step.Batch(ctx, tx, in)
		if err != nil {
			return err
		}
		if !last.Done || last.Scanned != 0 {
			t.Fatalf("cursor repeated rows: %+v", last)
		}
		replay, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if replay.Changed != 0 {
			t.Fatal("replay changed a canonical row")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsScalar(t, `SELECT email FROM contact_email WHERE id=$1`, row); got != correspondence.Fold(raw) {
		t.Fatalf("stored %q", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1`, contact); got != before+1 {
		t.Fatalf("audit count %d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='identifier.spelling_changed'`); got != 1 {
		t.Fatalf("event count %d", got)
	}
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error { _, err := step.Batch(e.Admin(), tx, datatask.BatchIn{Limit: 1}); return err }); err == nil {
		t.Fatal("human caller bypassed system-only repair")
	}
}

func TestIdentifierSpellingRepairLeavesCollidingUsersForReview(t *testing.T) {
	e := integration.Setup(t)
	legacyColumn(t, e, "app_user", "email")
	first, second := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user(id,email,display_name) VALUES($1,'duplicate@example.test','First'),($2,$3,'Second')`, first, second, "dupli\u200bcate@example.test")
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	step := stepFor(t, "app_user")
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 100, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 0 || out.Action == nil || len(out.Action.Records) != 1 || out.Action.Records[0].ID != second {
			t.Fatalf("collision: %+v", out)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM app_user WHERE id=ANY($1::uuid[])`, []ids.UUID{first, second}); got != 2 {
		t.Fatal("identities were merged")
	}
	if got := e.WsScalar(t, `SELECT email FROM app_user WHERE id=$1`, second); !strings.Contains(got, "\u200b") {
		t.Fatal("collision changed the second identity")
	}
}

func TestIdentifierSQLAndGoUseTheSameUnicodeSpelling(t *testing.T) {
	e := integration.Setup(t)
	for _, raw := range []string{" \tre\u200bp@example.test\u00a0", "E\u0301@EXAMPLE.TEST", "\u3000REP@EXAMPLE.TEST\u202f", "\r\nA@B.TEST\t"} {
		got := e.WsScalar(t, `SELECT fold_identifier($1)`, raw)
		if want := correspondence.Fold(raw); got != want {
			t.Errorf("SQL %q Go %q for %q", got, want, raw)
		}
	}
}

func TestOwnDomainRepairUsesAnOpaqueCursor(t *testing.T) {
	e := integration.Setup(t)
	legacyColumn(t, e, "workspace_email_domain", "domain")
	e.WsExec(t, `INSERT INTO workspace_email_domain(domain,source) VALUES($1,'admin')`, " exam\u200bple.test ")
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	step := stepFor(t, "workspace_email_domain")
	if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 1 || strings.Contains(string(out.Cursor), "EXAM") {
			t.Fatalf("domain page: %+v", out)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM workspace_email_domain WHERE domain='example.test'`); got != 1 {
		t.Fatal("domain was not repaired")
	}
}
