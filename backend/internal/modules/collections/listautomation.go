// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// What an automation rule reads of the lists it names: whether a list is
// still usable, and what one check of a Live List saw change, as the rule's
// owner may see it. The rules themselves live in the automation module, which
// compose binds to these.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RuleList is what a rule needs to know about one list the caller can find.
type RuleList struct {
	ID         ids.ListID
	Name       string
	EntityType string
	Live       bool
	Archived   bool
	Version    int64
	// Invalid says the latest check could not evaluate the filter.
	Invalid bool
}

// ObservedChange is one entered or left event a check recorded.
type ObservedChange struct {
	EventID    ids.UUID
	EntityType string
	EntityID   ids.UUID
	Action     string
}

// ListForRule reads a list the caller can find, archived or not, with the
// outcome of its latest check.
func (s *Store) ListForRule(ctx context.Context, id ids.ListID) (RuleList, error) {
	l, err := s.GetList(ctx, id)
	if err != nil {
		return RuleList{}, err
	}
	out := RuleList{
		ID: l.ID, Name: l.Name, EntityType: l.EntityType, Live: l.ListType == listTypeDynamic,
		Archived: l.ArchivedAt != nil, Version: l.Version,
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var outcome string
		err := tx.QueryRow(ctx, `SELECT outcome FROM list_evaluation WHERE list_id = @list_id`,
			pgx.StrictNamedArgs{listIDField: id}).Scan(&outcome)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		out.Invalid = outcome == CheckInvalid
		return err
	})
	return out, err
}

// ObservedChanges answers the entered or left events one check of a Live List
// recorded at checkedAt under version, of records the caller can see now:
// at most limit of them, oldest first, and how many there are in all. Only an
// ordinary check's events count; the first check after the filter changed
// reports the new filter's difference, not records moving.
func (s *Store) ObservedChanges(ctx context.Context, id ids.ListID, version int64, checkedAt time.Time, actions []string, limit int) ([]ObservedChange, int, error) {
	// The same grant a list read asks: a rule owner who lost it is told nothing
	// the list holds.
	if err := auth.Require(ctx, listObject, principal.ActionRead); err != nil {
		return nil, 0, err
	}
	var out []ObservedChange
	total := 0
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		l, err := readVisibleList(ctx, tx, id)
		if err != nil {
			return err
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		visible, err := observedVisibleClause(ctx, l.EntityType, arg)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT ev.id, ev.entity_type, ev.entity_id, ev.action, count(*) OVER ()
			FROM list_member_event ev
			WHERE ev.list_id = $%d AND ev.occurred_at = $%d AND ev.definition_version = $%d
			  AND ev.reason = $%d AND ev.action = ANY($%d) AND %s
			ORDER BY ev.id LIMIT $%d`,
			arg(id), arg(checkedAt), arg(version), arg(reasonEvaluated), arg(actions), visible, arg(limit)), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c ObservedChange
			if err := rows.Scan(&c.EventID, &c.EntityType, &c.EntityID, &c.Action, &total); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, total, err
}
