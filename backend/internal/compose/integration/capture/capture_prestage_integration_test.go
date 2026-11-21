// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A new message's files are stored before its capture transaction takes the
// authorization lock, and a guess the transaction overrules leaves no bytes.

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lockWatchingBlob asks, during every put, whether any session holds the
// authorization lock a capture transaction takes, and counts puts and deletes.
type lockWatchingBlob struct {
	blobstore.Store
	e *integration.SearchEnv

	mu                    sync.Mutex
	puts, deletes, locked int
	failure               error
}

func (b *lockWatchingBlob) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	var held int
	err := database.WithWorkspaceTx(b.e.Admin(), b.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_locks
			 WHERE locktype = 'advisory' AND granted
			   AND objid::bigint = hashtext('margince:authz')::bigint & 4294967295`).Scan(&held)
	})
	b.mu.Lock()
	b.puts++
	b.locked += held
	if err != nil {
		b.failure = err
	}
	b.mu.Unlock()
	return b.Store.Put(ctx, key, r, size, contentType)
}

func (b *lockWatchingBlob) Delete(ctx context.Context, key string) error {
	b.mu.Lock()
	b.deletes++
	b.mu.Unlock()
	return b.Store.Delete(ctx, key)
}

func TestAMessagesFilesAreStoredBeforeItsTransactionTakesTheAuthorizationLock(t *testing.T) {
	blob := &lockWatchingBlob{Store: blobstore.NewMemory()}
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	blob.e = env.e
	env.sync(t, emailWithFile("her@globex.example", captureOwner, "prestage-a@globex.example", "", "contract.pdf", signedContract))

	if blob.failure != nil {
		t.Fatal(blob.failure)
	}
	if blob.puts != 1 {
		t.Fatalf("put %d objects for one file, want 1", blob.puts)
	}
	if blob.locked != 0 {
		t.Fatal("the file was uploaded while a session held the authorization lock")
	}
	if keys := fileKeys(t, env.e, "prestage-a@globex.example"); len(keys) != 1 || keys[0] == "" {
		t.Fatalf("the message names stored files %v, want the one uploaded", keys)
	}
}

func TestFilesStoredForAMessageTheCaptureDropsAreDeleted(t *testing.T) {
	blob := &lockWatchingBlob{Store: blobstore.NewMemory()}
	env := newCaptureEnvWithBlob(t, captureNow, blob)
	blob.e = env.e
	owner := humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead, principal.ScopeWrite})
	// An exclusion is a gate the short read does not ask, so the files are
	// stored and the transaction then drops the message.
	if _, err := capture.NewExclusionStore(env.e.DB()).Add(owner, capture.ExclusionScopeUser, capture.ExclusionKindAddress, "her@globex.example", ids.Nil); err != nil {
		t.Fatal(err)
	}
	env.sync(t, emailWithFile("her@globex.example", captureOwner, "prestage-b@globex.example", "", "contract.pdf", signedContract))

	if n := countRows(t, env.e, `SELECT count(*) FROM activity WHERE source_id = 'prestage-b@globex.example'`); n != 0 {
		t.Fatalf("%d activity rows for an excluded message, want 0; the fixture proves nothing", n)
	}
	if blob.puts != 1 || blob.deletes != 1 {
		t.Fatalf("put %d and deleted %d objects for a dropped message, want 1 and 1", blob.puts, blob.deletes)
	}
	if n := countRows(t, env.e, `SELECT count(*) FROM stored_object_intent`); n != 0 {
		t.Fatalf("%d provisional objects remain after the drop, want 0", n)
	}
}
