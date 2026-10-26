// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Mail captured before the masking rules keeps its password until the repair.
//
// The row is seeded with a statement rather than through the capture sink,
// because the sink masks on write now.
//
// The state this repairs cannot be reached through the live writer any more,
// which is the whole reason the repair exists.
func TestTheRepairMasksAPasswordStoredBeforeTheRulesAndLeavesTypedTextAlone(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	// Both texts carry a label the rules recognise, which is the contract. The
	// repair masks what the shared rules find, so a credential behind a label
	// they do not know is not this task's to remove.
	const password = "Hunter2-Example!"
	const body = "Login: anna@example.com\nPasswort: " + password
	mail := ids.NewV7()
	note := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', $3, $4, now(), 'gmail:cred-1', 'connector:gmail'),
		       ($2, 'note',  $3, $4, now(), 'manual',       'user')`,
		mail, note, "Kennwort: "+password, body)

	run := func() int {
		var changed int
		if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			out, err := maskStoredCredentialsDataTask(e.Pool).Steps[0].Batch(
				system, tx, datatask.BatchIn{Limit: 50})
			changed = out.Changed
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return changed
	}

	if changed := run(); changed != 1 {
		t.Fatalf("the first run masked %d rows, want the mail alone", changed)
	}
	for _, column := range []string{"subject", "body"} {
		stored := e.WsScalar(t, `SELECT `+column+` FROM activity WHERE id = $1`, mail)
		if strings.Contains(stored, seededCredential) {
			t.Errorf("the mail's %s still holds the password: %q", column, stored)
		}
		if !strings.Contains(stored, credentialmask.Marker) {
			t.Errorf("the mail's %s was rewritten without the marker: %q", column, stored)
		}
	}
	// A note keeps what a member typed. Masking it would put the marker where
	// they wrote a word, and the rules exempt its kind for that reason.
	if stored := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, note); !strings.Contains(stored, password) {
		t.Errorf("the note's body was masked: %q", stored)
	}
	if changed := run(); changed != 0 {
		t.Errorf("the second run masked %d rows, want 0: masked text masks to itself", changed)
	}
}

// The audit images of an edit lose the password, and the trail says so.
//
// audit_log is append-only, so this is the one permitted rewrite of an image.
// seededCredential is the password the audit-image tests plant and then assert
// the absence of.
const seededCredential = "Hunter2-Example!"

// seedAMailWithACredentialInItsAuditImage writes the state this repair exists
// for. A mail whose text holds a password, and the audit images of an edit to
// it, which hold the same password.
//
// Written with statements rather than through the live writers, because those
// mask on write now. The state is unreachable through them, which is why a
// repair is owed at all.
func seedAMailWithACredentialInItsAuditImage(
	t *testing.T, e *integration.Env,
) (mail, auditRow ids.UUID) {
	t.Helper()
	mail, auditRow = ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, now(), 'gmail:cred-2', 'connector:gmail')`,
		mail, "Passwort: "+seededCredential)
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before, after)
		VALUES ($1, 'human', 'rep', 'update', 'activity', $2, $3, $4)`,
		auditRow, mail,
		[]byte(`{"body":"Passwort: `+seededCredential+`"}`),
		[]byte(`{"body":"Passwort: `+seededCredential+` (korrigiert)"}`))
	return mail, auditRow
}

// The trigger admits it while the repair's setting is on and refuses it
// otherwise. Both halves are asserted here.
//
// A repair nobody can run and a trail anybody can rewrite are each worse than
// the other.
func TestTheRepairMasksAnAuditImageOnlyUnderItsOwnSetting(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	mail, row := seedAMailWithACredentialInItsAuditImage(t, e)

	// An UPDATE without the setting is refused, which is the trail's ordinary
	// rule and the thing the repair is an exception to.
	if err := e.WsExecErr(t, `UPDATE audit_log SET before = '{}'::jsonb WHERE id = $1`, row); err == nil {
		t.Fatal("an audit image was rewritten with the repair's setting off; the trail is not append-only")
	}

	var changed int
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		out, err := maskStoredCredentialsDataTask(e.Pool).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 50})
		changed = out.Changed
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("the repair masked the images of %d mails, want 1", changed)
	}
	for _, column := range []string{"before", "after"} {
		stored := e.WsScalar(t, `SELECT `+column+`::text FROM audit_log WHERE id = $1`, row)
		if strings.Contains(stored, seededCredential) {
			t.Errorf("the %s image still holds the password: %q", column, stored)
		}
		if !strings.Contains(stored, credentialmask.Marker) {
			t.Errorf("the %s image was rewritten without the marker: %q", column, stored)
		}
	}
	// The repair records that it reached this mail, on evidence and with no
	// content. An image here would be projected as a field change.
	//
	// It would also have to name what the trail held to say anything.
	note := e.WsScalar(t, `SELECT coalesce(before::text, '') || coalesce(after::text, '')
			|| ' evidence=' || evidence::text
		   FROM audit_log
		  WHERE entity_id = $1 AND action = 'erase'
		    AND evidence ? $2`, mail, storekit.EvidenceKeyMaskedAuditImages)
	if !strings.Contains(note, "true") {
		t.Errorf("the repair left no row saying it masked this mail's trail: %q", note)
	}
	if strings.Contains(note, seededCredential) {
		t.Errorf("the repair's own audit row carries the password: %q", note)
	}
	if before, _, _ := strings.Cut(note, " evidence="); before != "" {
		t.Errorf("the repair's own audit row carries images: %q", before)
	}
}

// The provider payload goes too, because it holds the text the row no longer does.
//
// The step derives its own work from the marker rather than from what an
// earlier step reported. A run that stopped after the text still reaches the
// originals on its next pass.
//
// The steps are driven here in the order the task declares them.
func TestTheRepairErasesTheStoredOriginalOfAMailItMasked(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	const password = "Hunter2-Example!"
	original := ids.NewV7()
	mail := ids.NewV7()
	e.WsExec(t, `INSERT INTO raw_capture (id, source_system, source_id, payload)
		VALUES ($1, 'gmail', 'cred-3', $2)`, original, []byte(`{"body":"Passwort: `+password+`"}`))
	e.WsExec(t, `INSERT INTO activity (id, kind, body, occurred_at, source, captured_by, raw_capture_id)
		VALUES ($1, 'email', $2, now(), 'gmail:cred-3', 'connector:gmail', $3)`,
		mail, "Passwort: "+password, original)

	run := func(step int) int {
		var changed int
		if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			out, err := maskStoredCredentialsDataTask(e.Pool).Steps[step].Batch(
				system, tx, datatask.BatchIn{Limit: 50})
			changed = out.Changed
			return err
		}); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		return changed
	}

	if changed := run(0); changed != 1 {
		t.Fatalf("the text step masked %d rows, want the mail", changed)
	}
	// The original is still there, and still holds the password: masking the
	// row does not reach it, which is why this step exists.
	if n := e.WsScalar(t, `SELECT count(*)::text FROM raw_capture WHERE id = $1`, original); n != "1" {
		t.Fatalf("the stored original went before its own step ran: count %s", n)
	}
	if changed := run(2); changed != 1 {
		t.Fatalf("the originals step erased %d, want the masked mail's", changed)
	}
	if n := e.WsScalar(t, `SELECT count(*)::text FROM raw_capture WHERE id = $1`, original); n != "0" {
		t.Errorf("the stored original survived the repair: count %s", n)
	}
	if changed := run(2); changed != 0 {
		t.Errorf("a second pass erased %d originals, want 0", changed)
	}
}

// A mail under a statutory hold keeps its text, credential and all.
//
// The hold says the record is not to be altered, and that outranks this repair.
// The credential is then named in the hold's own review rather than masked out
// from under it.
func TestTheRepairLeavesAMailUnderAStatutoryHoldAlone(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	const body = "Passwort: Hunter2-Example!"
	held := ids.NewV7()
	// A held row is an archived row by CHECK, and a hold carries its reason, its
	// end and a retention class. The repair reaches archived mail, so the hold
	// is what refuses here and not the archive.
	e.WsExec(t, `INSERT INTO activity
			(id, kind, body, occurred_at, source, captured_by,
			 archived_at, restricted_at, restricted_reason, restricted_until,
			 retention_class, retention_class_at)
		VALUES ($1, 'email', $2, now(), 'gmail:held', 'connector:gmail',
			now(), now(), 'a tax audit of the quarter', now() + interval '2 years',
			'commercial_correspondence', now())`, held, body)

	var changed int
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		out, err := maskStoredCredentialsDataTask(e.Pool).Steps[0].Batch(
			system, tx, datatask.BatchIn{Limit: 50})
		changed = out.Changed
		return err
	}); err != nil {
		t.Fatalf("the text step: %v", err)
	}
	if changed != 0 {
		t.Errorf("the repair masked %d held rows, want none", changed)
	}
	if got := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, held); got != body {
		t.Errorf("the held mail's text changed to %q", got)
	}
}

// Masking the row takes the credential out of search as well.
//
// search_tsv is generated from subject and body, so the index follows the
// columns with nothing to rebuild. This asserts the acceptance rather than the
// mechanism: a reader who changes how search is fed should see this fail.
func TestTheRepairTakesTheMaskedPasswordOutOfSearch(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	const password = "Hunter2xample"
	mail := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, now(), 'gmail:cred-4', 'connector:gmail')`,
		mail, "Passwort: "+password)

	matches := func() string {
		return e.WsScalar(t, `SELECT count(*)::text FROM activity
			 WHERE id = $1 AND search_tsv @@ plainto_tsquery('simple', f_unaccent($2))`,
			mail, password)
	}
	if n := matches(); n != "1" {
		t.Fatalf("the password was not searchable before the repair: count %s", n)
	}
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		_, err := maskStoredCredentialsDataTask(e.Pool).Steps[0].Batch(
			system, tx, datatask.BatchIn{Limit: 50})
		return err
	}); err != nil {
		t.Fatalf("the text step: %v", err)
	}
	if n := matches(); n != "0" {
		t.Errorf("the password still matches a search after the repair: count %s", n)
	}
}

// strictAuditTrigger restores the append-only function as it stood before this
// task's migration, so a test can meet the installation the task has to
// tolerate.
const strictAuditTrigger = `
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END; $$;`

// restoreTheStrictTrigger puts the append-only function back as it stood before
// this task's migration. It restores the migration's version afterwards, so the
// clone is left as the next test expects it.
//
// It connects as the owner, because replacing a function is schema and the app
// role cannot. The DSN is the one Setup reads.
func restoreTheStrictTrigger(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(ctx, permissiveAuditTrigger); err != nil {
			t.Errorf("restoring the repair's own trigger: %v", err)
		}
		if err := owner.Close(ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	if _, err := owner.Exec(ctx, strictAuditTrigger); err != nil {
		t.Fatalf("restoring the strict trigger: %v", err)
	}
}

// permissiveAuditTrigger is the function this task's migration installs, kept
// here so the test above can put back what it replaced.
const permissiveAuditTrigger = `
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND current_setting('margince.audit_credential_repair', true) = 'on'
     AND (to_jsonb(NEW) - 'before' - 'after') = (to_jsonb(OLD) - 'before' - 'after')
  THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'audit_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END; $$;`

// An installation that restored the strict trigger still gets its rows masked.
//
// Migrations run before data tasks, so one upgrading across two releases at
// once meets this state with the task still in the catalog. The step leaves the
// trail alone and reports nothing, rather than failing the repair.
func TestTheRepairSkipsTheAuditStepWhereTheTrailRefusesIt(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	mail, auditRow := seedAMailWithACredentialInItsAuditImage(t, e)
	restoreTheStrictTrigger(t)

	var changed int
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		out, err := maskStoredCredentialsDataTask(e.Pool).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 50})
		changed = out.Changed
		return err
	}); err != nil {
		t.Fatalf("the audit step failed where it should have stood down: %v", err)
	}
	if changed != 0 {
		t.Errorf("the step reported %d rewrites against a refusing trail", changed)
	}
	if got := e.WsScalar(t, `SELECT after::text FROM audit_log WHERE id = $1`, auditRow); !strings.Contains(got, "Hunter2") {
		t.Errorf("the strict trail was rewritten anyway: %s", got)
	}
	// The step the credential's own row depends on is unaffected: the trail
	// refusing a rewrite is not a reason to leave the mail readable.
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		_, err := maskStoredCredentialsDataTask(e.Pool).Steps[0].Batch(
			system, tx, datatask.BatchIn{Limit: 50})
		return err
	}); err != nil {
		t.Fatalf("the text step: %v", err)
	}
	if got := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, mail); strings.Contains(got, "Hunter2") {
		t.Errorf("the mail kept its password: %q", got)
	}
}

// The grant is column-scoped and the trigger checks the columns, so the setting
// buys a rewrite of the images and nothing else.
//
// Two nets rather than one, because the setting is reachable by anything
// running in the transaction.
//
// A bug that turned it on elsewhere must still not be able to rewrite what an
// audit row says happened.
func TestTheAuditExceptionRefusesAChangeToWhatTheRowSaysHappened(t *testing.T) {
	e := integration.Setup(t)
	_, auditRow := seedAMailWithACredentialInItsAuditImage(t, e)

	for _, change := range []struct {
		name, set string
	}{
		{"the action", `action = 'delete'`},
		{"the entity", `entity_type = 'contact'`},
		{"the actor", `actor_id = gen_random_uuid()`},
	} {
		t.Run(change.name, func(t *testing.T) {
			ws := principal.WithWorkspaceID(context.Background(), e.WS)
			err := database.WithWorkspaceTx(ws, e.Pool, func(tx pgx.Tx) error {
				if _, err := tx.Exec(ws,
					`SELECT set_config('margince.audit_credential_repair', 'on', true)`); err != nil {
					return err
				}
				_, err := tx.Exec(ws,
					`UPDATE audit_log SET `+change.set+` WHERE id = $1`, auditRow)
				return err
			})
			if err == nil {
				t.Fatalf("%s was rewritable under the repair's setting", change.name)
			}
			if !strings.Contains(err.Error(), "append-only") &&
				!strings.Contains(err.Error(), "permission denied") {
				t.Errorf("the refusal was not the trail's: %v", err)
			}
			// As the owner, where no grant binds and the trigger's column check
			// is the only net left. Without this the check is untested: the app
			// role is refused by the narrow grant whatever the trigger says.
			if err := ownerUpdateUnderTheRepairSetting(t, change.set, auditRow); err == nil {
				t.Errorf("%s was rewritable by the owner under the repair's setting", change.name)
			} else if !strings.Contains(err.Error(), "append-only") {
				t.Errorf("the owner's refusal was not the trigger's: %v", err)
			}
		})
	}
}

// ownerUpdateUnderTheRepairSetting runs one UPDATE of audit_log as the owner
// with the repair's setting on, and reports what the trail answered.
//
// The owner holds every grant, so this reaches the trigger directly. Both
// statements share one transaction because the setting is local to it.
func ownerUpdateUnderTheRepairSetting(t *testing.T, set string, auditRow ids.UUID) error {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(ctx); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	}()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the owner probe: %v", err)
		}
	}()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('margince.audit_credential_repair', 'on', true)`); err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `UPDATE audit_log SET `+set+` WHERE id = $1`, auditRow)
	return err
}
