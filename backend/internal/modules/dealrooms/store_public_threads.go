// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// The buyer's side of the conversation: reading it, opening a thread and
// replying. Every write needs the room LIVE and a capability that admits it;
// every statement is predicated on the session's room.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BuyerThreads lists the conversation. Empty while the room serves no content.
//
// A thread on a document is shown only while the latest release NAMES that
// document: a thread the seller opened on a file they have not yet published
// would otherwise hand the buyer the file's existence and the seller's words
// about it. Room-level threads are always shown.
func (s *Store) BuyerThreads(ctx context.Context, sess Session, documentID *ids.UUID) ([]crmcontracts.DealRoomThread, error) {
	if sess.ID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []crmcontracts.DealRoomThread
	err := s.tx(ctx, func(tx pgx.Tx) error {
		docs, err := visibleDocuments(ctx, tx, sess.RoomID, time.Now())
		if err != nil {
			return err
		}
		visible := map[openapi_types.UUID]bool{}
		for _, d := range docs {
			visible[d.Id] = true
		}
		if documentID != nil && !visible[openapi_types.UUID(*documentID)] {
			out = []crmcontracts.DealRoomThread{}
			return nil
		}
		all, err := threadRows(ctx, tx, sess.RoomID, documentID)
		if err != nil {
			return err
		}
		out = make([]crmcontracts.DealRoomThread, 0, len(all))
		for _, th := range all {
			if th.DocumentId == nil || visible[*th.DocumentId] {
				out = append(out, th)
			}
		}
		return nil
	})
	return out, err
}

// liveRoomForBuyerWrite settles what every buyer write needs: the room is
// live (paused refuses reversibly, the finished states as a record) and the
// capability admits writing. Returns the room as the transaction bodies want it.
func liveRoomForBuyerWrite(ctx context.Context, tx pgx.Tx, sess Session) (crmcontracts.DealRoom, error) {
	if sess.Preview {
		return crmcontracts.DealRoom{}, errPreviewSession
	}
	if sess.Capability == capabilityView {
		return crmcontracts.DealRoom{}, errViewerCannotWrite
	}
	st, err := readStanding(ctx, tx, sess.RoomID)
	if err != nil {
		return crmcontracts.DealRoom{}, err
	}
	switch access := st.access(time.Now()); access {
	case accessLive:
	case accessPaused:
		return crmcontracts.DealRoom{}, pausedForBuyer()
	default:
		return crmcontracts.DealRoom{}, notContentEditable(access)
	}
	if _, err := storekit.LockRow(ctx, tx, roomObject, sess.RoomID.UUID, storekit.LiveOnly); err != nil {
		return crmcontracts.DealRoom{}, err
	}
	var room crmcontracts.DealRoom
	room.Id = openapi_types.UUID(sess.RoomID.UUID)
	if err := tx.QueryRow(ctx, `SELECT deal_id FROM deal_room WHERE id = $1`, sess.RoomID).Scan(&room.DealId); err != nil {
		return crmcontracts.DealRoom{}, fmt.Errorf("read deal room for a buyer write: %w", err)
	}
	return room, nil
}

// requireBuyerVisibleDocument refuses a document this buyer cannot see: one
// that never was in the room, one the seller has taken back, or one whose file
// has left the deal's Files area. Absent rather than forbidden, which is the
// same answer an id that never existed gets.
func requireBuyerVisibleDocument(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, documentID ids.UUID) error {
	docs, err := visibleDocuments(ctx, tx, roomID, time.Now())
	if err != nil {
		return err
	}
	if _, ok := findVisible(docs, ids.From[ids.DealRoomDocumentKind](documentID)); !ok {
		return apperrors.ErrNotFound
	}
	return nil
}

// OpenBuyerThread opens a thread as the buyer. A document thread is about a
// document the latest release names, nothing else.
func (s *Store) OpenBuyerThread(ctx context.Context, sess Session, in OpenThreadInput) (crmcontracts.DealRoomThread, error) {
	if sess.ID == ids.Nil {
		return crmcontracts.DealRoomThread{}, apperrors.ErrPermissionDenied
	}
	var out crmcontracts.DealRoomThread
	err := s.tx(ctx, func(tx pgx.Tx) error {
		room, err := liveRoomForBuyerWrite(ctx, tx, sess)
		if err != nil {
			return err
		}
		if in.DocumentID != nil {
			if err := requireBuyerVisibleDocument(ctx, tx, sess.RoomID, *in.DocumentID); err != nil {
				return err
			}
		}
		out, err = openThreadTx(ctx, tx, room, in, threadAuthor{participantID: &sess.ParticipantID})
		return err
	})
	return out, err
}

// ReplyAsBuyer appends to an open thread of the session's room.
func (s *Store) ReplyAsBuyer(ctx context.Context, sess Session, threadID ids.UUID, body, source string) (crmcontracts.DealRoomThread, error) {
	if sess.ID == ids.Nil {
		return crmcontracts.DealRoomThread{}, apperrors.ErrPermissionDenied
	}
	var out crmcontracts.DealRoomThread
	err := s.tx(ctx, func(tx pgx.Tx) error {
		room, err := liveRoomForBuyerWrite(ctx, tx, sess)
		if err != nil {
			return err
		}
		// A thread is only the buyer's to speak in while its document is still
		// theirs to see. The list already hides a thread whose document left
		// the room; without the same rule here, a buyer holding the id from an
		// earlier read could go on talking in it — and replyTx hands back the
		// whole conversation, so the refusal has to happen before that.
		if err := ensureThreadStillVisible(ctx, tx, sess, threadID); err != nil {
			return err
		}
		out, err = replyTx(ctx, tx, room, threadID, body, source, threadAuthor{participantID: &sess.ParticipantID})
		return err
	})
	return out, err
}

// ensureThreadStillVisible refuses a thread the buyer's own list would not show
// them: one about a document that has left the room, or whose file has left the
// deal's Files area. A thread about the room as a whole is always theirs.
//
// ErrNotFound, not a permission error: the buyer is entitled to be here, and
// telling them a specific thread exists but is closed to them says more about
// the seller's material than the refusal needs to.
func ensureThreadStillVisible(ctx context.Context, tx pgx.Tx, sess Session, threadID ids.UUID) error {
	var documentID *ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT document_id FROM deal_room_thread WHERE id = $1 AND room_id = $2`,
		threadID, sess.RoomID).Scan(&documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read the thread's document: %w", err)
	}
	if documentID == nil {
		return nil
	}
	return requireBuyerVisibleDocument(ctx, tx, sess.RoomID, *documentID)
}
