// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The account scan as a job: queued by the api role when an account page is
// opened, read on the worker role under the reader's own authority.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/org360"
	"github.com/margince/margince/backend/internal/compose/orgscan"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AccountScanArgs is one queued account scan: the tenant, the account, the
// row the worker claims, and the reader whose authority the read runs under.
// The row is the authority on all four; the args are what lets the worker
// find it.
type AccountScanArgs struct {
	Workspace      ids.UUID `json:"workspace_id"`
	OrganizationID ids.UUID `json:"organization_id"`
	ScanID         ids.UUID `json:"scan_id"`
	ViewerID       ids.UUID `json:"viewer_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (AccountScanArgs) Kind() string { return "account_scan" }

// WorkspaceID binds this scan to its tenant (jobs.WorkspaceScoped).
func (a AccountScanArgs) WorkspaceID() ids.UUID { return a.Workspace }

// accountScanQueue is the pool a reader is watching the page for: the
// transcript reading's, for the reason api/jobs.yaml gives.
const accountScanQueue = "transcript_read"

// accountScanInsertOpts routes the job and deduplicates it by args: the scan
// row's id is unique per read, so a re-submitted enqueue of the SAME read
// collapses while a fresh read always queues.
func accountScanInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue:       accountScanQueue,
		MaxAttempts: sweptJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// WithAccountScan enables the scan on the api role: ensure queues the read
// through the insert-only runner, and the worker role reads. brain is the
// api's own lane, which serves nothing but the in-request floor; nil
// inserter means a role with no job runner, which settles that floor for
// every ensure rather than queueing reads nothing works.
func WithAccountScan(inserter *jobs.Runner, brain completer, routingVersion func() string) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		var enqueue orgscan.Enqueue
		if inserter != nil {
			enqueue = func(ctx context.Context, tx pgx.Tx, scan orgscan.Queued) error {
				return inserter.EnqueueTx(ctx, tx, AccountScanArgs{
					Workspace: storekit.MustWorkspace(ctx), OrganizationID: scan.OrgID.UUID,
					ScanID: scan.ScanID, ViewerID: scan.ViewerID.UUID,
				}, accountScanInsertOpts())
			}
		}
		s.orgScanSvc = orgscan.NewService(pool, s.org360Svc, s.org360Svc, brain, enqueue, routingVersion, time.Now, s.log)
		s.orgScanHandlers = orgscan.NewHandlers(s.orgScanSvc, s.sorDispatch.isOverlay)
		s.org360Svc.RecogniseScanFindings(s.orgScanSvc)
	}
}

// accountScanWorker reads one queued scan.
type accountScanWorker struct {
	svc   *orgscan.Service
	users *identity.Service
	log   *slog.Logger
}

// newAccountScanWorker builds the worker role's scan service over its own
// composite read: the worker never queues, so it carries no enqueuer.
func newAccountScanWorker(pool *pgxpool.Pool, brain completer, routingVersion func() string, log *slog.Logger) *accountScanWorker {
	view := org360.NewService(pool, people.NewStore(InstallationDB(pool)),
		deals.NewStore(InstallationDB(pool), DealsInstallation()), ProjectsStore(pool),
		approvals.NewService(InstallationDB(pool)), time.Now)
	return &accountScanWorker{
		svc:   orgscan.NewService(pool, view, view, brain, nil, routingVersion, time.Now, log),
		users: identity.NewService(pool),
		log:   log,
	}
}

// Work reads the scan under the reader's own authority. A budget deferral
// snoozes the job until the window the router named; any other outcome is
// on the row, which the page and the rail read.
func (w *accountScanWorker) Work(ctx context.Context, job *river.Job[AccountScanArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	runCtx, err := orgscan.WorkerContext(wsCtx, w.users, ids.From[ids.UserKind](job.Args.ViewerID), job.Args.ScanID)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	err = w.svc.Run(runCtx, job.Args.ScanID, ids.From[ids.OrganizationKind](job.Args.OrganizationID))
	var deferral *ai.BudgetDeferralError
	if !errors.As(err, &deferral) {
		return jobs.FaultContext(ctx, err)
	}
	return river.JobSnooze(max(time.Until(deferral.NextAttemptAt), 0))
}
