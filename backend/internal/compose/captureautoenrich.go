// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The captured-company auto-enrich sweep (CAP-PARAM-7, ADR-0072/A118):
// a leader-elected periodic pass (run-on-start + daily) that gives every
// company with a primary domain and no dossier a governed web one — however it
// was named, since a person creating one is usually the moment they want it.
// The anchor is excluded; cold start has already read it. Per workspace, when
// the capture_auto_enrich flag is on, it enqueues a deep read
// (system:capture_auto_enrich, auto-applied on completion) for each due company —
// oldest first, under an atomically-reserved daily cap, so a company behind a
// day's worth of arrivals is still reached (ListDueCompanies says why). It is the
// self-healing reconciler: the prompt trigger is the company-event
// consumer (companyautoenrich.go), which queues this same workspace pass the
// moment a company appears, and anything that slips through — the worker
// down, an enqueue lost — is simply picked up next daily pass. The
// deep-read worker's auto-apply lane (deepread.go) records the terminal
// outcome on the sweep's cursor.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// defaultAutoEnrichDailyCap is the installation-wide ceiling on auto deep reads
// started in one UTC day — the counter (capture_auto_enrich_budget) is keyed on
// the date alone, so every workspace pass spends from the one pot. Reserved
// atomically, so two replicas never both slip past it.
//
// It is the THIRD bound on this fan-out, not the only one, and knowing what the
// other two already do is what sets the number:
//
//   - Concurrency is bounded by deepReadMaxWorkers (2): a burst never occupies
//     more than two workers however long the queue is.
//   - Money is bounded by the ADR-0020 AI budget: background model calls defer
//     to the next window at the monthly cap, whatever this counter says.
//   - And a read only ever happens for a company the workspace CREATED, which
//     the tiered gate allows only for an address the owner has corresponded with
//     or already has a person for. A stranger's mail defers to the ledger and
//     mints nothing, so an outsider cannot aim this at a domain of their choosing.
//
// What is left for this counter is PACING, and 500 is sized for the moment that
// decides whether the product is believed: a first backfill mints hundreds of
// companies at once, and what it is demonstrating is that the CRM fills itself
// (P5). A ceiling below that arrival rate turns the demonstration into a trickle
// and teaches the opposite, while a ceiling above it buys nothing the three
// bounds above do not already give. A backfill can still outrun the number — a
// mailbox that meets more than this many new domains in one UTC day leaves the
// excess pending until the next day's budget — which is what
// AutoEnrichDailyCapEnv exists for.
const defaultAutoEnrichDailyCap = 500

// AutoEnrichDailyCapEnv overrides defaultAutoEnrichDailyCap for a deployment
// whose backfills outrun it. Exported so the composition roots can declare it
// as part of their configurable surface without spelling the string a second
// time. Read by BOTH roles: the worker spends the budget in its sweeps, and the
// api spends it when an approval accept queues a triage read — a split value
// would let one role spend what the other believes is left.
const AutoEnrichDailyCapEnv = "MARGINCE_AUTO_ENRICH_DAILY_CAP"

// AutoEnrichDailyCapFromEnv resolves the daily cap: unset or 0 takes the
// compiled default, a positive integer replaces it, anything else is an error.
// Both cmd roles call this at boot so a typo refuses the boot rather than
// silently pacing at the wrong rate; the constructors below resolve through the
// same spelling, which is what keeps the two reads one rule.
func AutoEnrichDailyCapFromEnv(env config.Lookup) (int, error) {
	v := strings.TrimSpace(env(AutoEnrichDailyCapEnv))
	if v == "" {
		return defaultAutoEnrichDailyCap, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid %s %q: want a whole number >= 0 (0 takes the compiled default)", AutoEnrichDailyCapEnv, v)
	}
	if n == 0 {
		return defaultAutoEnrichDailyCap, nil
	}
	return n, nil
}

// autoEnrichDailyCap resolves the cap for a constructor that cannot refuse to
// boot. A booted role never reaches the error branch — both cmd roles refuse
// an invalid value before any constructor runs, and a process environment is
// fixed at exec — but entry points that dispatch before that refusal (the
// worker's siteread debug subcommand) construct through here too, so an
// unreadable value is reported and paced at the compiled default, the
// direction that spends less.
func autoEnrichDailyCap(log *slog.Logger) int {
	n, err := AutoEnrichDailyCapFromEnv(config.FromOS)
	if err != nil {
		log.Warn("auto-enrich: unreadable daily cap, pacing at the compiled default",
			"err", err, "default", defaultAutoEnrichDailyCap)
		return defaultAutoEnrichDailyCap
	}
	return n
}

// autoEnrichRetryBackoff is how long a triggered read's cursor is armed before
// the sweep may reconsider the company: long enough that an in-flight or
// just-failed read is not re-driven prematurely (ADR-0072: 7 days).
const autoEnrichRetryBackoff = 7 * 24 * time.Hour

// CaptureAutoEnrichSweepArgs is the periodic sweep's (empty) job payload.
type CaptureAutoEnrichSweepArgs struct{}

// Kind is the River job kind for the auto-enrich sweep.
func (CaptureAutoEnrichSweepArgs) Kind() string { return "capture_auto_enrich_sweep" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CaptureAutoEnrichSweepArgs) FleetWide() {}

// captureAutoEnrichSweepWorker runs one sweep pass across every workspace.
type captureAutoEnrichSweepWorker struct {
	pool       *pgxpool.Pool
	people     *people.Store
	settings   *capture.SettingsStore
	autoEnrich *capture.AutoEnrichStore
	dailyCap   int
	log        *slog.Logger
}

// newCaptureAutoEnrichSweepWorker builds the sweep worker over the pool.
func newCaptureAutoEnrichSweepWorker(pool *pgxpool.Pool, log *slog.Logger) *captureAutoEnrichSweepWorker {
	return &captureAutoEnrichSweepWorker{
		pool:       pool,
		people:     people.NewStore(InstallationDB(pool)),
		settings:   capture.NewSettings(NewSettingsStore(pool)),
		autoEnrich: capture.NewAutoEnrichStore(InstallationDB(pool)),
		dailyCap:   autoEnrichDailyCap(log),
		log:        log,
	}
}

// Work is the PASS: it walks the live workspaces and enriches each one.
//
// It used to be a dispatcher that enqueued one capture_auto_enrich_workspace
// per workspace (ADR-0103). The child kind is gone; the enumeration is not,
// because the collapse is about how many job kinds describe one pass and not
// about assuming a single tenant.
func (w *captureAutoEnrichSweepWorker) Work(ctx context.Context, _ *river.Job[CaptureAutoEnrichSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, func(ctx context.Context, ws ids.UUID) error {
		return w.sweepWorkspace(ctx, ids.From[ids.WorkspaceKind](ws))
	}))
}

// sweepWorkspace enriches the due companies of one workspace, respecting the flag
// and the daily cap. The flag is re-read at the top of every pass, so toggling
// it off stops new reads on the next sweep even for already-queued work.
func (w *captureAutoEnrichSweepWorker) sweepWorkspace(ctx context.Context, ws ids.WorkspaceID) error {
	wsCtx := w.workspaceCtx(ctx, ws)
	settings, err := w.settings.Get(wsCtx)
	if err != nil {
		return err
	}
	// The open domain questions are swept whatever the setting says. With
	// crawling off the worker settles each one from what the workspace already
	// knows; skipping them would leave every person captured under that setting
	// without a company, permanently.
	if err := w.sweepDomainTriage(wsCtx, w.dailyCap); err != nil {
		return err
	}
	if !settings.AutoEnrich {
		return nil
	}
	// Retire any cursors that have used every attempt, so they leave the due
	// index instead of being re-scanned every pass (the real 'exhausted' state).
	if err := w.autoEnrich.ExpireExhausted(wsCtx); err != nil {
		return err
	}
	due, err := w.autoEnrich.ListDueCompanies(wsCtx, w.dailyCap)
	if err != nil {
		return err
	}
	for _, company := range due {
		slot, err := w.autoEnrich.ReserveBudget(wsCtx, w.dailyCap)
		if err != nil {
			return err
		}
		if !slot.Reserved {
			// The day's cap is spent — stop; the rest wait for tomorrow's pass.
			return nil
		}
		if err := w.triggerEnrich(wsCtx, company, slot); err != nil {
			// A single company's trigger fault must not consume the pass; log it
			// and move on. The cursor stays due (nothing was queued), so the
			// next pass retries, and the slot it reserved has already gone back.
			w.log.WarnContext(wsCtx, "capture auto-enrich: trigger failed",
				"company", company.CompanyID.String(), "err", err)
			continue
		}
	}
	return nil
}

// triggerEnrich starts a system-requested deep read for one company and arms its
// cursor, returning the reserved slot when no read came of it.
func (w *captureAutoEnrichSweepWorker) triggerEnrich(ctx context.Context, company capture.DueCompany, slot capture.BudgetSlot) error {
	return startEnrichOrRefund(ctx, w.people, w.autoEnrich, company.CompanyID, company.Domain, slot)
}

// refundTimeout bounds the compensating write. Short: it is one indexed UPDATE,
// and a refund that hangs would hold the pass or the capture it runs on.
const refundTimeout = 5 * time.Second

// startEnrichOrRefund is the whole rule about a reserved slot, in one place
// because both callers own the same invariant: a slot buys a read or it goes
// back.
//
// The ordering is load-bearing. `started` is checked BEFORE `err`, because
// startAutoEnrichRead reports (true, err) for a read that was queued and whose
// cursor could not then be armed — refunding there would pay for that crawl
// twice, returning the slot to the day while the read still runs.
//
// The refund detaches from the caller's context, for the case that matters most:
// when the start failed BECAUSE that context was cancelled, the refund is owed
// and the context it would run on is already dead. Compensating on the dying
// context leaks the slot exactly when it is most likely to leak. Same
// detach-and-deadline shape the connector teardown and the AI tracer use.
func startEnrichOrRefund(ctx context.Context, peopleStore *people.Store,
	autoEnrich *capture.AutoEnrichStore, companyID ids.CompanyID, domain string, slot capture.BudgetSlot,
) error {
	started, err := startAutoEnrichRead(ctx, peopleStore, autoEnrich, companyID, domain)
	if started {
		return err
	}
	refundCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), refundTimeout)
	defer cancel()
	refundErr := autoEnrich.ReleaseBudget(refundCtx, slot)
	if err != nil {
		return errors.Join(err, refundErr)
	}
	return refundErr
}

// startAutoEnrichRead queues ONE governed deep read for one company and
// arms its sweep cursor. Shared by the periodic sweep and the on-capture
// trigger, because they differ only in what made the company interesting —
// the read they ask for, the ceiling it runs under, the principal it is
// attributed to, and the cursor that stops it being asked for twice must all be
// the one spelling.
//
// The dossier and the River job are one transaction (StartSiteReadQueued +
// InsertTx), so a crash can never leave a dossier without its job; the in-flight
// uniqueness index dedupes a concurrent start.
//
// It does NOT crawl. This queues work and returns — the pages are fetched and
// the model is called in the deep-read worker, on its own job.
//
// It reports whether a read was actually STARTED, as opposed to joined onto one
// already in flight. Both callers reserve a budget slot before calling, and a
// join means that slot bought nothing: without the distinction, a sweep and a
// capture racing on one company spend two of the day's reads on a single
// crawl and charge that company two of its bounded attempts. The
// cursor is armed only by the caller that started something, for the same
// reason.
func startAutoEnrichRead(ctx context.Context, peopleStore *people.Store,
	autoEnrich *capture.AutoEnrichStore, companyID ids.CompanyID, domain string,
) (bool, error) {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return false, err
	}
	seedURL := "https://" + domain
	_, joined, err := peopleStore.StartSiteReadQueued(ctx, companyID, seedURL, systemAutoEnrichActor,
		func(ctx context.Context, tx pgx.Tx, read people.SiteRead) error {
			_, insErr := client.InsertTx(ctx, tx, SiteDeepReadArgs{
				Workspace:   storekit.MustWorkspace(ctx),
				CompanyID:   companyID.UUID,
				SiteReadID:  read.ID,
				RequestedBy: read.RequestedBy,
				// Declared in the payload as well as enforced at the worker: a
				// job carries what it was queued to cost, so an operator
				// reading river_job sees the ceiling without inferring it.
				MaxPages: autoEnrichMaxPages,
			}, siteDeepReadInsertOpts())
			return insErr
		})
	if err != nil {
		return false, err
	}
	if joined {
		// Someone else's read is already in flight for this company; the
		// uniqueness index arbitrated it. Arming the cursor again would charge a
		// second attempt for one crawl.
		return false, nil
	}
	return true, autoEnrich.MarkQueued(ctx, companyID, autoEnrichRetryBackoff)
}

// workspaceCtx binds the sweep's system principal on the given workspace. A
// PrincipalSystem is unbounded (auth.Unbounded), so it passes the
// company-update/visibility gates StartSiteReadQueued and the settings read
// enforce, without impersonating any human.
func (w *captureAutoEnrichSweepWorker) workspaceCtx(ctx context.Context, ws ids.WorkspaceID) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: systemAutoEnrichActor,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}
