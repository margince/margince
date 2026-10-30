// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Done on any Worklist row: "nothing more to do here, take it off my list".
//
// One reader's own judgement, like a pin. A row built from an activity keeps it
// in activity_reader_state as `handled`. Its lane hides what the reader set
// aside before it applies its own cap. Every other row keeps it in
// worklist_done. The Worklist shows it again once its subject moves after done_at.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// doneOnActivity names the rows whose id is an activity, with the kind of
// activity each one stands for.
var doneOnActivity = map[crmcontracts.WorklistItemSource]func(context.Context, pgx.Tx, ids.ActivityID) error{
	crmcontracts.WorklistItemSourceCustomerWaiting: ensureInboundMessage,
	crmcontracts.WorklistItemSourceAwaitingReply:   ensureOwnMessage,
	crmcontracts.WorklistItemSourceMeetingFollowUp: ensureMeeting,
}

// maxDoneRows bounds one press. Bulk Mark done sends what the reader ticked on
// the loaded pages, and two hundred is far past what those pages hold.
const maxDoneRows = 200

// The two acts, as the audit trail spells them.
const (
	doneActionDone     = "done"
	doneActionReopened = "reopened"
)

// MarkWorklistRowsDone takes rows off this reader's own Worklist in one
// transaction. A batch lands whole or not at all, and one Undo reverses it.
func (s *Store) MarkWorklistRowsDone(ctx context.Context, rows []WorklistRowRef) error {
	return s.judgeRows(ctx, rows, func(ctx context.Context, tx pgx.Tx, reader ids.UUID, capturedBy string, row WorklistRowRef) error {
		if shape, onActivity := doneOnActivity[crmcontracts.WorklistItemSource(row.Source)]; onActivity {
			id, err := activityRow(row)
			if err != nil {
				return err
			}
			return judgeInTx(ctx, tx, id, capturedBy, s.writeReaderState(id, stateHandled, nil, "", nil, shape))
		}
		// done_at moves forward on a second press. A row that came back and is
		// marked done again holds until its subject moves after the new press.
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		query := sprintf(`
			INSERT INTO worklist_done (reader_id, source, row_id, done_at, set_by)
			VALUES ($%[1]d, $%[2]d, $%[3]d, $%[4]d, $%[5]d)
			ON CONFLICT (reader_id, source, row_id)
			DO UPDATE SET done_at = EXCLUDED.done_at, set_by = EXCLUDED.set_by`,
			arg(reader), arg(row.Source), arg(row.RowID), arg(s.now().UTC()), arg(capturedBy))
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("activities: marking a worklist row done: %w", err)
		}
		return recordDone(ctx, tx, reader, row, doneActionDone)
	})
}

// ReopenWorklistRows puts rows back on this reader's Worklist: the undo behind
// Done. Idempotent: a row nobody marked done is the same success.
func (s *Store) ReopenWorklistRows(ctx context.Context, rows []WorklistRowRef) error {
	return s.judgeRows(ctx, rows, func(ctx context.Context, tx pgx.Tx, reader ids.UUID, capturedBy string, row WorklistRowRef) error {
		if _, onActivity := doneOnActivity[crmcontracts.WorklistItemSource(row.Source)]; onActivity {
			id, err := activityRow(row)
			if err != nil {
				return err
			}
			return judgeInTx(ctx, tx, id, capturedBy, s.pickUp(id, stateHandled))
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		query := sprintf(`
			DELETE FROM worklist_done
			 WHERE reader_id = $%[1]d AND source = $%[2]d AND row_id = $%[3]d`,
			arg(reader), arg(row.Source), arg(row.RowID))
		tag, err := tx.Exec(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("activities: reopening a worklist row: %w", err)
		}
		// Audited only when a Done actually went, so the trail records no
		// reopening nobody needed.
		if tag.RowsAffected() == 0 {
			return nil
		}
		return recordDone(ctx, tx, reader, row, doneActionReopened)
	})
}

// DoneRows is when this reader marked each of these rows done, and every group
// row they marked. Asked about the rows a page could draw, so it reads no row
// it does not need and has no cap to cut one short. A group's id is minted
// after the lanes are read, so group rows are read whole; a reader holds one
// per kind of group.
//
// No object grant: it reads the caller's own choices and nothing else.
func (s *Store) DoneRows(ctx context.Context, tx pgx.Tx, among []WorklistRowRef) (map[WorklistRowRef]time.Time, error) {
	reader, err := pinReader(ctx)
	if err != nil {
		return nil, err
	}
	sources := make([]string, 0, len(among))
	rowIDs := make([]string, 0, len(among))
	for _, ref := range among {
		sources, rowIDs = append(sources, ref.Source), append(rowIDs, ref.RowID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := sprintf(`
		SELECT source, row_id, done_at FROM worklist_done
		 WHERE reader_id = $%[1]d
		   AND (source = $%[2]d
		     OR (source, row_id) IN (SELECT * FROM unnest($%[3]d::text[], $%[4]d::text[])))`,
		arg(reader), arg(string(crmcontracts.WorklistItemSourceBatch)), arg(sources), arg(rowIDs))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading worklist rows marked done: %w", err)
	}
	defer rows.Close()
	out := map[WorklistRowRef]time.Time{}
	for rows.Next() {
		var ref WorklistRowRef
		var at time.Time
		if err := rows.Scan(&ref.Source, &ref.RowID, &at); err != nil {
			return nil, fmt.Errorf("activities: reading worklist rows marked done: %w", err)
		}
		out[ref] = at
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading worklist rows marked done: %w", err)
	}
	return out, nil
}

// judgeRows is the gate both Done writes take: a human reader, a well-formed
// batch with each row once, and one transaction for all of it.
//
// Whether the reader is shown a row is the Worklist's question, asked before
// this by attention.Service.MarkRowsDone. A row on an activity also takes the
// activity grant and the message's content gate, so an unreadable id answers 404.
func (s *Store) judgeRows(
	ctx context.Context, rows []WorklistRowRef,
	write func(context.Context, pgx.Tx, ids.UUID, string, WorklistRowRef) error,
) error {
	reader, err := pinReader(ctx)
	if err != nil {
		return err
	}
	rows = distinctRows(rows)
	if len(rows) == 0 || len(rows) > maxDoneRows {
		return httperr.Validation("rows", "out_of_range", fmt.Sprintf("a Done names 1 to %d rows", maxDoneRows))
	}
	onActivity := false
	for _, row := range rows {
		if err := validRowRef(row); err != nil {
			return err
		}
		_, backed := doneOnActivity[crmcontracts.WorklistItemSource(row.Source)]
		onActivity = onActivity || backed
	}
	if onActivity {
		if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
			return err
		}
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		for _, row := range rows {
			if err := write(ctx, tx, reader, capturedBy, row); err != nil {
				return err
			}
		}
		return nil
	})
}

// distinctRows keeps the first of each row, so a batch naming one row twice
// writes it once.
func distinctRows(rows []WorklistRowRef) []WorklistRowRef {
	seen := make(map[WorklistRowRef]bool, len(rows))
	out := make([]WorklistRowRef, 0, len(rows))
	for _, row := range rows {
		if !seen[row] {
			seen[row] = true
			out = append(out, row)
		}
	}
	return out
}

// validRowRef refuses a source the queue does not produce and an id no row
// could carry, the same two checks a pin makes.
func validRowRef(row WorklistRowRef) error {
	if !crmcontracts.WorklistItemSource(row.Source).Valid() {
		return fmt.Errorf("activities: %q is not a lane this queue produces: %w",
			row.Source, apperrors.ErrInvalidArgument)
	}
	if row.RowID == "" || len(row.RowID) > maxPinnedRowIDLen {
		return fmt.Errorf("activities: a row's id is 1 to %d characters: %w",
			maxPinnedRowIDLen, apperrors.ErrInvalidArgument)
	}
	return nil
}

// activityRow reads the activity id a message or meeting row carries.
func activityRow(row WorklistRowRef) (ids.ActivityID, error) {
	parsed, err := ids.Parse(row.RowID)
	if err != nil {
		return ids.ActivityID{}, fmt.Errorf("activities: a %s row is named by its activity id: %w",
			row.Source, apperrors.ErrInvalidArgument)
	}
	return ids.From[ids.ActivityKind](parsed), nil
}

// recordDone is the audit half of the write shape. No event, for the reason
// recordPin gives. The Done changes one reader's own page and nothing else
// reads it, so a subscriber could act on nothing.
func recordDone(ctx context.Context, tx pgx.Tx, reader ids.UUID, row WorklistRowRef, action string) error {
	after := map[string]any{
		"worklist_done": action,
		"source":        row.Source,
		fieldRowID:      row.RowID,
		"reader_id":     reader.String(),
	}
	_, err := storekit.AuditEvent(ctx, tx, "update", "worklist_done", pinEntityID(row), after)
	return err
}
