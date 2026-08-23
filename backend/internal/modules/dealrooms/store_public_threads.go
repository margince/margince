// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// The buyer's side of the conversation: reading it, opening a thread,
// replying, and deciding on a document version. Every write needs the room
// LIVE and a capability that admits it; every statement is predicated on the
// session's room.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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
		docs, err := publishedDocuments(ctx, tx, sess.RoomID, time.Now())
		if err != nil {
			return err
		}
		published := map[openapi_types.UUID]bool{}
		for _, d := range docs {
			published[d.ID] = true
		}
		if documentID != nil && !published[openapi_types.UUID(*documentID)] {
			out = []crmcontracts.DealRoomThread{}
			return nil
		}
		all, err := threadRows(ctx, tx, sess.RoomID, documentID)
		if err != nil {
			return err
		}
		out = make([]crmcontracts.DealRoomThread, 0, len(all))
		for _, th := range all {
			if th.DocumentId == nil || published[*th.DocumentId] {
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
func liveRoomForBuyerWrite(ctx context.Context, tx pgx.Tx, sess Session, needs string) (crmcontracts.DealRoom, error) {
	if sess.Preview {
		return crmcontracts.DealRoom{}, errPreviewSession
	}
	if sess.Capability == capabilityView || (needs == capabilityReviewer && sess.Capability != capabilityReviewer) {
		if needs == capabilityReviewer {
			return crmcontracts.DealRoom{}, errNotReviewer
		}
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

// errNotReviewer refuses a decision from a participant the seller did not make
// a reviewer. A confirmation carries weight in a negotiation, so it is granted
// deliberately and never by default.
var errNotReviewer = &fieldError{
	field: fieldCapability,
	code:  "reviewer_required",
	msg:   "only a reviewer can decide on a document; ask your contact to make you one",
}

// publishedDocumentVersion is the attachment the latest release names for a
// document — what a buyer's thread or decision is about. A document the buyer
// cannot see is absent.
func publishedDocumentVersion(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, documentID ids.UUID) (ids.UUID, error) {
	docs, err := publishedDocuments(ctx, tx, roomID, time.Now())
	if err != nil {
		return ids.Nil, err
	}
	published, ok := findPublished(docs, ids.From[ids.DealRoomDocumentKind](documentID))
	if !ok {
		return ids.Nil, apperrors.ErrNotFound
	}
	return ids.UUID(published.AttachmentID), nil
}

// OpenBuyerThread opens a thread as the buyer. A document thread is about a
// document the latest release names, nothing else.
func (s *Store) OpenBuyerThread(ctx context.Context, sess Session, in OpenThreadInput) (crmcontracts.DealRoomThread, error) {
	if sess.ID == ids.Nil {
		return crmcontracts.DealRoomThread{}, apperrors.ErrPermissionDenied
	}
	var out crmcontracts.DealRoomThread
	err := s.tx(ctx, func(tx pgx.Tx) error {
		room, err := liveRoomForBuyerWrite(ctx, tx, sess, capabilityComment)
		if err != nil {
			return err
		}
		if in.DocumentID != nil {
			if _, err := publishedDocumentVersion(ctx, tx, sess.RoomID, *in.DocumentID); err != nil {
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
		room, err := liveRoomForBuyerWrite(ctx, tx, sess, capabilityComment)
		if err != nil {
			return err
		}
		out, err = replyTx(ctx, tx, room, threadID, body, source, threadAuthor{participantID: &sess.ParticipantID})
		return err
	})
	return out, err
}

// ErrDecisionsRetired refuses a buyer decision on a document version.
//
// Sharing a document with a buyer is sharing it. The product no longer asks
// them to formally accept each file — a buyer reading "confirm this version"
// under a call transcript cannot tell what they would be agreeing to, and what
// they want to say about a document they say in the thread under it.
//
// The refusal lives HERE rather than in the client, because hiding a button is
// not removing an authority: a reviewer seat holds a live credential and can
// call the endpoint directly. The operation stays on the contract and the
// existing deal_room_decision rows stay readable — a decision somebody
// genuinely made is a record of what happened — but no new one is written.
//
// The WRITER is gone with it — the lint bar refuses dead code, and a retired
// path kept "just in case" is how a removed feature quietly comes back. What
// bringing approvals back would cost is written down in
// https://github.com/margince/margince/issues/2382.
var ErrDecisionsRetired = &retiredError{
	code: "document_decisions_retired",
	msg: "deal room documents are shared rather than submitted for approval: " +
		"say what you need in the document's own thread instead",
}

// DecideAsBuyer refuses every decision: see ErrDecisionsRetired.
func (s *Store) DecideAsBuyer(_ context.Context, sess Session, _ ids.UUID, _ string, _ *string) (crmcontracts.DealRoomDecision, error) {
	if sess.ID == ids.Nil {
		return crmcontracts.DealRoomDecision{}, apperrors.ErrPermissionDenied
	}
	return crmcontracts.DealRoomDecision{}, ErrDecisionsRetired
}
