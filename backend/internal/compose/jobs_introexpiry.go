// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tick that closes an introduction nobody answered.
//
// `due_at` was written on every ask and read by nobody, so a colleague's
// silence never became an answer. The rep kept waiting on somebody who had
// moved on, and — because the open-route index is a partial unique index — the
// route stayed blocked: the duplicate guard, which exists to stop two tabs
// racing, became a refusal nothing could clear.
//
// The decision belongs to the introductions module: the status, the
// system-actor audit row, the event. This file supplies only the clock, because
// a policy that fires on a schedule still needs something to be scheduled.
//
// One job, not a dispatcher and a child (ADR-0103 §1): the pass is one indexed
// scan over an installation-wide table, so there is nothing to fan out over.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// IntroExpiryArgs closes the asks whose due date has passed.
type IntroExpiryArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (IntroExpiryArgs) Kind() string { return "intro_expiry" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
//
// One attempt: this pass's retry is its own next tick, and every ask it did not
// reach is still due then — the predicate is a clock, and a clock does not need
// a second rung to come back to something.
func (IntroExpiryArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

type introExpiryWorker struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	log      *slog.Logger
}

// Work closes what is due, and reaches no other module.
//
// The obvious extra step is notifying the requester here. It is not this job's
// to take: the expiry emits intro_request.closed in the SAME transaction that
// writes it, through the outbox, so anything that must tell somebody rides that
// event with a delivery guarantee this file would otherwise have to reproduce.
// A second loop here would work most of the time and leave a window — a crash
// between the two would close an ask nobody was ever told about, and the next
// tick scans for OPEN rows, so it would never come back to it.
func (w *introExpiryWorker) Work(ctx context.Context, _ *river.Job[IntroExpiryArgs]) error {
	passCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// The clock is the actor. Every expiry writes an audit row and an event and
	// both need a principal — but nobody decided any of this, so binding a
	// human would put their name on a refusal they never made. The correlation
	// id groups one tick's expiries as the single pass they are.
	passCtx = principal.WithActor(passCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: introductions.ExpiryActor,
	})
	passCtx = principal.WithCorrelationID(passCtx, ids.NewV7())

	store := introductions.NewStore(InstallationDB(w.pool), time.Now)
	expired, err := store.ExpireDue(passCtx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if expired > 0 {
		// Worth a line: an expired ask is a favour nobody answered, so a run of
		// them is the shape of a team not reading its queue rather than a
		// healthy sweep.
		w.logger().InfoContext(ctx,
			"introduction expiry: asks closed unanswered", "count", expired)
	}
	return nil
}

func (w *introExpiryWorker) logger() *slog.Logger {
	if w.log == nil {
		return slog.Default()
	}
	return w.log
}
