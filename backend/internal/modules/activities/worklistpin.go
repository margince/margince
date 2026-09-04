// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A worklist row one reader put at the top of their own day.
//
// The ranking has carried a pin level since it was written and nothing could
// ever set it, so the one override the page offers a reader did not exist.
// Every other control changes what the server THINKS — a disposition, a
// snooze, a filter. This is the only one that says "I know, and I want this
// first anyway".
//
// It lives beside activity_reader_state because it is the same kind of fact:
// what ONE reader decided about a queue row, holding for them and nobody else.
// It differs in reach — a pin names a row from any lane, not only an activity —
// which is why it is keyed on the row identity rather than on an activity id.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WorklistRowRef names one row of the assembled day.
//
// The SOURCE and the id together, because that pair is what identifies a row.
// The lanes mint ids independently — a task and a waiting message can carry the
// same underlying record's id — and the client has always spelled a row's
// identity this way. Keyed on the id alone, a pin on one row would silently
// pin another.
type WorklistRowRef struct {
	Source string
	RowID  string
}

// PinWorklistRow puts a row at the top of this reader's own day.
//
// Pinning the same row again is the same success: the reader's goal state
// already holds, and refusing would make a double-click an error.
func (s *Store) PinWorklistRow(ctx context.Context, row WorklistRowRef) error {
	// The queue's own grant. A pin reorders what the reader is shown and reveals
	// nothing they could not already read — the row was on their page — so the
	// authority it needs is the authority to have the page at all.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if row.Source == "" || row.RowID == "" {
		return fmt.Errorf(
			"activities: a pinned row is named by its source and id: %w",
			apperrors.ErrInvalidArgument)
	}
	reader, err := pinReader(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO worklist_pin (reader_id, source, row_id, set_by)
			VALUES ($%[1]d, $%[2]d, $%[3]d, $%[4]d)
			ON CONFLICT (reader_id, source, row_id) DO NOTHING`,
			arg(reader), arg(row.Source), arg(row.RowID), arg(by)),
			args...); err != nil {
			return fmt.Errorf("activities: pinning a worklist row: %w", err)
		}
		return recordPin(ctx, tx, reader, row, pinActionPinned)
	})
}

// UnpinWorklistRow gives the row back to the ranking.
//
// Idempotent: unpinning a row nobody pinned is the same success.
func (s *Store) UnpinWorklistRow(ctx context.Context, row WorklistRowRef) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	reader, err := pinReader(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		tag, err := tx.Exec(ctx, fmt.Sprintf(`
			DELETE FROM worklist_pin
			 WHERE reader_id = $%[1]d AND source = $%[2]d AND row_id = $%[3]d`,
			arg(reader), arg(row.Source), arg(row.RowID)), args...)
		if err != nil {
			return fmt.Errorf("activities: unpinning a worklist row: %w", err)
		}
		// Audited only when a pin actually went. Unpinning a row that was never
		// pinned changed nothing, and a trail entry for it would record a
		// decision nobody made.
		if tag.RowsAffected() == 0 {
			return nil
		}
		return recordPin(ctx, tx, reader, row, pinActionUnpinned)
	})
}

// PinnedRows is which rows THIS reader has pinned.
//
// Returned as the identity the caller compares against, so the assembler never
// has to re-derive what a row is called. A reader with no pins gets an empty
// set rather than an error: no pins is the ordinary state.
func (s *Store) PinnedRows(ctx context.Context, tx pgx.Tx) (map[WorklistRowRef]bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	reader, err := pinReader(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT source, row_id FROM worklist_pin WHERE reader_id = $1`, reader)
	if err != nil {
		return nil, fmt.Errorf("activities: reading worklist pins: %w", err)
	}
	defer rows.Close()
	out := map[WorklistRowRef]bool{}
	for rows.Next() {
		var ref WorklistRowRef
		if err := rows.Scan(&ref.Source, &ref.RowID); err != nil {
			return nil, fmt.Errorf("activities: reading worklist pins: %w", err)
		}
		out[ref] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading worklist pins: %w", err)
	}
	return out, nil
}

// The two verbs, as the audit trail spells them.
const (
	pinActionPinned   = "pinned"
	pinActionUnpinned = "unpinned"
)

// recordPin is the write shape's second half: the audit row, in the same
// transaction as the pin itself.
//
// No event. Every other reader-state write announces itself because somebody
// else's view depends on it — a not_sales settles a thread for the workspace,
// a dismissal changes a lane. A pin changes the ORDER of one person's own page
// and nothing else reads it, so an announcement would be a bus message no
// consumer could act on. The audit row is what "who reordered their day, and
// when" is answered from.
func recordPin(
	ctx context.Context, tx pgx.Tx, reader ids.UUID, row WorklistRowRef, action string,
) error {
	// The reader is on the row rather than only in captured_by, because the
	// entity is the row's own record and a trail reader asking "whose page did
	// this reorder" cannot get that from the subject.
	after := map[string]any{
		"worklist_pin": action,
		"source":       row.Source,
		"reader_id":    reader.String(),
	}
	_, err := storekit.AuditEvent(ctx, tx, "update", "worklist_pin", pinEntityID(row), after)
	return err
}

// pinEntityID is the audit row's subject.
//
// A pin names a row that may not BE a record — a folded group carries a
// synthetic key — so there is no record id to point at for every case. The
// row's own id is used where it is a uuid, and the nil id where it is not,
// with the source and the identity carried in the image either way.
func pinEntityID(row WorklistRowRef) ids.UUID {
	parsed, err := ids.Parse(row.RowID)
	if err != nil {
		return ids.UUID{}
	}
	return parsed
}

// pinReader is whose day this pin orders.
//
// A pin binds ONE reader, so a principal with no human behind it has no day to
// order — refused rather than written against a zero id, which would be a row
// every agent shared.
func pinReader(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}
