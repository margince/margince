// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GetDocument reads one room document by its own id, under its room's read, as
// the record seam names it. A document of a hidden room is absent.
func (s *Store) GetDocument(ctx context.Context, id ids.UUID) (crmcontracts.DealRoomDocument, error) {
	var out crmcontracts.DealRoomDocument
	err := s.inPartsRoom(ctx, documentObject, id, func(tx pgx.Tx, roomID ids.DealRoomID) (err error) {
		out, err = readDocument(ctx, tx, roomID, ids.DealRoomDocumentID{UUID: id})
		return err
	})
	return out, err
}

// GetParticipant reads one participant by its own id, under its room's read.
func (s *Store) GetParticipant(ctx context.Context, id ids.UUID) (crmcontracts.DealRoomParticipant, error) {
	var out crmcontracts.DealRoomParticipant
	err := s.inPartsRoom(ctx, participantObject, id, func(tx pgx.Tx, roomID ids.DealRoomID) (err error) {
		out, err = readParticipant(ctx, tx, roomID, ids.DealRoomParticipantID{UUID: id})
		return err
	})
	return out, err
}

// GetComment reads one comment by its own id, as its thread carries it.
func (s *Store) GetComment(ctx context.Context, id ids.UUID) (crmcontracts.DealRoomComment, error) {
	var out crmcontracts.DealRoomComment
	err := s.inPartsRoom(ctx, commentObject, id, func(tx pgx.Tx, roomID ids.DealRoomID) error {
		var threadID ids.UUID
		if err := tx.QueryRow(ctx, `SELECT thread_id FROM deal_room_comment WHERE id = $1`, id).Scan(&threadID); err != nil {
			return fmt.Errorf("find the comment's thread: %w", err)
		}
		thread, err := readThread(ctx, tx, roomID, threadID)
		if err != nil {
			return err
		}
		at := slices.IndexFunc(thread.Comments, func(c crmcontracts.DealRoomComment) bool { return ids.UUID(c.Id) == id })
		if at < 0 {
			return apperrors.ErrNotFound
		}
		out = thread.Comments[at]
		return nil
	})
	return out, err
}

// inPartsRoom finds the room a part of one belongs to and runs read inside
// that room's own read, which applies the caller's deal scope.
func (s *Store) inPartsRoom(ctx context.Context, table string, id ids.UUID, read func(pgx.Tx, ids.DealRoomID) error) error {
	if err := auth.Require(ctx, roomObject, principal.ActionRead); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		var roomID ids.DealRoomID
		err := tx.QueryRow(ctx, `SELECT room_id FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1`, id).Scan(&roomID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("find the room of %s: %w", table, err)
		}
		if _, err := readRoom(ctx, tx, roomID); err != nil {
			return err
		}
		return read(tx, roomID)
	})
}
