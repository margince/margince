// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type memberRow struct {
	// ID is the list_member row id for a Shortlist member, but the record's
	// own id for a Live List member (which owns no member row) — an
	// overloaded identifier with no single entity kind, so it stays untyped.
	ID     ids.UUID
	ListID ids.ListID
	// EntityType + EntityID are the polymorphic member target (any entity),
	// so the id stays untyped (rule 6).
	EntityType string
	EntityID   ids.UUID
	AddedBy    string
	CreatedAt  time.Time
	Note       *string
	// Values is what each field a Live List's filter names holds on the
	// member, as this reader may see it; nil for a Shortlist member.
	Values map[string]storekit.FieldValue
}

// MemberRead is one members read: a page by Limit and Cursor, or exactly the
// members among Only, which takes no cursor.
type MemberRead struct {
	Limit  int
	Cursor string
	Only   []ids.UUID
}

// memberIDsField names the query input Only arrives as.
const memberIDsField = "entity_id"

// checked bounds Only by the largest page and settles the page size.
func (r MemberRead) checked() (MemberRead, error) {
	if len(r.Only) == 0 {
		r.Only, r.Limit = nil, pageSize(r.Limit)
		return r, nil
	}
	if r.Cursor != "" {
		return r, &BadInputError{Field: memberIDsField, Reason: "takes no cursor"}
	}
	n := len(r.Only)
	if most := storekit.ClampLimit(&n); most != n {
		return r, &BadInputError{Field: memberIDsField, Reason: fmt.Sprintf("names at most %d records", most)}
	}
	r.Limit = len(r.Only)
	return r, nil
}

// pageSize reads a limit of 0 as none named: the web routes and the agent tool
// both send 0 when their caller names no limit, which asks for the default page.
func pageSize(limit int) int {
	if limit == 0 {
		return storekit.ClampLimit(nil)
	}
	return storekit.ClampLimit(&limit)
}

// ListMembers reads one page of a list's members that this caller may see.
// Sharing a list never widens record visibility: a Shortlist member is shown
// only when its record passes the reader's row scope, and a Live List is the
// filter evaluated inside that scope, so two readers may see different pages
// of one list.
func (s *Store) ListMembers(ctx context.Context, listID ids.ListID, limit int, cursor string) ([]memberRow, storekit.Page, error) {
	return s.readMembers(ctx, listID, MemberRead{Limit: limit, Cursor: cursor})
}

func (s *Store) readMembers(ctx context.Context, listID ids.ListID, read MemberRead) ([]memberRow, storekit.Page, error) {
	read, err := read.checked()
	if err != nil {
		return nil, storekit.Page{}, err
	}
	// GetList commits before the dynamic branch resolves its vocabulary, which
	// opens a connection of its own (see evaluateSegment).
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	if list.ListType == listTypeDynamic {
		return s.evaluateSegment(ctx, list, read)
	}
	if err := auth.Require(ctx, list.EntityType, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	var out []memberRow
	var page storekit.Page
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureListVisible(ctx, tx, listID); err != nil {
			return err
		}
		var listErr error
		out, page, listErr = s.listStaticMembers(ctx, tx, list, read)
		return listErr
	})
	return out, page, err
}

// visibleMemberClause is the row-scope test a Shortlist member's record must
// pass for this reader, over list_member aliased lm: live, and inside the
// reader's scope for the list's record type.
func visibleMemberClause(ctx context.Context, entityType string, arg func(any) int) (string, error) {
	scope, err := recordScope(ctx, entityType, "e", arg)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM %s e WHERE e.id = lm.entity_id AND e.archived_at IS NULL AND %s)",
		pgx.Identifier{entityType}.Sanitize(), scope), nil
}

// recordScope is the reader's row scope over a record of table aliased alias,
// TRUE for a reader who sees every row: the empty clause is how auth says so,
// not a missing predicate to be defaulted open.
func recordScope(ctx context.Context, table, alias string, arg func(any) int) (string, error) {
	scope, err := auth.ScopeClauseFor(ctx, table, alias, arg)
	if err != nil || scope != "" {
		return scope, err
	}
	return "TRUE", nil
}

// listStaticMembers reads the members of a Shortlist this reader may see,
// keyset-paged over the member row id.
func (s *Store) listStaticMembers(ctx context.Context, tx pgx.Tx, list listRow, read MemberRead) ([]memberRow, storekit.Page, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := visibleMemberClause(ctx, list.EntityType, arg)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	sql := fmt.Sprintf(`SELECT lm.id, lm.list_id, lm.entity_type, lm.entity_id, lm.added_by, lm.created_at, lm.note
		FROM list_member lm WHERE lm.list_id = $%d AND %s`, arg(list.ID), visible)
	if read.Only != nil {
		sql += fmt.Sprintf(" AND lm.entity_id = ANY($%d)", arg(read.Only))
	}
	limit := read.Limit
	if read.Cursor != "" {
		after, err := ids.Parse(read.Cursor)
		if err != nil {
			return nil, storekit.Page{}, &storekit.MalformedCursorError{}
		}
		sql += fmt.Sprintf(" AND lm.id > $%d", arg(after))
	}
	sql += fmt.Sprintf(" ORDER BY lm.id LIMIT $%d", arg(limit+1))
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (memberRow, error) {
		var m memberRow
		err := rowScanMember(row, &m)
		return m, err
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	if len(out) > limit {
		out = out[:limit]
		return out, storekit.Page{HasMore: true, NextCursor: out[limit-1].ID.String()}, nil
	}
	return out, storekit.Page{}, nil
}

// CountMembers answers how many members of the list this reader may see:
// never the list's whole size, which would tell them how many records they
// cannot see.
func (s *Store) CountMembers(ctx context.Context, listID ids.ListID) (int, error) {
	list, err := s.GetList(ctx, listID)
	if err != nil {
		return 0, err
	}
	if list.ListType == listTypeDynamic {
		engine, pred, err := s.liveFilter(ctx, list)
		if err != nil {
			return 0, err
		}
		var n int
		err = s.db.Tx(ctx, func(tx pgx.Tx) error {
			n, err = engine.CountMatching(ctx, tx, pred)
			return err
		})
		return n, err
	}
	if err := auth.Require(ctx, list.EntityType, principal.ActionRead); err != nil {
		return 0, err
	}
	var n int
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		visible, err := visibleMemberClause(ctx, list.EntityType, arg)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, fmt.Sprintf(
			"SELECT count(*) FROM list_member lm WHERE lm.list_id = $%d AND %s", arg(listID), visible),
			args...).Scan(&n)
	})
	return n, err
}

func rowScanMember(row pgx.Row, m *memberRow) error {
	return row.Scan(&m.ID, &m.ListID, &m.EntityType, &m.EntityID, &m.AddedBy, &m.CreatedAt, &m.Note)
}

// dynamicAddedBy marks a computed Live List member: it was never explicitly
// added, so its provenance is the filter itself, not a user.
const dynamicAddedBy = "dynamic"

// liveFilter resolves a Live List's engine and decoded filter. The engine is
// resolved BEFORE any transaction opens: SegmentEngine reaches the field
// catalog, which opens its own transaction on this store's pool, and one
// already held cannot wait on a second connection from the same pool without
// risking a deadlock under load.
func (s *Store) liveFilter(ctx context.Context, list listRow) (storekit.Query, storekit.Predicate, error) {
	engine, ok, err := s.SegmentEngine(ctx, list.EntityType)
	if err != nil {
		return storekit.Query{}, storekit.Predicate{}, err
	}
	if !ok {
		// A stored list.entity_type outside the segment set is a schema
		// invariant break, not a client error — surface it, never guess.
		return storekit.Query{}, storekit.Predicate{}, fmt.Errorf("no dynamic segment engine for entity_type %q", list.EntityType)
	}
	// A stored definition that no longer decodes is an invariant break in the
	// same class: the tree was compiled before it was stored, and this reader
	// sent only the list's id.
	pred, err := predicateFromDefinition(list.Definition)
	if err != nil {
		return storekit.Query{}, storekit.Predicate{}, fmt.Errorf("stored definition for list %s: %w", list.ID, err)
	}
	return engine, pred, nil
}

// evaluateSegment reads one page of a Live List: the stored filter evaluated
// in SQL inside the caller's row scope, keyset-paged over the record id, so
// every page of a set of any size is complete. Each member carries the values
// of the fields its filter names.
func (s *Store) evaluateSegment(ctx context.Context, list listRow, read MemberRead) ([]memberRow, storekit.Page, error) {
	engine, pred, err := s.liveFilter(ctx, list)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var after *ids.UUID
	if read.Cursor != "" {
		parsed, err := ids.Parse(read.Cursor)
		if err != nil {
			return nil, storekit.Page{}, &storekit.MalformedCursorError{}
		}
		after = &parsed
	}
	matched := read.Only
	var more bool
	var values map[ids.UUID]map[string]storekit.FieldValue
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var selectErr error
		if matched == nil {
			if matched, more, selectErr = engine.SelectPage(ctx, tx, pred, after, read.Limit); selectErr != nil {
				return selectErr
			}
		}
		values, selectErr = engine.SelectedValues(ctx, tx, pred, matched)
		return selectErr
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	out := make([]memberRow, 0, len(matched))
	for _, entityID := range matched {
		held, ok := values[entityID]
		if !ok {
			// Named in Only but not selected, or seen leaving between the
			// page and its values: not a member of this answer.
			continue
		}
		out = append(out, memberRow{
			ID: entityID, ListID: list.ID, EntityType: list.EntityType, EntityID: entityID,
			AddedBy: dynamicAddedBy, Values: held,
		})
	}
	if read.Only != nil {
		slices.SortFunc(out, func(a, b memberRow) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	}
	if more {
		return out, storekit.Page{HasMore: true, NextCursor: matched[len(matched)-1].String()}, nil
	}
	return out, storekit.Page{}, nil
}

// MemberFilter resolves a list into the narrowing a record list read applies
// for its list_id: a Shortlist's chosen members, or a Live List's filter. The
// list must be one the caller may find (else ErrNotFound) and of the read's
// record type. The narrowing carries no row scope; the read applies its own.
func (s *Store) MemberFilter(ctx context.Context, listID ids.UUID, entityType string) (storekit.ListMemberFilter, error) {
	list, err := s.GetList(ctx, ids.From[ids.ListKind](listID))
	if err != nil {
		return nil, err
	}
	if list.EntityType != entityType {
		return nil, &BadInputError{Field: listIDField, Reason: "names a list of " + list.EntityType + ", not of " + entityType}
	}
	if list.ListType == listTypeDynamic {
		engine, pred, err := s.liveFilter(ctx, list)
		if err != nil {
			return nil, err
		}
		return func(idColumn string, arg func(any) int) (string, error) {
			return engine.MatchClause(pred, idColumn, arg)
		}, nil
	}
	return func(idColumn string, arg func(any) int) (string, error) {
		return fmt.Sprintf(`EXISTS (SELECT 1 FROM list_member lm
			WHERE lm.list_id = $%d AND lm.entity_type = $%d AND lm.entity_id = %s)`,
			arg(list.ID), arg(list.EntityType), idColumn), nil
	}, nil
}
