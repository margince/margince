// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// currentCard serves the cached card while its fingerprint stands, and writes
// and caches a new one when it does not.
func (s *Service) currentCard(
	ctx context.Context, userID ids.UserID, dealID ids.DealID, f facts, policy cardReadPolicy,
) (crmcontracts.DealStatusCard, error) {
	mv := decideMove(f)
	in := project(f, mv)
	fingerprint, err := Fingerprint(in, userID.UUID, s.routingVersion, f.now, f.lang)
	if err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	verdict := s.decideFromCache(ctx, userID, dealID, fingerprint, policy.refresh, f.now)
	if verdict.serve {
		return verdict.card, nil
	}
	card, laneFailed := s.write(ctx, f, mv, in, verdict.askModel && !policy.factsOnly)
	if laneFailed {
		// The lane was wired and did not answer: a timeout, a budget, an
		// unparseable reply. The reader gets the floor, a working card. Stored
		// under the current fingerprint, it would make one outage permanent,
		// because a matching fingerprint is served without asking again.
		return card, nil
	}
	if err := s.save(ctx, userID, dealID, stored{
		Fingerprint: fingerprint,
		GeneratedAt: card.GeneratedAt,
		GeneratedBy: card.GeneratedBy,
		Card:        card,
	}); err != nil {
		return crmcontracts.DealStatusCard{}, err
	}
	return card, nil
}

// cacheVerdict says what to do with the stored card.
type cacheVerdict struct {
	// card is served as-is when serve is true: the facts have not moved.
	card  crmcontracts.DealStatusCard
	serve bool
	// askModel is false when the facts moved but a model call is inside the
	// floor. The card is still rewritten, deterministically, so the reader
	// sees the change rather than the world as it was.
	askModel bool
}

// decideFromCache reads the stored card and says whether it still stands.
func (s *Service) decideFromCache(
	ctx context.Context, userID ids.UserID, dealID ids.DealID, fingerprint string, refresh bool, now time.Time,
) cacheVerdict {
	if refresh {
		return cacheVerdict{askModel: true}
	}
	cached, found, err := s.cached(ctx, userID, dealID)
	if err != nil || !found {
		// A cache that cannot be read is a miss, never a failed request. The
		// card is derived content, and writing it again is the whole fallback.
		return cacheVerdict{askModel: true}
	}
	if cached.Fingerprint == fingerprint {
		return cacheVerdict{card: cached.Card, serve: true}
	}
	return cacheVerdict{askModel: now.Sub(cached.GeneratedAt) >= modelCallFloor}
}
