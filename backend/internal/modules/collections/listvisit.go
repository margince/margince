// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// What a Live List's checks recorded, as one reader may see it: when the list
// was last checked, and what it gained and lost since this reader last opened
// it. The visit is the reader's own view state, so it carries no audit row and
// no event, the saved-view ruling; nobody else reads it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// ListVisit is a recorded visit and the visit "since your last visit" now
// counts from, nil on a first visit.
type ListVisit struct {
	ListID    ids.ListID
	VisitedAt time.Time
	Previous  *time.Time
}

// visitBaseline is when "since your last visit" counts from, over list_visit
// aliased v. A visit recorded in the last half hour is the one still in
// progress, so the count runs from the visit before it: a page read before
// and a page read after its own visit is recorded give the same answer.
const visitBaseline = `CASE WHEN v.visited_at >= now() - interval '30 minutes'
	THEN v.previous_visited_at ELSE v.visited_at END`

// joinedCap bounds the members a list read names as joined since the visit.
const joinedCap = 500

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
		// A visit after the half hour starts a new one and keeps the last as
		// the baseline; one inside it only extends the visit in progress.
		// GREATEST keeps a slow tab's late visit from moving the mark back.
		return tx.QueryRow(ctx, `
			INSERT INTO list_visit (user_id, list_id, visited_at) VALUES (@user_id, @list_id, now())
			ON CONFLICT (user_id, list_id) DO UPDATE SET
				previous_visited_at = CASE WHEN list_visit.visited_at < now() - interval '30 minutes'
					THEN list_visit.visited_at ELSE list_visit.previous_visited_at END,
				visited_at = GREATEST(list_visit.visited_at, EXCLUDED.visited_at)
			RETURNING visited_at, previous_visited_at`,
			pgx.StrictNamedArgs{"user_id": p.UserID, listIDField: id}).Scan(&out.VisitedAt, &out.Previous)
	})
	return out, err
}

// observedFor adds each Live List's latest check and this reader's pulse to
// its summary, in one transaction and a fixed number of statements however
// many lists there are. A Shortlist has neither.
func (s *Store) observedFor(ctx context.Context, summaries []*listSummary) error {
	byID := map[ids.ListID]*listSummary{}
	byType := map[string][]ids.ListID{}
	for _, l := range summaries {
		if l.ListType == listTypeDynamic {
			byID[l.ID] = l
			byType[l.EntityType] = append(byType[l.EntityType], l.ID)
		}
	}
	if len(byID) == 0 {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lastChecks(ctx, tx, byID); err != nil {
			return err
		}
		for entityType, listIDs := range byType {
			if err := pulses(ctx, tx, entityType, listIDs, byID); err != nil {
				return err
			}
		}
		return nil
	})
}

// lastChecks reads the latest check of every list in byID.
func lastChecks(ctx context.Context, tx pgx.Tx, byID map[ids.ListID]*listSummary) error {
	listIDs := make([]ids.ListID, 0, len(byID))
	for id := range byID {
		listIDs = append(listIDs, id)
	}
	rows, err := tx.Query(ctx, `SELECT list_id, evaluated_at, outcome FROM list_evaluation WHERE list_id = ANY(@ids)`,
		pgx.StrictNamedArgs{"ids": listIDs})
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.ListID
		var check listCheck
		if err := rows.Scan(&id, &check.CheckedAt, &check.Outcome); err != nil {
			return err
		}
		byID[id].LastCheck = &check
	}
	return rows.Err()
}

// pulses counts, for each list of one record type, the entered and left
// events since the reader's last visit, of records the reader can see now. A
// list they have no earlier visit to count from gets no pulse.
func pulses(ctx context.Context, tx pgx.Tx, entityType string, listIDs []ids.ListID, byID map[ids.ListID]*listSummary) error {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == (ids.UUID{}) {
		return nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := observedVisibleClause(ctx, entityType, arg)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT v.list_id, b.since,
		       count(ev.id) FILTER (WHERE ev.action = $%[1]d),
		       count(ev.id) FILTER (WHERE ev.action = $%[2]d)
		FROM list_visit v
		CROSS JOIN LATERAL (SELECT %[6]s AS since) b
		LEFT JOIN list_member_event ev ON ev.list_id = v.list_id AND ev.occurred_at > b.since
		     AND ev.action IN ($%[1]d, $%[2]d) AND %[3]s
		WHERE v.user_id = $%[4]d AND v.list_id = ANY($%[5]d) AND b.since IS NOT NULL
		GROUP BY v.list_id, b.since`, arg(memberEntered), arg(memberLeft), visible, arg(p.UserID), arg(listIDs), visitBaseline),
		args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.ListID
		var pulse listPulse
		if err := rows.Scan(&id, &pulse.Since, &pulse.Entered, &pulse.Left); err != nil {
			return err
		}
		byID[id].Pulse = &pulse
	}
	return rows.Err()
}

// joinedSinceVisit names the members of one Live List this reader can see
// that a check saw joining since their last visit and that are members still,
// newest first and at most joinedCap: what the list page marks as new.
func (s *Store) joinedSinceVisit(ctx context.Context, l listRow) ([]ids.UUID, error) {
	p, ok := principal.Actor(ctx)
	if l.ListType != listTypeDynamic || !ok || p.UserID == (ids.UUID{}) {
		return nil, nil
	}
	var joined []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		visible, err := observedVisibleClause(ctx, l.EntityType, arg)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT ev.entity_id FROM list_visit v
			JOIN list_member_event ev ON ev.list_id = v.list_id AND ev.action = $%[1]d
			  AND ev.occurred_at > (%[2]s) AND %[3]s
			WHERE v.user_id = $%[4]d AND v.list_id = $%[5]d
			  AND EXISTS (SELECT 1 FROM list_live_member s WHERE s.list_id = ev.list_id AND s.entity_id = ev.entity_id)
			GROUP BY ev.entity_id ORDER BY max(ev.occurred_at) DESC LIMIT $%[6]d`,
			arg(memberEntered), visitBaseline, visible, arg(p.UserID), arg(l.ID), arg(joinedCap)), args...)
		if err != nil {
			return err
		}
		joined, err = storekit.ScanUUIDColumn(rows, "the members joined since the last visit")
		return err
	})
	return joined, err
}

// observedVisibleClause is the test a membership event's record must pass for
// this reader, over list_member_event aliased ev: a record of the list's type
// they may see NOW, whatever they could see when it changed. A reader refused
// the record type sees none; the choice is made before the scope is built, so
// its arguments are registered only when its SQL is used.
func observedVisibleClause(ctx context.Context, entityType string, arg func(any) int) (string, error) {
	visible := "FALSE"
	if auth.Require(ctx, entityType, principal.ActionRead) == nil {
		scope, err := recordScope(ctx, entityType, "e", arg)
		if err != nil {
			return "", err
		}
		visible = fmt.Sprintf("EXISTS (SELECT 1 FROM %s e WHERE e.id = ev.entity_id AND %s)",
			pgx.Identifier{entityType}.Sanitize(), scope)
	}
	return visible, nil
}
