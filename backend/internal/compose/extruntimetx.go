// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The unit's own transaction: opening one, and the one thing holding one
// forbids.
//
// It sits apart from the Runtime's other capabilities because the two halves
// here are one fact read from opposite ends. Tx hands a unit a transaction on
// the invocation's tenant; the counter beside it is how the INGRESS port knows
// one is open, since capture opens a transaction of its own and a second
// acquire inside the first does not fail on a small pool — it waits for a
// connection this Runtime is holding.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/pkg/extension"
)

// enterTx claims one open transaction, and REFUSES while an ingest is in
// flight.
//
// The refusal is the other half of insideTx's, and without it the pair is a
// check-then-use race rather than a guarantee: an ingest that read txDepth == 0
// and then handed its record to capture would have capture's transaction opened
// AFTER a sibling goroutine took the pool's connection through Tx, which is
// exactly the wait the check exists to prevent. Claimed under one lock, in both
// directions, the two are mutually exclusive per runtime — and a handler that
// genuinely needs both does them one after the other, which is what the poll's
// read-close-ingest-write shape already is.
func (r *callRuntime) enterTx() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ingesting > 0 {
		return extension.ErrNestedIngest
	}
	r.txDepth++
	return nil
}

func (r *callRuntime) leaveTx() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.txDepth--
}

// beginIngest claims the ingest slot, refusing while this Runtime holds a
// transaction. It is insideTx's check and the claim in ONE critical section,
// which is what makes the answer still true when capture opens its own
// transaction a moment later.
//
// A handler goroutine ingesting while an UNRELATED transaction of the same
// runtime is open is refused too. That false positive is accepted rather than
// engineered away: the alternative — marking the context Tx hands its callback
// — is evaded by a handler that ingests with the outer context from inside the
// callback, which still hangs. Refusing a call that would have worked is a
// better failure than hanging a worker.
func (r *callRuntime) beginIngest() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.txDepth > 0 {
		return extension.ErrNestedIngest
	}
	r.ingesting++
	return nil
}

// endIngest releases the slot.
func (r *callRuntime) endIngest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ingesting--
}

// Tx opens ONE transaction on the workspace the invocation belongs to, and
// hands the callback the published seam over it.
//
// It goes through database.WithWorkspaceTx rather than a second mechanism this
// surface invents, so the tier and the core open a transaction the same way and
// there is one place to audit either. The workspace comes from scoped — the
// INVOCATION's tenant, not whatever tenant the handler's own ctx happens to
// carry.
//
// What the transaction does NOT do is bound fn. Nothing in this database keys
// on a tenant: an installation holds one workspace (ADR-0061), no table carries
// workspace_id and no policy reads one, so SQL issued here reaches whatever the
// application role can name. The boundary that holds is the published seam
// itself — the grant surface extmigrategate polices and the `ext` schema a unit
// owns.
func (r *callRuntime) Tx(ctx context.Context, fn func(ctx context.Context, tx extension.Tx) error) error {
	ctx, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	// Derived BEFORE the transaction opens. The unit name was validated at
	// registration, so an invalid one here is a composition that should never
	// have booted — and learning that inside a transaction would only make the
	// report harder to read than it needs to be.
	namespace, err := extension.Name(r.unit).Namespace()
	if err != nil {
		return fmt.Errorf("compose: the invoking unit's name has no SQL namespace: %w", err)
	}
	if err := r.enterTx(); err != nil {
		return err
	}
	defer r.leaveTx()
	return database.WithWorkspaceTx(ctx, r.deps.pool, func(tx pgx.Tx) error {
		// Re-checked inside: opening a transaction is a round trip, and a
		// Runtime released during it must not reach the callback with a live
		// handle. Refusing here rolls the (empty) transaction back.
		if err := r.usable(); err != nil {
			return err
		}
		return fn(ctx, extensionTx{
			tx: tx,
			core: extensionCore{
				tx: tx, unattended: r.unattended, deps: r.deps, authority: r.scoped, unit: r.unit,
			},
			ledger: extensionLedger{tx: tx, namespace: namespace, authority: r.scoped},
		})
	})
}
