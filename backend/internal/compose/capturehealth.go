// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether capture's judgement queues are keeping up.
//
// Two sweeps repair backlogs that were previously invisible and permanent —
// settled threads whose messages never took the verdict, and captured contacts
// nobody was ever asked about. They log their counts and nothing else, so
// nobody could answer "is anything stuck".
//
// The contacts are the reason this is worth serving at all: they are
// owner-private, and `ownerPrivateTables` makes them invisible to every reader
// but their owner — not even an administrator. So the backlog cannot be seen by
// looking, and a count is the only thing that can report it.
//
// COUNTS AND AGES ONLY. Never a subject, a body, or the reason a thread was
// held: those describe the correspondence, and an operational page is not an
// exemption from the boundary the rest of capture is built around. A mailbox is
// named because an administrator cannot act on "somewhere in the installation";
// what is waiting inside it is not named at all.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

type captureHealthHandlers struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// GetCaptureHealth reports what the judgement queues are holding.
func (h captureHealthHandlers) GetCaptureHealth(w http.ResponseWriter, r *http.Request) {
	if !admitHealthReader(w, r) {
		return
	}
	now := h.now()
	serveHealthReport(w, r, h.pool, "capture health",
		func(ctx context.Context, tx pgx.Tx) (crmcontracts.CaptureHealth, error) {
			return readCaptureHealth(ctx, tx, now)
		})
}
