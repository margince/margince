// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The workspace facts capture judges mail by, and the version that says when
// one of them moved.
//
// A transaction remembers each fact it reads. An import run keeps them across
// its transactions, and a capture transaction takes the run's copy only while
// it holds the facts lock shared (HoldCaptureFactsTx). A writer takes that
// lock exclusively before it advances the version, so no writer commits
// between a reader's check and its use of the copy.
//
// Lock order: a capture transaction takes the facts lock after its connection
// row and the authorization lock, and a writer after its own row locks.

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CaptureFactsMemo prefixes every capture fact a transaction remembers.
const CaptureFactsMemo = "capture.facts:"

const (
	captureFactsWritten = "capture.facts-written"
	captureFactsDirty   = "capture.facts-dirty"
	captureFactsHeld    = "capture.facts-held"
	captureFactsVersion = "capture.facts-version"
)

// captureFactsLock is the advisory lock readers hold shared and writers hold
// exclusively, each until its transaction ends.
const captureFactsLock = `hashtext('margince:capture-facts')::bigint`

// CaptureFactsChanged is called by every writer of a capture fact, in its own
// transaction. The rest of that transaction reads the facts again, and every
// import run reads them again from its next transaction on. It waits for the
// capture transactions that hold the facts lock.
// gates/capturefactwriters_test.go finds every such writer.
func CaptureFactsChanged(ctx context.Context, tx pgx.Tx) error {
	database.Forget(tx, CaptureFactsMemo)
	owner := database.CommitOwner(tx)
	database.Forget(owner, CaptureFactsMemo)
	database.Mark(owner, captureFactsDirty)
	if !database.Mark(tx, captureFactsWritten) {
		return nil
	}
	_, err := tx.Exec(ctx, `WITH held AS MATERIALIZED (SELECT pg_advisory_xact_lock(`+captureFactsLock+`))
		SELECT nextval('capture_facts_version_seq') FROM held`)
	return err
}

// OwnCompanyDomainsChanged is CaptureFactsChanged for a write of companies'
// domains. Only the own company's domains are a capture fact. A customer's new
// domain, which capture writes all day, changes none.
func OwnCompanyDomainsChanged(ctx context.Context, tx pgx.Tx, companies ...ids.UUID) error {
	var own bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM company WHERE id = ANY($1) AND is_anchor)`, companies).Scan(&own); err != nil {
		return err
	}
	if !own {
		return nil
	}
	return CaptureFactsChanged(ctx, tx)
}

// CaptureFactsChangedBy is CaptureFactsChanged for a conditional write: one
// that moved no row changed nothing.
func CaptureFactsChangedBy(ctx context.Context, tx pgx.Tx, written pgconn.CommandTag) error {
	if written.RowsAffected() == 0 {
		return nil
	}
	return CaptureFactsChanged(ctx, tx)
}

// HoldCaptureFactsTx takes the facts lock shared until tx ends and reads the
// version under it. The run's copy is then current for the rest of tx. A sequence
// nobody has advanced reads as 0, since its first nextval leaves last_value.
func HoldCaptureFactsTx(ctx context.Context, tx pgx.Tx) error {
	var version int64
	if err := tx.QueryRow(ctx, `WITH held AS MATERIALIZED (SELECT pg_advisory_xact_lock_shared(`+captureFactsLock+`))
		SELECT CASE WHEN is_called THEN last_value ELSE 0 END FROM capture_facts_version_seq, held`).Scan(&version); err != nil {
		return err
	}
	if _, err := database.Remember(tx, captureFactsVersion, func() (int64, error) { return version, nil }); err != nil {
		return err
	}
	database.Mark(tx, captureFactsHeld)
	return nil
}

// heldCaptureFactsVersion is the version tx read under the facts lock.
func heldCaptureFactsVersion(tx pgx.Tx) (int64, bool) {
	if !database.Marked(tx, captureFactsHeld) {
		return 0, false
	}
	version, err := database.Remember(tx, captureFactsVersion, func() (int64, error) { return 0, errFactsLockNotHeld })
	return version, err == nil
}

var errFactsLockNotHeld = errors.New("storekit: the capture facts lock is not held")

// captureFactsRun is what one import run read, at one version.
type captureFactsRun struct {
	mu      sync.Mutex
	version int64
	values  map[string]any
}

type captureFactsRunKey struct{}

// WithCaptureFactsRun gives ctx a run copy of the capture facts, shared by
// every transaction that carries it.
func WithCaptureFactsRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, captureFactsRunKey{}, &captureFactsRun{values: map[string]any{}})
}

//craft:ignore naked-any one run holds facts of several types; RememberCaptureFact asserts each back to its own
func (r *captureFactsRun) get(version int64, key string) (any, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.version != version {
		return nil, false
	}
	value, ok := r.values[key]
	return value, ok
}

//craft:ignore naked-any one run holds facts of several types; RememberCaptureFact asserts each back to its own
func (r *captureFactsRun) put(version int64, key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case version < r.version:
		return
	case version > r.version:
		r.version, r.values = version, map[string]any{}
	}
	r.values[key] = value
}

// RememberCaptureFact answers one capture fact: from this transaction's memo,
// else from the run's copy while the version holds, else from read.
//
// A transaction that wrote a fact reads fresh and keeps nothing for the run,
// because what it read may never commit.
func RememberCaptureFact[T any](ctx context.Context, tx pgx.Tx, key string, read func() (T, error)) (T, error) {
	return database.Remember(tx, key, func() (T, error) {
		// Outside a transaction that holds the facts lock, a writer could
		// commit while the copy is in use, so the facts are read.
		run, ok := ctx.Value(captureFactsRunKey{}).(*captureFactsRun)
		version, held := heldCaptureFactsVersion(tx)
		if !held {
			// A savepoint inherits a lock the root took before it opened. Its
			// reads remain separate; the run's versioned facts are still protected.
			version, held = heldCaptureFactsVersion(database.CommitOwner(tx))
		}
		if !ok || !held || wroteCaptureFacts(tx) {
			return read()
		}
		if cached, ok := run.get(version, key); ok {
			if value, typed := cached.(T); typed {
				return value, nil
			}
		}
		value, err := read()
		if err != nil {
			return value, err
		}
		if !wroteCaptureFacts(tx) {
			run.put(version, key, value)
		}
		return value, nil
	})
}

func wroteCaptureFacts(tx pgx.Tx) bool {
	return database.Marked(tx, captureFactsWritten) || database.Marked(database.CommitOwner(tx), captureFactsDirty)
}
