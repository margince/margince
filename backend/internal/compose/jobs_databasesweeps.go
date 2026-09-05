// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// addDatabaseOnlySweepJobs registers every periodic pass that needs nothing but
// the database.
//
// It takes no JobRunnerConfig, and that is what the group IS rather than an
// economy of arguments — there is no lane, credential or registry any of these
// could be gated on, so every role that runs jobs runs all of them.
//
// Two shapes are here, and the difference decides what gets scheduled:
//
//   - Dispatcher plus workspace worker — close-date hygiene, the follow-up
//     reconcile, the automation clock scan, idempotency-key retention. Only the
//     dispatcher is ticked (the schedules in wireJobs); the workspace worker is
//     enqueued by it and never scheduled.
//   - Installation-wide, with no workspace fan-out — agent-task retention,
//     approval expiry, intro expiry, approval auto-apply. One row does the whole
//     installation, so there is no child kind to enqueue.
//
// It also pulls in the AI-activity and brief-generate groups. Those are their
// own functions because their WORKERS need collaborators the rest of this group
// does not — a projection store, and the brief engine plus the identity service
// — not because they are gated differently: the gating is the same nothing, and
// that is why they belong here rather than behind a condition of their own.
func addDatabaseOnlySweepJobs(reg *jobRegistry, pool *pgxpool.Pool, log *slog.Logger, briefMail BriefMailConfig) {
	addDeclaredWorker[CloseDateSweepArgs](reg, &closeDateSweepWorker{pool: pool, corrector: NewCloseDateCorrector(pool, log)})
	addDeclaredWorker[FollowUpReconcileArgs](reg, &followUpReconcileWorker{pool: pool, reconciler: NewFollowUpReconciler(pool, log)})
	addDeclaredWorker[AssuranceSweepArgs](reg, &assuranceSweepWorker{
		pool: pool, now: func() time.Time { return time.Now().UTC() }, log: log,
	})
	addDeclaredWorker[ForecastSnapshotSweepArgs](reg, &forecastSnapshotSweepWorker{
		pool: pool, now: func() time.Time { return time.Now().UTC() }, log: log,
	})
	addDeclaredWorker[TimeScanArgs](reg, &timeScanWorker{pool: pool, log: log})
	addDeclaredWorker[IdempotencyRetentionArgs](reg, &idempotencyRetentionWorker{
		pool: pool, sweeper: NewIdempotencyRetentionSweeper(pool, log),
	})
	addDeclaredWorker[AgentTaskRetentionArgs](reg, &agentTaskRetentionWorker{
		sweeper: NewAgentTaskRetentionSweeper(pool, log), identity: identity.NewService(pool),
	})
	addDeclaredWorker[ApprovalExpiryArgs](reg, &approvalExpiryWorker{
		pool: pool, identity: identity.NewService(pool), log: log,
	})
	addDeclaredWorker[IntroExpiryArgs](reg, &introExpiryWorker{
		pool: pool, identity: identity.NewService(pool), log: log,
	})
	addDeclaredWorker[ApprovalAutoApplyArgs](reg, &approvalAutoApplyWorker{
		pool: pool, identity: identity.NewService(pool), log: log,
	})
	addAIActivitySweepJobs(reg, pool, log)
	addBriefGenerateJobs(reg, pool, log, briefMail)
}
