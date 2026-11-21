// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The workspace facts capture judges mail by, and the version that says when
// one of them moved.
//
// A transaction remembers each fact it reads. An import run keeps them across
// its transactions. A transaction takes the run's copy only after it read the
// version and saw no writer still open. A writer advances the version and
// holds a shared lock until it ends. The version moves before the writer
// commits, so the lock is how a reader knows the rows may still be old. A
// reader that cannot take the lock for an instant reads the facts itself.

import (
	"context"
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
	captureFactsVersion = "capture.facts-version"
)

// captureFactsLock is the advisory lock a writer of a fact holds shared.
const captureFactsLock = `hashtext('margince:capture-facts')::bigint`

// CaptureFactsVersionSQL selects the version and whether no writer of a fact is
// still open, for a statement a capture transaction already sends.
const CaptureFactsVersionSQL = `(SELECT last_value FROM capture_facts_version_seq),
       CASE WHEN pg_try_advisory_lock(` + captureFactsLock + `)
            THEN pg_advisory_unlock(` + captureFactsLock + `) ELSE false END`

// CaptureFactsChanged is called by every writer of a capture fact, in its own
// transaction. The rest of that transaction reads the facts again, and every
// import run reads them again from its next transaction on.
// gates/capturefactwriters_test.go finds every such writer.
func CaptureFactsChanged(ctx context.Context, tx pgx.Tx) error {
	database.Forget(tx, CaptureFactsMemo)
	if !database.Mark(tx, captureFactsWritten) {
		return nil
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(`+captureFactsLock+`), nextval('capture_facts_version_seq')`)
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

// factsVersion is what one transaction read of the version.
type factsVersion struct {
	value   int64
	settled bool
}

// NoteCaptureFactsVersion records the version a transaction's own statement
// read through CaptureFactsVersionSQL, so the facts ask for it no second time.
func NoteCaptureFactsVersion(tx pgx.Tx, version int64, settled bool) error {
	_, err := database.Remember(tx, captureFactsVersion, func() (factsVersion, error) {
		return factsVersion{value: version, settled: settled}, nil
	})
	return err
}

func captureFactsVersionTx(ctx context.Context, tx pgx.Tx) (factsVersion, error) {
	return database.Remember(tx, captureFactsVersion, func() (factsVersion, error) {
		var read factsVersion
		err := tx.QueryRow(ctx, `SELECT `+CaptureFactsVersionSQL).Scan(&read.value, &read.settled)
		return read, err
	})
}

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
		// A savepoint has no memo to hold the version, so it would read the
		// version once per fact; it reads the facts instead.
		run, ok := ctx.Value(captureFactsRunKey{}).(*captureFactsRun)
		if !ok || !database.Memoized(tx) || wroteCaptureFacts(tx) {
			return read()
		}
		version, err := captureFactsVersionTx(ctx, tx)
		if err != nil {
			var none T
			return none, err
		}
		if !version.settled {
			return read()
		}
		if held, ok := run.get(version.value, key); ok {
			if value, typed := held.(T); typed {
				return value, nil
			}
		}
		value, err := read()
		if err != nil {
			return value, err
		}
		if !wroteCaptureFacts(tx) {
			run.put(version.value, key, value)
		}
		return value, nil
	})
}

func wroteCaptureFacts(tx pgx.Tx) bool {
	return database.Marked(tx, captureFactsWritten)
}
