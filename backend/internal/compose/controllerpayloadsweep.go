// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The backstop for one-time link material no terminal outcome retired.
//
// A controller send destroys its own payload when it is accepted and when it is
// parked (comms.Dispatcher.retireLink), so on a healthy installation this pass
// finds nothing and that is the point. What the happy paths cannot reach is a
// delivery that reaches NO terminal outcome — a worker killed mid-send, a stack
// torn down between the provider's acceptance and the retire — and the retire
// itself failing, which is logged and deliberately does not change the
// delivery's disposition rather than sending the message twice to protect a
// credential.
//
// Both leave a live confirmation link in the key vault past its expiry, and a
// link that outlives its expiry is a credential to somebody's mailbox that
// nobody is watching. This is not the erasure path: Art. 17 destroys the
// material regardless of what happens here, and a sweep that a subject had to
// wait for would be the wrong answer to a deletion request.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ControllerPayloadSweepArgs carries nothing: the pass reads what has expired
// rather than being told, because the rows it exists to find are exactly the
// ones nobody knows about.
type ControllerPayloadSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ControllerPayloadSweepArgs) Kind() string { return "comms_controller_payload_sweep" }

// controllerPayloadSweepBatch bounds one pass. A backlog is retired across several runs rather
// than in one long transaction — the pass runs every quarter hour, and a sweep
// draining an outage's whole backlog at once would hold its slot for as long as
// the outage lasted.
const controllerPayloadSweepBatch = 200

// controllerPayloadSweepCap bounds the whole pass, not one page. The paging
// above walks past rows it cannot destroy, and without a ceiling a large
// backlog would hold this slot for as long as it took to walk all of it.
const controllerPayloadSweepCap = 2000

// controllerPayloadSweepWorker destroys link material past its expiry.
type controllerPayloadSweepWorker struct {
	identity *identity.Service
	store    *comms.Store
	// controllerPayloads and not a raw keyvault.Vault: the adapter already
	// resolves the workspace off the context the way every other reader of this
	// material does, and a second spelling of that resolution is a second answer
	// to "whose vault is this".
	vault controllerPayloads
	clock func() time.Time
	log   *slog.Logger
}

func newControllerPayloadSweepWorker(
	idsvc *identity.Service,
	store *comms.Store,
	vault controllerPayloads,
	log *slog.Logger,
) *controllerPayloadSweepWorker {
	return &controllerPayloadSweepWorker{
		identity: idsvc, store: store, vault: vault, clock: time.Now, log: log,
	}
}

func (w *controllerPayloadSweepWorker) Work(ctx context.Context, _ *river.Job[ControllerPayloadSweepArgs]) error {
	ctx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("comms_controller_payload_sweep: resolving the installation: %w", err))
	}
	ctx = sendWorkerScope(ctx)

	// Paged past what this run already attempted, so a batch that will never
	// delete costs one batch rather than the whole backlog: without it the same
	// oldest rows are re-selected every cadence and every newer payload behind
	// them stays undestroyed while the pass reports itself busy.
	var expired []ids.UUID
	var cursor time.Time
	for len(expired) < controllerPayloadSweepCap {
		batch, last, err := w.store.ExpiredControllerPayloads(
			ctx, w.clock(), cursor, controllerPayloadSweepBatch)
		if err != nil {
			return jobs.FaultContext(ctx, fmt.Errorf("comms_controller_payload_sweep: reading expired link material: %w", err))
		}
		if len(batch) == 0 {
			break
		}
		expired = append(expired, batch...)
		cursor = last
	}
	if len(expired) == 0 {
		return nil
	}

	// AFTER the read, not before it. This worker's vault comes from the config
	// of the process it was built in, and says nothing about the process that
	// staged the rows: a worker role deployed without a vault while the API
	// role has one would stage material and then silently refuse to sweep it —
	// which is the exact scenario this pass exists for. Asking first would hide
	// it; asking here names the count nobody is destroying. A deployment with
	// no vault genuinely has no expired rows, so this stays quiet on its own
	// rather than printing every quarter hour.
	if w.vault.v == nil {
		w.log.ErrorContext(ctx, "expired link material found and no vault is configured to destroy it",
			"found", len(expired))
		return nil
	}

	var retired, failed int
	for _, id := range expired {
		if err := w.retire(ctx, id); err != nil {
			// One payload that will not destroy must not strand the rest: they
			// are independent, and the cadence re-reads what is still expired.
			failed++
			w.log.ErrorContext(ctx, "destroying expired link material failed",
				"delivery_id", id, "err", err)
			continue
		}
		retired++
	}
	// After the loop and counting outcomes, so the line cannot claim a
	// destruction that did not happen. WARN because reaching here at all means
	// material outlived its expiry, which an operator wants to know even though
	// it is now gone. `failed` staying non-zero across passes is a row no
	// cadence will heal, and this line is the only place that is visible.
	w.log.WarnContext(ctx, "one-time link material outlived its expiry",
		"found", len(expired), "retired", retired, "failed", failed)
	return nil
}

// retire destroys the vault entry, then clears the column that names it.
//
// That order, and not the reverse: clearing first and failing would leave an
// orphan in the vault with nothing left pointing at it, which no later pass can
// find. Failing between the two leaves the reference intact and the next
// cadence retries it — the material is already gone, and keyvault.Delete is
// idempotent, so a second attempt is harmless.
func (w *controllerPayloadSweepWorker) retire(ctx context.Context, id ids.UUID) error {
	ref, err := w.store.PayloadRefFor(ctx, id)
	if err != nil {
		return err
	}
	if ref == "" {
		// Retired between the read and here, by the send path finishing its own
		// work. Nothing to do and not an error: this pass is the backstop.
		return nil
	}
	if err := w.vault.Delete(ctx, ref); err != nil {
		return fmt.Errorf("destroying the one-time link material: %w", err)
	}
	return w.store.ClearPayloadRef(ctx, id)
}

// addControllerPayloadSweepJob registers the pass and its cadence.
//
// Gated on exactly what jobs.yaml declares — SendRegistry and SendDelivery,
// what the controller lane itself needs — and NOT additionally on the vault.
// The declaration is the contract the job census reads, and a guard demanding
// more than it says makes this kind declared-but-unregistered: the census fails,
// and a role that ticked its dispatcher would queue rows nothing can claim.
//
// A missing vault is handled where it can be said honestly, in the worker: the
// pass reads first and then reports the count it cannot destroy, rather than
// returning early and reporting nothing.
func addControllerPayloadSweepJob(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	if cfg.SendRegistry == nil || cfg.SendDelivery == nil {
		return nil
	}
	db := InstallationDB(pool)
	worker := newControllerPayloadSweepWorker(
		identity.NewService(pool),
		comms.NewStore(db, time.Now, activities.NewStore(db)),
		controllerPayloads{v: cfg.ControllerVault},
		log)
	addDeclaredWorker[ControllerPayloadSweepArgs](reg, worker)
	return periodicFor(cfg, ControllerPayloadSweepArgs{})
}
