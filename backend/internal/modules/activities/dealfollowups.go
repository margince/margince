// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NextMeetingFollowUpsByDealTx selects the newest actionable follow-up for each deal.
// The feed's global page bound cannot hide another deal's first matching meeting.
func (s *Store) NextMeetingFollowUpsByDealTx(ctx context.Context, tx pgx.Tx, asOf time.Time, dealIDs []ids.UUID) (map[ids.UUID][]AwaitingReply, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	days, err := followUpAfterDays(ctx, tx)
	if err != nil {
		return nil, err
	}
	var dealScope string
	query, args, err := meetingFollowUpsStatement(ctx, asOf, days, func(arg func(any) int) (string, error) {
		var err error
		dealScope, err = readableDealBatchSQL(ctx, dealIDs, arg)
		return `cardinality(attendees.recipients) > 0 AND EXISTS (SELECT 1 FROM activity_link on_deal
   WHERE on_deal.activity_id = a.id AND on_deal.deal_id = d.id)`, err
	}, 1)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT owed.id, owed.subject, owed.occurred_at, owed.recipients,
  owed.company_id, d.id, owed.precision FROM deal d CROSS JOIN LATERAL (`+query+`)
  owed(id, subject, occurred_at, recipients, company_id, deal_id, precision)
  WHERE `+dealScope+` ORDER BY d.id`, args...)
	if err != nil {
		return nil, err
	}
	meetings, err := collectMeetingFollowUps(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID][]AwaitingReply, len(meetings))
	for _, meeting := range meetings {
		out[meeting.DealID] = append(out[meeting.DealID], meeting)
	}
	return out, nil
}

// NextMeetingFollowUpOnDeal shares the batch selector so page bounds cannot change the answer.
func (s *Store) NextMeetingFollowUpOnDeal(ctx context.Context, asOf time.Time, dealID ids.UUID) ([]AwaitingReply, error) {
	var out []AwaitingReply
	err := s.tx(ctx, func(tx pgx.Tx) error {
		byDeal, err := s.NextMeetingFollowUpsByDealTx(ctx, tx, asOf, []ids.UUID{dealID})
		out = byDeal[dealID]
		return err
	})
	return out, err
}
