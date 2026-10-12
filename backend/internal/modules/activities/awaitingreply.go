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
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// AwaitingReplyLookbackDays is how long past its follow-up window a sent
// message is still followed up on. Past it the conversation belongs to the
// record's timeline, the same fortnight the worklist's other reminders keep.
const AwaitingReplyLookbackDays = 14

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

// followUpGates are the gate fragments both follow-up reads compose, under
// the aliases both statements use.
type followUpGates struct {
	content, later, back, links, customer string
}

func followUpGatesFor(ctx context.Context, arg func(any) int) (followUpGates, error) {
	var g followUpGates
	var err error
	// The row publishes a message's subject and that nobody answered, both
	// derived from the thread, so it takes the content gate.
	if g.content, err = auth.ActivityContentClause(ctx, "a", arg); err != nil {
		return g, err
	}
	// A later message the reader may not see must not decide anything.
	if g.later, err = auth.ActivityContentClause(ctx, "later", arg); err != nil {
		return g, err
	}
	if g.back, err = auth.ActivityContentClause(ctx, "back", arg); err != nil {
		return g, err
	}
	if g.links, err = auth.LinkTargetVisibleClause(ctx, "wl", arg); err != nil {
		return g, err
	}
	if g.links == "" {
		g.links = scopeUnbounded
	}
	g.customer, err = customerArms(ctx, liveRecord(openDealPredicate, "d"), liveRecord(workingLeadPredicate, "ld"))
	return g, err
}

// scanFollowUps reads the rows both follow-up statements select.
func scanFollowUps(rows pgx.Rows, what string) ([]AwaitingReply, error) {
	defer rows.Close()
	var out []AwaitingReply
	for rows.Next() {
		var r AwaitingReply
		if err := rows.Scan(&r.ActivityID, &r.Subject, &r.SentAt, &r.ContactID, &r.CompanyID, &r.DealID); err != nil {
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
	instant := arg(asOf)
	g, err := followUpGatesFor(ctx, arg)
	if err != nil {
		return nil, err
	}
	reader := arg(readerOrNobody(ctx))
	rows, err := tx.Query(ctx, fmt.Sprintf(awaitingRepliesSQL,
		instant, g.content, g.links, arg(readerAddresses), arg(windowDays),
		AwaitingReplyLookbackDays, g.later, relstrength.InteractionCountsSQL("later"),
		reader, messageSnoozeLiftedSQL(fmt.Sprintf("$%d", instant), g.back),
		g.customer, AwaitingReplyScanCap,
	), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading replies the reader is waiting for: %w", err)
	}
	return scanFollowUps(rows, "a reply the reader is waiting for")
}

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
	                '00000000-0000-0000-0000-000000000000'::uuid)
	  FROM activity a
	  LEFT JOIN activity_link wl ON wl.activity_id = a.id AND (%[3]s)
	 WHERE a.kind = 'email'
	   AND a.direction = 'outbound'
	   AND a.archived_at IS NULL
	   AND a.occurred_at <= $%[1]d::timestamptz - make_interval(days => $%[5]d::int)
	   AND a.occurred_at >= $%[1]d::timestamptz - make_interval(days => $%[5]d::int + %[6]d)
	   AND %[2]s
	   -- The reader sent it. Mail sent through the product and mail captured
	   -- from a mailbox stamp the sender as the seat with no address; mail
	   -- logged by hand names the address.
	   AND EXISTS (SELECT 1 FROM activity_participant me
	                WHERE me.activity_id = a.id AND me.role = 'from'
	                  AND (me.user_id = $%[9]d OR lower(me.address) = ANY($%[4]d::text[])))
	   -- Still the last word on its conversation with its contacts: nothing
	   -- newer on the thread from a record it went to. A thread key is a
	   -- header the sender chose, so a message on it counts only when it is
	   -- filed under the same contact or company.
	   AND NOT EXISTS (SELECT 1 FROM activity later
	                    WHERE later.archived_at IS NULL
	                      AND %[7]s
	                      AND a.thread_key IS NOT NULL AND a.thread_key <> ''
	                      AND later.thread_key = a.thread_key
	                      AND later.kind = a.kind
	                      AND later.channel_provider IS NOT DISTINCT FROM a.channel_provider
	                      AND (later.occurred_at, later.id) > (a.occurred_at, a.id)
	                      AND later.occurred_at <= $%[1]d
	                      AND EXISTS (SELECT 1 FROM activity_link heard
	                                    JOIN activity_link sent ON sent.activity_id = a.id
	                                   WHERE heard.activity_id = later.id
	                                     AND (heard.contact_id = sent.contact_id
	                                       OR heard.company_id = sent.company_id)))
	   -- Nor any contact since with someone it went to, on any channel: a call
	   -- or a meeting with them is a follow-up already made.
	   AND NOT EXISTS (SELECT 1 FROM activity_link sent
	                     JOIN activity_link heard ON heard.contact_id = sent.contact_id
	                     JOIN activity later ON later.id = heard.activity_id
	                    WHERE sent.activity_id = a.id AND sent.contact_id IS NOT NULL
	                      AND later.id <> a.id
	                      AND later.archived_at IS NULL
	                      AND %[7]s
	                      AND %[8]s
	                      AND later.occurred_at > a.occurred_at
	                      AND later.occurred_at <= $%[1]d)
	   -- A customer: a lead, someone at a prospect or customer company, or a
	   -- record with an open deal or a lead being worked.
	   AND EXISTS (SELECT 1 FROM activity_link sales WHERE sales.activity_id = a.id AND %[11]s)
	   -- Set aside by this reader: not theirs, or snoozed and not yet lifted.
	   AND NOT EXISTS (SELECT 1 FROM activity_reader_state mine
	                    WHERE mine.activity_id = a.id AND mine.reader_id = $%[9]d
	                      AND (mine.state = 'not_mine' OR (mine.state = 'snoozed' AND NOT %[10]s)))
	 GROUP BY a.id, a.subject, a.occurred_at
	 ORDER BY a.occurred_at DESC, a.id DESC
	 LIMIT %[12]d`
