// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database"
)

// CaptureFactsMemo prefixes every workspace fact a capture transaction
// remembers: the consumer-mail list, the own domains, the members' addresses,
// a seat's identities and a mailbox's sharing setting.
const CaptureFactsMemo = "capture.facts:"

// CaptureFactsChanged is called by every writer of a capture fact, in its own
// transaction, so the rest of that transaction reads the facts again.
// The census in gates/capturefactwriters_test.go finds every such writer.
func CaptureFactsChanged(_ context.Context, tx pgx.Tx) error {
	database.Forget(tx, CaptureFactsMemo)
	return nil
}

// CaptureFactsChangedBy is CaptureFactsChanged for a conditional write: one
// that moved no row changed nothing.
func CaptureFactsChangedBy(ctx context.Context, tx pgx.Tx, written pgconn.CommandTag) error {
	if written.RowsAffected() == 0 {
		return nil
	}
	return CaptureFactsChanged(ctx, tx)
}
