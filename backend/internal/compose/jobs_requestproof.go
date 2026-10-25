// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requestProofExpiryActor is the principal the pass's audit rows carry.
const requestProofExpiryActor = "system:privacy_request_proof_expiry"

// requestProofExpiryBatch bounds one transaction of the pass. The pass runs
// batches until one comes back short, so a year's proofs all go in one run.
const requestProofExpiryBatch = 200

// RequestProofExpiryArgs schedules one deletion of the privacy-request proofs
// past their retention date.
type RequestProofExpiryArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (RequestProofExpiryArgs) Kind() string { return "privacy_request_proof_expiry" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
func (RequestProofExpiryArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

type requestProofExpiryWorker struct {
	requests *consent.Store
	identity *identity.Service
	now      func() time.Time
	log      *slog.Logger
}

func (w *requestProofExpiryWorker) Work(ctx context.Context, _ *river.Job[RequestProofExpiryArgs]) error {
	wsCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = principal.SystemActing(wsCtx, requestProofExpiryActor)
	now := w.now()
	total := 0
	for {
		expired, err := w.requests.ExpireRequestProofs(wsCtx, now, requestProofExpiryBatch)
		if err != nil {
			return jobs.FaultContext(ctx, err)
		}
		total += expired
		if expired < requestProofExpiryBatch {
			break
		}
	}
	if total > 0 {
		w.log.InfoContext(wsCtx, "privacy request proofs past their retention date deleted", "proofs", total)
	}
	return nil
}
