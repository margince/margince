// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ExcludeEdges renders a predicate that removes candidate contacts before the
// LIMIT, over alias `e`, the contact's strongest edge. It judges the contact.
// A test on one colleague's edge drops the contact; it promotes no other edge.
//
// A HOLE rather than a join: the deciding rows belong to another module. The
// caller numbers its fragment through the same `arg` counter.
type ExcludeEdges func(arg func(any) int) (string, error)

// QuietRead is one ask of the quiet-relationship candidates.
//
// The contact's OWNER answers for a quiet relationship; an edge only proves the
// relationship existed. Being copied on a colleague's mail, or having written
// it, makes nobody responsible for chasing the contact months later.
type QuietRead struct {
	// Owner narrows to contacts this live seat owns. Zero with Unowned false
	// reads every contact the caller may see, owned or not.
	Owner ids.UUID
	// Unowned narrows to contacts with no live owner: none, or one who left.
	Unowned bool
	// QuietBefore is the lower bound of the silence, from relstrength.QuietDays.
	QuietBefore time.Time
	// TouchedAfter is the silence ceiling: the newest live touch must be later.
	TouchedAfter time.Time
	// AsOf is the instant the read is made at. An exchange dated after it has
	// not happened yet: a booked meeting is not a touch. The window above is
	// measured from the same instant, so the read has one clock. Zero reads
	// the database's own now().
	AsOf    time.Time
	Limit   int
	Exclude ExcludeEdges
}

// QuietContact is one candidate: the live colleague's edge that carries the
// most exchanges, and the contact's live owner (zero when there is none).
type QuietContact struct {
	Edge    InteractionEdge
	OwnerID ids.UUID
}

// liveExchangeClock selects the newest exchange any live colleague has with the
// candidate contact. It reads the activities, not the edge's last_at. A
// colleague's message on a thread the contact only belongs to then cannot hide
// a quiet relationship, and the same colleague's older exchange still counts.
// The colleague is an EXISTS, so it is asked only of the contact's exchanges.
func liveExchangeClock(asOf string) string {
	return fmt.Sprintf(`SELECT max(a.occurred_at) AS touched
		      FROM activity_participant cp
		      JOIN activity a ON a.id = cp.activity_id AND a.archived_at IS NULL%s
		       AND %s
		     WHERE cp.contact_id = e.contact_id AND cp.role IN %s
		       AND EXISTS (SELECT 1 FROM activity_participant up
		              JOIN app_user lu ON lu.id = up.user_id AND %s
		             WHERE up.activity_id = a.id AND up.role IN %s
		               AND %s)`,
		audienceWorkspaceOnly, relstrength.ContactExchangeSQL("a", "e.contact_id", asOf),
		interactionRoles, workforce.LiveMemberSQL("lu"), interactionRoles, auth.ParticipantStandsSQL("up", "a"))
}

// QuietContacts answers "which relationships have gone silent", most exchanged
// first. It is the candidate half of the decay lane. Whether a silence is
// worth reporting stays with the relationship-change derivation in contacts.
//
// A relationship is silent when no live colleague has an exchange with the
// contact newer than QuietBefore. It is still current when some live colleague
// has one newer than TouchedAfter. An exchange follows the contact's last-touch
// rule, so a colleague's message on a thread the contact only belongs to is not
// one. The silence and its ceiling belong to the contact, not to one colleague.
// A pair with fewer than two interactions is a cold contact, not a lapsed
// relationship.
//
// The contact row scope applies at source, before the LIMIT, so an unreadable
// contact neither leaks through an edge row nor evicts a readable lapse.
//
// Each contact's strongest edge is chosen first, and the exchange clock runs
// once per contact after it. The clock reads only the contact, so running it
// per edge repeated the same aggregate for every colleague who wrote to them.
func QuietContacts(ctx context.Context, tx pgx.Tx, read QuietRead) ([]QuietContact, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if actor, ok := principal.Actor(ctx); !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	beforePos := arg(read.QuietBefore)
	afterPos := arg(read.TouchedAfter)
	asOf := "now()"
	if !read.AsOf.IsZero() {
		asOf = fmt.Sprintf("$%d::timestamptz", arg(read.AsOf))
	}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	excluded := "TRUE"
	if read.Exclude != nil {
		got, err := read.Exclude(arg)
		if err != nil {
			return nil, err
		}
		if got != "" {
			excluded = got
		}
	}
	// One owner's read starts from that owner's contacts and reaches their
	// edges through a fenced lateral. Planned freely, it put the few live
	// colleagues outermost and rescanned the owner's contacts once per colleague.
	source := `FROM graph_interaction_edge e
		      ` + liveMemberJoin + `
		      JOIN contact p ON p.id = e.contact_id`
	if !read.Owner.IsZero() {
		source = `FROM contact p
		      CROSS JOIN LATERAL (SELECT e.* FROM graph_interaction_edge e
		              ` + liveMemberJoin + `
		             WHERE e.contact_id = p.id OFFSET 0) e`
	}
	owned := quietOwnerClause(read, arg)
	exchangedPos := arg(2)
	limitPos := arg(read.Limit)
	// The exclusions are judged per contact, so they leave each contact's
	// strongest edge alone and run after it, only for the candidates the clock
	// reaches. Strongest first, the clock stops once the limit is filled and its
	// last count is complete.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH strongest AS MATERIALIZED (
		    SELECT DISTINCT ON (e.contact_id) e.user_id, e.contact_id,
		           e.last_inbound_at, e.last_outbound_at, e.count_90d, e.in_count_90d,
		           e.out_count_90d, e.count_total, o.id AS live_owner
		      %s
		      LEFT JOIN app_user o ON o.id = p.owner_id AND %s
		     WHERE e.count_total >= $%d
		       AND p.archived_at IS NULL
		       AND (%s)
		       AND (%s)
		     ORDER BY e.contact_id, e.count_total DESC, e.user_id
		)
		SELECT e.user_id, e.contact_id, x.touched, e.last_inbound_at, e.last_outbound_at,
		       e.count_90d, e.in_count_90d, e.out_count_90d, e.count_total, e.live_owner
		  FROM (SELECT * FROM strongest ORDER BY count_total DESC) e
		  JOIN LATERAL (%s) x ON x.touched <= $%d AND x.touched > $%d
		 WHERE (%s)
		 ORDER BY e.count_total DESC, x.touched ASC, e.contact_id
		 LIMIT $%d`, source, workforce.LiveMemberSQL("o"),
		exchangedPos, owned, scope,
		liveExchangeClock(asOf), beforePos, afterPos, excluded, limitPos), args...)
	if err != nil {
		return nil, fmt.Errorf("search: reading quiet relationships: %w", err)
	}
	defer rows.Close()
	return scanQuietContacts(rows)
}

// quietOwnerClause narrows the candidates to one live owner, to contacts with
// none, or to neither; `o` is the contact's live owner join. The owner is read
// off the contact so the owner index can serve it.
func quietOwnerClause(read QuietRead, arg func(any) int) string {
	switch {
	case !read.Owner.IsZero():
		return fmt.Sprintf("p.owner_id = $%d AND o.id IS NOT NULL", arg(read.Owner))
	case read.Unowned:
		return "o.id IS NULL"
	default:
		return "TRUE"
	}
}

func scanQuietContacts(rows pgx.Rows) ([]QuietContact, error) {
	var out []QuietContact
	for rows.Next() {
		var c QuietContact
		var owner *ids.UUID
		e := &c.Edge
		if err := rows.Scan(&e.UserID, &e.ContactID, &e.LastAt, &e.LastInboundAt, &e.LastOutbound,
			&e.Count90d, &e.InCount90d, &e.OutCount90d, &e.CountTotal, &owner); err != nil {
			return nil, err
		}
		if owner != nil {
			c.OwnerID = *owner
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
