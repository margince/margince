// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The published transaction seam: the three SQL verbs a unit's own tables
// need (run by unitsql), and the door onto the governed core port. These are
// the types a unit holds; extruntime.go owns the Runtime's lifetime and scope.

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/unitsql"
	"github.com/margince/margince/backend/pkg/extension"
)

// extensionTx bridges the published three-verb seam to pgx. It lives in
// internal/ and not in pkg/ because pkg/ is stdlib-only (depguard
// pkg-purity, and TestPublishedSurfaceIsPure): a published interface can
// describe a transaction, but only the core can hold one.
//
// There is no lifetime guard on this type. A Tx used after its transaction
// ended answers pgx's own ErrTxClosed, which is already an honest refusal —
// and it is the accurate one, because the transaction can end while the
// Runtime is still perfectly live (the callback returned, the call did not).
// ErrRuntimeExpired there would name the wrong fault.
type extensionTx struct {
	// sql runs the unit's own statements as the role that holds only its
	// tables. Core and Record write outside that role.
	sql unitsql.Tx
	// core is what the transaction can reach BESIDE the unit's own SQL, and it
	// is built by the Runtime rather than by this type: whether the invocation
	// has a caller, and what the role bound at boot, are facts about the call,
	// not about the transaction.
	core extensionCore
	// ledger records what the unit's OWN SQL did — the audit row and the bus
	// event the three verbs below cannot write for it. Built by the Runtime for
	// the same reason core is: which unit is writing, and under whose identity,
	// are facts about the invocation rather than about the transaction.
	ledger extensionLedger
}

//nolint:ireturn // returning the published port IS the seam: a unit holds extension.Core, never a core type.
func (t extensionTx) Core() extension.Core {
	return t.core
}

func (t extensionTx) Record(ctx context.Context, ch extension.Change, ev extension.Event) error {
	return t.ledger.Record(ctx, ch, ev)
}

func (t extensionTx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return t.sql.Exec(ctx, sql, args...)
}

//nolint:ireturn // Rows is the published cursor; a unit must never hold pgx.Rows.
func (t extensionTx) Query(ctx context.Context, sql string, args ...any) (extension.Rows, error) {
	return t.sql.Query(ctx, sql, args...)
}

//nolint:ireturn // Row is the published deferred read; the error is deferred to Scan by design.
func (t extensionTx) QueryRow(ctx context.Context, sql string, args ...any) extension.Row {
	return t.sql.QueryRow(ctx, sql, args...)
}
