// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// addDatabaseOnlySweepJobs registers the periodic passes that need nothing but
// the database: the deals close-date hygiene, the follow-up reconcile, the
// automation clock scan, and the idempotency-key retention sweep.
//
// It takes no JobRunnerConfig, and that is the group rather than an economy of
// arguments — there is no lane, credential or registry any of these could be
// gated on, so every role that runs jobs runs all four. Each is a dispatcher
// plus a workspace worker: only the dispatcher is ticked (the schedules in
// wireJobs), and the workspace worker is enqueued by it, never scheduled.
func addDatabaseOnlySweepJobs(reg *jobRegistry, pool *pgxpool.Pool, log *slog.Logger) {
	addDeclaredWorker[CloseDateSweepArgs](reg, &closeDateSweepWorker{pool: pool})
	addDeclaredWorker[CloseDateWorkspaceArgs](reg, &closeDateWorkspaceWorker{corrector: NewCloseDateCorrector(pool, log)})
	addDeclaredWorker[FollowUpReconcileArgs](reg, &followUpReconcileWorker{pool: pool})
	addDeclaredWorker[FollowUpWorkspaceArgs](reg, &followUpWorkspaceWorker{reconciler: NewFollowUpReconciler(pool, log)})
	addDeclaredWorker[TimeScanArgs](reg, &timeScanWorker{pool: pool})
	addDeclaredWorker[TimeScanWorkspaceArgs](reg, &timeScanWorkspaceWorker{pool: pool, log: log})
	addDeclaredWorker[IdempotencyRetentionArgs](reg, &idempotencyRetentionWorker{pool: pool})
	addDeclaredWorker[IdempotencyRetentionWorkspaceArgs](reg, &idempotencyRetentionWorkspaceWorker{sweeper: NewIdempotencyRetentionSweeper(pool, log)})
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
	addBriefGenerateJobs(reg, pool, log)
}
