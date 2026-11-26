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
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/activitywhen"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// LastTouch is when they last wrote to us and when we last wrote to them. Nil
// means no message the caller can open records it. A withheld answer is a
// refusal from LastTouchFor, never a zero in its map.
type LastTouch struct {
	InboundAt  *time.Time
	OutboundAt *time.Time
	// The rows the statement picked for those times. Evidence cites them by id:
	// two rows can share a time, and a row may sit past the timeline page.
	InboundID, OutboundID, LiveInboundID *ids.UUID
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

// Direction is which sides have written, by relstrength's one rule.
func (t LastTouch) Direction() relstrength.Direction {
	return relstrength.Sides{TheyWrote: t.InboundAt != nil, WeWrote: t.OutboundAt != nil}.Direction()
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

// lastTouchSection reads the two directions separately, because which side
// wrote last is what a reader acts on. It is LastTouchFor over a set of one,
// so an unscoped page and a Worklist row naming the contact give one answer.
func (s *Service) lastTouchSection(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, opts AssembleOptions, out *crmcontracts.Contact360) error {
	touched, err := LastTouchFor(ctx, tx, []ids.ContactID{contactID}, opts)
	if err != nil {
		return err
	}
	touch := touched[contactID]
	basis := crmcontracts.Contact360LastTouchBasisReadableByCaller
	out.LastTouchBasis = &basis
	out.LastInboundAt = touch.InboundAt
	out.LastOutboundAt = touch.OutboundAt
	out.LastInboundActivityId = wireActivityID(touch.InboundID)
	out.LastOutboundActivityId = wireActivityID(touch.OutboundID)
	out.LastLiveInboundActivityId = wireActivityID(touch.LiveInboundID)
	out.LastLiveInboundAt = touch.LiveInboundAt
	out.LastContactAt = touch.ContactAt
	out.LastInboundPrecision = activitywhen.Stated(touch.InboundPrecision)
	out.LastOutboundPrecision = activitywhen.Stated(touch.OutboundPrecision)
	out.LastContactPrecision = activitywhen.Stated(touch.ContactPrecision)
	out.LastLiveInboundPrecision = activitywhen.Stated(touch.LiveInboundPrecision)
	direction := crmcontracts.RelationshipDirection(touch.Direction())
	out.RelationshipDirection = &direction
	return nil
}

// wireActivityID carries a picked row's id onto the wire, nil when no row was.
func wireActivityID(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	wire := openapi_types.UUID(*id)
	return &wire
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
// It counts the messages whose content the caller may open. These dates drive
// one reader's follow-up advice, so a reply that reader can open counts and
// one they cannot open does not. A row they may only discover moves no date.
// Shared numbers (strength, the colleague graph) use auth.AudienceWorkspaceOnly
// instead, because every colleague must read the same number.
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
	unresolved, err := lastTouchesWithin(ctx, tx, contactIDs, opts, lastTouchWindow, out)
	if err != nil || len(unresolved) == 0 {
		return out, err
	}
	// The newest rows of a direction were all closed to the reader, and older
	// ones exist: those contacts are judged on their whole history.
	_, err = lastTouchesWithin(ctx, tx, unresolved, opts, 0, out)
	return out, err
}

// lastTouchWindow is how many of a contact's newest rows per direction the
// first pass judges. A row the reader cannot open is rare, so the newest
// readable row is almost always among them, and the gate runs on few rows.
const lastTouchWindow = 16

// lastTouchesWithin answers the contacts into out. It judges only each
// direction's newest window rows, or every row when window is 0; a tie at the
// edge is in or out together. It returns the contacts whose window held no
// readable row of a direction that has older rows, whose answer may lie back.
func lastTouchesWithin(ctx context.Context, tx pgx.Tx, contactIDs []ids.ContactID, opts AssembleOptions, window int, out map[ids.ContactID]LastTouch) ([]ids.ContactID, error) {
	query, args, err := lastTouchStatement(ctx, contactIDs, opts, window)
	if err != nil {
		return nil, err
	}
	var unresolved []ids.ContactID
	// Inside a composed read it reuses its generic plan. It keys on the contacts
	// and spells every kind as a literal, and it plans far slower than it runs.
	err = database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		unresolved, err = scanLastTouches(ctx, tx, query, args, out)
		return err
	})
	return unresolved, err
}

// lastTouchStatement is lastTouchesWithin's statement and its arguments.
func lastTouchStatement(ctx context.Context, contactIDs []ids.ContactID, opts AssembleOptions, window int) (string, []any, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return "", nil, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return "", nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wantedPos := arg(contactIDs)
	visible, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return "", nil, err
	}
	if visible == "" {
		visible = scopeAll
	}
	// One copy of the content gate for all directions, over the kept rows only.
	readable, err := auth.ContentReadableActivitiesSQL(ctx, "SELECT kept.id FROM kept", arg)
	if err != nil {
		return "", nil, err
	}
	asOf := "now()"
	if !opts.AsOf.IsZero() {
		asOf = fmt.Sprintf("$%d::timestamptz", arg(opts.AsOf))
	}
	humanClock := relstrength.ExternalActivityCountsSQL("a", asOf) + " AND " + relstrength.WrittenByHumanSQL("a")
	inbound := relstrength.TheyWroteSQL("reach.contact_id", "a") + " AND " + humanClock
	// Each direction is judged once per row; the walks below read the flags.
	judged := fmt.Sprintf(`SELECT DISTINCT reach.contact_id, a.id, a.occurred_at,
		coalesce(%[1]s, false) AS theirs, coalesce(%[2]s, false) AS ours,
		coalesce(%[1]s AND NOT %[3]s, false) AS live, coalesce(%[4]s, false) AS exchanged
		FROM reach JOIN activity a ON a.id = reach.activity_id
		WHERE a.archived_at IS NULL%[5]s%[6]s`,
		inbound, relstrength.WeWroteSQL("a")+" AND "+humanClock, activities.ImportedHistorySQL,
		relstrength.ContactExchangeSQL("a", "reach.contact_id", asOf), projectScope(opts, arg), auth.ReaderlessAudience(ctx, "a"))
	limit := "NULL::int"
	if window > 0 {
		limit = fmt.Sprintf("$%d::int", arg(window))
	}
	return fmt.Sprintf(lastTouchSQL, fmt.Sprintf(contactReach, wantedPos), judged, limit, readable,
		wantedPos, visible, newestTheirs, newestOurs, newestRow), args, nil
}

// lastTouchSQL ranks each contact's rows newest first per direction and keeps
// a direction's first window rows. It gates those once and walks them for the
// newest readable row of each direction. RANGE ranks a tie alike, so the kept
// rows of a direction are always every row newer than some instant.
const lastTouchSQL = `
	WITH reach AS MATERIALIZED (%[1]s),
	judged AS MATERIALIZED (%[2]s),
	ranked AS MATERIALIZED (
		SELECT judged.*,
		       count(*) FILTER (WHERE theirs) OVER newer AS theirs_rank, count(*) FILTER (WHERE ours) OVER newer AS ours_rank,
		       count(*) FILTER (WHERE live) OVER newer AS live_rank, count(*) FILTER (WHERE exchanged) OVER newer AS exchanged_rank
		FROM judged
		WINDOW newer AS (PARTITION BY contact_id ORDER BY occurred_at DESC RANGE BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)), -- occurred_at raw: an ordering of rows
	kept AS MATERIALIZED (SELECT * FROM (
		SELECT contact_id, id,
		       theirs AND coalesce(theirs_rank <= %[3]s, true) AS theirs, ours AND coalesce(ours_rank <= %[3]s, true) AS ours,
		       live AND coalesce(live_rank <= %[3]s, true) AS live, exchanged AND coalesce(exchanged_rank <= %[3]s, true) AS exchanged
		FROM ranked) windowed WHERE theirs OR ours OR live OR exchanged),
	beyond AS (
		SELECT contact_id, bool_or(theirs AND theirs_rank > %[3]s) AS theirs, bool_or(ours AND ours_rank > %[3]s) AS ours,
		       bool_or(live AND live_rank > %[3]s) AS live, bool_or(exchanged AND exchanged_rank > %[3]s) AS exchanged
		FROM ranked GROUP BY contact_id),
	readable_activity AS MATERIALIZED (%[4]s)
	SELECT c.id, inbound.touched_at, outbound.touched_at, live.touched_at, exchange.touched_at, preceding.touched_at,
	       coalesce(inbound.touched_precision, ''), coalesce(outbound.touched_precision, ''), coalesce(exchange.touched_precision, ''),
	       coalesce(live.touched_precision, ''), coalesce(preceding.touched_precision, ''),
	       inbound.touched_id, outbound.touched_id, live.touched_id,
	       coalesce((inbound.touched_at IS NULL AND beyond.theirs) OR (outbound.touched_at IS NULL AND beyond.ours)
	             OR (live.touched_at IS NULL AND beyond.live) OR (exchange.touched_at IS NULL AND beyond.exchanged)
	             OR (preceding.touched_at IS NULL AND inbound.touched_at IS NOT NULL AND beyond.exchanged), false)
	FROM contact c
	LEFT JOIN beyond ON beyond.contact_id = c.id
	LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision, a.id AS touched_id
	                     FROM kept JOIN activity a ON a.id = kept.id WHERE kept.contact_id = c.id AND kept.theirs
	                      AND a.id IN (SELECT id FROM readable_activity) %[7]s) inbound ON true
	LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision, a.id AS touched_id
	                     FROM kept JOIN activity a ON a.id = kept.id WHERE kept.contact_id = c.id AND kept.ours
	                      AND a.id IN (SELECT id FROM readable_activity) %[8]s) outbound ON true
	LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision, a.id AS touched_id
	                     FROM kept JOIN activity a ON a.id = kept.id WHERE kept.contact_id = c.id AND kept.live
	                      AND a.id IN (SELECT id FROM readable_activity) %[7]s) live ON true
	LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
	                     FROM kept JOIN activity a ON a.id = kept.id WHERE kept.contact_id = c.id AND kept.exchanged
	                      AND a.id IN (SELECT id FROM readable_activity) %[9]s) exchange ON true
	LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
	                     FROM kept JOIN activity a ON a.id = kept.id WHERE kept.contact_id = c.id AND kept.exchanged
	                      AND a.id IN (SELECT id FROM readable_activity)
	                      AND a.occurred_at < inbound.touched_at %[9]s) preceding ON true -- occurred_at raw: the touch before the newest inbound, an ordering of rows
	WHERE c.id = ANY($%[5]d) AND c.archived_at IS NULL AND (%[6]s)`

func scanLastTouches(ctx context.Context, tx pgx.Tx, query string, args []any, out map[ids.ContactID]LastTouch) ([]ids.ContactID, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var unresolved []ids.ContactID
	for rows.Next() {
		var contactID ids.ContactID
		var touch LastTouch
		var further bool
		if err := rows.Scan(&contactID, &touch.InboundAt, &touch.OutboundAt, &touch.LiveInboundAt,
			&touch.ContactAt, &touch.PrecedingContactAt,
			&touch.InboundPrecision, &touch.OutboundPrecision, &touch.ContactPrecision,
			&touch.LiveInboundPrecision, &touch.PrecedingPrecision,
			&touch.InboundID, &touch.OutboundID, &touch.LiveInboundID, &further); err != nil {
			return nil, err
		}
		if further {
			unresolved = append(unresolved, contactID)
			continue
		}
		out[contactID] = touch
	}
	return unresolved, rows.Err()
}

// contactReach is contactReachesActivity for a set of contacts, each activity
// paired with the contact it reaches. An activity can reach a contact twice,
// by a link and as a participant; the newest-row pick is the same row either way.
const contactReach = `
		SELECT l.contact_id, l.activity_id FROM activity_link l WHERE l.contact_id = ANY($%[1]d)
		UNION ALL
		SELECT ap.contact_id, ap.activity_id FROM activity_participant ap WHERE ap.contact_id = ANY($%[1]d)`

// newestRow picks a direction's newest row, the one max(a.occurred_at) names.
const newestRow = "ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1"

// newestTheirs and newestOurs break a tie by precision, so occurredday.Answers between the two picks decides the
// whole tie.
var (
	newestTheirs = "ORDER BY a.occurred_at DESC, " + occurredday.AnsweredTieOrderSQL("a") + ", a.id DESC LIMIT 1"
	newestOurs   = "ORDER BY a.occurred_at DESC, " + occurredday.AnswerTieOrderSQL("a") + ", a.id DESC LIMIT 1"
)
