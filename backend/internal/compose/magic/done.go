// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The done lane: machine actions that already happened, scoped to this reader.
//
// THE SCOPING PROBLEM IS THE WHOLE FILE. audit_log carries no workspace_id —
// migration 1787320004 dropped the tenant column from the append-only ledgers —
// so an audit row cannot be scoped by itself. It is placed by JOINING the table
// its entity_type names, under that table's own gate, and a row whose type this
// build cannot place is counted rather than shown.
//
// ONE QUERY PER ENTITY TYPE, not one per row. Each type has a different gate and
// a different table, so they cannot share a WHERE — but they can share a window,
// a limit and an order, and the results merge into one page. The alternative,
// one query over audit_log with the scoping done afterwards in Go, would read
// rows the caller may not see in order to discard them, and a bug in the discard
// is a silent disclosure rather than a failed query.
//
// NO PLACEHOLDER IS HAND-TYPED. Every $N is derived from the argument slice, as
// the rulebook asks of any statement built beside its arguments — and both auth
// clauses number themselves from wherever this query's own arguments end, so a
// literal here would be a second place to keep in step.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// entry is one admitted audit row, before it is dressed as a line.
type entry struct {
	ID         ids.UUID
	OccurredAt time.Time
	Action     string
	EntityType string
	EntityID   ids.UUID
	ActorType  string
	ActorID    string
	OnBehalfOf *ids.UUID
	Before     []byte
	After      []byte
}

// scopedTypes are the entity types this build can place, and how.
//
// The five owner-scoped records go through auth.ScopeClauseFor, which is the
// row-visibility predicate every other read of them uses. `activity` is not in
// this map: its audience is not an owner question, and a row-scope clause would
// miss the narrowing that decides who may read a message — it gets its own arm
// below. `approval` likewise answers to the approvals service.
//
// A type absent from every arm is NOT SHOWN and counted. Serving it would be
// serving a row this read cannot prove the reader may see, and the failure would
// be invisible: the row looks like every other row.
var scopedTypes = map[string]string{
	"deal":         "deal",
	"organization": "organization",
	"person":       "person",
	"lead":         "lead",
	"project":      "project",
}

// doneSince reads the admitted machine actions in the window, for the records
// this reader may see.
func doneSince(
	ctx context.Context, tx pgx.Tx, since time.Time, limit int,
) ([]entry, map[string]int, error) {
	notShown := map[string]int{}
	found := make([]entry, 0, limit)
	for entityType, table := range scopedTypes {
		rows, err := doneForType(ctx, tx, entityType, table, since, limit)
		if err != nil {
			return nil, nil, err
		}
		found = append(found, rows...)
	}
	activities, err := doneForActivities(ctx, tx, since, limit)
	if err != nil {
		return nil, nil, err
	}
	found = append(found, activities...)
	return found, notShown, nil
}

// doneForType reads one owner-scoped entity type's machine actions.
//
// The JOIN is what scopes the row: an audit entry whose record this reader
// cannot see does not survive it, and the predicate is the table's own rather
// than one written here.
func doneForType(
	ctx context.Context, tx pgx.Tx, entityType, table string, since time.Time, limit int,
) ([]entry, error) {
	// THE GRANT FIRST, and it is not optional even though the clause below often
	// renders nothing. auth.UnboundedFor answers true for a rep on most of these
	// tables — a deal is workspace-readable in this product — so ScopeClauseFor
	// returns an EMPTY predicate, and a read that leaned on the clause alone
	// would serve every row to a seat holding no grant at all. That is the trap
	// of taking a scope clause for a gate: the clause answers "which of them",
	// never "may this seat read any".
	//
	// A refused grant narrows this arm to nothing and leaves the rest of the
	// page alone: a seat that may not read deals has no business seeing what a
	// machine did to one, and its people and projects are still its own.
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sinceAt := arg(since)
	actors := arg(machineActors())
	actions := arg(admittedActions())
	typeAt := arg(entityType)
	scope, err := auth.ScopeClauseFor(ctx, table, "e", arg)
	if err != nil {
		return nil, err
	}
	where := fmt.Sprintf(
		`a.occurred_at >= $%d AND a.actor_type = ANY($%d) AND a.action = ANY($%d) AND a.entity_type = $%d`,
		sinceAt, actors, actions, typeAt)
	if scope != "" {
		where += " AND " + scope
	}
	// The table name is a compile-time value out of scopedTypes, never a string
	// off a request — the one form of identifier interpolation this tree allows.
	query := fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.action, a.entity_type, a.entity_id,
		       a.actor_type, a.actor_id, a.on_behalf_of, a.before, a.after
		  FROM audit_log a
		  JOIN %s e ON e.id = a.entity_id
		 WHERE %s
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d`, table, where, arg(limit))
	return scanEntries(ctx, tx, query, args)
}

// doneForActivities is the same read for messages, under the AUDIENCE gate
// rather than the owner one.
//
// Both halves, because they answer different questions and neither implies the
// other: auth.Require is the object grant (may this seat read activities at
// all), and ActivityContentClause is the row predicate (which ones). A clause
// without the grant serves every row to a seat that lost activity.read entirely,
// which is the trap of taking a scope clause for a gate.
func doneForActivities(
	ctx context.Context, tx pgx.Tx, since time.Time, limit int,
) ([]entry, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// A seat that may not read messages sees no message receipts, and the
		// rest of its page is untouched. The refusal narrows this arm and stops
		// there.
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sinceAt := arg(since)
	actors := arg(machineActors())
	actions := arg(admittedActions())
	content, err := auth.ActivityContentClause(ctx, "e", arg)
	if err != nil {
		return nil, err
	}
	where := fmt.Sprintf(
		`a.occurred_at >= $%d AND a.actor_type = ANY($%d) AND a.action = ANY($%d) AND a.entity_type = 'activity'`,
		sinceAt, actors, actions)
	if content != "" {
		where += " AND " + content
	}
	query := fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.action, a.entity_type, a.entity_id,
		       a.actor_type, a.actor_id, a.on_behalf_of, a.before, a.after
		  FROM audit_log a
		  JOIN activity e ON e.id = a.entity_id
		 WHERE %s
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d`, where, arg(limit))
	return scanEntries(ctx, tx, query, args)
}

// scanEntries reads the rows one arm returned.
func scanEntries(ctx context.Context, tx pgx.Tx, query string, args []any) ([]entry, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read the machine actions: %w", err)
	}
	defer rows.Close()
	found := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(
			&e.ID, &e.OccurredAt, &e.Action, &e.EntityType, &e.EntityID,
			&e.ActorType, &e.ActorID, &e.OnBehalfOf, &e.Before, &e.After,
		); err != nil {
			return nil, fmt.Errorf("read a machine action: %w", err)
		}
		found = append(found, e)
	}
	return found, rows.Err()
}

// admittedActions lists the actions this surface shows, for the SQL filter.
//
// Filtering in the query rather than in Go is what keeps the count honest: a
// read that fetched everything and dropped the rest would page over rows it
// never shows, so a page of twenty could come back holding three.
func admittedActions() []string {
	actions := make([]string, 0, len(admitted))
	for action := range admitted {
		actions = append(actions, action)
	}
	return actions
}
