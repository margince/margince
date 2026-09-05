// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The worker groups that carry an engine or a store of their own.
//
// Split out of jobs.go, which holds the runner, its config and the registry
// assembly. Each group here is already its own function for the reason its own
// comment gives — it needs a dependency the rest of its registration group does
// not — and three of them together is a concept rather than a tail: what the
// registry needs BUILT before it can register a worker, as opposed to how it
// registers one.

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
)

// addBriefGenerateJobs registers the overnight Morning-Brief assembly. Its own
// function because the workspace worker needs the brief engine and the identity
// service, neither of which the group's other members carry.
//
// The engine is built WITHOUT WithL2Ranker, and that is the declared posture
// rather than an omission: this role holds no brief_ranking model lane, so the
// overnight run is the deterministic composite order. The model re-order stays
// what the api role adds on a rep's explicit refresh.
func addBriefGenerateJobs(reg *jobRegistry, pool *pgxpool.Pool, log *slog.Logger, mail BriefMailConfig) {
	addDeclaredWorker[BriefGenerateArgs](reg, &briefGenerateWorker{
		engine: briefs.NewBriefEngine(pool, people.NewStore(InstallationDB(pool))),
		pool:   pool,
		users:  identity.NewService(pool),
		now:    time.Now,
		log:    log,
		mail:   mail,
	})
}

// addAIActivitySweepJobs registers the AI-activity projection's two passes. It
// is its own function rather than four more lines above because both workers
// need a store the group's other members do not, and the compiler's closed args
// set forces one addDeclaredWorker per kind anyway.
func addAIActivitySweepJobs(reg *jobRegistry, pool *pgxpool.Pool, log *slog.Logger) {
	db := InstallationDB(pool)
	addDeclaredWorker[AIActivityReconcileArgs](reg, &aiActivityReconcileWorker{
		activities: activities.NewStore(db), identity: identity.NewService(pool),
		now: time.Now, log: log,
	})
	addDeclaredWorker[AIActivityRetentionArgs](reg, &aiActivityRetentionWorker{
		projection: aiactivity.NewStore(db), identity: identity.NewService(pool),
		now: time.Now, log: log,
	})
}
