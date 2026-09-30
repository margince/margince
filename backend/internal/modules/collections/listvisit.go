// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// What a Live List's checks recorded, as one reader may see it: when the list
// was last checked, and what it gained and lost since this reader last opened
// it. The visit is the reader's own view state, so it carries no audit row and
// no event, the saved-view ruling; nobody else reads it.

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

// listCheck is a Live List's latest check.
type listCheck struct {
	CheckedAt time.Time
	Outcome   string
}

// listPulse is what a Live List gained and lost since the reader's last visit.
type listPulse struct {
	Since   time.Time
	Entered int
	Left    int
}

// ListVisit is a recorded visit and the one before it, nil on a first visit.
type ListVisit struct {
	ListID    ids.ListID
	VisitedAt time.Time
	Previous  *time.Time
}

// VisitList records that the signed-in user opened the list. Only a human
// visits: an agent reading through a passport carries its human's user id,
// and a visit written for it would spend that user's "since your last visit"
// on a page they never opened.
func (s *Store) VisitList(ctx context.Context, id ids.ListID) (ListVisit, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalHuman || p.UserID == (ids.UUID{}) {
		return ListVisit{}, fmt.Errorf("only a signed-in user visits a list: %w", apperrors.ErrPermissionDenied)
	}
	if err := auth.Require(ctx, listObject, principal.ActionRead); err != nil {
		return ListVisit{}, err
	}
	out := ListVisit{ListID: id}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureListVisible(ctx, tx, id); err != nil {
			return err
		}
		// GREATEST keeps a slow tab's late visit from moving the mark back.
		return tx.QueryRow(ctx, `
			WITH before AS (SELECT visited_at FROM list_visit WHERE user_id = @user_id AND list_id = @list_id)
			INSERT INTO list_visit (user_id, list_id, visited_at) VALUES (@user_id, @list_id, now())
			ON CONFLICT (user_id, list_id) DO UPDATE SET visited_at = GREATEST(list_visit.visited_at, EXCLUDED.visited_at)
			RETURNING visited_at, (SELECT visited_at FROM before)`,
			pgx.StrictNamedArgs{"user_id": p.UserID, listIDField: id}).Scan(&out.VisitedAt, &out.Previous)
	})
	return out, err
}

// observedFor adds a Live List's latest check and this reader's pulse to its
// summary. A Shortlist has neither.
func (s *Store) observedFor(ctx context.Context, out *listSummary) error {
	if out.ListType != listTypeDynamic {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var check listCheck
		err := tx.QueryRow(ctx, `SELECT evaluated_at, outcome FROM list_evaluation WHERE list_id = @list_id`,
			pgx.StrictNamedArgs{listIDField: out.ID}).Scan(&check.CheckedAt, &check.Outcome)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		default:
			out.LastCheck = &check
		}
		out.Pulse, err = pulseSince(ctx, tx, out.listRow)
		return err
	})
}

// pulseSince counts the entered and left events since the reader's last
// visit, of records the reader can see now; nil when they never visited.
func pulseSince(ctx context.Context, tx pgx.Tx, l listRow) (*listPulse, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == (ids.UUID{}) {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := observedVisibleClause(ctx, l.EntityType, arg)
	if err != nil {
		return nil, err
	}
	var pulse listPulse
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT v.visited_at,
		       count(ev.id) FILTER (WHERE ev.action = $%[1]d),
		       count(ev.id) FILTER (WHERE ev.action = $%[2]d)
		FROM list_visit v
		LEFT JOIN list_member_event ev ON ev.list_id = v.list_id AND ev.occurred_at > v.visited_at
		     AND ev.action IN ($%[1]d, $%[2]d) AND %[3]s
		WHERE v.user_id = $%[4]d AND v.list_id = $%[5]d
		GROUP BY v.visited_at`, arg(memberEntered), arg(memberLeft), visible, arg(p.UserID), arg(l.ID)),
		args...).Scan(&pulse.Since, &pulse.Entered, &pulse.Left)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pulse, nil
}

// observedVisibleClause is the test a membership event's record must pass for
// this reader, over list_member_event aliased ev: a record of the list's type
// they may see NOW, whatever they could see when it changed. A reader refused
// the record type sees none; the choice is made before the scope is built, so
// its arguments are registered only when its SQL is used.
func observedVisibleClause(ctx context.Context, entityType string, arg func(any) int) (string, error) {
	if auth.Require(ctx, entityType, principal.ActionRead) != nil {
		return "FALSE", nil
	}
	scope, err := recordScope(ctx, entityType, "e", arg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM %s e WHERE e.id = ev.entity_id AND %s)",
		pgx.Identifier{entityType}.Sanitize(), scope), nil
}
