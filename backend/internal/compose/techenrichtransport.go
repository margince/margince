// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The technical lookup's api-role half: start queues the per-company job
// through the insert-only runner (the api never looks anything up in-request —
// the worker role does, jobs_techenrich.go), and status answers the SPA's poll
// with what each lane last did.

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// technicalEnqueuer is the slice of *jobs.Runner the start handler needs;
// tests fake it to count inserts.
type technicalEnqueuer interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// technicalHandlers shadows the generated TechnicalEnrichCompany and
// GetLatestTechnicalEnrich stubs. Nil until WithTechnicalEnrich wires them.
type technicalHandlers struct {
	pool    *pgxpool.Pool
	people  *people.Store
	enqueue technicalEnqueuer
}

// WithTechnicalEnrich wires the lookup's HTTP surface.
//
// A role with no job runner wires nothing, and the handlers answer 501 —
// declared absent rather than a button that appears to work and queues into a
// process that will never pick the job up.
func WithTechnicalEnrich(enqueue technicalEnqueuer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		if enqueue == nil {
			return
		}
		s.technicalHandlers = technicalHandlers{
			pool: pool, people: people.NewStore(InstallationDB(pool)), enqueue: enqueue,
		}
	}
}

// TechnicalEnrichCompany queues a lookup of what this company publicly runs.
func (h technicalHandlers) TechnicalEnrichCompany(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if h.enqueue == nil {
		httperr.NotImplemented(w, r, "technicalEnrichCompany (no job runner configured)")
		return
	}
	started, err := h.startTechnicalEnrich(r.Context(), ids.UUID(id))
	if err != nil {
		// EnsureVisible's existence-hiding 404, the no-domain 422 and the rest
		// all ride the sentinel/DetailedError mapping.
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, started)
}

// startTechnicalEnrich is the lookup's start with no transport in it, so the
// HTTP route and the `enrich` tool mean the same thing on both wires.
//
// It reads the domain HERE, before queueing, purely so a company with none is
// told so rather than queueing a job that will quietly do nothing. The worker
// re-reads it when it runs, which is the read that counts.
func (h technicalHandlers) startTechnicalEnrich(
	ctx context.Context, id ids.UUID,
) (crmcontracts.TechnicalEnrichStarted, error) {
	orgID := ids.From[ids.OrganizationKind](id)
	_, ok, err := h.people.TechnicalDomain(ctx, orgID)
	if err != nil {
		return crmcontracts.TechnicalEnrichStarted{}, err
	}
	if !ok {
		return crmcontracts.TechnicalEnrichStarted{}, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity,
			Code:   companyUnreadable,
			Detail: "This company has no domain on file. Add one to look up what they run.",
		}
	}
	// The tenant comes from the bound transaction rather than being read
	// separately: WithWorkspaceTx refuses outside a workspace, so a job row
	// without one — which River would never dequeue — cannot be written here.
	ws, bound := principal.WorkspaceID(ctx)
	if !bound {
		return crmcontracts.TechnicalEnrichStarted{}, database.ErrNoWorkspace
	}
	err = database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		return h.enqueue.EnqueueTx(ctx, tx, TechnicalEnrichOrganizationArgs{
			Workspace:      ws,
			OrganizationID: id,
		}, technicalInsertOpts())
	})
	if err != nil {
		return crmcontracts.TechnicalEnrichStarted{}, err
	}
	return crmcontracts.TechnicalEnrichStarted{
		OrganizationId: openapi_types.UUID(id),
		// Queued is what this call did. River's uniqueness makes a second press
		// join the first rather than queue again, so the honest word for both
		// is the same: the lookup this rep asked for is on its way.
		Status: crmcontracts.TechnicalEnrichStartedStatusQueued,
	}, nil
}

// GetLatestTechnicalEnrich answers what each lane last did for this account.
func (h technicalHandlers) GetLatestTechnicalEnrich(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	if h.pool == nil {
		httperr.NotImplemented(w, r, "getLatestTechnicalEnrich (no job runner configured)")
		return
	}
	lanes, err := h.people.TechnicalLaneState(r.Context(), ids.From[ids.OrganizationKind](ids.UUID(id)))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if len(lanes) == 0 {
		// Never looked up. The honest difference between "never tried" and
		// "tried and got nothing", which the lane outcomes themselves express.
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.TechnicalEnrichStatus{
		OrganizationId: openapi_types.UUID(id),
		Lanes:          technicalLanesWire(lanes),
	})
}

// technicalLanesWire carries the ledger onto the wire.
func technicalLanesWire(lanes []people.TechnicalLaneState) []crmcontracts.TechnicalEnrichLane {
	wire := make([]crmcontracts.TechnicalEnrichLane, 0, len(lanes))
	for _, lane := range lanes {
		row := crmcontracts.TechnicalEnrichLane{
			Lane:          crmcontracts.TechnicalEnrichLaneLane(lane.Lane),
			Attempts:      lane.Attempts,
			LastSuccessAt: lane.LastSuccessAt,
			NextAttemptAt: lane.NextAttemptAt,
		}
		if lane.Outcome != "" {
			outcome := crmcontracts.TechnicalEnrichLaneOutcome(lane.Outcome)
			row.Outcome = &outcome
		}
		wire = append(wire, row)
	}
	return wire
}
