// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The VAT-check worker: a company's stated VAT ID becomes a checked one.
//
// It runs on its own queue at ONE worker, and that bound is the service's terms
// rather than a performance choice. VIES describes itself as a service for
// occasional verification, throttles by IP and blocks abusers, so a second
// worker would be a second requester however carefully each paced itself. The
// pacer in platform/vatcheck enforces the interval; the queue's max_workers
// enforces the single thread. Neither alone is enough.
//
// THE TRIGGER IS INGESTION, not a sweep. A company is checked when its imprint
// is read and a VAT number is written, so consultations arrive at the rate
// imprints are read rather than in a daily burst. Re-reading a website is the
// re-check.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/vatcheck"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CheckOrganizationVatArgs is one queued consultation: the tenant and the
// company.
//
// The NUMBER IS NOT among them, deliberately. The worker reads it from the row
// when it runs, so a consultation queued before a correction asks about the
// number the company actually states rather than the one it stated when the job
// was made. A copy in the args would be a receipt for the wrong number.
type CheckOrganizationVatArgs struct {
	Workspace      ids.UUID `json:"workspace_id"`
	OrganizationID ids.UUID `json:"organization_id"`
	// Requested marks a consultation a PERSON asked for, and it does two things
	// that both matter.
	//
	// It tells the worker to ask even when the stored answer already names this
	// number: the automatic lanes ask only about a number they have not seen,
	// because nothing re-reads a website on a schedule either, but a person
	// pressing the button has said the stored answer is not good enough.
	//
	// It also makes the args DIFFER from a write-queued job's, which is what
	// lets the request through the by-args dedupe below. Without it a request
	// arriving while an automatic consultation was pending would be swallowed
	// and the reader would watch a button do nothing.
	Requested bool `json:"requested,omitempty"`
}

// Kind is the stable job identifier River persists in river_job.
func (CheckOrganizationVatArgs) Kind() string { return "check_organization_vat" }

// WorkspaceID binds this consultation to its tenant (jobs.WorkspaceScoped).
func (a CheckOrganizationVatArgs) WorkspaceID() ids.UUID { return a.Workspace }

// vatCheckQueue is declared in api/jobs.yaml at one worker; the name is spelled
// here because the insert has to name it.
const (
	vatCheckQueue = "vat_check"
	// vatCheckMaxWorkers mirrors api/jobs.yaml. One, for the reason stated at
	// the top of this file.
	vatCheckMaxWorkers = 1
	// vatCheckMaxAttempts bounds how often the service is asked about one
	// number. A consultation that keeps failing is the service being unwell,
	// and spending the installation's shared rate on it starves the companies
	// that would answer.
	vatCheckMaxAttempts = 3
)

// VatCheckEnqueueFor builds the in-transaction enqueue the people store calls
// when a VAT number is written.
//
// Nil-safe by contract, and nil is a real composition: a deployment that checks
// no VAT numbers writes the number and queues nothing. The number is what the
// page stated; the verification is what this installation can offer.
func VatCheckEnqueueFor(enqueue vatCheckEnqueuer) people.VatCheckEnqueue {
	if enqueue == nil {
		return nil
	}
	return func(ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, requested bool) error {
		ws, ok := principal.WorkspaceID(ctx)
		if !ok {
			// No tenant bound means no job that could ever be worked: River
			// scopes by workspace, and a row without one is a job nobody
			// dequeues. Refusing is louder than inserting an orphan.
			return errors.New("compose: checking a company's VAT number outside any workspace")
		}
		return enqueue.EnqueueTx(ctx, tx, CheckOrganizationVatArgs{
			Workspace:      ws,
			OrganizationID: orgID.UUID,
			Requested:      requested,
		}, vatCheckInsertOpts())
	}
}

// vatCheckEnqueuer is the insert-only half of the job runner: the api role
// queues, the worker role works.
type vatCheckEnqueuer interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// vatCheckInsertOpts puts the job on its own queue and DEDUPLICATES by args.
//
// A rep correcting a VAT number character by character would otherwise queue a
// consultation per save, each spending an interval of a rate the whole
// installation shares. One pending consultation per company is all that is ever
// useful, because the worker reads the number when it RUNS rather than from the
// args.
func vatCheckInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue: vatCheckQueue,
		// ByArgs across every ACTIVE state: River requires the unique set to
		// include pending and running and refuses a narrower one outright.
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
		MaxAttempts: vatCheckMaxAttempts,
	}
}

// vatCheckWorker consults the register about one company.
//
// It is registered whether or not a checker is configured, which is deliberate:
// a queued job on a deployment that later loses its provider must be
// answerable, and a worker that was never registered leaves it stuck rather
// than recorded.
type vatCheckWorker struct {
	river.WorkerDefaults[CheckOrganizationVatArgs]
	pool    *pgxpool.Pool
	checker vatcheck.Checker
	clock   func() time.Time
}

// newVatCheckWorker assembles the worker-role consultation. checker may be nil
// — a deployment with no provider records nothing rather than retrying.
//
// No logger: every failure this worker has is RETURNED, through
// jobs.FaultContext, so it lands in river_job.errors where an operator looking
// at a stuck job will actually find it.
func newVatCheckWorker(pool *pgxpool.Pool, checker vatcheck.Checker, clock func() time.Time) *vatCheckWorker {
	if clock == nil {
		clock = time.Now
	}
	return &vatCheckWorker{pool: pool, checker: checker, clock: clock}
}

// Work consults the register and records what it answered.
//
// An INVALID number is written like a valid one. It is the answer, not a
// failure, and a company whose stated number is not real is exactly the finding
// this lane exists to surface — forgetting it would re-ask forever and tell
// nobody.
func (w *vatCheckWorker) Work(ctx context.Context, job *river.Job[CheckOrganizationVatArgs]) error {
	args := job.Args
	// Bound through the shared helper, so the args' own WorkspaceID() IS the
	// binding: a worker that picked its own could claim one workspace and work
	// in another.
	wsCtx, err := workspaceJobCtx(ctx, args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = vatCheckJobActor(wsCtx)
	store := people.NewStore(database.Bind(w.pool, func(context.Context) (ids.WorkspaceID, error) {
		return ids.From[ids.WorkspaceKind](args.Workspace), nil
	}))
	orgID := ids.From[ids.OrganizationKind](args.OrganizationID)

	number, ok, err := store.VatNumberForCheck(wsCtx, orgID)
	if err != nil {
		return jobs.FaultContext(wsCtx, fmt.Errorf("reading the VAT number to check: %w", err))
	}
	// A person who pressed the button asked for THIS consultation, so the only
	// thing that stops it is the company stating no number at all. The staleness
	// rule the automatic lanes obey is about not spending the installation's
	// shared rate on questions nobody asked; this one was asked.
	if args.Requested && number != "" {
		ok = true
	}
	if !ok {
		// Nothing to ask about: the company states no VAT number, or the one it
		// states has already been consulted. Both mean "do not ask".
		return nil
	}
	if w.checker == nil {
		// A deployment with no checker should not have queued this. Recording
		// nothing keeps the row honest — an absent check is not a failed one —
		// and the job succeeds rather than retrying against a provider that
		// will never exist.
		return nil
	}

	result, err := w.checker.Check(wsCtx, number)
	if errors.Is(err, vatcheck.ErrMalformedNumber) {
		// The page stated something that is not a VAT ID. No request was made,
		// so the register said NOTHING — recording `invalid` here would be
		// indistinguishable from a negative answer it never gave. It is
		// recorded as unanswered, which is what actually happened.
		return jobs.FaultContext(wsCtx, store.RecordVatCheck(wsCtx, people.VatCheck{
			OrganizationID: orgID,
			Number:         number,
			Status:         people.VatCheckUnavailable,
			CheckedAt:      w.clock(),
		}))
	}
	var refused *vatcheck.ProviderRefusedError
	if errors.As(err, &refused) && refused.RetryAfter > 0 {
		// The service told us when to come back, and coming back sooner is how
		// an installation gets blocked. River's own schedule would.
		return river.JobSnooze(refused.RetryAfter)
	}
	if err != nil {
		// A refusal or a transport failure is the consultation not completing.
		// Returned so River retries it on the attempt ledger rather than
		// recording an answer nobody gave.
		return jobs.FaultContext(wsCtx, fmt.Errorf("consulting the VAT register: %w", err))
	}

	// The date on the receipt is the REGISTER's, because that is what the
	// receipt attests to. Our clock stands in only when the service sent none.
	consultedAt := result.RequestDate
	if consultedAt.IsZero() {
		consultedAt = w.clock()
	}
	return jobs.FaultContext(wsCtx, store.RecordVatCheck(wsCtx, people.VatCheck{
		OrganizationID:     orgID,
		Number:             number,
		Status:             people.VatCheckStatus(result.Status),
		ConsultationNumber: result.ConsultationNumber,
		RegisteredName:     result.Name,
		RegisteredAddress:  result.Address,
		CheckedAt:          consultedAt,
	}))
}

// WithVatChecking lets a VAT-number write queue a consultation of the EU
// register.
//
// Without it a company's VAT number still writes and nothing is checked, which
// is the honest posture for a deployment that reaches no EU service: the number
// says what the page said and claims nothing more.
//
// It wires the ENQUEUE, not the provider. The provider lives on the worker role
// (JobRunnerConfig.VatChecker); this is the api role's half, and the two are
// deliberately separate — an installation can queue consultations from one
// process and make them in another, which is what keeps the single-requester
// rule enforceable at one worker.
func WithVatChecking(inserter *jobs.Runner) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		enqueue := VatCheckEnqueueFor(inserter)
		// BOTH, because they are two stores: the services read s.peopleStore
		// and the HTTP transport carries its own. Wiring one would leave every
		// VAT number a rep corrects unchecked with nothing coming to ask.
		s.peopleStore = s.peopleStore.WithVatCheckEnqueue(enqueue)
		//nolint:staticcheck // QF1008: the embedded name is load-bearing — s.Handlers resolves to briefs.Handlers, a different embedded type
		s.peopleHandlers = s.peopleHandlers.WithVatCheckEnqueue(enqueue)
		// And every store compose builds from a pool — the approval effects,
		// the capture sink, the verdict engine — which is where a site read's
		// accepted VAT number actually lands.
		BindVatChecking(enqueue)
	}
}

// vatCheckBinding is the enqueue every people.Store built inside compose picks
// up, held at package scope for the reason WithVatChecking cannot cover.
//
// The Option wires the two stores the SERVER holds. The approval effects — the
// path a site read's accepted fields actually take — build their own store from
// a pool (newCounterpartyStore), as do the capture sink and the verdict engine,
// and none of those can reach the job runner: they are constructed from a pool
// alone, several layers below the composition that has one. Threading a runner
// through all of them is a wider change than this lane, and leaving them unwired
// meant the main path queued nothing at all.
var vatCheckBinding struct {
	mu      sync.RWMutex
	enqueue people.VatCheckEnqueue
}

// BindVatChecking records the enqueue for every store compose builds from a
// pool. Called once at boot by the role that queues, after the job runner
// exists. A nil enqueue is a deployment that checks no VAT numbers, and every
// such store then writes the number and queues nothing.
func BindVatChecking(enqueue people.VatCheckEnqueue) {
	vatCheckBinding.mu.Lock()
	defer vatCheckBinding.mu.Unlock()
	vatCheckBinding.enqueue = enqueue
}

// boundVatCheckEnqueue reads the binding. Read per STORE rather than captured
// once, so the ordering between binding and building a store cannot matter —
// only the ordering against the first write, which is after boot either way.
func boundVatCheckEnqueue() people.VatCheckEnqueue {
	vatCheckBinding.mu.RLock()
	defer vatCheckBinding.mu.RUnlock()
	return vatCheckBinding.enqueue
}

// vatCheckJobActor is the installation acting for itself: the consultation is
// made on the deployment's own behalf, under its own VAT number, and no user
// asked for it.
func vatCheckJobActor(ctx context.Context) context.Context {
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:vatcheck",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}
