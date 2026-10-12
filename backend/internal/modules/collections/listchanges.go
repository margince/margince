// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// What changed on a Live List since the reader's last visit, in a form a
// sentence can be built from: how many records joined and left, the most
// recent few of each by name, and how often the filter changed. It is counted
// straight from the list's history under the reader's row scope, with no
// model involved, so every record it names is one the reader can open and
// every count is one their own history would add up to.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// changeSummaryNamed is how many records of each direction the summary names.
const changeSummaryNamed = 3

// changeSummary is a Live List's changes since the reader's last visit.
type changeSummary struct {
	Since         time.Time
	Joined        changeGroup
	Left          changeGroup
	FilterChanges int
}

// changeGroup is the distinct records that moved one way, and the newest of
// them by name.
type changeGroup struct {
	Count   int
	Records []namedRecord
}

type namedRecord struct {
	ID   ids.UUID
	Name *string
}

// changesSinceVisit summarizes a Live List's changes since this reader's last
// visit; false for a Shortlist, a first visit, or a reader who is not signed in.
func (s *Store) changesSinceVisit(ctx context.Context, l listRow) (changeSummary, bool, error) {
	p, ok := principal.Actor(ctx)
	if l.ListType != listTypeDynamic || !ok || p.UserID == (ids.UUID{}) {
		return changeSummary{}, false, nil
	}
	var out changeSummary
	found := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var since *time.Time
		err := tx.QueryRow(ctx, `SELECT `+visitBaseline+` FROM list_visit v WHERE v.user_id = @user_id AND v.list_id = @list_id`,
			pgx.StrictNamedArgs{"user_id": p.UserID, listIDField: l.ID}).Scan(&since)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && since == nil) {
			return nil
		}
		if err != nil {
			return err
		}
		out, found = changeSummary{Since: *since}, true
		if err := movedSince(ctx, tx, l, *since, &out); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM list_revision r
			JOIN list_revision p ON p.list_id = r.list_id AND p.version = r.version - 1
			WHERE r.list_id = @list_id AND r.changed_at > @since AND r.definition IS DISTINCT FROM p.definition`,
			pgx.StrictNamedArgs{listIDField: l.ID, "since": *since}).Scan(&out.FilterChanges)
	})
	return out, found, err
}

// movedSince counts the distinct records the reader can see that joined and
// that left since, naming the most recent few of each.
func movedSince(ctx context.Context, tx pgx.Tx, l listRow, since time.Time, out *changeSummary) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := observedVisibleClause(ctx, l.EntityType, arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH moved AS (
			SELECT ev.action, ev.entity_id, max(ev.occurred_at) AS at
			FROM list_member_event ev
			WHERE ev.list_id = $%[1]d AND ev.occurred_at > $%[2]d AND ev.action IN ($%[3]d, $%[4]d) AND %[5]s
			GROUP BY ev.action, ev.entity_id),
		ranked AS (
			SELECT m.action, m.entity_id, count(*) OVER (PARTITION BY m.action) AS total,
			       row_number() OVER (PARTITION BY m.action ORDER BY m.at DESC, m.entity_id) AS n
			FROM moved m)
		SELECT r.action, r.total, r.entity_id, %[6]s
		FROM ranked r LEFT JOIN %[7]s t ON t.id = r.entity_id
		WHERE r.n <= $%[8]d ORDER BY r.action, r.n`,
		arg(l.ID), arg(since), arg(memberEntered), arg(memberLeft), visible,
		recordNameExpr(l.EntityType), pgx.Identifier{l.EntityType}.Sanitize(), arg(changeSummaryNamed)), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		var total int
		var record namedRecord
		if err := rows.Scan(&action, &total, &record.ID, &record.Name); err != nil {
			return err
		}
		group := &out.Joined
		if action == memberLeft {
			group = &out.Left
		}
		group.Count = total
		group.Records = append(group.Records, record)
	}
	return rows.Err()
}

// recordNameExpr is what a record of entityType is called, over alias t: the
// filter vocabulary's own name field, so a list names its records the way its
// filter matches them. A type with none is named by nobody.
func recordNameExpr(entityType string) string {
	if field, ok := standardFields[entityType][nameFilterField]; ok {
		return field.Expr
	}
	return "NULL::text"
}
