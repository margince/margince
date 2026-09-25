// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The door a human opens onto the input check.
//
// Until this, the check ran only on the calendar: the scanner had one caller
// and it was the nightly sweep. Two things were waiting on a door. A fresh
// workspace was checked before anybody chose to be — the first pass over a
// backlog raises every exception at once, which is the queue nobody reads. And
// a manager who had just fixed a record could not make the panel agree with it
// before the call; the finding stood until the next night.
//
// Both are the same act, which is why they are one route: the first press
// starts the cycle, every press after it is a recheck. Enrolment is the run
// history itself (assurance.Store.EverRun), so this route enrols by doing the
// work and the preview beside it cannot enrol anybody by being read.
//
// In compose rather than in the module because the pass is assembled from the
// two seams that already exist for it — AssuranceSubjects and AssuranceCoverage
// read deals and connector health, which assurance owns nothing of.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// assuranceRunEnqueuer is the one thing this transport needs from the job
// runner, narrowed to the unique insert: the answer it gives — inserted, or
// collapsed into one already in flight — is what tells 202 from 409.
type assuranceRunEnqueuer interface {
	EnqueueTxUnique(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (bool, error)
}

type assuranceRunHandlers struct {
	pool    *pgxpool.Pool
	enqueue assuranceRunEnqueuer
	now     func() time.Time
}

// PreviewForecastAssurance answers what a pass would find, recording none of it.
func (h assuranceRunHandlers) PreviewForecastAssurance(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		httperr.NotImplemented(w, r, "forecast assurance preview")
		return
	}
	ctx := r.Context()
	store := assurance.NewStore(InstallationDB(h.pool))
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig())
	// The gate is inside InPreviewTx, on create rather than read: the only
	// reason to look is to decide whether to start a pass.
	preview, err := scanner.Preview(ctx, h.now())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, previewToWire(preview))
}

// StartForecastAssuranceRun runs the check now, for whoever asked.
func (h assuranceRunHandlers) StartForecastAssuranceRun(w http.ResponseWriter, r *http.Request) {
	if h.enqueue == nil {
		httperr.NotImplemented(w, r, "forecast assurance run")
		return
	}
	ctx := r.Context()
	// The same grant StartRun opens with. A seat that may not record a run may
	// not ask for one either, and reading that off one object keeps the button
	// and the pass from ever disagreeing about who may.
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	args := AssuranceRunArgs{
		Workspace:   storekit.MustWorkspace(ctx),
		RequestedBy: requestedBy(ctx),
	}
	var started bool
	err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		started, err = h.enqueue.EnqueueTxUnique(ctx, tx, args, assuranceRunInsertOpts())
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if !started {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   "assurance_run_running",
			Detail: "a check is already running for this workspace; its findings will appear on the panel when it lands",
		})
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, crmcontracts.AssuranceRunAccepted{
		Status: crmcontracts.AssuranceRunAcceptedStatusAssuranceRunEnqueued,
	})
}

// assuranceRunInsertOpts makes the in-flight job the lock.
//
// Unique on the workspace alone (AssuranceRunArgs tags it; RequestedBy is
// provenance and stays out of the hash), so two managers pressing at once get
// one pass and the second is told so — rather than two walks of one pipeline
// racing to upsert the same findings.
//
// One-off attempts: somebody pressed a button and nothing re-presses it. A pass
// that gives up records the run it opened and the next press starts over.
func assuranceRunInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}
}

func previewToWire(in assurance.Preview) crmcontracts.ForecastAssurancePreview {
	findings := make([]crmcontracts.AssuranceFindingCount, 0, len(in.Counts))
	for _, c := range in.Counts {
		findings = append(findings, crmcontracts.AssuranceFindingCount{
			Type:     c.Type,
			Severity: crmcontracts.AssuranceFindingCountSeverity(c.Severity),
			Count:    c.Count,
		})
	}
	out := crmcontracts.ForecastAssurancePreview{
		Started:       in.Started,
		EligibleDeals: in.EligibleDeals,
		Findings:      findings,
		Sources:       assurance.CoverageToWire(in.Coverage),
	}
	if in.Readiness != "" {
		readiness := crmcontracts.ForecastAssurancePreviewReadiness(in.Readiness)
		out.Readiness = &readiness
	}
	return out
}

// WithAssuranceRun wires the api role's insert-only runner into the two routes.
// Without it both stay their explicit 501.
func WithAssuranceRun(inserter assuranceRunEnqueuer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.assuranceRunHandlers = assuranceRunHandlers{
			pool: pool, enqueue: inserter, now: func() time.Time { return time.Now().UTC() },
		}
	}
}
