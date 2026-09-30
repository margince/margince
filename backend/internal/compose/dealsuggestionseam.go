// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealSuggestionEffects are an accepted suggestion's writes in the two
// modules deals may not import. Both run on the accepting rep's authority and
// inside the acceptance's own transaction.
type dealSuggestionEffects struct{}

// LinkActivityToDeal files the message under the new deal beside wherever it
// already is: the relink's own checks decide whether the rep may move it.
func (dealSuggestionEffects) LinkActivityToDeal(ctx context.Context, tx pgx.Tx, activityID, dealID ids.UUID) error {
	_, err := activities.RelinkActivityInTx(ctx, tx, ids.From[ids.ActivityKind](activityID),
		activities.RelinkActivityInput{EntityType: string(crmcontracts.ActivityLinkEntityTypeDeal), EntityID: dealID})
	return err
}

// AcknowledgeSignal settles a signal that raised the suggestion.
func (dealSuggestionEffects) AcknowledgeSignal(ctx context.Context, tx pgx.Tx, signalID ids.UUID) (bool, error) {
	return signals.AcknowledgeTx(ctx, tx, signalID, signals.ResolutionSourceDealScout)
}
