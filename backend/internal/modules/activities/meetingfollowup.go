// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The other follow-up reminder: a meeting the reader held with a customer,
// with nothing sent to them once the follow-up window has passed. It shares
// the window, the lookback and the customer test with awaitingreply.go.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// MeetingsAwaitingFollowUp answers which of the reader's meetings with a
// customer had nothing from the reader since. Each ended at least the
// follow-up window before asOf, inside the lookback. SentAt is when it started.
func (s *Store) MeetingsAwaitingFollowUp(ctx context.Context, asOf time.Time) ([]AwaitingReply, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []AwaitingReply
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		days, err := followUpAfterDays(ctx, tx)
		if err != nil {
			return err
		}
		out, err = s.queryMeetingFollowUps(ctx, tx, asOf, days)
		return err
	})
	return out, err
}

func (s *Store) queryMeetingFollowUps(
	ctx context.Context, tx pgx.Tx, asOf time.Time, windowDays int,
) ([]AwaitingReply, error) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	instant := arg(asOf)
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	laterContent, err := auth.ActivityContentClause(ctx, "later", arg)
	if err != nil {
		return nil, err
	}
	backContent, err := auth.ActivityContentClause(ctx, "back", arg)
	if err != nil {
		return nil, err
	}
	linkVisible, err := auth.LinkTargetVisibleClause(ctx, "wl", arg)
	if err != nil {
		return nil, err
	}
	if linkVisible == "" {
		linkVisible = scopeUnbounded
	}
	customer, err := customerArms(ctx, liveRecord(openDealPredicate, "d"), liveRecord(workingLeadPredicate, "ld"))
	if err != nil {
		return nil, err
	}
	at := fmt.Sprintf("$%d", instant)
	rows, err := tx.Query(ctx, fmt.Sprintf(meetingFollowUpsSQL,
		instant, content, linkVisible, arg(windowDays), AwaitingReplyLookbackDays,
		laterContent, relstrength.InteractionCountsSQL("later"), arg(readerOrNobody(ctx)),
		messageSnoozeLiftedSQL(at, backContent), customer, AwaitingReplyScanCap,
		relstrength.MeetingTookPlaceSQL("a", at),
	), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading meetings the reader owes a follow-up: %w", err)
	}
	defer rows.Close()
	var out []AwaitingReply
	for rows.Next() {
		var r AwaitingReply
		if err := rows.Scan(&r.ActivityID, &r.Subject, &r.SentAt, &r.ContactID, &r.CompanyID, &r.DealID); err != nil {
			return nil, fmt.Errorf("activities: reading a meeting the reader owes a follow-up: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// meetingEndSQL is when the meeting under alias a ended.
const meetingEndSQL = `(a.occurred_at + make_interval(secs => coalesce(a.duration_seconds, 0)))`

// meetingFollowUpsSQL: every rule sits before the cap.
//
//  1. as of, 2. content gate on a, 3. link visibility on wl, 4. the follow-up
//     window in days, 5. the lookback in days past it, 6. content gate on later,
//  7. what counts as contact for later, 8. the reader, 9. a snooze lifted,
//  10. the customer arms the reader may read, 11. the cap, 12. the meeting
//     took place.
var meetingFollowUpsSQL = `
	SELECT a.id, COALESCE(a.subject, ''), a.occurred_at,
	       COALESCE((array_agg(wl.contact_id ORDER BY wl.contact_id::text)
	                 FILTER (WHERE wl.contact_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.company_id ORDER BY wl.company_id::text)
	                 FILTER (WHERE wl.company_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.deal_id ORDER BY wl.deal_id::text)
	                 FILTER (WHERE wl.deal_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid)
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	 WHERE a.kind = 'meeting'
	   AND a.archived_at IS NULL
	   AND %[12]s
	   -- Measured from when it ENDED: the window, the lookback and what came
	   -- after all start there.
	   AND ` + meetingEndSQL + ` <= $%[1]d::timestamptz - make_interval(days => $%[4]d::int)
	   AND ` + meetingEndSQL + ` >= $%[1]d::timestamptz - make_interval(days => $%[4]d::int + %[5]d)
	   AND %[2]s
	   -- The reader was in it: they held it, their calendar imported it, or
	   -- they are one of its attendees. The last two are auth's
	   -- activityMembershipArm; change them there and here.
	   AND (a.host_user_id = $%[8]d
	        OR EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = $%[8]d)
	        OR EXISTS (SELECT 1 FROM activity_participant me
	                    WHERE me.activity_id = a.id AND me.user_id = $%[8]d))
	   -- Nothing from our side since to anyone it was with: a message, a call
	   -- or a later meeting is a follow-up already made. Their own mail after
	   -- it is the waiting queue's business, not this reminder's.
	   AND NOT EXISTS (SELECT 1 FROM activity_link met
	                     JOIN activity_link heard ON heard.contact_id = met.contact_id
	                     JOIN activity later ON later.id = heard.activity_id
	                    WHERE met.activity_id = a.id AND met.contact_id IS NOT NULL
	                      AND later.id <> a.id
	                      AND later.archived_at IS NULL
	                      AND later.direction IS DISTINCT FROM 'inbound'
	                      AND %[6]s
	                      AND %[7]s
	                      AND later.occurred_at >= ` + meetingEndSQL + `
	                      AND later.occurred_at <= $%[1]d)
	   AND EXISTS (SELECT 1 FROM activity_link sales WHERE sales.activity_id = a.id AND %[10]s)
	   AND NOT EXISTS (SELECT 1 FROM activity_reader_state mine
	                    WHERE mine.activity_id = a.id AND mine.reader_id = $%[8]d
	                      AND (mine.state = 'not_mine' OR (mine.state = 'snoozed' AND NOT %[9]s)))
	 GROUP BY a.id, a.subject, a.occurred_at
	 ORDER BY a.occurred_at DESC, a.id DESC
	 LIMIT %[11]d`
