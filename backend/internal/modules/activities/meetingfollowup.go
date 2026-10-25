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
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// MeetingsAwaitingFollowUp answers which of the reader's meetings with a
// customer had nothing from the reader since. Each ended at least the
// follow-up window before asOf, inside the lookback. SentAt is when it started.
func (s *Store) MeetingsAwaitingFollowUp(ctx context.Context, asOf time.Time) ([]AwaitingReply, error) {
	return s.readMeetingFollowUps(ctx, asOf, func(func(any) int) string { return scopeUnbounded })
}

// MeetingsAwaitingFollowUpOnDeal is MeetingsAwaitingFollowUp narrowed to the
// meetings filed under one deal, so the deal page and Home name the same ones.
func (s *Store) MeetingsAwaitingFollowUpOnDeal(ctx context.Context, asOf time.Time, dealID ids.UUID) ([]AwaitingReply, error) {
	return s.readMeetingFollowUps(ctx, asOf, func(arg func(any) int) string {
		return fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_link on_deal
		    WHERE on_deal.activity_id = a.id AND on_deal.deal_id = $%d)`, arg(dealID))
	})
}

// MeetingsStillAwaitingFollowUp answers which of these meetings the reader
// still owes a follow-up, so a stored suggestion can be checked again.
func (s *Store) MeetingsStillAwaitingFollowUp(ctx context.Context, asOf time.Time, meetingIDs []ids.UUID) (map[ids.UUID]bool, error) {
	owed := make(map[ids.UUID]bool, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return owed, nil
	}
	rows, err := s.readMeetingFollowUps(ctx, asOf, func(arg func(any) int) string {
		return fmt.Sprintf(`a.id = ANY($%d)`, arg(meetingIDs))
	})
	for _, row := range rows {
		owed[row.ActivityID] = true
	}
	return owed, err
}

// readMeetingFollowUps runs the one follow-up query; narrow renders the extra
// predicate on alias a that limits which meetings it considers.
func (s *Store) readMeetingFollowUps(
	ctx context.Context, asOf time.Time, narrow func(arg func(any) int) string,
) ([]AwaitingReply, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []AwaitingReply
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		days, err := followUpAfterDays(ctx, tx)
		if err != nil {
			return err
		}
		out, err = s.queryMeetingFollowUps(ctx, tx, asOf, days, narrow)
		return err
	})
	return out, err
}

func (s *Store) queryMeetingFollowUps(
	ctx context.Context, tx pgx.Tx, asOf time.Time, windowDays int, narrow func(arg func(any) int) string,
) ([]AwaitingReply, error) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	instant := arg(asOf)
	g, err := followUpGatesFor(ctx, arg)
	if err != nil {
		return nil, err
	}
	addressable, err := addressableContactClause(ctx, arg)
	if err != nil {
		return nil, err
	}
	narrowed := narrow(arg)
	at := fmt.Sprintf("$%d", instant)
	rows, err := tx.Query(ctx, fmt.Sprintf(meetingFollowUpsSQL,
		instant, g.content, g.links, arg(windowDays), AwaitingReplyLookbackDays,
		g.later, relstrength.InteractionCountsSQL("later"), arg(readerOrNobody(ctx)),
		messageSnoozeLiftedSQL(at, g.back), g.customer, AwaitingReplyScanCap,
		meetingHeldSQL("a", at), addressable, maxActivityLinks, narrowed,
	), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading meetings the reader owes a follow-up: %w", err)
	}
	defer rows.Close()
	var out []AwaitingReply
	for rows.Next() {
		var r AwaitingReply
		if err := rows.Scan(&r.ActivityID, &r.Subject, &r.SentAt, &r.Recipients, &r.CompanyID, &r.DealID, &r.Precision); err != nil {
			return nil, fmt.Errorf("activities: reading a meeting the reader owes a follow-up: %w", err)
		}
		if len(r.Recipients) > 0 {
			r.ContactID = r.Recipients[0]
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// addressableContactClause is which contacts under alias rc this reader may
// write a follow-up to. It is the attach rule a send files under, so a
// read-only shared contact is never offered a draft the send would refuse.
func addressableContactClause(ctx context.Context, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "contact") {
		return scopeNothing, nil
	}
	clause, err := auth.AttachClauseFor(ctx, "contact", "rc", arg)
	if err != nil || clause == "" {
		return scopeUnbounded, err
	}
	return clause, nil
}

// meetingEndSQL is when the meeting under alias a ended.
var meetingEndSQL = occurredday.EndSQL("a")

// meetingFollowUpsSQL: every rule sits before the cap.
//
//  1. as of, 2. content gate on a, 3. link visibility on wl, 4. the follow-up
//     window in days, 5. the lookback in days past it, 6. content gate on later,
//  7. what counts as contact for later, 8. the reader, 9. a snooze lifted,
//  10. the customer arms the reader may read, 11. the cap, 12. the meeting
//     took place, 13. the contacts the reader may write to, 14. the recipient
//     cap, 15. which meetings the caller narrowed the read to.
//
// The attendees are read once, in the LATERAL, and both the recipient list and
// the "nothing sent since" test use that one set.
var meetingFollowUpsSQL = `
	SELECT a.id, COALESCE(a.subject, ''), a.occurred_at,
	       (attendees.recipients)[1:%[14]d],
	       COALESCE((array_agg(wl.company_id ORDER BY wl.company_id::text)
	                 FILTER (WHERE wl.company_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.deal_id ORDER BY wl.deal_id::text)
	                 FILTER (WHERE wl.deal_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       a.occurred_precision
	  FROM activity a
	  -- Who it was with: the contacts it is filed under first, then the ones
	  -- its attendee list resolved to. A merged contact counts as its survivor.
	  -- A participant row with a seat is a colleague, the reader included.
	  CROSS JOIN LATERAL (
	       SELECT COALESCE(array_agg(met.contact_id ORDER BY met.rank, met.contact_id)
	                         FILTER (WHERE met.addressable), '{}') AS recipients
	         FROM (SELECT rc.id AS contact_id, min(src.rank) AS rank, bool_or(%[13]s) AS addressable
	                 FROM (SELECT l.contact_id, 0 AS rank FROM activity_link l
	                        WHERE l.activity_id = a.id AND l.contact_id IS NOT NULL
	                       UNION ALL
	                       SELECT p.contact_id, 1 FROM activity_participant p
	                        WHERE p.activity_id = a.id AND p.contact_id IS NOT NULL AND p.user_id IS NULL
	                          AND p.role IN ('organizer', 'attendee', 'to', 'cc')) src
	                 JOIN contact named ON named.id = src.contact_id
	                 JOIN contact rc ON rc.id = COALESCE(named.merged_into_id, named.id)
	                WHERE rc.archived_at IS NULL
	                GROUP BY rc.id) met) attendees
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	 WHERE a.kind = 'meeting'
	   AND a.archived_at IS NULL
	   AND NOT ` + ImportedHistorySQL + `
	   AND %[12]s
	   AND %[15]s
	   -- Measured from when it ENDED: the window, the lookback and what came
	   -- after all start there.
	   AND ` + meetingEndSQL + ` <= $%[1]d::timestamptz - make_interval(days => $%[4]d::int)
	   AND ` + meetingEndSQL + ` >= $%[1]d::timestamptz - make_interval(days => $%[4]d::int + %[5]d)
	   AND %[2]s
	   -- The reader took part in it, by the one meeting-responsibility rule.
	   -- Importing a copy or being copied on its mail is not taking part.
	   AND ` + storekit.MeetingResponsibilitySQL("a", "$%[8]d") + `
	   -- Nothing from our side since to anyone it was with, filed under them
	   -- or addressed to them: a message, a call or a later meeting is a
	   -- follow-up already made. Only the recipients count, uncapped: a
	   -- contact the reader cannot see must not make a row vanish. Their own
	   -- mail after it is the waiting queue's business, not this reminder's.
	   AND NOT EXISTS (SELECT 1 FROM (SELECT heard.activity_id FROM activity_link heard
	                                   WHERE heard.contact_id = ANY(attendees.recipients)
	                                  UNION ALL
	                                  SELECT reached.activity_id FROM activity_participant reached
	                                   WHERE reached.contact_id = ANY(attendees.recipients)) touched
	                     JOIN activity later ON later.id = touched.activity_id
	                    WHERE later.id <> a.id
	                      AND later.archived_at IS NULL
	                      AND later.direction IS DISTINCT FROM 'inbound'
	                      AND %[6]s
	                      AND %[7]s
	                      AND later.occurred_at >= ` + meetingEndSQL + `
	                      AND later.occurred_at <= $%[1]d)
	   AND EXISTS (SELECT 1 FROM activity_link sales WHERE sales.activity_id = a.id AND %[10]s)
	   AND NOT ` + readerSetAsideSQL("$%[8]d", "%[9]s") + `
	 GROUP BY a.id, a.subject, a.occurred_at, a.occurred_precision, attendees.recipients
	 ORDER BY a.occurred_at DESC, a.id DESC
	 LIMIT %[11]d`

// meetingHeldSQL is a meeting somebody said took place: its outcome was saved
// as held, and its start has passed. A booked or unmarked past meeting owes
// an outcome, which the outcome lane asks for once; it owes no follow-up yet.
func meetingHeldSQL(alias, now string) string {
	return "(" + alias + ".meeting_status = 'held' AND " + alias + ".occurred_at <= " + now + ")"
}
