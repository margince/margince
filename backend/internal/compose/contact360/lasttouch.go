// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The two directions of a contact's silence, answered for a SET of contacts in
// one statement. The page's last-touch section is this over a set of one; the
// ranked queue asks it for every contact a page names. One reader, so the row
// a rep acts on and the record it opens cannot disagree about who wrote last.
//
// It reads the caller's own correspondence: the messages whose content the
// caller may open. These dates drive personal follow-up advice — the
// Overview dates, the relationship wording, the gone-quiet card and the same
// reader's Worklist rows — so a reply the reader can open must count, and a
// reply the reader cannot open must not. The shared metrics are a different
// reading on purpose: relationship strength (contacts/strength.go) and the
// colleague graph count only what the whole workspace may see
// (auth.AudienceWorkspaceOnly), because every colleague reads the same number.

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
// on either side means no message the caller can open records it, not that
// the answer was withheld: a withheld answer is a refusal from LastTouchFor,
// never a zero in its map.
type LastTouch struct {
	InboundAt  *time.Time
	OutboundAt *time.Time
	// The rows behind InboundAt, OutboundAt and LiveInboundAt, picked by the
	// same statement. Evidence cites these ids rather than matching a time
	// against the capped timeline page: two rows can share a time, and the
	// row may sit past the first page.
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

// lastTouchSection reads the two directions separately. Folding them into
// one "last touch" hides the only distinction a reader acts on: a contact
// we mailed a fortnight ago with no reply and one who wrote to us this
// morning have the same last-touch date and opposite meanings.
//
// The set reader over a set of one: this page and a queue row naming the same
// contact are answered by the statement in lasttouch.go, under the same
// scopes. Both count the messages the caller can open (last_touch_basis), the
// same rows the History tab lets this reader open. The record read above already admitted the contact, so the set
// reader's own row scope narrows nothing here.
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
// The caller's content gate, not the workspace-only aggregate rule: a reply
// the caller may open is evidence for the caller's own follow-up advice, so a
// participant on a private thread sees it counted and a colleague outside
// that thread sees exactly what they saw before. Discovering that a
// row exists is not enough; the caller must be able to read it, so a row
// shown only as withheld moves no date. The system principal has no reader to
// answer for, and reads the workspace-only basis instead.
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
	// The content gate runs once, over every activity a wanted contact is on,
	// and each direction tests membership in that set: a copy of the gate per
	// direction is planned separately and held by the pooled connection.
	readable, err := auth.ContentReadableActivitiesSQL(ctx, fmt.Sprintf(`
		SELECT l.activity_id FROM activity_link l WHERE l.contact_id = ANY($%[1]d)
		UNION ALL
		SELECT ap.activity_id FROM activity_participant ap WHERE ap.contact_id = ANY($%[1]d)`, wantedPos), arg)
	if err != nil {
		return nil, err
	}
	// The activities this contact is on, shared by both directions; the
	// inbound arm adds authorship on top.
	reached := fmt.Sprintf(`FROM activity a
		WHERE a.archived_at IS NULL AND %s AND a.id IN (SELECT id FROM readable_activity)%s%s`,
		fmt.Sprintf(contactReachesActivity, "c.id"), projectScope(opts, arg), readerlessBasis(ctx))
	asOf := "now()"
	if !opts.AsOf.IsZero() {
		asOf = fmt.Sprintf("$%d::timestamptz", arg(opts.AsOf))
	}
	humanClock := relstrength.LastActivityCountsSQL("a", asOf) + " AND " + relstrength.WrittenByHumanSQL("a")
	inbound := relstrength.TheyWroteSQL("c.id", "a") + " AND " + humanClock
	outbound := relstrength.WeWroteSQL("a") + " AND " + humanClock
	exchange := relstrength.ContactExchangeSQL("a", "c.id", asOf)
	// One newest row per direction carries its time and its precision together.
	query := fmt.Sprintf(`
		WITH readable_activity AS MATERIALIZED (%[9]s)
		SELECT c.id, inbound.touched_at, outbound.touched_at, live.touched_at, exchange.touched_at, preceding.touched_at,
		       coalesce(inbound.touched_precision, ''), coalesce(outbound.touched_precision, ''), coalesce(exchange.touched_precision, ''),
		       coalesce(live.touched_precision, ''), coalesce(preceding.touched_precision, ''),
		       inbound.touched_id, outbound.touched_id, live.touched_id
		FROM contact c
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision, a.id AS touched_id
		                     %[1]s AND %[2]s %[8]s) inbound ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision, a.id AS touched_id
		                     %[1]s AND %[5]s %[8]s) outbound ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision, a.id AS touched_id
		                     %[1]s AND %[2]s AND NOT %[6]s %[8]s) live ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
		                     %[1]s AND %[7]s %[8]s) exchange ON true
		LEFT JOIN LATERAL (SELECT a.occurred_at AS touched_at, a.occurred_precision AS touched_precision
		                     %[1]s AND %[7]s AND a.occurred_at < inbound.touched_at %[8]s) preceding ON true -- occurred_at raw: the touch before the newest inbound, an ordering of rows
		WHERE c.id = ANY($%[3]d) AND c.archived_at IS NULL AND (%[4]s)`,
		reached, inbound, wantedPos, visible, outbound, activities.ImportedHistorySQL, exchange, newestRow, readable)
	// Inside a composed read it reuses its generic plan. It keys on the contacts
	// and spells every kind as a literal, and it plans far slower than it runs.
	err = database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		return scanLastTouches(ctx, tx, query, args, out)
	})
	return out, err
}

func scanLastTouches(ctx context.Context, tx pgx.Tx, query string, args []any, out map[ids.ContactID]LastTouch) error {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID ids.ContactID
		var touch LastTouch
		if err := rows.Scan(&contactID, &touch.InboundAt, &touch.OutboundAt, &touch.LiveInboundAt,
			&touch.ContactAt, &touch.PrecedingContactAt,
			&touch.InboundPrecision, &touch.OutboundPrecision, &touch.ContactPrecision,
			&touch.LiveInboundPrecision, &touch.PrecedingPrecision,
			&touch.InboundID, &touch.OutboundID, &touch.LiveInboundID); err != nil {
			return err
		}
		out[contactID] = touch
	}
	return rows.Err()
}

// readerlessBasis narrows the reading to workspace-visible rows when no human
// reads it. The content gate reads every audience for the system principal,
// and a date computed for nobody must not count a private message.
func readerlessBasis(ctx context.Context) string {
	if p, ok := principal.Actor(ctx); ok && p.Type == principal.PrincipalSystem {
		return auth.AudienceWorkspaceOnly("a")
	}
	return ""
}

// newestRow picks a direction's newest row, the one max(a.occurred_at) names.
const newestRow = "ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1"
