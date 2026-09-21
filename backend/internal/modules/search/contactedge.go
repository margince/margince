// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// ContactEdge is one observed contact↔contact pair: two external contacts seen
// on the same captured activities. Canonically ordered (A < B) because
// co-participation has no direction — neither wrote "to" the other through us.
type ContactEdge struct {
	ContactA   ids.UUID
	ContactB   ids.UUID
	LastAt     time.Time
	Count90d   int
	CountTotal int
}

// StrengthOf scores the pair at an instant through the same §4 kernel every
// other edge uses. No directed counts exist for a pair we only co-observed,
// and none are invented: the reciprocity term simply has nothing to say.
func (e ContactEdge) StrengthOf(now time.Time) relstrength.Score {
	last := e.LastAt
	return relstrength.Compute(relstrength.Inputs{
		LastInteraction: &last,
		Count90d:        e.Count90d,
	}, now)
}

// contactPair is one canonical (contact_a < contact_b) key.
type contactPair struct {
	a ids.UUID
	b ids.UUID
}

// affectedContactPairs resolves which contact edges the named activities can
// affect: every distinct external pair on their participant rows, plus every
// EXISTING edge either end of which is still on the activities. The second arm
// is the relink case: when a participant was already repointed away before the
// event arrives, the former pair cannot be named from the rows alone — but its
// surviving member still is, so the stale edge is refolded and pruned instead
// of standing until the nightly rebuild.
func affectedContactPairs(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) ([]contactPair, error) {
	rows, err := tx.Query(ctx, `
		WITH present AS (
		    SELECT DISTINCT contact_id
		      FROM activity_participant
		     WHERE activity_id = ANY($1) AND contact_id IS NOT NULL
		)
		SELECT DISTINCT x.contact_id, y.contact_id
		  FROM activity_participant x
		  JOIN activity_participant y
		    ON y.activity_id = x.activity_id AND y.contact_id > x.contact_id
		 WHERE x.activity_id = ANY($1) AND x.contact_id IS NOT NULL
		 UNION
		SELECT e.contact_a, e.contact_b
		  FROM graph_contact_edge e
		 WHERE e.contact_a IN (SELECT contact_id FROM present)
		    OR e.contact_b IN (SELECT contact_id FROM present)`, activityIDs)
	if err != nil {
		return nil, fmt.Errorf("search: resolving the contact pairs an activity touches: %w", err)
	}
	defer rows.Close()
	return scanContactPairs(rows)
}

func scanContactPairs(rows pgx.Rows) ([]contactPair, error) {
	var out []contactPair
	for rows.Next() {
		var pr contactPair
		if err := rows.Scan(&pr.a, &pr.b); err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}

// recomputeContactPairs re-folds the named pairs from the base tables in one
// statement and deletes the ones that no longer qualify — the same
// atomic-with-capture shape recomputePairs keeps, for the same reason.
//
// The audience rule and the role set are graph_interaction_edge's own, applied unchanged:
// a limited-audience activity contributes NOTHING here, exactly as it
// contributes nothing to graph_interaction_edge — who talked to whom on a
// limited thread is content, and this table is readable without reading it.
func recomputeContactPairs(ctx context.Context, tx pgx.Tx, pairs []contactPair) error {
	if len(pairs) == 0 {
		return nil
	}
	first := make([]ids.UUID, 0, len(pairs))
	second := make([]ids.UUID, 0, len(pairs))
	for _, p := range pairs {
		first = append(first, p.a)
		second = append(second, p.b)
	}
	window := fmt.Sprintf("now() - interval '%d days'", relstrength.WindowDays)

	if _, err := tx.Exec(ctx, `
		WITH target AS (
		    SELECT DISTINCT contact_a, contact_b
		      FROM unnest($1::uuid[], $2::uuid[]) AS t(contact_a, contact_b)
		),
		folded AS (
		    SELECT t.contact_a, t.contact_b,
		           max(a.occurred_at) AS last_at,
		           count(DISTINCT `+graphInteractionUnit+`) FILTER (WHERE a.occurred_at >= `+window+`) AS count_90d,
		           count(DISTINCT `+graphInteractionUnit+`) AS count_total
		      FROM target t
		      JOIN activity_participant pa
		        ON pa.contact_id = t.contact_a AND pa.role IN `+interactionRoles+`
		      JOIN activity_participant pb
		        ON pb.activity_id = pa.activity_id
		       AND pb.contact_id = t.contact_b AND pb.role IN `+interactionRoles+`
		      JOIN activity a
		        ON a.id = pa.activity_id AND a.archived_at IS NULL`+audienceWorkspaceOnly+`
		     GROUP BY t.contact_a, t.contact_b
		)
		INSERT INTO graph_contact_edge AS e
		    (contact_a, contact_b, last_at, count_90d, count_total, computed_at)
		SELECT f.contact_a, f.contact_b, f.last_at, f.count_90d, f.count_total, now()
		  FROM folded f
		ON CONFLICT (contact_a, contact_b) DO UPDATE SET
		    last_at     = EXCLUDED.last_at,
		    count_90d   = EXCLUDED.count_90d,
		    count_total = EXCLUDED.count_total,
		    computed_at = EXCLUDED.computed_at`,
		first, second); err != nil {
		return fmt.Errorf("search: recomputing contact edges: %w", err)
	}

	// A pair whose last shared activity was archived or limited loses its row.
	// An edge outliving its evidence would keep asserting an acquaintance the
	// record no longer supports.
	if _, err := tx.Exec(ctx, `
		DELETE FROM graph_contact_edge e
		 USING unnest($1::uuid[], $2::uuid[]) AS t(contact_a, contact_b)
		 WHERE e.contact_a = t.contact_a AND e.contact_b = t.contact_b
		   AND NOT EXISTS (
		       SELECT 1
		         FROM activity_participant pa
		         JOIN activity_participant pb ON pb.activity_id = pa.activity_id
		         JOIN activity a ON a.id = pa.activity_id AND a.archived_at IS NULL`+audienceWorkspaceOnly+`
		        WHERE pa.contact_id = t.contact_a AND pa.role IN `+interactionRoles+`
		          AND pb.contact_id = t.contact_b AND pb.role IN `+interactionRoles+`)`,
		first, second); err != nil {
		return fmt.Errorf("search: pruning contact edges that lost their evidence: %w", err)
	}
	return nil
}

// contactPairsForContact names every pair one contact can appear in: partners
// on their shared activities, plus every existing row either end of which is
// them — so a merge or archive prunes rows the participants alone would no
// longer name.
func contactPairsForContact(ctx context.Context, tx pgx.Tx, contactID ids.UUID) ([]contactPair, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT least(p.contact_id, o.contact_id), greatest(p.contact_id, o.contact_id)
		  FROM activity_participant p
		  JOIN activity_participant o
		    ON o.activity_id = p.activity_id
		   AND o.contact_id IS NOT NULL AND o.contact_id <> p.contact_id
		 WHERE p.contact_id = $1
		 UNION
		SELECT contact_a, contact_b FROM graph_contact_edge
		 WHERE contact_a = $1 OR contact_b = $1`, contactID)
	if err != nil {
		return nil, fmt.Errorf("search: resolving the contacts a contact is observed with: %w", err)
	}
	defer rows.Close()
	return scanContactPairs(rows)
}

// PeerEdge is one observed acquaintance of an anchor contact, named in the
// same scoped read so a caller drawing the peer needs no second query.
type PeerEdge struct {
	Peer     ids.UUID
	FullName string
	Edge     ContactEdge
}

// ContactEdgesForContact answers "who else does this contact talk to", most
// recent first, capped by the caller's density budget.
//
// Two gates, both load-bearing, both at source. The caller must be able to
// read the ANCHOR — an edge list would otherwise disclose that a withheld
// contact exists and who they correspond with. And the OTHER end of every row
// is filtered through the caller's contact row scope before the LIMIT, so a
// peer the caller may not read is absent rather than a blank slot, and
// unreadable peers cannot evict readable ones from the budget.
func ContactEdgesForContact(ctx context.Context, tx pgx.Tx, contactID ids.UUID, limit int) ([]PeerEdge, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, "contact", contactID); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	anchorPos := arg(contactID)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	limitPos := arg(limit)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT e.contact_a, e.contact_b, e.last_at, e.count_90d, e.count_total, p.id, p.full_name
		  FROM graph_contact_edge e
		  JOIN contact p
		    ON p.id = CASE WHEN e.contact_a = $%d THEN e.contact_b ELSE e.contact_a END
		   AND p.archived_at IS NULL
		 WHERE (e.contact_a = $%d OR e.contact_b = $%d) AND (%s)
		 ORDER BY e.last_at DESC, e.contact_a, e.contact_b
		 LIMIT $%d`, anchorPos, anchorPos, anchorPos, scope, limitPos), args...)
	if err != nil {
		return nil, fmt.Errorf("search: reading who a contact is observed with: %w", err)
	}
	defer rows.Close()
	var out []PeerEdge
	for rows.Next() {
		var pe PeerEdge
		if err := rows.Scan(&pe.Edge.ContactA, &pe.Edge.ContactB, &pe.Edge.LastAt,
			&pe.Edge.Count90d, &pe.Edge.CountTotal, &pe.Peer, &pe.FullName); err != nil {
			return nil, err
		}
		out = append(out, pe)
	}
	return out, rows.Err()
}

// ContactEdgesAmong answers which of a SET of contacts are observed together —
// what an account picture needs to stop being hub-and-spoke through our team.
//
// It does NOT probe each contact's visibility, on the same contract
// EdgesForContacts states: the caller assembled the set from its own row-scoped
// read, and both ends of every returned row are members of that set, so
// nothing leaves the audience the caller already established.
func ContactEdgesAmong(ctx context.Context, tx pgx.Tx, contacts []ids.UUID) ([]ContactEdge, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(contacts) < 2 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT e.contact_a, e.contact_b, e.last_at, e.count_90d, e.count_total
		  FROM graph_contact_edge e
		 WHERE e.contact_a = ANY($1) AND e.contact_b = ANY($1)
		 ORDER BY e.last_at DESC, e.contact_a, e.contact_b`, contacts)
	if err != nil {
		return nil, fmt.Errorf("search: reading which of a contact set know each other: %w", err)
	}
	defer rows.Close()
	return scanContactEdges(rows)
}

func scanContactEdges(rows pgx.Rows) ([]ContactEdge, error) {
	var out []ContactEdge
	for rows.Next() {
		var e ContactEdge
		if err := rows.Scan(&e.ContactA, &e.ContactB, &e.LastAt, &e.Count90d, &e.CountTotal); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
