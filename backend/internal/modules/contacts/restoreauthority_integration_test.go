// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asSeat is e.as() for another seat: user, with grant on contacts and
// companies over every row.
func (e *dedupeEnv) asSeat(t *testing.T, user ids.UUID, grant principal.ObjectGrant) context.Context {
	t.Helper()
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	actor.ID, actor.UserID = "human:"+user.String(), user
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	actor.Permissions.Objects[contactEntity] = grant
	actor.Permissions.Objects[companyEntity] = grant
	return principal.WithActor(e.as(), actor)
}

// restoreRecord is one record type's archive and restore, for the shared walk.
type restoreRecord struct {
	archive func(ctx context.Context) error
	restore func(ctx context.Context, tx pgx.Tx) error
}

// walkRestoreAuthority holds what every restore asks of a seat with edit
// rights alone. The owner restores only an archive they made. An all-rows
// editor who does not own the record restores none. A delete holder restores
// any archive.
func walkRestoreAuthority(t *testing.T, e *dedupeEnv, record restoreRecord) {
	t.Helper()
	owner := e.as()
	manager := e.asSeat(t, ids.NewV7(), principal.ObjectGrant{Read: true, Update: true, Delete: true})
	editor := e.asSeat(t, ids.NewV7(), principal.ObjectGrant{Read: true, Update: true})
	restoreAs := func(ctx context.Context) error {
		return e.store.tx(ctx, func(tx pgx.Tx) error { return record.restore(ctx, tx) })
	}

	if err := record.archive(manager); err != nil {
		t.Fatalf("the manager's archive: %v", err)
	}
	if _, refused := errors.AsType[*auth.RestoreAuthorityError](restoreAs(owner)); !refused {
		t.Error("the owner restored an archive the manager made, want restore_archiver_required")
	}
	if err := restoreAs(manager); err != nil {
		t.Fatalf("the manager's restore: %v", err)
	}

	if err := record.archive(owner); err != nil {
		t.Fatalf("the owner's archive with edit rights: %v", err)
	}
	if _, refused := errors.AsType[*auth.RestoreAuthorityError](restoreAs(editor)); !refused {
		t.Error("an editor of every record who owns none restored it, want restore_archiver_required")
	}
	if err := restoreAs(owner); err != nil {
		t.Fatalf("the owner's restore of their own archive: %v", err)
	}
}

func TestAContactRestoreAsksTheArchivesOwnAuthority(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	created, err := e.store.CreateContact(e.as(), CreateContactInput{FullName: "Owned Olga", Source: "test"})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(created.Id))
	walkRestoreAuthority(t, e, restoreRecord{
		archive: func(ctx context.Context) error {
			_, err := e.store.ArchiveContact(ctx, id, nil)
			return err
		},
		restore: func(ctx context.Context, tx pgx.Tx) error {
			_, err := e.store.RestoreContactTx(ctx, tx, id, nil, pairRestore)
			return err
		},
	})
}

func TestACompanyRestoreAsksTheArchivesOwnAuthority(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	created, err := e.store.CreateCompany(e.as(), CreateCompanyInput{DisplayName: "Owned Works", Source: "test"})
	if err != nil {
		t.Fatalf("create company: %v", err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(created.Id))
	walkRestoreAuthority(t, e, restoreRecord{
		archive: func(ctx context.Context) error {
			_, err := e.store.ArchiveCompany(ctx, id, nil)
			return err
		},
		restore: func(ctx context.Context, tx pgx.Tx) error {
			_, err := e.store.RestoreCompanyTx(ctx, tx, id, nil, pairRestore)
			return err
		},
	})
}

// The restore judges the contact under its row lock. A reassignment that
// commits while the restore waits on the lock moves the contact to another
// owner, and the former owner's restore is refused.
func TestAContactRestoreRacingAReassignmentIsRefusedForTheFormerOwner(t *testing.T) {
	t.Parallel()
	e := setupDedupe(t)
	ctx := e.as()
	created, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Handed Hanna", Source: "test"})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	id := ids.From[ids.ContactKind](ids.UUID(created.Id))
	if _, err := e.store.ArchiveContact(ctx, id, nil); err != nil {
		t.Fatalf("the owner's archive: %v", err)
	}
	reassign, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the reassignment: %v", err)
	}
	t.Cleanup(func() {
		if err := reassign.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the reassignment: %v", err)
		}
	})
	var blocker int32
	if err := reassign.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := reassign.Exec(ctx, `UPDATE contact SET owner_id = $1 WHERE id = $2`, e.otherRep, id); err != nil {
		t.Fatalf("reassigning the contact: %v", err)
	}

	restored := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		restored <- e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := e.store.RestoreContactTx(ctx, tx, id, nil, pairRestore)
			return err
		})
		close(returned)
	}()
	testdb.AwaitQueuedBehind(t, e.store.db.Pool(), blocker, 1, returned, "the restore")
	if err := reassign.Commit(ctx); err != nil {
		t.Fatalf("committing the reassignment: %v", err)
	}
	if _, refused := errors.AsType[*auth.RestoreAuthorityError](<-restored); !refused {
		t.Error("the former owner restored a contact reassigned while the restore waited, want it refused")
	}
}
