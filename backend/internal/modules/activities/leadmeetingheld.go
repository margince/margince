// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
)

// askLeadsToQualifyTx files the qualify task on every lead a held meeting is
// filed on, in the transaction that recorded it held. The writer keeps one
// open task per lead, so a second held meeting files no second task.
func (s *Store) askLeadsToQualifyTx(ctx context.Context, tx pgx.Tx, meeting crmcontracts.Activity) error {
	if s.leadQualifyTasks == nil || meeting.Kind != crmcontracts.ActivityKindMeeting ||
		meeting.MeetingStatus == nil || !meetingoutcome.IsHeld(string(*meeting.MeetingStatus)) {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT lead_id FROM activity_link WHERE activity_id = $1 AND lead_id IS NOT NULL`, meeting.Id)
	if err != nil {
		return fmt.Errorf("activities: reading the leads a held meeting is filed on: %w", err)
	}
	leads, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: reading the leads a held meeting is filed on: %w", err)
	}
	for _, lead := range leads {
		if err := s.leadQualifyTasks.FileLeadMeetingHeldTx(ctx, tx, ids.From[ids.LeadKind](lead), ids.UUID(meeting.Id)); err != nil {
			return err
		}
	}
	return nil
}
