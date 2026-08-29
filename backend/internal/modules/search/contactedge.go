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

// ContactEdge is one observed contact↔contact pair: two external people seen
// on the same captured activities. Canonically ordered (A < B) because
// co-participation has no direction — neither wrote "to" the other through us.
type ContactEdge struct {
	PersonA    ids.UUID
	PersonB    ids.UUID
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

// contactPair is one canonical (person_a < person_b) key.
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
		    SELECT DISTINCT person_id
		      FROM activity_participant
		     WHERE activity_id = ANY($1) AND person_id IS NOT NULL
		)
		SELECT DISTINCT x.person_id, y.person_id
		  FROM activity_participant x
		  JOIN activity_participant y
		    ON y.activity_id = x.activity_id AND y.person_id > x.person_id
		 WHERE x.activity_id = ANY($1) AND x.person_id IS NOT NULL
		 UNION
		SELECT e.person_a, e.person_b
		  FROM graph_contact_edge e
		 WHERE e.person_a IN (SELECT person_id FROM present)
		    OR e.person_b IN (SELECT person_id FROM present)`, activityIDs)
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
		    SELECT DISTINCT person_a, person_b
		      FROM unnest($1::uuid[], $2::uuid[]) AS t(person_a, person_b)
		),
		folded AS (
		    SELECT t.person_a, t.person_b,
		           max(a.occurred_at) AS last_at,
		           count(DISTINCT a.id) FILTER (WHERE a.occurred_at >= `+window+`) AS count_90d,
		           count(DISTINCT a.id) AS count_total
		      FROM target t
		      JOIN activity_participant pa
		        ON pa.person_id = t.person_a AND pa.role IN `+interactionRoles+`
		      JOIN activity_participant pb
		        ON pb.activity_id = pa.activity_id
		       AND pb.person_id = t.person_b AND pb.role IN `+interactionRoles+`
		      JOIN activity a
		        ON a.id = pa.activity_id AND a.archived_at IS NULL`+audienceWorkspaceOnly+`
		     GROUP BY t.person_a, t.person_b
		)
		INSERT INTO graph_contact_edge AS e
		    (person_a, person_b, last_at, count_90d, count_total, computed_at)
		SELECT f.person_a, f.person_b, f.last_at, f.count_90d, f.count_total, now()
		  FROM folded f
		ON CONFLICT (person_a, person_b) DO UPDATE SET
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
		 USING unnest($1::uuid[], $2::uuid[]) AS t(person_a, person_b)
		 WHERE e.person_a = t.person_a AND e.person_b = t.person_b
		   AND NOT EXISTS (
		       SELECT 1
		         FROM activity_participant pa
		         JOIN activity_participant pb ON pb.activity_id = pa.activity_id
		         JOIN activity a ON a.id = pa.activity_id AND a.archived_at IS NULL`+audienceWorkspaceOnly+`
		        WHERE pa.person_id = t.person_a AND pa.role IN `+interactionRoles+`
		          AND pb.person_id = t.person_b AND pb.role IN `+interactionRoles+`)`,
		first, second); err != nil {
		return fmt.Errorf("search: pruning contact edges that lost their evidence: %w", err)
	}
	return nil
}

// contactPairsForPerson names every pair one contact can appear in: partners
// on their shared activities, plus every existing row either end of which is
// them — so a merge or archive prunes rows the participants alone would no
// longer name.
func contactPairsForPerson(ctx context.Context, tx pgx.Tx, personID ids.UUID) ([]contactPair, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT least(p.person_id, o.person_id), greatest(p.person_id, o.person_id)
		  FROM activity_participant p
		  JOIN activity_participant o
		    ON o.activity_id = p.activity_id
		   AND o.person_id IS NOT NULL AND o.person_id <> p.person_id
		 WHERE p.person_id = $1
		 UNION
		SELECT person_a, person_b FROM graph_contact_edge
		 WHERE person_a = $1 OR person_b = $1`, personID)
	if err != nil {
		return nil, fmt.Errorf("search: resolving the contacts a person is observed with: %w", err)
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

// ContactEdgesForPerson answers "who else does this contact talk to", most
// recent first, capped by the caller's density budget.
//
// Two gates, both load-bearing, both at source. The caller must be able to
// read the ANCHOR — an edge list would otherwise disclose that a withheld
// contact exists and who they correspond with. And the OTHER end of every row
// is filtered through the caller's person row scope before the LIMIT, so a
// peer the caller may not read is absent rather than a blank slot, and
// unreadable peers cannot evict readable ones from the budget.
func ContactEdgesForPerson(ctx context.Context, tx pgx.Tx, personID ids.UUID, limit int) ([]PeerEdge, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, "person", personID); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	anchorPos := arg(personID)
	scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	limitPos := arg(limit)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT e.person_a, e.person_b, e.last_at, e.count_90d, e.count_total, p.id, p.full_name
		  FROM graph_contact_edge e
		  JOIN person p
		    ON p.id = CASE WHEN e.person_a = $%d THEN e.person_b ELSE e.person_a END
		   AND p.archived_at IS NULL
		 WHERE (e.person_a = $%d OR e.person_b = $%d) AND (%s)
		 ORDER BY e.last_at DESC, e.person_a, e.person_b
		 LIMIT $%d`, anchorPos, anchorPos, anchorPos, scope, limitPos), args...)
	if err != nil {
		return nil, fmt.Errorf("search: reading who a contact is observed with: %w", err)
	}
	defer rows.Close()
	var out []PeerEdge
	for rows.Next() {
		var pe PeerEdge
		if err := rows.Scan(&pe.Edge.PersonA, &pe.Edge.PersonB, &pe.Edge.LastAt,
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
// It does NOT probe each person's visibility, on the same contract
// EdgesForPeople states: the caller assembled the set from its own row-scoped
// read, and both ends of every returned row are members of that set, so
// nothing leaves the audience the caller already established.
func ContactEdgesAmong(ctx context.Context, tx pgx.Tx, people []ids.UUID) ([]ContactEdge, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(people) < 2 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT e.person_a, e.person_b, e.last_at, e.count_90d, e.count_total
		  FROM graph_contact_edge e
		 WHERE e.person_a = ANY($1) AND e.person_b = ANY($1)
		 ORDER BY e.last_at DESC, e.person_a, e.person_b`, people)
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
		if err := rows.Scan(&e.PersonA, &e.PersonB, &e.LastAt, &e.Count90d, &e.CountTotal); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
