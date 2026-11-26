// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnImportCannotAdoptAnObjectDuringOrAfterItsReap(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	key := "ws/import/" + ids.NewV7().String()
	orphanedObjectOfKind(t, e, blob, key, storedobjects.KindImport)
	perms := importerPerms()
	grant := perms.Objects["import_run"]
	grant.Read = true
	perms.Objects["import_run"] = grant
	ctx := e.As(e.Rep1, nil, perms)
	runs := migration.NewRunStore(e.DB())
	assertRefused := func() {
		t.Helper()
		_, err := runs.CreateDraftRun(ctx, migration.CreateDraftRunInput{
			SourceRef: key, Source: "import_api", Profile: []byte(`{"files":[]}`),
		})
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("adopting a condemned source = %v, want upload-again refusal", err)
		}
	}
	called := false
	blob.onDelete = func(deleting string) error {
		if deleting == key {
			called = true
			assertRefused()
		}
		return nil
	}
	if err := reapWorkerOver(e, blob).reapWorkspace(context.Background(), e.WS, unbudgeted); err != nil {
		t.Fatal(err)
	}
	if !called || !blob.deleted[key] {
		t.Fatal("the production reaper did not reach the planted source")
	}
	assertRefused()
	if err := runs.RecordImportSourceIntent(ctx, key); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reusing a reaped key = %v, want refusal", err)
	}
	if _, err := runs.RunOfSource(ctx, key); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the refused adoption left a run: %v", err)
	}
}

func TestAnAdopterHoldingTheIntentMakesTheSweepLeaveItsBytes(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	key := "reap/locked/" + ids.NewV7().String()
	orphanedObject(t, e, blob, key)
	ctx, cancel := context.WithTimeout(e.Admin(), 10*time.Second)
	defer cancel()
	held, proceed := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	release := sync.OnceFunc(func() { close(proceed) })
	go func() {
		finished <- e.DB().Tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT 1 FROM stored_object_intent WHERE storage_key=@key FOR UPDATE`, pgx.StrictNamedArgs{"key": key}); err != nil {
				return err
			}
			close(held)
			<-proceed
			return storedobjects.Clear(ctx, tx, key)
		})
	}()
	defer func() {
		release()
		if err := <-finished; err != nil {
			t.Errorf("adopter transaction: %v", err)
		}
	}()
	select {
	case <-held:
	case err := <-finished:
		finished <- err
		t.Fatalf("adopter did not acquire its intent: %v", err)
	}
	if err := reapWorkerOver(e, blob).reapWorkspace(ctx, e.WS, unbudgeted); err != nil {
		t.Fatal(err)
	}
	if blob.deleted[key] {
		t.Fatal("the sweep deleted bytes while their adopter held the intent")
	}
}

func TestAnImportCannotAdoptWhileAClaimedDeleteNeedsARetry(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	key := "reap/retry-import/" + ids.NewV7().String()
	orphanedObjectOfKind(t, e, blob, key, storedobjects.KindImport)
	blob.refuseDelete = errors.New("temporary object-store failure")
	worker := reapWorkerOver(e, blob)
	if err := worker.reapWorkspace(context.Background(), e.WS, unbudgeted); err != nil {
		t.Fatal(err)
	}
	runs := migration.NewRunStore(e.DB())
	_, err := runs.CreateDraftRun(e.As(e.Rep1, nil, importerPerms()), migration.CreateDraftRunInput{SourceRef: key, Source: "import_api", Profile: []byte(`{"files":[]}`)})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("adoption during delete retry was not refused: %v", err)
	}
	blob.refuseDelete = nil
	if err := worker.reapWorkspace(context.Background(), e.WS, unbudgeted); err != nil {
		t.Fatal(err)
	}
	if !blob.deleted[key] {
		t.Fatal("the claimed object was not deleted on retry")
	}
}
