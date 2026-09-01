// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"

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
// It also pulls in the AI-activity and brief-generate groups, which are their
// own functions because they are longer, not because they are gated differently.
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
