// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// "Who owes ME an answer": the mirror of the waiting queue. It finds a message
// the reader sent to a customer that nobody answered within the follow-up
// window. The worklist then reminds them to follow up.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// AwaitingReplyLookbackDays is how long past its follow-up window a sent
// message is still followed up on. Past it the conversation belongs to the
// record's timeline, the same fortnight the worklist's other reminders keep.
const AwaitingReplyLookbackDays = 14

// ImportedHistorySQL is the test that an activity on alias a came from an
// import. A reminder to follow up is about what happened lately. An import
// brings history that may fall inside the lookback though nobody acted on it
// in this CRM. The importer's namespace is reserved, so no live write carries it.
const ImportedHistorySQL = "starts_with(COALESCE(a.source_system, ''), '" + provenance.ReservedSourceSystemPrefix + "')"

// AwaitingReplyScanCap bounds one read. Newest first, so the cap drops the
// oldest reminders, which are the closest to aging out anyway.
const AwaitingReplyScanCap = 200

// AwaitingReply is one message the reader sent that nobody has answered.
// The ids are zero when the message is filed under no such record.
type AwaitingReply struct {
	ActivityID ids.UUID
	Subject    string
	SentAt     time.Time
	ContactID  ids.UUID
	CompanyID  ids.UUID
	DealID     ids.UUID
	// Recipients is who a meeting follow-up goes to, best first; ContactID is
	// the first of them. Empty on a sent message.
	Recipients []ids.UUID
	// Precision is how to read SentAt: a logged day email or a meeting dated
	// only by day names its date, not a moment.
	Precision string
}

// AwaitingReplies answers which messages the reader sent that are still the
// last word with their customer. Each was sent at least the follow-up window
// before asOf, inside the lookback. It also returns the window it applied.
func (s *Store) AwaitingReplies(ctx context.Context, asOf time.Time) ([]AwaitingReply, int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, 0, err
	}
	var out []AwaitingReply
	var windowDays int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		days, err := followUpAfterDays(ctx, tx)
		if err != nil {
			return err
		}
		windowDays = days
		readerAddresses, err := s.readerAddressList(ctx, tx, readerOrNobody(ctx))
		if err != nil {
			return err
		}
		rows, err := s.queryAwaitingReplies(ctx, tx, asOf, days, readerAddresses)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	return out, windowDays, err
}

// customerArms renders the "is this a customer" test, one arm per kind of
// record. An arm over a kind the reader may not read is FALSE. Whether a deal
// is open is the deal's business, and a reminder must not disclose it.
func customerArms(ctx context.Context, openDeal, workingLead string) (string, error) {
	arms := []struct {
		objects []string
		sql     string
	}{
		{[]string{"company"}, `EXISTS (SELECT 1 FROM company co
		    WHERE co.id = sales.company_id AND co.archived_at IS NULL
		      AND co.lifecycle IN ('prospect', 'opportunity', 'customer'))`},
		{[]string{"company", "relationship"}, `EXISTS (SELECT 1 FROM relationship job
		    JOIN company co ON co.id = job.company_id
		    WHERE job.contact_id = sales.contact_id AND job.kind = 'employment'
		      AND ` + employment.IsCurrentSQL("job.ended_at") + ` AND co.archived_at IS NULL
		      AND co.lifecycle IN ('prospect', 'opportunity', 'customer'))`},
		{[]string{"lead"}, `EXISTS (SELECT 1 FROM lead ld
		    WHERE (ld.id = sales.lead_id OR ld.from_contact_id = sales.contact_id
		           OR ld.promoted_contact_id = sales.contact_id) AND ` + workingLead + `)`},
		{[]string{"deal"}, `EXISTS (SELECT 1 FROM deal d WHERE d.id = sales.deal_id AND ` + openDeal + `)`},
	}
	rendered := make([]string, 0, len(arms))
	for _, arm := range arms {
		readable, err := mayReadAll(ctx, arm.objects...)
		if err != nil {
			return "", err
		}
		if readable {
			rendered = append(rendered, arm.sql)
		}
	}
	if len(rendered) == 0 {
		return scopeNothing, nil
	}
	return "(" + strings.Join(rendered, " OR ") + ")", nil
}

func mayReadAll(ctx context.Context, objects ...string) (bool, error) {
	for _, object := range objects {
		err := auth.Require(ctx, object, principal.ActionRead)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// A follow-up statement first names its candidates, then gates every activity
// it reads once, as followUpReadable. Postgres plans each copy of the content
// gate, and a pooled connection keeps that memory, so each alias tests
// membership instead of carrying a copy.
const (
	followUpCandidates = "follow_up_candidates"
	followUpReadable   = "follow_up_readable"
	followUpHeardSince = "follow_up_heard_since"
)

// followUpGates are the gate fragments both follow-up reads compose, under
// the aliases both statements use.
type followUpGates struct {
	readable, content, later, back, links, customer string
}

// followUpGatesFor gates need: the statement's candidates and every activity
// its later and back aliases may read.
func followUpGatesFor(ctx context.Context, arg func(any) int, need string) (followUpGates, error) {
	var g followUpGates
	var err error
	if g.readable, err = auth.ContentReadableActivitiesSQL(ctx, need, arg); err != nil {
		return g, err
	}
	// The row publishes a message's subject and that nobody answered, both
	// derived from the thread, so it takes the content gate. A later message
	// the reader may not see must not decide anything.
	g.content, g.later, g.back = followUpReadableSQL("a"), followUpReadableSQL("later"), followUpReadableSQL("back")
	if g.links, err = auth.LinkTargetReadableClause(ctx, "wl", arg); err != nil {
		return g, err
	}
	if g.links == "" {
		g.links = scopeUnbounded
	}
	g.customer, err = customerArms(ctx, liveRecord(openDealPredicate, "d"), liveRecord(workingLeadPredicate, "ld"))
	return g, err
}

func followUpReadableSQL(alias string) string {
	return alias + ".id IN (SELECT id FROM " + followUpReadable + ")"
}

// followUpStatement puts the candidates and the one gated set before body.
func followUpStatement(candidates string, g followUpGates, body string, sets ...string) string {
	with := append([]string{
		followUpCandidates + " AS MATERIALIZED (" + candidates + ")",
		followUpReadable + " AS MATERIALIZED (" + g.readable + ")",
	}, sets...)
	return "WITH " + strings.Join(with, ",\n") + "\n" + body
}

// scanFollowUps reads the rows both follow-up statements select.
func scanFollowUps(rows pgx.Rows, what string) ([]AwaitingReply, error) {
	defer rows.Close()
	var out []AwaitingReply
	for rows.Next() {
		var r AwaitingReply
		if err := rows.Scan(&r.ActivityID, &r.Subject, &r.SentAt, &r.ContactID, &r.CompanyID, &r.DealID, &r.Precision); err != nil {
			return nil, fmt.Errorf("activities: reading %s: %w", what, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) queryAwaitingReplies(
	ctx context.Context, tx pgx.Tx, asOf time.Time, windowDays int, readerAddresses []string,
) ([]AwaitingReply, error) {
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	instant, reader, addresses, window := arg(asOf), arg(readerOrNobody(ctx)), arg(readerAddresses), arg(windowDays)
	g, err := followUpGatesFor(ctx, arg, awaitingNeedSQL())
	if err != nil {
		return nil, err
	}
	at := fmt.Sprintf("$%d", instant)
	sentBy := sentByReaderSQL(fmt.Sprintf("$%d", reader), fmt.Sprintf("$%d", addresses))
	parts := []any{
		instant, g.content, g.links, addresses, window,
		AwaitingReplyLookbackDays, g.later, relstrength.InteractionCountsSQL("later"),
		reader, messageSnoozeLiftedSQL(at, g.back),
		g.customer, AwaitingReplyScanCap,
	}
	query := followUpStatement(awaitingCandidatesSQL(at, sentBy, fmt.Sprintf("$%d", window)), g,
		fmt.Sprintf(awaitingRepliesSQL, parts...), fmt.Sprintf(awaitingHeardSinceSQL, parts...))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading replies the reader is waiting for: %w", err)
	}
	return scanFollowUps(rows, "a reply the reader is waiting for")
}

// The kinds awaitingTheirAnswerSQL judges. The follow-up lane asks after mail;
// the History badge asks after chat messages too.
const (
	mailKinds         = `'email'`
	conversationKinds = `'email', 'message'`
)

// awaitingTheirAnswerSQL is a mail on alias a the reader sent that still waits
// on the other side, for the badge and the follow-up lane. sentByReader is
// sentByReaderSQL. laterContent gates the later message, so a reply the reader
// may not read decides nothing. A calendar copy asks nobody a question. A
// thread key is chosen by the sender, so a later message counts only when
// filed under the same record.
func awaitingTheirAnswerSQL(asOf, sentByReader, laterContent, kinds string) string {
	return `(a.kind IN (` + kinds + `)
	   AND a.direction = 'outbound'
	   AND a.archived_at IS NULL
	   AND NOT ` + ImportedHistorySQL + `
	   AND ` + relstrength.WrittenByHumanSQL("a") + `
	   AND NOT ` + calendarCopySQL + `
	   AND ` + sentByReader + `
	   AND NOT EXISTS (SELECT 1 FROM activity later
	                    WHERE later.archived_at IS NULL
	                      AND ` + laterContent + `
	                      AND ` + relstrength.WrittenByHumanSQL("later") + `
	                      AND a.thread_key IS NOT NULL AND a.thread_key <> ''
	                      AND later.thread_key = a.thread_key
	                      AND later.kind = a.kind
	                      AND later.channel_provider IS NOT DISTINCT FROM a.channel_provider
	                      AND (later.occurred_at, later.id) > (a.occurred_at, a.id)
	                      AND ` + occurredday.BegunSQL("later", asOf) + `
	                      AND EXISTS (SELECT 1 FROM activity_link heard
	                                    JOIN activity_link sent ON sent.activity_id = a.id
	                                   WHERE heard.activity_id = later.id
	                                     AND (heard.contact_id = sent.contact_id
	                                       OR heard.company_id = sent.company_id))))`
}

// sentByReaderSQL is true when the reader sent the mail on alias a. Sent and
// captured mail stamp the sender as the seat. Mail logged by hand names one of
// the reader's addresses. A seat row the legacy backfill guessed for bare
// mail does not stand (auth.ParticipantStandsSQL), so its seat owes no reply.
func sentByReaderSQL(reader, addresses string) string {
	return `EXISTS (SELECT 1 FROM activity_participant me
	   WHERE me.activity_id = a.id AND me.role = 'from'
	     AND (me.user_id = ` + reader + ` OR lower(me.address) = ANY(` + addresses + `::text[]))
	     AND ` + auth.ParticipantStandsSQL("me", "a") + `)`
}

// awaitingCandidatesSQL is the mail the follow-up lane may report: what the
// reader sent inside the window and its lookback. Its terms are ones
// awaitingRepliesSQL also tests, so it never drops a row the lane reports.
func awaitingCandidatesSQL(asOf, sentByReader, window string) string {
	return `SELECT a.id, a.thread_key, a.kind, a.channel_provider, a.occurred_at FROM activity a
	 WHERE a.kind IN (` + mailKinds + `) AND a.direction = 'outbound' AND a.archived_at IS NULL
	   AND ` + sentByReader + `
	   ` + fmt.Sprintf("AND a.occurred_at <= %s::timestamptz - make_interval(days => %s::int) -- occurred_at raw: an age of whole days\n", asOf, window) + `
	   AND ` + occurredday.FromSQL("a", fmt.Sprintf("%s::timestamptz - make_interval(days => %s::int + %d)", asOf, window, AwaitingReplyLookbackDays))
}

// awaitingNeedSQL names the activities the follow-up statement reads: each
// candidate, its thread, and what came after it with a contact it went to.
// Each arm keeps only terms the statement's own test also holds.
func awaitingNeedSQL() string {
	return threadNeedSQL(followUpCandidates) + `
	 UNION SELECT later.id FROM ` + followUpCandidates + ` c
	   JOIN activity_link sent ON sent.activity_id = c.id AND sent.contact_id IS NOT NULL
	   JOIN activity_link heard ON heard.contact_id = sent.contact_id
	   JOIN activity later ON later.id = heard.activity_id AND later.restricted_at IS NULL
	    AND later.occurred_at > c.occurred_at -- occurred_at raw: the statement's own after-the-mail order, which this set may not narrow
`
}

// awaitingHeardSinceSQL lists the candidates whose recipients were in contact
// since, on any channel: a call or a meeting with them is a follow-up already
// made. It is one set over every candidate. Asked per candidate, it walked each
// recipient's whole history once per mail. It reads awaitingRepliesSQL's slots.
var awaitingHeardSinceSQL = followUpHeardSince + ` AS MATERIALIZED (
	SELECT DISTINCT c.id FROM ` + followUpCandidates + ` c
	  JOIN activity_link sent ON sent.activity_id = c.id AND sent.contact_id IS NOT NULL
	  JOIN activity_link heard ON heard.contact_id = sent.contact_id
	  JOIN activity later ON later.id = heard.activity_id
	 WHERE later.id <> c.id
	   AND later.archived_at IS NULL
	   AND %[7]s
	   AND %[8]s
	   AND later.occurred_at > c.occurred_at -- occurred_at raw: heard after the mail, an ordering of rows
	   AND ` + occurredday.BegunSQL("later", "$%[1]d") + `)`

// awaitingRepliesSQL: every rule sits before the cap.
//
//  1. as of, 2. content gate on a, 3. link visibility on wl, 4. the reader's
//     addresses, 5. the follow-up window in days, 6. the lookback in days past
//     it, 7. content gate on later, 8. what counts as contact for later, 9. the
//     reader, 10. a snooze lifted, 11. the customer arms the reader may read,
//  12. the cap.
var awaitingRepliesSQL = `
	SELECT a.id, COALESCE(a.subject, ''), a.occurred_at,
	       COALESCE((array_agg(wl.contact_id ORDER BY wl.contact_id::text)
	                 FILTER (WHERE wl.contact_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.company_id ORDER BY wl.company_id::text)
	                 FILTER (WHERE wl.company_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       COALESCE((array_agg(wl.deal_id ORDER BY wl.deal_id::text)
	                 FILTER (WHERE wl.deal_id IS NOT NULL))[1],
	                '00000000-0000-0000-0000-000000000000'::uuid),
	       a.occurred_precision
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	 WHERE ` + awaitingTheirAnswerSQL("$%[1]d", sentByReaderSQL("$%[9]d", "$%[4]d"), "%[7]s", mailKinds) + `
	   AND a.occurred_at <= $%[1]d::timestamptz - make_interval(days => $%[5]d::int) -- occurred_at raw: an age of whole days
	   AND ` + occurredday.FromSQL("a", "$%[1]d::timestamptz - make_interval(days => $%[5]d::int + %[6]d)") + `
	   AND %[2]s
	   -- Nor any contact since with someone it went to: awaitingHeardSinceSQL.
	   AND a.id NOT IN (SELECT id FROM ` + followUpHeardSince + `)
	   -- A customer: a lead, someone at a prospect or customer company, or a
	   -- record with an open deal or a lead being worked.
	   AND EXISTS (SELECT 1 FROM activity_link sales WHERE sales.activity_id = a.id AND %[11]s)
	   AND NOT ` + readerSetAsideSQL("$%[9]d", "%[10]s") + `
	 GROUP BY a.id, a.subject, a.occurred_at
	 ORDER BY a.occurred_at DESC, a.id DESC
	 LIMIT %[12]d`

// threadNeedSQL names each of candidates and the live rows on its thread. A
// statement gates that set once when its later-message and snooze-lift tests
// read the same thread; both match by correspondence.ThreadSQL. candidates
// names a relation with an activity's id, thread_key, kind and channel_provider.
func threadNeedSQL(candidates string) string {
	return `SELECT c.id FROM ` + candidates + ` c
	 UNION SELECT t.id FROM ` + candidates + ` c JOIN activity t ON ` + correspondence.ThreadSQL("t", "c") + ` AND t.restricted_at IS NULL`
}
