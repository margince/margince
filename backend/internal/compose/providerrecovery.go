// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Operator recovery from a provider outage: reopen the capture work an outage
// parked, so the lanes judge it now that the provider answers. The seam lives
// here because reopening a counterparty question also withdraws its review
// offer, which the approvals module owns.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ParkedWork counts capture work by kind.
type ParkedWork struct {
	// Counterparties are sender questions retired to `unsure` for lack of a verdict.
	Counterparties int
	// Enrichments are companies whose auto-enrich ran out of attempts.
	Enrichments int
}

// DefaultRecoveryBatch bounds how much one run reopens, per kind and workspace.
// A reopened row is due at once, so the batch is the most model calls a run can
// release; the operator runs again for the rest.
const DefaultRecoveryBatch = 200

// ProviderRecovery reopens what a past outage parked.
type ProviderRecovery struct {
	pool      *pgxpool.Pool
	pending   *capture.PendingStore
	enrich    *capture.AutoEnrichStore
	approvals *approvals.Service
}

// NewProviderRecovery builds the recovery on an installation pool.
func NewProviderRecovery(pool *pgxpool.Pool) *ProviderRecovery {
	return &ProviderRecovery{
		pool:      pool,
		pending:   capture.NewPendingStore(InstallationDB(pool)),
		enrich:    capture.NewAutoEnrichStore(InstallationDB(pool)),
		approvals: approvals.NewService(InstallationDB(pool)),
	}
}

// Count reports what Reopen would act on, writing nothing. It counts the whole
// window, not one batch, so the operator sees how many runs it will take.
func (r *ProviderRecovery) Count(ctx context.Context, w capture.ReopenWindow) (ParkedWork, error) {
	return r.eachWorkspace(ctx, w, func(wsCtx context.Context) (ParkedWork, error) {
		var out ParkedWork
		var err error
		if out.Counterparties, err = r.pending.CountParkedByExhaustion(wsCtx, w); err != nil {
			return out, err
		}
		out.Enrichments, err = r.enrich.CountParkedEnrichments(wsCtx, w)
		return out, err
	})
}

// Reopen reopens up to batch rows of each kind per workspace and reports how
// many it reopened. A row a human decided in the meantime is skipped.
func (r *ProviderRecovery) Reopen(ctx context.Context, w capture.ReopenWindow, batch int) (ParkedWork, error) {
	if batch <= 0 {
		batch = DefaultRecoveryBatch
	}
	return r.eachWorkspace(ctx, w, func(wsCtx context.Context) (ParkedWork, error) {
		var out ParkedWork
		var err error
		if out.Counterparties, err = r.reopenCounterparties(wsCtx, w, batch); err != nil {
			return out, err
		}
		out.Enrichments, err = r.reopenEnrichments(wsCtx, w, batch)
		return out, err
	})
}

// eachWorkspace runs one stage per workspace as the recovery's own system actor
// and sums what each reports. Counts from workspaces that finished are kept
// when a later one fails, so the operator sees how far the run got.
func (r *ProviderRecovery) eachWorkspace(
	ctx context.Context, w capture.ReopenWindow, stage func(context.Context) (ParkedWork, error),
) (ParkedWork, error) {
	if err := w.Validate(); err != nil {
		return ParkedWork{}, err
	}
	var total ParkedWork
	err := runPerWorkspace(ctx, r.pool, func(ctx context.Context, ws ids.UUID) error {
		wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, ws), "system:provider_recovery")
		got, err := stage(wsCtx)
		total.Counterparties += got.Counterparties
		total.Enrichments += got.Enrichments
		return err
	})
	return total, err
}

func (r *ProviderRecovery) reopenCounterparties(ctx context.Context, w capture.ReopenWindow, batch int) (int, error) {
	parked, err := r.pending.ParkedByExhaustion(ctx, w, batch)
	if err != nil {
		return 0, err
	}
	reopened := 0
	for _, id := range parked {
		ok, err := r.reopenOneCounterparty(ctx, w, id)
		if err != nil {
			return reopened, fmt.Errorf("recovery: reopening a parked sender question: %w", err)
		}
		if ok {
			reopened++
		}
	}
	return reopened, nil
}

// reopenOneCounterparty withdraws the standing offer and reopens the row in ONE
// transaction, in the order that makes a concurrent human decision safe: lock
// the row, take the offer off the inbox, then reopen. A decision that got there
// first leaves the offer no longer pending and this backs out unchanged.
func (r *ProviderRecovery) reopenOneCounterparty(ctx context.Context, w capture.ReopenWindow, id ids.UUID) (bool, error) {
	reopened := false
	err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		proposalID, ok, err := r.pending.ClaimParkedForReopen(ctx, tx, w, id)
		if err != nil || !ok {
			return err
		}
		if proposalID != nil {
			withdrawn, err := r.approvals.WithdrawInTx(ctx, tx, ids.From[ids.ApprovalKind](*proposalID),
				"the provider outage that parked this question has been recovered")
			if err != nil || !withdrawn {
				return err
			}
		}
		if err := r.pending.ReopenParkedTx(ctx, tx, id, w); err != nil {
			return err
		}
		reopened = true
		return nil
	})
	return reopened, err
}

func (r *ProviderRecovery) reopenEnrichments(ctx context.Context, w capture.ReopenWindow, batch int) (int, error) {
	parked, err := r.enrich.ParkedEnrichments(ctx, w, batch)
	if err != nil {
		return 0, err
	}
	reopened := 0
	for _, id := range parked {
		ok, err := r.enrich.ReopenEnrichment(ctx, w, id)
		if err != nil {
			return reopened, fmt.Errorf("recovery: reopening a parked enrichment: %w", err)
		}
		if ok {
			reopened++
		}
	}
	return reopened, nil
}
