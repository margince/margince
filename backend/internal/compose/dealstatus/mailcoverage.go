// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/mailcoverage"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithMailCoverage keeps connection observations outside the record reads.
func (s *Service) WithMailCoverage(read mailcoverage.Reader) *Service {
	s.mailCoverage = read
	return s
}

func (s *Service) gatherMailCoverage(ctx context.Context, f *facts) {
	var from *time.Time
	if source, _, ok := requestForMove(*f); ok {
		if source.OccurredPrecision == crmcontracts.OccurredPrecisionOccurredPrecisionInstant {
			from = &source.OccurredAt
		}
	} else if source, ok := lastContact(*f); ok {
		if source.OccurredPrecision == crmcontracts.OccurredPrecisionOccurredPrecisionInstant {
			from = &source.OccurredAt
		}
	}
	coverage := mailcoverage.Read(ctx, s.mailCoverage, from, f.now)
	f.mailCoverage = &coverage
}

func (s *Service) refreshMoveCoverage(ctx context.Context, moves map[ids.UUID]crmcontracts.DealStatusCardMove) {
	if len(moves) == 0 {
		return
	}
	coverage := mailcoverage.Read(ctx, s.mailCoverage, nil, s.now())
	for id, move := range moves {
		fresh := coverage
		if move.MailCoverage != nil {
			fresh.IntervalFrom = move.MailCoverage.IntervalFrom
		}
		if move.Basis != nil {
			basis := *move.Basis
			fresh.IntervalFrom = nil
			if basis.SourcePrecision == crmcontracts.OccurredPrecisionOccurredPrecisionInstant {
				fresh.IntervalFrom = &basis.SourceAt
			}
			basis.MailCoverage = &fresh
			move.Basis = &basis
		}
		move.MailCoverage = &fresh
		moves[id] = move
	}
}
