// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A refusal recorded while retention's contact/erase was on its way keeps the
// contact. The stop writer holds the subject's stop lock, the erasure queues
// behind it, and the refusal it then sees is inside its floor.
func TestARefusalRecordedDuringRetentionErasureKeepsTheContact(t *testing.T) {
	t.Parallel()
	env := setupSARIdentifiers(t)
	ctx := context.Background()
	holder, err := pgx.Connect(ctx, testdb.For(t).OwnerDSN)
	if err != nil {
		t.Fatalf("opening the stop writer's connection: %v", err)
	}
	t.Cleanup(func() {
		if err := holder.Close(context.Background()); err != nil {
			t.Errorf("closing the stop writer's connection: %v", err)
		}
	})
	stopWriter, err := holder.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mustExec(ctx, t, stopWriter, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, env.contact)
	recordKeptRefusal(ctx, t, stopWriter, env.contact)
	var holderPID int32
	if err := stopWriter.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID); err != nil {
		t.Fatal(err)
	}

	svc := &RetentionService{db: env.db, eraser: NewEraser(env.db)}
	erased := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		erased <- svc.apply(env.ctx, retentionPolicy{ID: ids.NewV7(), ObjectType: "contact", Action: actionErase, RetainDays: 30}, env.contact.UUID)
	}()
	testdb.AwaitQueuedBehind(t, env.owner, holderPID, 1, returned, "retention's contact/erase")
	if err := stopWriter.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-erased; err != nil {
		t.Fatalf("a contact the floor keeps should be skipped, got %v", err)
	}

	if name := contactName(ctx, t, env); name != "Sara Subject" {
		t.Errorf("retention erased a contact whose refusal is inside its floor: name is now %q", name)
	}
	var refusals int
	if err := env.owner.QueryRow(ctx,
		`SELECT count(*) FROM communication_suppression WHERE contact_id = $1`, env.contact).Scan(&refusals); err != nil {
		t.Fatal(err)
	}
	if refusals != 1 {
		t.Errorf("the refusal recorded during the pass is gone: %d left, want 1", refusals)
	}
}

// The refusal floor holds retention only. An Art. 17 request erases the same
// contact, because its cascade puts the addresses on the erasure list.
func TestARequestErasesAContactWhoseRefusalIsInsideItsFloor(t *testing.T) {
	t.Parallel()
	env := setupSARIdentifiers(t)
	ctx := context.Background()
	recordKeptRefusal(ctx, t, env.owner, env.contact)

	if err := NewEraser(env.db).EraseContact(env.ctx, env.contact.UUID, "request"); err != nil {
		t.Fatal(err)
	}
	if name := contactName(ctx, t, env); name != erasedName {
		t.Errorf("an Art. 17 request left the contact named %q, want %q", name, erasedName)
	}
}

func recordKeptRefusal(ctx context.Context, t *testing.T, conn execer, contact ids.ContactID) {
	t.Helper()
	mustExec(ctx, t, conn, `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level, kept_until)
		VALUES ($1, 'marketing_objection', 'operator_ui', 'human:x', 'user', now() + interval '1 year')`, contact)
}

func contactName(ctx context.Context, t *testing.T, env *sarIdentifierEnv) string {
	t.Helper()
	var name string
	if err := env.owner.QueryRow(ctx, `SELECT full_name FROM contact WHERE id = $1`, env.contact).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

// An erasure clears the register a rep chose for a named contact.
func TestErasingAContactClearsItsGreetingRegister(t *testing.T) {
	env := setupSARIdentifiers(t)
	ctx := context.Background()
	mustExec(ctx, t, env.owner, `UPDATE contact SET greeting_register = 'formal' WHERE id = $1`, env.contact)

	if err := NewEraser(env.db).EraseContact(env.ctx, env.contact.UUID, "request"); err != nil {
		t.Fatalf("erasing: %v", err)
	}
	var register *string
	if err := env.owner.QueryRow(ctx,
		`SELECT greeting_register FROM contact WHERE id = $1`, env.contact).Scan(&register); err != nil {
		t.Fatal(err)
	}
	if register != nil {
		t.Errorf("the greeting register %q survived the erasure", *register)
	}
}
