// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBulkUndoHoldsEveryContactBeforeTakingIdentityLocks(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	contacts := seedSurroundedContacts(t, e, 2)
	engine := bulkEngineFor(e)
	archived, err := engine.Execute(ctx, archiveContacts(itemsOf(contacts)))
	if err != nil {
		t.Fatal(err)
	}
	first, second := contacts[0], contacts[1]
	if first.item.Id.String() > second.item.Id.String() {
		first, second = second, first
	}
	version := int64(e.WsCount(t, `SELECT version FROM contact WHERE id=@id`, pgx.NamedArgs{"id": second.item.Id}))
	release := make(chan struct{})
	finishSingle := sync.OnceFunc(func() { close(release) })
	defer finishSingle()
	ready, singleDone := make(chan int, 1), make(chan error, 1)
	go func() {
		singleDone <- e.DB().Tx(ctx, func(tx pgx.Tx) error {
			if _, err := storekit.LockRow(ctx, tx, "contact", ids.UUID(second.item.Id), storekit.IncludeArchived); err != nil {
				ready <- 0
				return err
			}
			ready <- backendPID(t, tx)
			<-release
			_, err := e.Contacts.RestoreContactTx(ctx, tx, ids.From[ids.ContactKind](ids.UUID(second.item.Id)), &version, unarchiveWith(nil))
			return err
		})
	}()
	holder := <-ready
	if holder == 0 {
		t.Fatal(<-singleDone)
	}
	bulkDone := make(chan error, 1)
	go func() {
		_, err := engine.Undo(ctx, ids.UUID(archived.BatchId), "")
		bulkDone <- err
	}()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		waitForBackendBlockedBy(t, tx, holder, bulkDone)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	probePID, probeDone, returned := make(chan int32, 1), make(chan error, 1), make(chan struct{})
	go func() {
		defer close(returned)
		probeDone <- e.DB().Tx(ctx, func(tx pgx.Tx) error {
			probePID <- int32(backendPID(t, tx))
			return storekit.LockSubjectKeys(ctx, tx, nil, []string{first.email})
		})
	}()
	observer, err := e.Pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Release()
	testdb.AwaitReturnWithoutWaiting(t, observer.Conn(), <-probePID, returned, "the first contact's identity lock")
	if err := <-probeDone; err != nil {
		t.Fatal(err)
	}
	finishSingle()
	if err := <-singleDone; err != nil {
		t.Fatalf("single restore: %v", err)
	}
	if err := <-bulkDone; err != nil {
		t.Fatalf("bulk Undo: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM contact WHERE id=ANY(@ids::uuid[]) AND archived_at IS NULL`, pgx.NamedArgs{"ids": []ids.UUID{ids.UUID(first.item.Id), ids.UUID(second.item.Id)}}); n != 2 {
		t.Fatalf("restored %d contacts, want both", n)
	}
}
