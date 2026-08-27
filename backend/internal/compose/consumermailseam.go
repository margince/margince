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

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

// consumerMailSeam answers agents.ConsumerMail over capture's per-workspace
// matcher.
type consumerMailSeam struct {
	db *database.DB
}

// newConsumerMailSeam takes the handle the registry is ALREADY bound to, not a
// pool to re-derive one from. `NewRegistryFor` exists precisely because a
// harness can name a workspace that is not the installation's singleton, and a
// seam that called InstallationDB itself would read that workspace's operator
// list at every other door and the installation's here — one credential, two
// answers about the same domain.
func newConsumerMailSeam(db *database.DB) consumerMailSeam {
	return consumerMailSeam{db: db}
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
