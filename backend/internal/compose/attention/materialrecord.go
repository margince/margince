// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MaterialAtRisk answers which deals the queue calls material and at risk now,
// for the pass that writes the verdict down.
//
// The same lane read, the same pricing and the same bar the queue ranks by
// (materialOf), so the recorded verdict is the one a reader was shown rather
// than a second opinion of it. The bar is taken over the deals THIS caller can
// see; the pass runs as the system principal, which sees the whole pipeline —
// the same pipeline every human seat reads, since no row scope narrows a deal
// read.
//
// An installation with no at-risk lane bound has nothing to judge.
func (s *Service) MaterialAtRisk(ctx context.Context) ([]ids.UUID, error) {
	if s.atRisk == nil {
		return nil, nil
	}
	risky, _, err := s.atRisk.Quiet(ctx)
	if err != nil {
		return nil, err
	}
	items := renderEach(risky, riskItem)
	day := crmcontracts.Attention{AsOf: s.now(), AtRisk: &items}
	money, err := s.priceTheDay(ctx, day)
	if err != nil {
		return nil, err
	}
	bar := materialBarOf(day, money)
	var material []ids.UUID
	for i, item := range items {
		if _, _, clears := materialOf(item, bar, money); clears {
			material = append(material, risky[i].DealID)
		}
	}
	return material, nil
}
