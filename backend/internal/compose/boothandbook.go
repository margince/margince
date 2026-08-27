// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The boot step that files this release's operator handbook into its corpus.
//
// The handbook belongs to the RELEASE, not to the installation: the binary
// carries the pages, so an upgrade must move the corpus with it. Running it on
// every start rather than once at install is the whole design — a corpus seeded
// at install and never touched again would answer questions about a version
// that stopped running months ago, and answer them WITH A CITATION, which is a
// claim that the quoted text is what the handbook says.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// handbookLedgerFact names this step's advisory lock. Several api replicas boot
// at once during a rollout and every one of them reconciles; without it each
// transaction reads the same absent corpus and every one tries to create it,
// and all but one fail on the partial unique index — turning a rollout into a
// boot failure for no reason.
const handbookLedgerFact = "handbook-corpus"

// ReconcileHandbookCorpus files the pages this binary carries into the shipped
// handbook corpus, creating the corpus the first time.
//
// Pre-bootstrap there is no workspace to write against, so there is nothing to
// reconcile and that is NOT an error — the same "not yet" every other boot fact
// takes. The first boot after bootstrap files the handbook.
func ReconcileHandbookCorpus(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, inserter *jobs.Runner) error {
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:handbook-corpus")
	if err != nil {
		return err
	}
	if !bootstrapped {
		log.Info("handbook corpus not filed: installation not bootstrapped yet")
		return nil
	}

	store := knowledge.NewStore(InstallationDB(pool))
	queue := knowledgeIngestEnqueue(inserter)

	// Taken INSIDE the reconciliation's own transaction. pg_advisory_xact_lock
	// releases at commit, so a lock taken in a transaction of its own here
	// would be gone before the write it is meant to serialize even starts —
	// a guard reporting success while holding nothing.
	serialize := func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, bootLedgerLock, handbookLedgerFact); err != nil {
			return fmt.Errorf("compose: serializing the handbook reconciliation: %w", err)
		}
		return nil
	}

	written, err := store.ReconcileHandbook(ctx, serialize, queue)
	if err != nil {
		return fmt.Errorf("compose: filing this release's handbook: %w", err)
	}
	// Logged only when something moved. Every boot runs this and the ordinary
	// answer is zero; a line per replica per restart saying "nothing changed"
	// is what makes an operator stop reading boot logs.
	if written > 0 {
		log.Info("handbook corpus reconciled with this release", "pages_written", written)
	}
	return nil
}
