// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Margince reopening a lost deal, as its owner's agent, when Deal Scout finds
// new buying evidence at the deal's company. An owner who left or switched
// deal_revive off keeps the suggestion for a human to decide.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reviveOne reopens one lost deal from one suggestion as the deal owner's
// agent. An owner who left answers not found, and the suggestion stays.
func (o scoutDealOpener) reviveOne(ctx context.Context, suggestionID ids.UUID, deal ids.DealID, owner ids.UUID) (bool, error) {
	ownerCtx, err := ownersAgent(ctx, o.users, owner, marginceActorID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	suggestion, err := o.deals.GetSuggestion(ownerCtx, suggestionID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	basis := scoutBasis(suggestion)
	return o.deals.ReviveLostDeal(ownerCtx, deals.ReviveLostDealInput{
		SuggestionID: suggestionID, DealID: deal, Owner: owner, Basis: basis,
		EvidenceHolds: scoutEvidenceHolds, Arrival: scoutEvidenceArrival,
		SwitchOn: func(ctx context.Context, tx pgx.Tx) (bool, error) {
			mode, err := approvals.AutoApplyModeTx(ctx, tx, owner, deals.DealReviveKind)
			return mode == approvals.ModeAuto, err
		},
		Record:  o.recordRevivalOnTimeline,
		AsOwner: ownersAgentTx(o.users, owner, marginceActorID),
	})
}

// scoutEvidenceArrival answers when the cited evidence reached Margince: the
// server time each meeting, document, signal and cited message was stored.
// A date the evidence carries can be set before it arrived, so it is not used.
func scoutEvidenceArrival(ctx context.Context, tx pgx.Tx, evidence []deals.SuggestionEvidence) (deals.EvidenceArrival, bool, error) {
	var meetings, attachments, signals []ids.UUID
	for _, item := range evidence {
		switch {
		case item.ActivityID != nil:
			meetings = append(meetings, *item.ActivityID)
		case item.AttachmentID != nil:
			attachments = append(attachments, *item.AttachmentID)
		case item.SignalID != nil:
			signals = append(signals, *item.SignalID)
		}
	}
	var earliest, latest *time.Time
	err := tx.QueryRow(ctx, `
		SELECT min(arrived.at), max(arrived.at) FROM (
		  SELECT a.created_at AS at FROM activity a WHERE a.id = ANY($1) AND a.archived_at IS NULL
		  UNION ALL
		  SELECT at.created_at FROM attachment at WHERE at.id = ANY($2)
		  UNION ALL
		  SELECT a.created_at FROM attachment at JOIN activity a ON a.id = at.activity_id
		   WHERE at.id = ANY($2) AND a.archived_at IS NULL
		  UNION ALL
		  SELECT s.created_at FROM signal s WHERE s.id = ANY($3)
		  UNION ALL
		  SELECT ca.created_at FROM signal s CROSS JOIN jsonb_array_elements(s.evidence) cite
		    JOIN activity ca ON ca.id = (`+storekit.CitedActivityID("cite")+`)::uuid
		   WHERE s.id = ANY($3) AND cite->>'source_type' = 'activity' AND ca.archived_at IS NULL) arrived`,
		meetings, attachments, signals).Scan(&earliest, &latest)
	if err != nil {
		return deals.EvidenceArrival{}, false, fmt.Errorf("compose: reading when the evidence arrived: %w", err)
	}
	if earliest == nil || latest == nil {
		return deals.EvidenceArrival{}, false, nil
	}
	return deals.EvidenceArrival{Earliest: *earliest, Latest: *latest}, true, nil
}

// recordRevivalOnTimeline writes the reopen on the deal's timeline as a note.
// Its origin keeps it out of every recency clock. The evidence is named only
// on the owner's receipt, so the note does not cite it.
func (o scoutDealOpener) recordRevivalOnTimeline(ctx context.Context, tx pgx.Tx, deal ids.DealID, name string) error {
	subject := "Margince reopened " + name
	body := "New buying evidence arrived at the company after this deal was lost. Undo it from Home if the deal is still lost."
	_, _, err := o.activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote), Subject: &subject, Body: &body, Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{{EntityType: entityTypeDeal, EntityID: deal.UUID}},
	})
	if err != nil {
		return fmt.Errorf("compose: writing the reopen on the deal's timeline: %w", err)
	}
	return nil
}
