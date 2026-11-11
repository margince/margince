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
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// MeetingsAwaitingFollowUp answers which of the reader's meetings with a
// customer had nothing from the reader since. Each ended at least the
// follow-up window before asOf, inside the lookback. SentAt is when it started.
func (s *Store) MeetingsAwaitingFollowUp(ctx context.Context, asOf time.Time) ([]AwaitingReply, error) {
	return s.readMeetingFollowUps(ctx, asOf, func(func(any) int) (string, error) { return scopeUnbounded, nil })
}

// MeetingsStillAwaitingFollowUp answers which of these meetings the reader
// still owes a follow-up, so a stored suggestion can be checked again.
func (s *Store) MeetingsStillAwaitingFollowUp(ctx context.Context, asOf time.Time, meetingIDs []ids.UUID) (map[ids.UUID]bool, error) {
	owed := make(map[ids.UUID]bool, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return owed, nil
	}
	rows, err := s.readMeetingFollowUps(ctx, asOf, func(arg func(any) int) (string, error) {
		return fmt.Sprintf(`a.id = ANY($%d)`, arg(meetingIDs)), nil
	})
	for _, row := range rows {
		owed[row.ActivityID] = true
	}
	return owed, err
}

// readMeetingFollowUps runs the one follow-up query; narrow renders the extra
// predicate on alias a that limits which meetings it considers.
func (s *Store) readMeetingFollowUps(
	ctx context.Context, asOf time.Time, narrow func(arg func(any) int) (string, error),
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
	ctx context.Context, tx pgx.Tx, asOf time.Time, windowDays int, narrow func(arg func(any) int) (string, error),
) ([]AwaitingReply, error) {
	query, args, err := meetingFollowUpsStatement(ctx, asOf, windowDays, narrow, AwaitingReplyScanCap)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading meetings the reader owes a follow-up: %w", err)
	}
	return collectMeetingFollowUps(rows)
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

// followUpFromSQL is where a follow-up to the meeting under alias a may start:
// its end for an hour, its date's start for a day. A later touch is placed by
// its own date too (occurredday.FromSQL).
var followUpFromSQL = `(CASE WHEN a.occurred_precision = 'day' THEN ` + occurredday.StartSQL("a") +
	` ELSE ` + meetingEndSQL + ` END)`

// The recipient set is materialized before link fan-out, so each meeting is
// resolved once. Every refusal and scope filter sits before the page cap.
var meetingFollowUpsSQL = `
	SELECT a.id, COALESCE(a.subject, ''), a.occurred_at,
	       (recipient_set.recipients)[1:%[13]d],
	       COALESCE((array_agg(wl.company_id ORDER BY wl.company_id::text)
	                 FILTER (WHERE wl.company_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.deal_id ORDER BY wl.deal_id::text)
	                 FILTER (WHERE wl.deal_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       a.occurred_precision
	  FROM activity a
	  JOIN ` + meetingAttendeeSet + ` recipient_set ON recipient_set.activity_id = a.id
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	 WHERE a.kind = 'meeting'
	   AND a.archived_at IS NULL
	   AND NOT ` + ImportedHistorySQL + `
	   AND %[12]s
	   AND %[14]s
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
	   -- The recipient set must not be scanned once per later activity, and a
	   -- later activity is read by its key alone. The LIMIT keeps the tests
	   -- below out of that read, so no planner can walk everything logged
	   -- since the meeting instead, whatever its statistics say. The columns
	   -- are the ones those tests read; a test that needs another fails loudly.
	   AND NOT EXISTS (WITH touched AS MATERIALIZED (SELECT heard.activity_id FROM activity_link heard
	                                   WHERE heard.contact_id = ANY(recipient_set.recipients)
	                                  UNION ALL
	                                  SELECT reached.activity_id FROM activity_participant reached
	                                   WHERE reached.contact_id = ANY(recipient_set.recipients))
	                   SELECT 1 FROM touched CROSS JOIN LATERAL ` + laterActivityFromLinkSQL + ` later
	                    WHERE later.id <> a.id
	                      AND later.archived_at IS NULL
	                      AND later.direction IS DISTINCT FROM 'inbound'
	                      AND %[6]s
	                      AND %[7]s
	                      AND %[15]s
	                      AND ` + occurredday.BegunSQL("later", "$%[1]d::timestamptz") + `)
	   AND EXISTS (SELECT 1 FROM activity_link sales WHERE sales.activity_id = a.id AND %[10]s)
	   AND NOT ` + readerSetAsideSQL("$%[8]d", "%[9]s") + `
	 GROUP BY a.id, a.subject, a.occurred_at, a.occurred_precision, recipient_set.recipients
	 ORDER BY a.occurred_at DESC, a.id DESC
	 %[11]s`

const meetingAttendeeSet = "follow_up_meeting_attendees"

// Link and deal joins must not repeat a meeting's recipient resolution when
// the planner changes its join order. The set keys on the candidate's id and
// never joins activity, which stale statistics can turn into a scan per meeting.
func meetingAttendeesSQL(addressable string) string {
	return meetingAttendeeSet + " AS MATERIALIZED (" + `
 SELECT candidate.id AS activity_id, attendees.recipients
 FROM ` + followUpCandidates + ` candidate
` + fmt.Sprintf(`	  CROSS JOIN LATERAL (
	       SELECT COALESCE(array_agg(met.contact_id ORDER BY met.rank, met.contact_id)
	                         FILTER (WHERE met.addressable), '{}') AS recipients
	         FROM (SELECT rc.id AS contact_id, min(src.rank) AS rank, bool_or(%s) AS addressable
	                 FROM (SELECT l.contact_id, 0 AS rank FROM activity_link l
	                        WHERE l.activity_id = candidate.id AND l.contact_id IS NOT NULL
	                       UNION ALL
	                       SELECT p.contact_id, 1 FROM activity_participant p
	                        WHERE p.activity_id = candidate.id AND p.contact_id IS NOT NULL AND p.user_id IS NULL
	                          AND p.role IN ('organizer', 'attendee', 'to', 'cc')) src
	                 JOIN contact named ON named.id = src.contact_id
	                 JOIN contact rc ON rc.id = COALESCE(named.merged_into_id, named.id)
	                WHERE rc.archived_at IS NULL
	                GROUP BY rc.id) met) attendees`, addressable) + ")"
}

// laterActivityColumns are the columns the "nothing sent since" tests read from
// a later activity.
const laterActivityColumns = `keyed.id, keyed.kind, keyed.direction, keyed.origin, keyed.occurred_at,
	keyed.occurred_precision, keyed.archived_at, keyed.restricted_at, keyed.meeting_status, keyed.call_outcome,
	keyed.calendar_method, keyed.sender_is_machine, keyed.bulk_mail_attested, keyed.internal_mail_authorization`

// The limit keeps the keyed lookup ahead of predicates on the later activity,
// even when stale statistics favor scanning the whole activity table.
const laterActivityFromLinkSQL = `(SELECT ` + laterActivityColumns + ` FROM activity keyed
 WHERE keyed.id = touched.activity_id LIMIT 1)`

// meetingHeldSQL is a meeting somebody said took place: its outcome was saved
// as held, and it has started (occurredday.BegunSQL). A booked or unmarked
// past meeting owes an outcome, which the outcome lane asks for once; it owes
// no follow-up yet.
func meetingHeldSQL(alias, now string) string {
	return "(" + meetingoutcome.HeldSQL(alias) + " AND " + occurredday.BegunSQL(alias, now) + ")"
}

// meetingFollowUpsStatement is the follow-up read, newest first, holding at
// most limit meetings; a limit of 0 holds none.
func meetingFollowUpsStatement(ctx context.Context, asOf time.Time, windowDays int, narrow func(func(any) int) (string, error), limit int) (string, []any, error) {
	if limit < 0 {
		return "", nil, fmt.Errorf("activities: a meeting follow-up page of %d meetings", limit)
	}
	return meetingFollowUpsPage(ctx, asOf, windowDays, narrow, fmt.Sprintf("LIMIT %d", limit))
}

// unpagedMeetingFollowUpsStatement is the same read without a page, for a
// caller whose own narrowing bounds it.
func unpagedMeetingFollowUpsStatement(ctx context.Context, asOf time.Time, windowDays int, narrow func(func(any) int) (string, error)) (string, []any, error) {
	return meetingFollowUpsPage(ctx, asOf, windowDays, narrow, "")
}

func meetingFollowUpsPage(ctx context.Context, asOf time.Time, windowDays int, narrow func(func(any) int) (string, error), page string) (string, []any, error) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	instant, reader, window := arg(asOf), arg(readerOrNobody(ctx)), arg(windowDays)
	g, err := followUpGatesFor(ctx, arg, meetingNeedSQL())
	if err != nil {
		return "", nil, err
	}
	addressable, err := addressableContactClause(ctx, arg)
	if err != nil {
		return "", nil, err
	}
	narrowed, err := narrow(arg)
	if err != nil {
		return "", nil, err
	}
	at := fmt.Sprintf("$%d", instant)
	held := meetingHeldSQL("a", at)
	candidates := meetingCandidatesSQL(at, fmt.Sprintf("$%d", window), fmt.Sprintf("$%d", reader), held)
	return followUpStatement(candidates, g, fmt.Sprintf(meetingFollowUpsSQL,
		instant, g.content, g.links, window, AwaitingReplyLookbackDays,
		g.later, relstrength.InteractionCountsSQL("later", at), reader,
		messageSnoozeLiftedSQL(at, g.back), g.customer, page,
		held, maxActivityLinks, narrowed, occurredday.FromSQL("later", followUpFromSQL),
	), meetingAttendeesSQL(addressable)), args, nil
}

// meetingCandidatesSQL is the meetings the lane may report: held, inside the
// window, and the reader took part. The caller's narrowing stays out, since it
// may name the attendees or an outer row. follow_from is where a later contact
// starts to count.
func meetingCandidatesSQL(asOf, window, reader, held string) string {
	return `SELECT a.id, a.thread_key, a.kind, a.channel_provider, ` + followUpFromSQL + ` AS follow_from
	  FROM activity a
	 WHERE a.kind = 'meeting' AND a.archived_at IS NULL AND NOT ` + ImportedHistorySQL + `
	   AND ` + held + `
	   AND ` + meetingEndSQL + ` <= ` + asOf + `::timestamptz - make_interval(days => ` + window + `::int)
	   AND ` + meetingEndSQL + fmt.Sprintf(` >= %s::timestamptz - make_interval(days => %s::int + %d)`, asOf, window, AwaitingReplyLookbackDays) + `
	   AND ` + storekit.MeetingResponsibilitySQL("a", reader)
}

// meetingNeedSQL names the activities the meeting statement reads: each
// candidate, its thread, and what reached a contact it was with after it. A
// contact merged away counts as its survivor, as the statement's recipients do.
// Each arm keeps only terms the statement's own test also holds. A contact's
// history is read once, from its earliest candidate, not once per meeting.
// Each meeting's attendees are looked up by its id, never by a scan of every
// link and participant.
func meetingNeedSQL() string {
	return threadNeedSQL(followUpCandidates) + `
	 UNION SELECT later.id FROM (
	        SELECT COALESCE(named.merged_into_id, named.id) AS contact_id, min(c.follow_from) AS since
	          FROM ` + followUpCandidates + ` c
	          CROSS JOIN LATERAL (SELECT l.contact_id FROM activity_link l WHERE l.activity_id = c.id AND l.contact_id IS NOT NULL
	                UNION ALL SELECT p.contact_id FROM activity_participant p
	                 WHERE p.activity_id = c.id AND p.contact_id IS NOT NULL AND p.user_id IS NULL) met
	          JOIN contact named ON named.id = met.contact_id
	         GROUP BY 1) recipient
	   JOIN (SELECT heard.activity_id, heard.contact_id FROM activity_link heard WHERE heard.contact_id IS NOT NULL
	         UNION ALL SELECT reached.activity_id, reached.contact_id FROM activity_participant reached
	          WHERE reached.contact_id IS NOT NULL) touched ON touched.contact_id = recipient.contact_id
	   CROSS JOIN LATERAL ` + laterActivityFromLinkSQL + ` later
	  WHERE later.restricted_at IS NULL
	    AND later.occurred_at >= recipient.since -- occurred_at raw: FromSQL implies it, so the set is never narrower
`
}

func collectMeetingFollowUps(rows pgx.Rows) ([]AwaitingReply, error) {
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
