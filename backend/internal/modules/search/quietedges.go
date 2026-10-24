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
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// ExcludeEdges renders a predicate that removes candidates before the LIMIT,
// over the alias `e` this projection gives the edge row.
//
// A HOLE rather than a join, because the rows that decide it belong to another
// module and this one never imports a sibling. The caller supplies the SQL and
// the arguments through the same `arg` counter, so the fragment is numbered
// against this statement's own list. It runs before the cut: a contact
// filtered afterwards has already spent a slot.
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
	Limit        int
	// Offset skips candidates an earlier page of the same read already gave.
	Offset  int
	Exclude ExcludeEdges
}

// QuietContact is one candidate: the live colleague's edge that carries the
// most exchanges, and the contact's live owner (zero when there is none).
type QuietContact struct {
	Edge    InteractionEdge
	OwnerID ids.UUID
}

// recentMemberJoin is the live-member rule for the ceiling's own edge alias.
var recentMemberJoin = `JOIN app_user ru ON ru.id = recent.user_id AND ` + workforce.LiveMemberSQL("ru")

// QuietContacts answers "which relationships have gone silent", most exchanged
// first. It is the candidate half of the decay lane. Whether a silence is
// worth reporting stays with the relationship-change derivation in contacts.
//
// A relationship is silent when no live colleague's edge is newer than
// QuietBefore, and still current when some live colleague's edge is newer than
// TouchedAfter. Both bounds read every live member's edge, so the silence and
// its ceiling belong to the contact, not to one colleague. A pair with fewer
// than two interactions is a cold contact, not a lapsed relationship.
//
// The contact row scope applies at source, before the LIMIT, so an unreadable
// contact neither leaks through an edge row nor evicts a readable lapse.
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
	owned := quietOwnerClause(read, arg)
	exchangedPos := arg(2)
	limitPos := arg(read.Limit)
	offsetPos := arg(read.Offset)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT q.user_id, q.contact_id, q.last_at, q.last_inbound_at, q.last_outbound_at,
		       q.count_90d, q.in_count_90d, q.out_count_90d, q.count_total, q.live_owner
		  FROM (
		    SELECT DISTINCT ON (e.contact_id) e.user_id, e.contact_id, e.last_at,
		           e.last_inbound_at, e.last_outbound_at, e.count_90d, e.in_count_90d,
		           e.out_count_90d, e.count_total, o.id AS live_owner
		      FROM graph_interaction_edge e
		      %s
		      JOIN contact p ON p.id = e.contact_id AND p.archived_at IS NULL
		      LEFT JOIN app_user o ON o.id = p.owner_id AND %s
		     WHERE e.last_at <= $%d
		       AND e.count_total >= $%d
		       AND NOT EXISTS (SELECT 1 FROM graph_interaction_edge later
		                         %s
		                        WHERE later.contact_id = e.contact_id AND later.last_at > $%d)
		       AND EXISTS (SELECT 1 FROM graph_interaction_edge recent
		                     %s
		                    WHERE recent.contact_id = e.contact_id AND recent.last_at > $%d)
		       AND (%s)
		       AND (%s)
		       AND (%s)
		     ORDER BY e.contact_id, e.count_total DESC, e.last_at DESC, e.user_id
		  ) q
		 ORDER BY q.count_total DESC, q.last_at ASC, q.contact_id
		 LIMIT $%d OFFSET $%d`, liveMemberJoin, workforce.LiveMemberSQL("o"), beforePos,
		exchangedPos, laterMemberJoin, beforePos, recentMemberJoin, afterPos,
		owned, scope, excluded, limitPos, offsetPos), args...)
	if err != nil {
		return nil, fmt.Errorf("search: reading quiet relationships: %w", err)
	}
	defer rows.Close()
	return scanQuietContacts(rows)
}

// quietOwnerClause narrows the candidates to one live owner, to contacts with
// none, or to neither; `o` is the contact's live owner join.
func quietOwnerClause(read QuietRead, arg func(any) int) string {
	switch {
	case !read.Owner.IsZero():
		return fmt.Sprintf("o.id = $%d", arg(read.Owner))
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
