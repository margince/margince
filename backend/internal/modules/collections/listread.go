// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// What a reader learns about one list beyond its row: how many of its members
// they may see, whether it is healthy, what uses it, why a record is on it,
// and what changed on it. Every answer here is row-scoped to the reader.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The health a list reports (List.health).
const (
	healthOK        = "ok"
	healthOwnerless = "ownerless"
	healthInvalid   = "invalid"
)

// ExportDetailListID is the system_log detail key a filtered export of a list
// records the list under, and what the list's dependencies read back.
const ExportDetailListID = "list_id"

// dependencyCap bounds the usage a list reports: the newest uses, not a log.
const dependencyCap = 20

// listSummary is a list as a reader sees it: the row, the members they may
// see, its health and whether they may change it.
type listSummary struct {
	listRow
	VisibleCount *int
	Health       string
	CanEdit      bool
	Dependencies []listDependency
}

type listDependency struct {
	Kind       string
	OccurredAt time.Time
	Actor      *string
}

// summarize counts what this reader may see of a list and judges its health.
// A Live List whose filter no longer compiles reports invalid and no count
// rather than failing the read, so the library still opens.
func (s *Store) summarize(ctx context.Context, l listRow) (listSummary, error) {
	out := listSummary{listRow: l, Health: healthOK, CanEdit: mayEditList(ctx, l)}
	count, err := s.CountMembers(ctx, l.ID)
	var pred *storekit.PredicateError
	switch {
	case errors.As(err, &pred), errors.Is(err, errNotAFilterTree):
		out.Health = healthInvalid
	case err != nil:
		return listSummary{}, err
	default:
		out.VisibleCount = &count
	}
	if out.Health != healthOK {
		return out, nil
	}
	gone, err := s.stewardGone(ctx, l)
	if err != nil {
		return listSummary{}, err
	}
	if gone {
		out.Health = healthOwnerless
	}
	return out, nil
}

// stewardGone says nobody looks after the list: it has no steward, or one who
// may no longer act.
func (s *Store) stewardGone(ctx context.Context, l listRow) (bool, error) {
	if l.StewardID == nil {
		return true, nil
	}
	if s.liveSteward == "" {
		return false, nil
	}
	var live bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM app_user u WHERE u.id = @steward_id AND "+s.liveSteward+")",
			pgx.StrictNamedArgs{stewardIDField: l.StewardID}).Scan(&live)
	})
	return !live, err
}

// Dependencies reads what uses a list. Filtered exports are the one kind in
// this release; each is a finished extraction, so none blocks a change.
func (s *Store) Dependencies(ctx context.Context, id ids.ListID) ([]listDependency, error) {
	var out []listDependency
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := readVisibleList(ctx, tx, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT occurred_at, actor_id FROM system_log
			WHERE action = 'export' AND detail ->> @detail_key = @list_id::text
			ORDER BY occurred_at DESC LIMIT @cap`,
			pgx.StrictNamedArgs{"detail_key": ExportDetailListID, listIDField: id.String(), "cap": dependencyCap})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (listDependency, error) {
			d := listDependency{Kind: "export"}
			err := row.Scan(&d.OccurredAt, &d.Actor)
			return d, err
		})
		return err
	})
	return out, err
}

// MemberExplanation says why a record is or is not on a list, for a reader
// who can see the record.
type MemberExplanation struct {
	ListType string
	Member   bool
	Eligible bool
	Clauses  *storekit.ExplainNode
	AddedBy  *string
	AddedAt  *time.Time
	Note     *string
}

// ExplainMember answers for a Live List with every clause judged by the SQL
// that decides membership, and for a Shortlist with who added the record,
// when and why. A record outside the reader's scope answers ErrNotFound.
func (s *Store) ExplainMember(ctx context.Context, listID ids.ListID, entityID ids.UUID) (MemberExplanation, error) {
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return MemberExplanation{}, err
	}
	if list.ListType == listTypeDynamic {
		engine, pred, err := s.liveFilter(ctx, list)
		if err != nil {
			return MemberExplanation{}, err
		}
		var why storekit.Explanation
		err = s.db.Tx(ctx, func(tx pgx.Tx) error {
			why, err = engine.Explain(ctx, tx, pred, entityID)
			return err
		})
		if err != nil {
			return MemberExplanation{}, err
		}
		return MemberExplanation{
			ListType: list.ListType, Member: why.Selected, Eligible: why.PassesBase, Clauses: &why.Root,
		}, nil
	}
	return s.explainChosen(ctx, list, entityID)
}

func (s *Store) explainChosen(ctx context.Context, list listRow, entityID ids.UUID) (MemberExplanation, error) {
	if err := auth.Require(ctx, list.EntityType, principal.ActionRead); err != nil {
		return MemberExplanation{}, err
	}
	out := MemberExplanation{ListType: list.ListType}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, list.EntityType, entityID); err != nil {
			return err
		}
		var addedBy string
		var addedAt time.Time
		err := tx.QueryRow(ctx, `
			SELECT added_by, created_at, note FROM list_member
			WHERE list_id = @list_id AND entity_type = @entity_type AND entity_id = @entity_id`,
			pgx.StrictNamedArgs{listIDField: list.ID, entityTypeField: list.EntityType, entityIDField: entityID}).Scan(&addedBy, &addedAt, &out.Note)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out.Member, out.AddedBy, out.AddedAt = true, &addedBy, &addedAt
		return nil
	})
	return out, err
}

// HistoryEntry is one change on a list: a Shortlist membership change, or a
// revision of its definition.
type HistoryEntry struct {
	ID         ids.UUID
	Kind       string
	OccurredAt time.Time
	Actor      string
	EntityType *string
	EntityID   *ids.UUID
	Reason     *string
	Note       *string
	Version    *int64
	Name       *string
	Definition map[string]any
	Sharing    *string
}

// History pages through a list's changes, newest first. A membership change
// about a record the reader cannot see now is left out, whatever they could
// see when it happened.
func (s *Store) History(ctx context.Context, listID ids.ListID, limit int, cursor string) ([]HistoryEntry, storekit.Page, error) {
	limit = pageSize(limit)
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var after *storekit.Cursor
	if cursor != "" {
		decoded, err := storekit.DecodeCursor(cursor)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		after = &decoded
	}
	var out []HistoryEntry
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		sql, err := historySQL(ctx, list, after, limit, arg)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanHistory)
		return err
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	if len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		next, err := storekit.EncodeCursor(last.OccurredAt, last.ID)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		return out, storekit.Page{HasMore: true, NextCursor: next}, nil
	}
	return out, storekit.Page{}, nil
}

// historySQL is the union of the list's revisions and the membership changes
// of records this reader can see, keyset-paged newest first.
func historySQL(ctx context.Context, list listRow, after *storekit.Cursor, limit int, arg func(any) int) (string, error) {
	listPos := arg(list.ID)
	// A reader refused the record type sees none of its membership changes.
	// Decided before the scope is built, so its arguments are registered only
	// when its SQL is used.
	visible := "FALSE"
	if auth.Require(ctx, list.EntityType, principal.ActionRead) == nil {
		scope, err := recordScope(ctx, list.EntityType, "e", arg)
		if err != nil {
			return "", err
		}
		visible = fmt.Sprintf("EXISTS (SELECT 1 FROM %s e WHERE e.id = ev.entity_id AND %s)",
			pgx.Identifier{list.EntityType}.Sanitize(), scope)
	}
	page := ""
	if after != nil {
		page = fmt.Sprintf("WHERE (h.occurred_at, h.id) < ($%d, $%d)", arg(after.CreatedAt), arg(after.ID))
	}
	return fmt.Sprintf(`SELECT h.* FROM (
		SELECT ev.id, 'member_' || ev.action AS kind, ev.occurred_at, ev.actor,
		       ev.entity_type, ev.entity_id, ev.reason, ev.note,
		       NULL::bigint AS version, NULL::text AS name, NULL::jsonb AS definition, NULL::text AS sharing
		FROM list_member_event ev WHERE ev.list_id = $%[1]d AND %[2]s
		UNION ALL
		SELECT r.id, 'revised', r.changed_at, r.changed_by,
		       NULL, NULL, NULL, NULL, r.version, r.name, r.definition, r.sharing
		FROM list_revision r WHERE r.list_id = $%[1]d
	) h %[3]s ORDER BY h.occurred_at DESC, h.id DESC LIMIT $%[4]d`,
		listPos, visible, page, arg(limit+1)), nil
}

func scanHistory(row pgx.CollectableRow) (HistoryEntry, error) {
	var h HistoryEntry
	err := row.Scan(&h.ID, &h.Kind, &h.OccurredAt, &h.Actor, &h.EntityType, &h.EntityID,
		&h.Reason, &h.Note, &h.Version, &h.Name, &h.Definition, &h.Sharing)
	return h, err
}

// actorNames resolves the display names of the human principals among
// actors; other principals have none.
func (s *Store) actorNames(ctx context.Context, actors []string) (map[string]string, error) {
	byUser := map[ids.UUID]string{}
	userIDs := make([]ids.UUID, 0, len(actors))
	for _, actor := range actors {
		if id, ok := principal.HumanUserID(actor); ok {
			byUser[id] = actor
			userIDs = append(userIDs, id)
		}
	}
	names := map[string]string{}
	if len(userIDs) == 0 {
		return names, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, display_name FROM app_user WHERE id = ANY(@ids)`,
			pgx.StrictNamedArgs{"ids": userIDs})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			names[byUser[id]] = name
		}
		return rows.Err()
	})
	return names, err
}
