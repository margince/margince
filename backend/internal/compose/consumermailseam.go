// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The consumer-mail question, asked once for both doors.
//
// `capture` owns the workspace's administered overlay — the operator's own
// "this host IS a mailbox provider" additions and the one B2B host the shipped
// baseline gets wrong. The agent tool `qualify_lead` asks the same question
// from the other end of the same capture path, and a module may not import a
// sibling, so the edge is injected here, exactly as `newCounterpartyStore`
// injects `capture.MatcherTx` into people's ensure ladder.
//
// It is the second half of one rule. `newCounterpartyStore` above makes the WEB
// door read the operator's list; without this, the AGENT door still read a
// fifteen-entry map compiled into the binary, and an operator who marked a host
// consumer would find one door honouring it and the other creating a company
// from it.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/modules/capture"
	"github.com/gradionhq/margince/backend/internal/platform/database"
)

// consumerMailSeam answers agents.ConsumerMail over capture's per-workspace
// matcher.
type consumerMailSeam struct {
	db *database.DB
}

func newConsumerMailSeam(pool *pgxpool.Pool) consumerMailSeam {
	return consumerMailSeam{db: InstallationDB(pool)}
}

// IsConsumer reads the overlay INSIDE the transaction that answers, rather than
// building the matcher once at boot: the table is administered while the server
// runs, and a matcher cached at startup would keep deriving companies from a
// host an operator marked consumer that morning.
func (s consumerMailSeam) IsConsumer(ctx context.Context, domain string) (bool, error) {
	consumer := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		matcher, err := capture.MatcherTx(ctx, tx)
		if err != nil {
			return err
		}
		consumer = matcher.IsConsumer(domain)
		return nil
	})
	if err != nil {
		return false, err
	}
	return consumer, nil
}
