// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package storedobjects is the intent ledger: a stored object says it is
// provisional until the row that owns it lands.
//
// Storing a file is two writes to two systems that cannot be made atomic. The put
// goes to the object store and the row to Postgres, so a failure between them leaves
// bytes nothing references — and an erasure walks rows, so it never reaches them.
//
// The key is recorded BEFORE the put and cleared in the SAME transaction that writes
// the owning row, so a key is provisional exactly while the pair is incomplete. A key
// still recorded after the grace period is an object whose row never arrived.
//
// In platform rather than in one module because five modules put bytes, and a module
// never imports a sibling to borrow a ledger.
package storedobjects

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Kind names what put the bytes, DECLARED by the writer rather than read off the key.
//
// One closed vocabulary, because the sweep has to ask the right table whether a key
// is still referenced and the key cannot tell it which: blobstore.WorkspaceKey lays a
// key out as <workspace>/<kind>/<id>, but the offer path writes
// offers/<workspace>/<id>/<revision>/<uuid>.pdf, where the kind is the FIRST segment
// and the workspace the second. A sweep parsing that layout reads one writer's
// workspace as another writer's kind.
type Kind string

// Every kind a writer may declare. A new one is added here and in the sweep's
// reference map together; nothing else may name one.
const (
	KindAttachment Kind = "attachment"
	KindKnowledge  Kind = "knowledge"
	KindLogo       Kind = "logo"
	KindOffer      Kind = "offer"
	KindImport     Kind = "import"
)

// AllKinds is the vocabulary itself, so a reader of the ledger can be held to
// covering all of it rather than to a list somebody keeps in step by hand.
func AllKinds() []Kind {
	return []Kind{KindAttachment, KindKnowledge, KindLogo, KindOffer, KindImport}
}

// declared reports whether a writer may record this kind.
//
// Its own predicate so the vocabulary can be held to the guard without a database
// behind it: asking Record the same question commits the answer.
func declared(kind Kind) bool {
	return slices.Contains(AllKinds(), kind)
}

// ProvisionalObjectGrace is how long a key may stay provisional before the sweep may
// offer it to a reaper.
//
// Generous against the window it has to clear: the transaction between the put and
// the owning row is milliseconds, and a long retry or a slow import is minutes. What
// it buys by being long is that a key past it is an orphan rather than a straggler.
const ProvisionalObjectGrace = 24 * time.Hour

// Ledger records bytes before the row that owns them.
type Ledger struct {
	db *database.DB
}

// NewLedger binds the ledger to the pool its writers commit against.
func NewLedger(db *database.DB) *Ledger {
	return &Ledger{db: db}
}

// Record declares a key provisional, naming the writer that is about to put it.
//
// It commits on its OWN transaction, and that is the point rather than an oversight:
// the row has to survive the failure of the transaction that was going to write the
// owning row. Recorded inside it, it would roll back with the row it exists to catch
// the absence of.
func (l *Ledger) Record(ctx context.Context, kind Kind, key string) error {
	// Refused here rather than discovered by the sweep. An unknown kind reaches the
	// sweep as a key nobody owns, which stops the whole pass for that workspace —
	// so a typo in one writer would strand every other writer's orphans.
	if !declared(kind) {
		return fmt.Errorf("record a provisional object: %q is not a declared kind", kind)
	}
	// Bound to the workspace from the CONTEXT rather than from whatever this ledger
	// was constructed against, and to the same one the key's own prefix carries. The
	// capture path reaches this from an installation-wide handle, where a
	// ledger-bound resolve has no single workspace to find.
	ws := ids.From[ids.WorkspaceKind](storekit.MustWorkspace(ctx))
	return l.db.ForWorkspace(ws).Tx(ctx, func(tx pgx.Tx) error {
		// ON CONFLICT because a retried upload of the same id is not an error worth
		// failing a caller's file over. Both columns are refreshed, and recorded_at
		// is the one that matters: a retry of a key whose earlier intent has already
		// aged past the grace period would otherwise inherit that timestamp and be
		// eligible for the reaper the moment it is recorded — so a sweep running
		// between this retry's put and its row could delete the bytes just written.
		// The clock restarts because the WINDOW restarts.
		_, err := tx.Exec(ctx, `
			INSERT INTO stored_object_intent (storage_key, kind) VALUES ($1, $2)
			ON CONFLICT (storage_key) DO UPDATE
			   SET kind = excluded.kind, recorded_at = now()`, key, string(kind))
		if err != nil {
			return fmt.Errorf("record a provisional object: %w", err)
		}
		return nil
	})
}

// Clear retires a key, on the CALLER's transaction.
//
// ON THE CALLER'S, so the clear and the owning row commit together: a clear that
// committed separately could land while the row's transaction then failed, which is
// the orphan this ledger exists to catch, re-created one step along.
func Clear(ctx context.Context, tx pgx.Tx, key string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM stored_object_intent WHERE storage_key = $1`, key); err != nil {
		return fmt.Errorf("retire a provisional object: %w", err)
	}
	return nil
}

// Provisional is one key the ledger still holds.
type Provisional struct {
	Key        string
	Kind       Kind
	RecordedAt time.Time
}

// ListProvisional answers the keys recorded before `before`, oldest first.
//
// It does NOT decide what may be deleted. Whether a key is still referenced is a
// question about the owning module's own table, and this package sits below every
// one of them — so the sweep asks each owner and this answers only what was declared.
//
// `limit` bounds one pass. A sweep that could run unbounded over a large backlog is
// one that holds a worker for as long as the backlog is deep.
func (l *Ledger) ListProvisional(ctx context.Context, before time.Time, limit int) ([]Provisional, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	var out []Provisional
	err := database.WithWorkspaceTx(ctx, l.db.Pool(), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT storage_key, kind, recorded_at
			  FROM stored_object_intent
			 WHERE recorded_at < $1
			 ORDER BY recorded_at
			 LIMIT $2`, before, limit)
		if err != nil {
			return fmt.Errorf("list provisional objects: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var p Provisional
			if err := rows.Scan(&p.Key, &p.Kind, &p.RecordedAt); err != nil {
				return fmt.Errorf("read a provisional object: %w", err)
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Retire removes one key once its bytes are gone.
//
// Called AFTER the object-store delete, so a failed delete leaves the key here and
// the next pass tries again. The other order would forget an object that is still
// there, which is the state this ledger exists to make impossible.
func (l *Ledger) Retire(ctx context.Context, key string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, l.db.Pool(), func(tx pgx.Tx) error {
		return Clear(ctx, tx, key)
	})
}
