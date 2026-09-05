// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The weekly retrospective's registration.
//
// Its own file for the same reason jobs_signals.go is: the workspace worker
// needs the weekly engine, the identity service and an optional narrator, none
// of which the neighbouring registrars carry — and jobs.go crossed its length
// ceiling the moment this grew a lane.

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
)

// addWeeklyReviewJobs registers the weekly retrospective's pass. Its own
// function for the same reason the brief's is: the workspace worker needs the
// weekly engine and the identity service, neither of which the group's other
// members carry.
func addWeeklyReviewJobs(reg *jobRegistry, pool *pgxpool.Pool, log *slog.Logger, narrator completer, mail WeeklyMailConfig) {
	addDeclaredWorker[WeeklyReviewGenerateArgs](reg, &weeklyGenerateWorker{
		// The job re-reads a snapshot it has just written when a later tick
		// finds one already there, and that read takes the same team gate a
		// lead's does — it runs under a MEMBER's own authority, not the system
		// principal, so it must be able to answer the membership question.
		engine:   weekly.NewEngine(pool, newTeammatesSeam(pool)),
		pool:     pool,
		users:    identity.NewService(pool),
		now:      time.Now,
		log:      log,
		narrator: narrator,
		mail:     mail,
	})
}
