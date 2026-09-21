// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The reap itself: what the pass does to the bytes and to the ledger.
//
// The ledger's reads are proved where they live (activities). This is the other
// half of the issue's acceptance — "a transaction that fails after a put leaves
// no object behind ONCE THE CLEANUP JOB HAS RUN" — which is a claim about the
// pass, and about the order it does its two deletes in.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
)

// orphanedObject plants an object and the ledger row that says it is
// provisional, dated past the grace period.
func orphanedObject(t *testing.T, e *integration.Env, blob blobstore.Store, key string) {
	t.Helper()
	ctx := context.Background()
	if err := blob.Put(ctx, key, strings.NewReader("bytes nobody claimed"), 20, "application/octet-stream"); err != nil {
		t.Fatalf("storing the orphan's bytes: %v", err)
	}
	if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO stored_object_intent (storage_key, recorded_at)
			VALUES ($1, now() - $2::interval)`, key, (2 * activities.ProvisionalObjectGrace).String())
		return err
	}); err != nil {
		t.Fatalf("planting the provisional key: %v", err)
	}
}

// provisionalKeys is what the ledger still holds.
func provisionalKeys(t *testing.T, e *integration.Env) []string {
	t.Helper()
	var keys []string
	if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT storage_key FROM stored_object_intent ORDER BY storage_key`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			keys = append(keys, key)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	return keys
}

func TestTheReapDestroysAnOrphansBytesAndRetiresItsKey(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	const key = "reap/attachment/orphan"
	orphanedObject(t, e, blob, key)

	worker := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: time.Now}
	if err := worker.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v", err)
	}

	if !blob.deleted[key] {
		t.Error("the orphan's bytes are still in the store — an object with no row is one an erasure cannot reach")
	}
	if got := provisionalKeys(t, e); len(got) != 0 {
		t.Errorf("the ledger still holds %v, so every later pass would delete bytes that are already gone", got)
	}
}

func TestAnObjectTheStoreWillNotDeleteStaysInTheLedger(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	blob.refuseDelete = errors.New("the bucket said no")
	const key = "reap/attachment/stubborn"
	orphanedObject(t, e, blob, key)

	worker := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: time.Now}
	if err := worker.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v — one key that will not delete must not stop the pass", err)
	}

	// The BYTES first, then the ledger. The other order forgets an object that
	// is still there, which is the state the whole ledger exists to make
	// impossible — so a refused delete has to leave the key for the next pass.
	if got := provisionalKeys(t, e); len(got) != 1 || got[0] != key {
		t.Errorf("the ledger holds %v, want the key whose bytes are still in the store", got)
	}
}

// reapBlobstore records what the pass deleted, and can refuse.
type reapBlobstore struct {
	blobstore.Store
	refuseDelete error
	deleted      map[string]bool
}

func newReapBlobstore() *reapBlobstore {
	return &reapBlobstore{Store: blobstore.NewMemory(), deleted: map[string]bool{}}
}

func (b *reapBlobstore) Delete(ctx context.Context, key string) error {
	if b.refuseDelete != nil {
		return b.refuseDelete
	}
	if err := b.Store.Delete(ctx, key); err != nil {
		return err
	}
	b.deleted[key] = true
	return nil
}

var _ io.Reader = strings.NewReader("")
