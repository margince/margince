// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The two directions of a contact's silence, answered for a SET of contacts in
// one statement. The page's last-touch section is this over a set of one; the
// ranked queue asks it for every contact a page names. One reader, so the row
// a rep acts on and the record it opens cannot disagree about who wrote last.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// LastTouch is when they last wrote to us and when we last wrote to them. Nil
// on either side means it never happened, not that the answer was withheld: a
// withheld answer is a refusal from LastTouchFor, never a zero in its map.
type LastTouch struct {
	InboundAt  *time.Time
	OutboundAt *time.Time
	// LiveInboundAt is InboundAt without the email an import brought in as
	// history. Only the re-engaged suggestion reads it; every other consumer
	// counts imported mail as engagement.
	LiveInboundAt *time.Time
	// ContactAt is the newest exchange of any direction, a meeting included:
	// the moment "quiet for N days" counts from. It passes the same human and
	// sender rules as the two clocks above. A colleague's message on a shared
	// thread cannot make a contact look recently heard from.
	ContactAt *time.Time
	// PrecedingContactAt is the newest exchange strictly before InboundAt,
	// the far side of the silence a reply broke.
	PrecedingContactAt *time.Time
	// The precisions of the rows behind each time: "day" when that row names
	// only a date, empty when there is no row.
	InboundPrecision, OutboundPrecision, ContactPrecision string
	LiveInboundPrecision, PrecedingPrecision              string
}

// Touches is what the relationship-change derivation needs from this answer.
func (t LastTouch) Touches() relstrength.Touches {
	return relstrength.Touches{
		LastTouch:     dated(t.ContactAt, t.ContactPrecision),
		LatestInbound: dated(t.InboundAt, t.InboundPrecision),
		Preceding:     dated(t.PrecedingContactAt, t.PrecedingPrecision),
	}
}

func dated(at *time.Time, precision string) *occurredday.Moment {
	if at == nil {
		return nil
	}
	return &occurredday.Moment{At: *at, DateOnly: occurredday.IsDay(precision)}
}

// LastTouchFor answers the two moments for every contact in the set the caller
// may read, in ONE statement: a queue page naming thirty contacts costs one
// read rather than thirty.
//
// A contact the caller may not read, or one that is archived, is absent from
// the answer — the same refusal the record read makes, one row at a time. The
// caller who cannot read activity at all is refused outright, which is what
// the page reports as a withheld section and a queue row as no moments.
//
// THE TWO DIRECTIONS ASK DIFFERENT QUESTIONS, and only one of them is about
// reachability. "You wrote to them" is satisfied by the message reaching them,
// which is what an outbound message linked to this contact means. "THEY wrote"
// is a claim about authorship, and a thread is linked to everybody it concerns
// — so reading it off reachability told a reader "they wrote last" about a
// message somebody else sent into a conversation this contact is on.
//
// Only a row a human wrote moves either clock (relstrength.WrittenByHumanSQL):
// a calendar invitation, a sending system's notice or the product's own mail
// is nobody writing.
//
// An aggregate, not a per-row read: it has to agree for every colleague, so it
// asks whether the WORKSPACE may see the row (auth.AudienceWorkspaceOnly)
// rather than the caller-scoped arm the timeline uses for content — a private
// message narrowed to its participants must not move the date a colleague
// outside them reads here, the same rule relationship strength holds
// (contacts/strength.go).
func LastTouchFor(ctx context.Context, tx pgx.Tx, contactIDs []ids.ContactID, opts AssembleOptions) (map[ids.ContactID]LastTouch, error) {
	if err := requireRead(ctx, "activity"); err != nil {
		return nil, err
	}
	if err := requireRead(ctx, "contact"); err != nil {
		return nil, err
	}
	out := make(map[ids.ContactID]LastTouch, len(contactIDs))
	if len(contactIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wantedPos := arg(contactIDs)
	visible, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return nil, err
	}
	if visible == "" {
		visible = scopeAll
	}
	scope, err := activityDiscoverScope(ctx, arg)
	if err != nil {
		return nil, err
	}
	// The activities this contact is on, shared by both directions; the
	// inbound arm adds authorship on top.
	reached := fmt.Sprintf(`FROM activity a
		WHERE a.archived_at IS NULL AND %s AND (%s)%s%s`,
		fmt.Sprintf(contactReachesActivity, "c.id"), scope, projectScope(opts, arg),
		auth.AudienceWorkspaceOnly("a"))
	humanClock := relstrength.LastActivityCountsSQL("a", "now()") + " AND " + relstrength.WrittenByHumanSQL("a")
	sender := relstrength.SenderSQL("c.id", "a")
	inbound := fmt.Sprintf(`a.direction = 'inbound' AND %s AND %s`, sender, humanClock)
	exchange := relstrength.ContactExchangeSQL("a", "c.id", "now()")
	// One newest row per direction carries its time and its precision together.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id, inbound.touched_at, outbound.touched_at, live.touched_at, exchange.touched_at, preceding.touched_at,
		       coalesce(inbound.touched_precision, ''), coalesce(outbound.touched_precision, ''), coalesce(exchange.touched_precision, ''),
		       coalesce(live.touched_precision, ''), coalesce(preceding.touched_precision, '')
		FROM contact c
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
		                     %[1]s AND %[2]s %[8]s) inbound ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
		                     %[1]s AND a.direction = 'outbound' AND %[5]s %[8]s) outbound ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
		                     %[1]s AND %[2]s AND NOT %[6]s %[8]s) live ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
		                     %[1]s AND %[7]s %[8]s) exchange ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
		                     %[1]s AND %[7]s AND a.occurred_at < inbound.touched_at %[8]s) preceding ON true -- occurred_at raw: the touch before the newest inbound, an ordering of rows
		WHERE c.id = ANY($%[3]d) AND c.archived_at IS NULL AND (%[4]s)`,
		reached, inbound, wantedPos, visible, humanClock, activities.ImportedHistorySQL, exchange, newestRow), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID ids.ContactID
		var touch LastTouch
		if err := rows.Scan(&contactID, &touch.InboundAt, &touch.OutboundAt, &touch.LiveInboundAt,
			&touch.ContactAt, &touch.PrecedingContactAt,
			&touch.InboundPrecision, &touch.OutboundPrecision, &touch.ContactPrecision,
			&touch.LiveInboundPrecision, &touch.PrecedingPrecision); err != nil {
			return nil, err
		}
		out[contactID] = touch
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// newestRow picks a direction's newest row, the one max(a.occurred_at) names.
const newestRow = "ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1"
