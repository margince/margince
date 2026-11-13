// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// connectionMatch names one capture_connection row in SQL. A seat's mailbox is
// found by (user, provider), a shared mailbox by its id. The settings writers
// take one, so a personal and a shared change run the same statements.
type connectionMatch struct {
	where string
	args  []any
	// seat owns the row; zero for a shared mailbox.
	seat ids.UUID
}

func seatConnection(user ids.UUID, provider string) connectionMatch {
	return connectionMatch{where: "user_id = $1 AND provider = $2", args: []any{user, provider}, seat: user}
}

func sharedConnection(connection ids.UUID) connectionMatch {
	return connectionMatch{where: "id = $1 AND user_id IS NULL", args: []any{connection}}
}

// shared reports whether the row is a shared mailbox.
func (m connectionMatch) shared() bool { return m.seat.IsZero() }

// with answers the match's arguments followed by more, and the placeholder the
// first of them takes.
func (m connectionMatch) with(more ...any) (string, []any) {
	args := append(append(make([]any, 0, len(m.args)+len(more)), m.args...), more...)
	return "$" + strconv.Itoa(len(m.args)+1), args
}

// auditTx writes the audit row of a change to the matched connection. A shared
// mailbox also announces the change, so a consumer re-reads who reads it.
func (m connectionMatch) auditTx(ctx context.Context, tx pgx.Tx, verb, change string, id ids.UUID, before, after map[string]any) error {
	if m.shared() {
		return announceSharedMailboxTx(ctx, tx, verb, change, id, before, after)
	}
	return auditLifecycle(ctx, tx, verb, captureConnectionObject, id, before, after)
}
