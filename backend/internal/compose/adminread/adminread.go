// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package adminread holds what every operational report on Settings → System
// health has in common: who may read it, and what a read that fails answers.
//
// Every System health report asks the same two questions. The ADMISSION must
// not be spelled per report: it is a fail-closed ladder, and its ORDER is the
// security property. An unbound actor is refused before RequireHuman, which
// reports one with an unmapped error that renders as a 500. A second copy is how
// one report silently loses a rung.
package adminread

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

// AdmitHealthReader admits a reader of an operational report, or writes the refusal.
// Every report gates on `job_health:read`, so one object states the page's access rules.
func AdmitHealthReader(w http.ResponseWriter, r *http.Request) bool {
	return AdmitOperator(w, r, "job_health", principal.ActionRead)
}

// AdmitOperator is the same ladder for an operator action on another object.
func AdmitOperator(w http.ResponseWriter, r *http.Request, object string, action principal.Action) bool {
	ctx := r.Context()
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return false
	}
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		httperr.Write(w, r, err)
		return false
	}
	if err := auth.Require(ctx, object, action); err != nil {
		httperr.Write(w, r, err)
		return false
	}
	return true
}

// ReadTimeout bounds an operational read. An operator waits longer than a scrape does.
// An unbounded read would hold a request thread and a pool connection.
const ReadTimeout = 10 * time.Second

// ServeReport runs one report inside a workspace transaction and writes it, or the failure.
// The report is built whole before anything is written, so a failed read never sends half of it.
func ServeReport[T any](w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool,
	what string, read func(ctx context.Context, tx pgx.Tx) (T, error),
) {
	ctx, cancel := context.WithTimeout(r.Context(), ReadTimeout)
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
