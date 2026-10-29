// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The export entry a core list leaves behind. It is queued on the port and
// written by Runtime.Tx once the unit's own transaction has ended.

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// The keys of an export entry's detail.
const (
	detailKind  = "kind"
	detailTable = "table"
	detailRows  = "row_count"
)

// readAudit holds the export entries of one unit transaction until it ends.
//
// An entry written on the unit's own transaction would roll back with it. A
// unit could then read personal data and return an error to erase the trace.
// Tx writes the queue in a transaction of its own after the unit's has ended,
// committed or not. That takes no second connection while the first is held.
type readAudit struct {
	mu      sync.Mutex
	pending []map[string]any
}

var errNoReadAudit = errors.New("compose: this core port was built without an audit queue, so a list cannot be recorded")

func (a *readAudit) add(detail map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = append(a.pending, detail)
}

// flush writes every queued entry in one transaction.
func (a *readAudit) flush(ctx context.Context, pool *pgxpool.Pool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.pending) == 0 {
		return nil
	}
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return a.write(ctx, tx)
	})
}

// flushOn writes the queued entries on a transaction the caller holds open.
func (a *readAudit) flushOn(ctx context.Context, tx pgx.Tx) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.write(ctx, tx)
}

func (a *readAudit) write(ctx context.Context, tx pgx.Tx) error {
	for _, detail := range a.pending {
		if _, err := storekit.LogSystem(ctx, tx, "export", detail); err != nil {
			return err
		}
	}
	return nil
}

// auditBulkRead queues a list as the export it is: who read which slice, and
// which unit carried the call. Filter values stay out because a search term can
// be personal data. The names of the filters used say what slice it was.
func (c extensionCore) auditBulkRead(table string, rows int, filters []string) error {
	if c.audit == nil {
		return errNoReadAudit
	}
	slices.Sort(filters)
	c.audit.add(map[string]any{
		detailKind:  "extension_read",
		"extension": c.unit,
		detailTable: table,
		detailRows:  rows,
		"filters":   filters,
	})
	return nil
}
