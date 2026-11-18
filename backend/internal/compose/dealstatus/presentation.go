// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"maps"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

func presentMove(ctx context.Context, f facts, card crmcontracts.DealStatusCard) crmcontracts.DealStatusCard {
	if card.Next == nil {
		return card
	}
	if systemtext.HasLocale(ctx) || f.say == "" {
		f.say = string(systemtext.Locale(ctx))
	}
	rendered := decideMove(f)
	next := *card.Next
	next.Reason = rendered.Reason
	if rendered.Arguments != nil {
		arguments := map[string]any{}
		if next.Arguments != nil {
			arguments = maps.Clone(*next.Arguments)
		}
		maps.Copy(arguments, *rendered.Arguments)
		next.Arguments = &arguments
	}
	card.Next = &next
	return card
}

func (s *Service) cardAndTimeline(
	ctx context.Context, dealID ids.DealID, policy cardReadPolicy,
) (crmcontracts.DealStatusCard, []crmcontracts.Activity, error) {
	userID, err := actingUser(ctx)
	if err != nil {
		return crmcontracts.DealStatusCard{}, nil, err
	}
	// Authorize fresh facts before consulting a cached card.
	f, err := s.gather(ctx, dealID)
	if err != nil {
		return crmcontracts.DealStatusCard{}, nil, err
	}
	canonical := f
	canonical.say = f.lang
	mv, err := assessMove(canonical, decideMove(canonical))
	if err != nil {
		return crmcontracts.DealStatusCard{}, nil, err
	}
	in := project(canonical, mv)
	fingerprint, err := Fingerprint(in, userID.UUID, s.routing(), f.now, f.loc, f.lang)
	if err != nil {
		return crmcontracts.DealStatusCard{}, nil, err
	}
	verdict := s.decideFromCache(ctx, userID, dealID, fingerprint, policy.refresh, f.now)
	if verdict.serve {
		card := verdict.card
		card.MailCoverage = f.mailCoverage
		if current, ok := unansweredInbound(f); ok {
			card.RequestBasis = requestBasis(f, current, basisCurrentRequest)
		}
		if card.Next != nil {
			next := *card.Next
			next.Basis = mv.Basis
			next.MailCoverage = f.mailCoverage
			next.Recommendation, next.Arguments = mv.Recommendation, mv.Arguments
			card.Next = &next
		}
		return presentMove(ctx, f, card), factActivities(f), nil
	}
	card, laneFailed := s.write(ctx, f, mv, in, verdict.askModel && !policy.factsOnly && mayWriteTheCard(ctx))
	if !mayWriteTheCard(ctx) {
		return presentMove(ctx, f, card), factActivities(f), nil
	}
	if laneFailed {
		// Caching a fallback would preserve a transient model failure
		// until the facts change.
		return presentMove(ctx, f, card), factActivities(f), nil
	}
	if err := s.save(ctx, userID, dealID, stored{
		Access:      accessForCard(f, in, mv),
		Fingerprint: fingerprint,
		GeneratedAt: card.GeneratedAt,
		GeneratedBy: card.GeneratedBy,
		Card:        card,
	}); err != nil {
		return crmcontracts.DealStatusCard{}, nil, err
	}
	return presentMove(ctx, f, card), factActivities(f), nil
}
