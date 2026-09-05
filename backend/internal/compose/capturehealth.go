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
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// captureHealthReadTimeout bounds the read for the reason the job-health one
// is bounded: an operator waiting on a page tolerates more latency than a
// scrape does, but an unbounded read holds a request thread and a pool
// connection for as long as it takes.
const captureHealthReadTimeout = 10 * time.Second

type captureHealthHandlers struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// GetCaptureHealth reports what the judgement queues are holding.
//
// Gate order, fail-closed and in the same order job health uses: no principal,
// then human-only, then admin. An admin-minted read-scoped passport satisfies
// every object grant, so human-only is asserted here rather than inferred from
// RBAC — this check is the layer that does not depend on the wiring being right.
func (h captureHealthHandlers) GetCaptureHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := principal.Actor(ctx); !ok {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The same operational question as job health, for the same reader, so the same
	// object gates it. Leaving this on the literal admin role would make System
	// health grant-derived for one card and role-derived for its neighbour.
	if err := auth.Require(ctx, "job_health", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, captureHealthReadTimeout)
	defer cancel()

	var out crmcontracts.CaptureHealth
	if err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		out, err = readCaptureHealth(ctx, tx, h.now())
		return err
	}); err != nil {
		// Never a partial 200. A page reporting half the queues as though they
		// were all of them is the answer this endpoint exists to replace.
		slog.ErrorContext(ctx, "capture health read failed", "err", err)
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
