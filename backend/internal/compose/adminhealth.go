// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What every operational report on Settings → System health has in common:
// who may read it, and what a read that fails answers.
//
// Three surfaces ask the same two questions — background jobs, capture's
// judgement queues, and what the core refused from a connector. The ADMISSION
// is the part that must not be spelled three times: it is a fail-closed ladder
// whose ORDER is the security property (an unbound actor before RequireHuman,
// because RequireHuman reports one with an unmapped error that renders as a
// 500 — a security surface should not answer "nobody asked" with a server
// fault), and a third copy is how one of them silently loses a rung.

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// admitHealthReader answers whether this caller may read an operational
// report, writing the refusal itself when they may not.
//
// All three reports gate on `job_health:read` and on a human session. The
// object is the same one deliberately: they answer one question for one reader,
// and a System health page with one card grant-derived and its neighbour
// role-derived is a page whose access rules nobody can state. Human-only is
// asserted here rather than inferred from RBAC, because an admin-minted
// read-scoped passport satisfies every object grant — this is the rung that
// does not depend on the wiring being right.
func admitHealthReader(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	if _, ok := principal.Actor(ctx); !ok {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return false
	}
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return false
	}
	if err := auth.Require(ctx, "job_health", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return false
	}
	return true
}

// healthReadTimeout bounds an operational read. An operator waiting on a page
// tolerates more latency than a scrape does, but an unbounded read holds a
// request thread and a pool connection for as long as it takes.
const healthReadTimeout = 10 * time.Second

// serveHealthReport runs one report inside a workspace transaction and writes
// it, or writes the failure.
//
// NEVER A PARTIAL 200, which is why the report is built whole before anything
// is written: a page reporting half of what it looked at as though it were all
// of it is the silence these endpoints exist to replace.
func serveHealthReport[T any](w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool,
	what string, read func(ctx context.Context, tx pgx.Tx) (T, error),
) {
	ctx, cancel := context.WithTimeout(r.Context(), healthReadTimeout)
	defer cancel()

	var out T
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		out, err = read(ctx, tx)
		return err
	}); err != nil {
		slog.ErrorContext(ctx, what+" read failed", "err", err)
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
