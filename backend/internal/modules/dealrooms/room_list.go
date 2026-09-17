// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// ListRoomsInput narrows a page of rooms.
type ListRoomsInput struct {
	DealID *ids.DealID
	State  *string
	// ParticipantEmail narrows to rooms this address holds a live seat in.
	// Lowercased by the mapping, like every address this module compares.
	ParticipantEmail *string
	IncludeArchived  bool
	Limit            *int
	Cursor           *string
	// Sort is the caller's spec, validated against roomSortFields. Nil and the
	// documented default spelling both mean the house order.
	Sort *string
}

// roomSortFields is what a caller may order this list by: the columns the room
// itself publishes, which is what a reader can see and therefore what they can
// order by.
//
// What is NOT here carries its reason. `deal_id` and `steward_user_id` are
// references — ordering by one orders by a record the caller may not be able to
// read, which the Sort component's Expr exists for and this list has no need
// of. `welcome_message` is a body rather than an axis. `source`, `captured_by`
// and `version` are provenance about the row rather than facts about the room.
// A field left out is refused with sort_field_not_allowed, which is an honest
// answer; silently falling back to another order is not.
var roomSortFields = map[string]storekit.SortField{
	"title":      storekit.Column(fieldcatalog.TypeText),
	"state":      storekit.Column(fieldcatalog.TypeText),
	"expires_at": storekit.Column(storekit.KindTimestamp),
	"closed_at":  storekit.Column(storekit.KindTimestamp),
	"created_at": storekit.Column(storekit.KindTimestamp),
	"updated_at": storekit.Column(storekit.KindTimestamp),
}

// ListRooms pages the Deal Rooms whose deals the caller can see.
func (s *Store) ListRooms(ctx context.Context, in ListRoomsInput) ([]crmcontracts.DealRoom, storekit.Page, error) {
	if err := auth.Require(ctx, roomObject, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	var (
		out  []crmcontracts.DealRoom
		page storekit.Page
	)
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, page, err = roomPage(ctx, tx, in)
		if err != nil {
			return err
		}
		// Answered here as well as by id, because the room screen reads its
		// room from the LIST — a field only GetRoom filled would be absent
		// exactly where the button that needs it is drawn.
		return StampPreviewAvailable(ctx, tx, out)
	})
	return out, page, err
}

func roomPage(ctx context.Context, tx pgx.Tx, in ListRoomsInput) ([]crmcontracts.DealRoom, storekit.Page, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The sort FIRST: it may bind parameters of its own and the cursor clause
	// is rendered from it, so both count through one counter.
	sorted, err := storekit.ParseListSort(ctx, in.Sort, roomSortFields, arg)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	scope, err := dealScopeClause(ctx, arg)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	// The deal is an EXISTS rather than a JOIN, and that is what lets the house
	// keyset name its columns: `deal` carries a `created_at` and an `id` of its
	// own, so with it in the FROM list the cursor tuple — alias-free by design,
	// since every other list it serves reads one table — is ambiguous and
	// Postgres refuses the statement. Nothing outside selects a deal column;
	// the join was only ever the room's row scope.
	where := []string{storekit.SQLf(
		`EXISTS (SELECT 1 FROM deal d WHERE d.id = r.deal_id AND (%s))`, scope)}
	if !in.IncludeArchived {
		where = append(where, "r.archived_at IS NULL")
	}
	if in.DealID != nil {
		where = append(where, storekit.SQLf("r.deal_id = $%d", arg(in.DealID)))
	}
	if in.State != nil {
		where = append(where, storekit.SQLf("r.state = $%d", arg(*in.State)))
	}
	if in.ParticipantEmail != nil {
		where = append(where, storekit.SQLf(
			`EXISTS (SELECT 1 FROM deal_room_participant p
			          WHERE p.room_id = r.id AND p.email = $%d AND p.revoked_at IS NULL AND NOT p.preview)`,
			arg(*in.ParticipantEmail)))
	}
	if in.Cursor != nil && *in.Cursor != "" {
		// Through the sort's own keyset, which refuses a token minted under a
		// different ordering rather than resuming on an axis this page is not
		// ordered by.
		keyset, err := sorted.KeysetClause(*in.Cursor, arg)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		where = append(where, keyset)
	}

	size := storekit.ClampLimit(in.Limit)
	rows, err := tx.Query(ctx, storekit.SQLf(
		`SELECT %s%s FROM deal_room r WHERE %s%s LIMIT %d`,
		roomColumns, sorted.CursorKeySuffix(), strings.Join(where, " AND "),
		sorted.OrderBy(), size+1), args...)
	if err != nil {
		return nil, storekit.Page{}, fmt.Errorf("list deal rooms: %w", err)
	}
	defer rows.Close()

	out := make([]crmcontracts.DealRoom, 0, size)
	var cursorKeys []*string
	for rows.Next() {
		room, key, err := scanRoomPage(rows, sorted)
		if err != nil {
			return nil, storekit.Page{}, fmt.Errorf("scan deal room: %w", err)
		}
		out = append(out, room)
		cursorKeys = append(cursorKeys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, storekit.Page{}, fmt.Errorf("read deal rooms: %w", err)
	}

	var page storekit.Page
	if len(out) > size {
		out = out[:size]
		createdAt, id := roomKey(out[size-1])
		next, err := sorted.EncodePageCursor(cursorKeys[size-1], createdAt, id)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		page = storekit.Page{HasMore: true, NextCursor: next}
	}
	return out, page, nil
}

// scanRoomPage scans one row of a sorted page: the published columns, and the
// sort's cursor key where the sort appends one.
func scanRoomPage(row pgx.Rows, sorted *storekit.ListSort) (crmcontracts.DealRoom, *string, error) {
	var key *string
	var dest []any
	if sorted.CursorKeySuffix() != "" {
		dest = append(dest, &key)
	}
	out, err := scanRoom(storekit.TrailingColumns{Row: row, Dest: dest})
	return out, key, err
}

// ArchiveRoom ends the room. Buyer access goes with it; the releases stay, so
// what a buyer was shown remains answerable after the room itself is gone.
func (s *Store) ArchiveRoom(ctx context.Context, id ids.DealRoomID, ifVersion *int64) (crmcontracts.DealRoom, error) {
	if err := auth.Require(ctx, roomObject, principal.ActionDelete); err != nil {
		return crmcontracts.DealRoom{}, err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.DealRoom{}, err
	}
	var out crmcontracts.DealRoom
	err := s.tx(ctx, func(tx pgx.Tx) error {
		current, err := readRoom(ctx, tx, id)
		if err != nil {
			return err
		}
		// Ending a room takes buyer access away, so an archived deal is no
		// reason to refuse it.
		if err := ensureDealRetractable(ctx, tx, current); err != nil {
			return err
		}
		if current.ArchivedAt != nil {
			out = current
			return nil
		}
		if err := archiveRoomTx(ctx, tx, current, ifVersion); err != nil {
			return err
		}
		out, err = readRoom(ctx, tx, id)
		return err
	})
	return out, err
}

func archiveRoomTx(ctx context.Context, tx pgx.Tx, room crmcontracts.DealRoom, ifVersion *int64) error {
	id := ids.UUID(room.Id)
	p := storekit.NewPatch()
	// state and archived_at move together: the schema's CHECK requires them to
	// agree, so setting one alone would be refused by the database rather than
	// leaving a row that reads live to the state machine and archived to every
	// list query.
	p.Set("state", string(room.State), stateArchived)
	p.Set("archived_at", nil, time.Now().UTC())

	if err := p.ApplyGuarded(ctx, tx, roomObject, id, ifVersion); err != nil {
		return fmt.Errorf("archive deal room: %w", err)
	}
	// Revoking sessions is what makes archiving mean anything to a buyer: the
	// room row alone is invisible to them, and a live session would keep
	// answering with the last release until it expired on its own.
	if _, err := tx.Exec(ctx,
		`UPDATE deal_room_session SET revoked_at = now()
		  WHERE room_id = $1 AND revoked_at IS NULL`, id); err != nil {
		return fmt.Errorf("revoke deal room sessions: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "archive", roomObject, id, p.Before(), p.After())
	if err != nil {
		return fmt.Errorf("audit deal room archive: %w", err)
	}
	archived := crmcontracts.PublicEventDealRoomArchived{DealId: room.DealId}
	if err := storekit.EmitEvent(ctx, tx, auditID, id, archived); err != nil {
		return fmt.Errorf("emit deal_room.archived: %w", err)
	}
	return nil
}
