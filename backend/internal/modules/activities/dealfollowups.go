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
	// Before any SQL, so a refused caller costs no read; the builder checks
	// again because it is where the deal table is read.
	if err := requireDealFollowUpRead(ctx); err != nil {
		return nil, err
	}
	days, err := followUpAfterDays(ctx, tx)
	if err != nil {
		return nil, err
	}
	query, args, err := meetingFollowUpsByDealStatement(ctx, asOf, days, dealIDs)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, query, args...)
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

// meetingFollowUpsByDealStatement selects each readable deal's newest meeting follow-up.
func meetingFollowUpsByDealStatement(ctx context.Context, asOf time.Time, days int, dealIDs []ids.UUID) (string, []any, error) {
	if err := requireDealFollowUpRead(ctx); err != nil {
		return "", nil, err
	}
	var dealScope string
	query, args, err := unpagedMeetingFollowUpsStatement(ctx, asOf, days, func(arg func(any) int) (string, error) {
		var err error
		dealScope, err = readableDealBatchSQL(ctx, dealIDs, arg)
		return `cardinality(attendees.recipients) > 0 AND EXISTS (SELECT 1 FROM activity_link on_deal
   JOIN deal d ON d.id = on_deal.deal_id WHERE on_deal.activity_id = a.id AND ` + dealScope + `)`, err
	})
	if err != nil {
		return "", nil, err
	}
	return `SELECT DISTINCT ON (d.id) owed.id, owed.subject, owed.occurred_at, owed.recipients,
  owed.company_id, d.id, owed.precision FROM (` + query + `)
  owed(id, subject, occurred_at, recipients, company_id, deal_id, precision)
  JOIN activity_link on_deal ON on_deal.activity_id = owed.id
  JOIN deal d ON d.id = on_deal.deal_id
  WHERE ` + dealScope + ` ORDER BY d.id, owed.occurred_at DESC, owed.id DESC`, args, nil
}

func requireDealFollowUpRead(ctx context.Context) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	return auth.Require(ctx, "deal", principal.ActionRead)
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
