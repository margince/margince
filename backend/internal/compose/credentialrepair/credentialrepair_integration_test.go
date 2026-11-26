// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package credentialrepair

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// repairTask is the task as compose registers it, with capture's own eraser.
func repairTask(e *integration.Env) datatask.Task {
	return DataTask(capture.NewPendingStore(e.DB()).PurgeRawCaptureTx)
}

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
			out, err := repairTask(e).Steps[0].Batch(
				system, tx, datatask.BatchIn{Limit: 50},
			)
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
// seededCredential is the password the audit-image tests plant and then assert
// the absence of.
const seededCredential = "Hunter2-Example!"

// auditTrailRefusal is the message audit_log_immutable() raises.
const auditTrailRefusal = "audit metadata is immutable"

// certifyTheRecordOf writes, in the caller's transaction, the system row the
// trigger reads as the repair's certificate for the record auditRow describes.
const certifyTheRecordOf = `
	INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, evidence)
	SELECT gen_random_uuid(), 'system', 'system:credential-repair', 'update', entity_type, entity_id,
	       '{"audit_credentials_masked": true}'::jsonb
	  FROM audit_log WHERE id = $1`

// certificateVariants are rows that each differ from certifyTheRecordOf in one
// column the trigger reads, so none of them may admit a rewrite.
var certificateVariants = []struct{ name, statement string }{
	{"by a human", strings.Replace(certifyTheRecordOf, "'system', 'system:credential-repair'", "'human', 'rep'", 1)},
	{"without the flag", strings.Replace(certifyTheRecordOf,
		`'{"audit_credentials_masked": true}'::jsonb`, `'{"note": true}'::jsonb`, 1)},
	{"carrying an image", strings.Replace(strings.Replace(certifyTheRecordOf,
		"entity_id, evidence)", "entity_id, evidence, after)", 1),
		"::jsonb\n", `::jsonb, '{"subject":"x"}'::jsonb`+"\n", 1)},
	{"of another verb", strings.Replace(certifyTheRecordOf, "'update', entity_type", "'export', entity_type", 1)},
	{"for another record", strings.Replace(certifyTheRecordOf,
		"entity_type, entity_id,\n", "entity_type, gen_random_uuid(),\n", 1)},
}

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
	mail = ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', $2, now(), 'gmail:cred-2', 'connector:gmail')`,
		mail, "Passwort: "+seededCredential)
	return mail, seedAnAuditImageOfAnEditTo(t, e, mail)
}

// seedAnAuditImageOfAnEditTo writes the audit row of an edit to one mail, with
// the credential in both images.
//
// Apart from the activity's own seed, because a held mail is written held: the
// restriction columns carry constraints an UPDATE cannot satisfy on its own.
func seedAnAuditImageOfAnEditTo(t *testing.T, e *integration.Env, mail ids.UUID) ids.UUID {
	t.Helper()
	auditRow := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before, after)
		VALUES ($1, 'human', 'rep', 'update', 'activity', $2, $3, $4)`,
		auditRow, mail,
		[]byte(`{"body":"Passwort: `+seededCredential+`"}`),
		[]byte(`{"body":"Passwort: `+seededCredential+` (korrigiert)"}`))
	return auditRow
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
	if err := e.WsExecErr(t, `UPDATE audit_log SET before = '{"body":"Passwort: [masked]"}'::jsonb WHERE id = $1`, row); err == nil {
		t.Fatal("an audit image was rewritten with the repair's setting off; the trail is not append-only")
	}

	var changed int
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		out, err := repairTask(e).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
		changed = out.Changed
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if changed != 1 {
		t.Fatalf("the repair masked the images of %d mails, want 1", changed)
	}
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		out, err := repairTask(e).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
		changed = out.Changed
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if changed != 0 {
		t.Errorf("a second pass masked the images of %d mails, want 0", changed)
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
		  WHERE entity_id = $1 AND action = 'update'
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
	const cleanBody = "Thanks for the call, see you Tuesday."
	cleanOriginal, cleanMail := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO raw_capture (id, source_system, source_id, payload)
		VALUES ($1, 'gmail', 'clean-3', $2)`, cleanOriginal, []byte(`{"body":"`+cleanBody+`"}`))
	e.WsExec(t, `INSERT INTO activity (id, kind, body, occurred_at, source, captured_by, raw_capture_id)
		VALUES ($1, 'email', $2, now(), 'gmail:clean-3', 'connector:gmail', $3)`,
		cleanMail, cleanBody, cleanOriginal)
	cleanImage := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, before, after)
		VALUES ($1, 'human', 'rep', 'update', 'activity', $2, $3, $4)`,
		cleanImage, cleanMail, []byte(`{"body":"`+cleanBody+`"}`), []byte(`{"body":"`+cleanBody+` "}`))
	cleanTrail := e.WsScalar(t, `SELECT before::text || after::text FROM audit_log WHERE id = $1`, cleanImage)

	run := func(step int) int {
		var changed int
		if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			out, err := repairTask(e).Steps[step].Batch(
				system, tx, datatask.BatchIn{Limit: 50},
			)
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
	if changed := run(1); changed != 0 {
		t.Errorf("the audit step rewrote %d mails' images, want none: only the clean mail has a trail", changed)
	}
	if got := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, cleanMail); got != cleanBody {
		t.Errorf("a mail with no credential was rewritten to %q", got)
	}
	if got := e.WsScalar(t, `SELECT before::text || after::text FROM audit_log WHERE id = $1`, cleanImage); got != cleanTrail {
		t.Errorf("the trail of a mail with no credential changed to %s", got)
	}
	if n := e.WsScalar(t, `SELECT count(*)::text FROM raw_capture WHERE id = $1`, cleanOriginal); n != "1" {
		t.Errorf("the original of a mail with no credential was erased: count %s", n)
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
		out, err := repairTask(e).Steps[0].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
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
		_, err := repairTask(e).Steps[0].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
		return err
	}); err != nil {
		t.Fatalf("the text step: %v", err)
	}
	if n := matches(); n != "0" {
		t.Errorf("the password still matches a search after the repair: count %s", n)
	}
}

// restoreTheStrictTrigger puts back the trigger as it stood before this task's
// migration, read from that migration's down file. It reinstalls the
// migration's own version when the test ends.
func restoreTheStrictTrigger(t *testing.T) {
	t.Helper()
	owner := integration.FreshOwnerConn(t)
	ctx := context.Background()
	t.Cleanup(func() {
		if _, err := owner.Exec(ctx, theRepairsOwnMigration(t, "up")); err != nil {
			t.Errorf("restoring the repair's own trigger: %v", err)
		}
	})
	if _, err := owner.Exec(ctx, theRepairsOwnMigration(t, "down")); err != nil {
		t.Fatalf("restoring the strict trigger: %v", err)
	}
}

// theRepairsOwnMigration reads one direction of this task's migration, found by
// name so a re-stamped migration does not break the test.
func theRepairsOwnMigration(t *testing.T, direction string) string {
	t.Helper()
	found, err := filepath.Glob(
		"../../../migrations/core/*_the_credential_repair_may_rewrite_an_audit_image." + direction + ".sql",
	)
	if err != nil {
		t.Fatalf("looking for the trigger's own migration: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d credential-repair migrations, want 1: %v", len(found), found)
	}
	body, err := os.ReadFile(found[0])
	if err != nil {
		t.Fatalf("reading the trigger's own migration: %v", err)
	}
	return string(body)
}

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

	var out datatask.BatchOut
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = repairTask(e).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
		return err
	}); err != nil {
		t.Fatalf("the audit step failed where it should have stood down: %v", err)
	}
	if !out.Done || out.Changed != 0 {
		t.Errorf("the step answered done=%t changed=%d against a refusing trail, want done and 0",
			out.Done, out.Changed)
	}
	if got := e.WsScalar(t, `SELECT after::text FROM audit_log WHERE id = $1`, auditRow); !strings.Contains(got, "Hunter2") {
		t.Errorf("the strict trail was rewritten anyway: %s", got)
	}
	// The step the credential's own row depends on is unaffected: the trail
	// refusing a rewrite is not a reason to leave the mail readable.
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		_, err := repairTask(e).Steps[0].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
		return err
	}); err != nil {
		t.Fatalf("the text step: %v", err)
	}
	if got := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, mail); strings.Contains(got, "Hunter2") {
		t.Errorf("the mail kept its password: %q", got)
	}
}

// A hold reaches the trail as well as the row.
//
// An audit image of an edit to a held mail quotes the text the row holds.
// Rewriting it would alter the record the hold protects.
//
// The step that masks the row already refuses a held mail, and this is its
// sibling query.
func TestTheRepairLeavesTheAuditImagesOfAHeldMailAlone(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	held := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity
			(id, kind, body, occurred_at, source, captured_by,
			 archived_at, restricted_at, restricted_reason, restricted_until,
			 retention_class, retention_class_at)
		VALUES ($1, 'email', $2, now(), 'gmail:held-trail', 'connector:gmail',
			now(), now(), 'a tax audit of the quarter', now() + interval '2 years',
			'commercial_correspondence', now())`, held, "Passwort: "+seededCredential)
	auditRow := seedAnAuditImageOfAnEditTo(t, e, held)

	var changed int
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		out, err := repairTask(e).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
		changed = out.Changed
		return err
	}); err != nil {
		t.Fatalf("the audit step: %v", err)
	}
	if changed != 0 {
		t.Errorf("the repair rewrote %d held mails' images", changed)
	}
	if got := e.WsScalar(t, `SELECT after::text FROM audit_log WHERE id = $1`, auditRow); !strings.Contains(got, seededCredential) {
		t.Errorf("a held mail's audit image was rewritten: %s", got)
	}
}

// errAcceptedOnPurpose rolls back an accepted rewrite, so the next case meets
// the seeded images.
var errAcceptedOnPurpose = errors.New("accepted, rolled back on purpose")

// An audit row outlives the record it describes, and its images keep their
// credential.
//
// The untold-lead erasure deletes notice mail and leaves the trail standing.
// A reader that joined the record would read those images as absent.
func TestTheRepairMasksTheTrailOfAMailThatIsGone(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	// No activity row for this id: the mail it described was deleted.
	orphan := seedAnAuditImageOfAnEditTo(t, e, ids.NewV7())

	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		_, err := repairTask(e).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 50},
		)
		return err
	}); err != nil {
		t.Fatalf("the audit step: %v", err)
	}
	got := e.WsScalar(t, `SELECT before::text FROM audit_log WHERE id = $1`, orphan)
	if strings.Contains(got, seededCredential) {
		t.Errorf("a deleted mail's trail kept its password: %s", got)
	}
	if !strings.Contains(got, credentialmask.Marker) {
		t.Errorf("the image was not masked: %s", got)
	}
}

// Every record type the masking rules reach is a type this repair reaches.
//
// A scheduled send's audit images store the subject, so a credential typed into
// one is stored the same way a mail's is. The step takes its record types from
// the rules rather than naming them again.
func TestTheRepairMasksEveryRecordTypeTheRulesReach(t *testing.T) {
	e := integration.Setup(t)
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})

	rows := map[string]ids.UUID{}
	for _, entityType := range storekit.MaskedAuditEntityTypes() {
		row := ids.NewV7()
		rows[entityType] = row
		e.WsExec(t, `
			INSERT INTO audit_log
				(id, actor_type, actor_id, action, entity_type, entity_id, before)
			VALUES ($1, 'human', 'rep', 'update', $2, $3, $4)`,
			row, entityType, ids.NewV7(),
			[]byte(`{"subject":"Passwort: `+seededCredential+`"}`))
	}

	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		_, err := repairTask(e).Steps[1].Batch(
			system, tx, datatask.BatchIn{Limit: 100},
		)
		return err
	}); err != nil {
		t.Fatalf("the audit step: %v", err)
	}
	for entityType, row := range rows {
		got := e.WsScalar(t, `SELECT before::text FROM audit_log WHERE id = $1`, row)
		if strings.Contains(got, seededCredential) {
			t.Errorf("a %s image kept its password: %s", entityType, got)
		}
	}
}

// runSteps drives the named steps of the repair in order, each in a
// transaction of its own as the engine runs them. It answers what each changed.
func runSteps(t *testing.T, e *integration.Env, steps ...int) []int {
	t.Helper()
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})
	changed := make([]int, 0, len(steps))
	for _, step := range steps {
		if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			out, err := repairTask(e).Steps[step].Batch(system, tx, datatask.BatchIn{Limit: 50})
			changed = append(changed, out.Changed)
			return err
		}); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
	}
	return changed
}

// seedMailWithAnOriginal writes a mail of kind with its stored original, the way
// capture stored mail before the masking rules.
func seedMailWithAnOriginal(t *testing.T, e *integration.Env, kind, sourceID, subject, body string) (mail, original ids.UUID) {
	t.Helper()
	mail, original = ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO raw_capture (id, source_system, source_id, payload)
		VALUES ($1, 'gmail', $2, $3)`, original, sourceID, []byte(`{"subject":"`+subject+`","body":"`+body+`"}`))
	e.WsExec(t, `INSERT INTO activity
			(id, kind, channel_provider, subject, body, occurred_at, source, captured_by, raw_capture_id)
		VALUES ($1, $2, CASE WHEN $2 = 'message' THEN 'telegram' END, $3, $4, now(),
			'gmail:'||$5, 'connector:gmail', $6)`,
		mail, kind, subject, body, sourceID, original)
	return mail, original
}

// A sender can type the marker. A mail whose text holds it and no credential
// was not masked by this repair, so it keeps its stored original.
func TestAMailWhoseSenderTypedTheMarkerKeepsItsOriginal(t *testing.T) {
	e := integration.Setup(t)
	const body = "The export shows [masked] where a field was hidden."
	mail, original := seedMailWithAnOriginal(t, e, "email", "typed-marker", "Export", body)

	if changed := runSteps(t, e, 0, 2); changed[0] != 0 || changed[1] != 0 {
		t.Errorf("the repair changed %v for a mail with no credential, want none", changed)
	}
	if got := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, mail); got != body {
		t.Errorf("the mail's body changed to %q", got)
	}
	if n := e.WsScalar(t, `SELECT count(*)::text FROM raw_capture WHERE id = $1`, original); n != "1" {
		t.Errorf("the original of a mail the repair did not mask was erased: count %s", n)
	}
}

// The repair reaches a channel message, a credential in the subject alone, and
// an archived mail under no hold. It erases each one's original.
func TestTheRepairReachesEveryMailItsRulesMask(t *testing.T) {
	e := integration.Setup(t)
	message, messageOriginal := seedMailWithAnOriginal(t, e, "message", "chat-1",
		"", "Passwort: "+seededCredential)
	subjectOnly, subjectOriginal := seedMailWithAnOriginal(t, e, "email", "subject-1",
		"Kennwort: "+seededCredential, "See the subject.")
	archived, archivedOriginal := seedMailWithAnOriginal(t, e, "email", "archived-1",
		"Archived", "Passwort: "+seededCredential)
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)

	if changed := runSteps(t, e, 0, 2); changed[0] != 3 || changed[1] != 3 {
		t.Fatalf("the repair masked %d rows and erased %d originals, want 3 and 3", changed[0], changed[1])
	}
	for name, mail := range map[string]ids.UUID{"message": message, "subject-only": subjectOnly, "archived": archived} {
		got := e.WsScalar(t, `SELECT coalesce(subject, '') || ' ' || coalesce(body, '') FROM activity WHERE id = $1`, mail)
		if strings.Contains(got, seededCredential) {
			t.Errorf("the %s mail kept its password: %q", name, got)
		}
	}
	for name, original := range map[string]ids.UUID{
		"message": messageOriginal, "subject-only": subjectOriginal, "archived": archivedOriginal,
	} {
		if n := e.WsScalar(t, `SELECT count(*)::text FROM raw_capture WHERE id = $1`, original); n != "0" {
			t.Errorf("the %s mail's original survived: count %s", name, n)
		}
	}
}

// A mail written by the send path is stored masked already. The repair finds
// nothing in it to change, and does not mask the marker a second time.
func TestTheRepairLeavesASentMailAsTheSendPathStoredIt(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Anna Example", nil)
	outbound := "outbound"
	sent, _, err := e.Activities.LogActivity(e.Admin(), activities.SentRecord(activities.LogActivityInput{
		Kind: "email", Source: "manual", Direction: &outbound,
		Subject: new("Access"), Body: new("Passwort: " + seededCredential),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}))
	if err != nil {
		t.Fatalf("writing the sent mail: %v", err)
	}
	stored := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, sent.Id)
	if strings.Contains(stored, seededCredential) {
		t.Fatalf("the send path stored the password: %q", stored)
	}
	if changed := runSteps(t, e, 0); changed[0] != 0 {
		t.Errorf("the repair rewrote %d sent mails, want none", changed[0])
	}
	if got := e.WsScalar(t, `SELECT body FROM activity WHERE id = $1`, sent.Id); got != stored {
		t.Errorf("the sent mail changed from %q to %q", stored, got)
	}
}

// A hold placed after a step read its page still wins. The step locks the mail
// row before it writes, and the hold is an UPDATE of that row. So the step
// waits for the hold, sees it, and leaves the mail's trail and original alone.
func TestAHoldPlacedAfterTheStepReadItsPageIsRespected(t *testing.T) {
	for _, c := range []struct {
		name string
		step int
		kept func(e *integration.Env, mail, original, auditRow ids.UUID) string
	}{
		{"the audit step", 1, func(e *integration.Env, _, _, auditRow ids.UUID) string {
			return e.WsScalar(t, `SELECT (before::text LIKE '%'||$2||'%')::text FROM audit_log WHERE id = $1`,
				auditRow, seededCredential)
		}},
		{"the raw-capture step", 2, func(e *integration.Env, _, original, _ ids.UUID) string {
			return e.WsScalar(t, `SELECT (count(*) = 1)::text FROM raw_capture WHERE id = $1`, original)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := integration.Setup(t)
			mail, original := seedMailWithAnOriginal(t, e, "email", "race-"+c.name, "Race",
				"Passwort: "+seededCredential)
			auditRow := seedAnAuditImageOfAnEditTo(t, e, mail)
			runSteps(t, e, 0)

			holder, err := integration.FreshOwnerConn(t).Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var holderPID int32
			if err := holder.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
				t.Fatal(err)
			}
			if _, err := holder.Exec(context.Background(), `
				INSERT INTO activity_retention_evidence (activity_id, basis, decided_by_name, reason, qualified_at)
				VALUES ($1, 'controller_pin', 'Test Controller', 'statutory retention under test', now())`,
				mail); err != nil {
				t.Fatal(err)
			}
			if _, err := holder.Exec(context.Background(), `
				UPDATE activity SET archived_at = now(), restricted_at = now(),
				       restricted_reason = 'a tax audit of the quarter', restricted_until = now() + interval '2 years',
				       retention_class = 'commercial_correspondence', retention_class_at = now()
				 WHERE id = $1`, mail); err != nil {
				t.Fatal(err)
			}

			returned := make(chan struct{})
			var changed []int
			go func() {
				defer close(returned)
				changed = runStepsQuietly(e, c.step)
			}()
			prober := integration.FreshOwnerConn(t)
			testdb.WaitForContention(t, returned,
				"the step finished without waiting for the hold's row lock",
				"the step never queued behind the hold",
				func(ctx context.Context) (bool, error) {
					queued, err := testdb.QueuedBehind(ctx, prober, holderPID)
					return queued > 0, err
				})
			if err := holder.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			<-returned
			if len(changed) != 1 || changed[0] != 0 {
				t.Errorf("the step answered %v for a mail held under it, want one step changing nothing", changed)
			}
			if got := c.kept(e, mail, original, auditRow); got != "true" {
				t.Errorf("the held mail's %s was changed", c.name)
			}
		})
	}
}

// runStepsQuietly is runSteps for a goroutine: it reports a failed step as no
// result instead of calling t.Fatal off the test's goroutine.
func runStepsQuietly(e *integration.Env, steps ...int) []int {
	system := principal.WithActor(e.Admin(),
		principal.Principal{Type: principal.PrincipalSystem, ID: "system:credential-repair"})
	var changed []int
	for _, step := range steps {
		if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			out, err := repairTask(e).Steps[step].Batch(system, tx, datatask.BatchIn{Limit: 50})
			changed = append(changed, out.Changed)
			return err
		}); err != nil {
			return nil
		}
	}
	return changed
}
