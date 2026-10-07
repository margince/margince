// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The rate-refresh transport: the admin-only FX propose-refresh endpoint
// enqueues an async River job through the api role's insert-only runner (the api
// never fetches in-request — the worker does) and returns 202 immediately. The
// unique window (ByArgs + activeSweepStates) makes a double-click a no-op rather
// than a second fetch. Without WithRateRefresh wired, the op stays 501.

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rateRefreshQueue isolates the FX refresh (fetch + LLM extract) from the
// default queue on their own bounded pool: each job is long, so a
// multi-workspace burst on the shared queue would starve the short maintenance
// jobs. Mirrors the deep-read precedent.
const (
	rateRefreshQueue      = "rate_refresh"
	rateRefreshMaxWorkers = 2
)

// rateRefreshEnqueuer is the enqueue seam (jobs.Runner.Enqueue); tests fake it.
type rateRefreshEnqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

type rateRefreshHandlers struct {
	enqueue rateRefreshEnqueuer
}

// ProposeAiModelRateRefresh is retired: model prices come from
// POST /ai-model-rates/refresh. The route stays for one release so an old
// client is told so rather than getting a missing-route error.
func (h rateRefreshHandlers) ProposeAiModelRateRefresh(w http.ResponseWriter, r *http.Request) {
	httperr.NotImplemented(w, r, "propose-refresh for model prices (use POST /ai-model-rates/refresh)")
}

func (h rateRefreshHandlers) ProposeFxRateRefresh(w http.ResponseWriter, r *http.Request) {
	if h.enqueue == nil {
		httperr.NotImplemented(w, r, "rate refresh")
		return
	}
	ctx := r.Context()
	// The same admission the staged effect's write (SetFxRate) opens with: a
	// refresh proposes both new rows and corrections to today's, so either
	// write grant admits proposing one. Which grant the apply actually needs is
	// settled inside that write, against the sheet.
	if err := auth.RequireAny(ctx, "fx_rate", principal.ActionCreate, principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	args := FxRateRefreshArgs{Workspace: storekit.MustWorkspace(ctx), RequestedBy: requestedBy(ctx)}
	// ByArgs uniqueness now hashes only the river:"unique"-tagged WorkspaceID
	// (RequestedBy is provenance, untagged), so two admins refreshing the same
	// workspace collapse to one in-flight refresh rather than racing two.
	opts := jobs.QueuedAs[FxRateRefreshArgs](&river.InsertOpts{
		// One-off: an admin pressed refresh and nothing re-presses it. The
		// sheet is diffed at the end, so a run that gives up stages nothing and
		// the next click starts over.
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	})
	if err := h.enqueue.Enqueue(ctx, args, opts); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, crmcontracts.RefreshAccepted{Status: crmcontracts.RefreshAcceptedStatusEnqueued})
}

// WithRateRefresh wires the api role's insert-only runner into the FX
// propose-refresh handler. Without it, the op stays its explicit 501.
func WithRateRefresh(inserter rateRefreshEnqueuer) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.rateRefreshHandlers = rateRefreshHandlers{enqueue: inserter}
	}
}
