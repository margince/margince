// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// What a buyer reads of the documents: the manifest the latest release froze,
// and the bytes of one document in it. The manifest never carries a storage
// locator; the download resolves it from the attachment row at request time,
// predicated on the session's room through the document row.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// buyerDocument is one document as the buyer's side needs it: what to show,
// plus the attachment the bytes come from.
type buyerDocument struct {
	crmcontracts.BuyerRoomDocument
	AttachmentID ids.UUID
}

// visibleDocuments reads the documents a buyer may see RIGHT NOW.
//
// Live rows, not a frozen manifest. A document the seller adds is shared the
// moment it is added and gone the moment it is removed, which is what a rep
// already believes when they press "Add to room" — the release cycle in
// between was a second gate behind the invitation, and its only visible effect
// was a buyer with a valid link reading an empty page.
//
// The audience rule (inTheDealsFilesArea) is what keeps this honest: a file
// archived, unlinked from the deal, or hidden from it stops being readable
// here even though its room entry survives for the seller to remove.
func visibleDocuments(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, now time.Time) ([]buyerDocument, error) {
	st, err := readStanding(ctx, tx, roomID)
	if err != nil {
		return nil, err
	}
	if !servesContent(st.access(now)) {
		return []buyerDocument{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT d.id, d.attachment_id, d.group_key, d.title, d.position,
		       a.filename, a.content_type, a.byte_size
		  FROM `+documentFrom+`
		 WHERE d.room_id = $1 AND d.archived_at IS NULL AND `+inTheDealsFilesArea+`
		 ORDER BY d.group_key, d.position, d.id`, roomID)
	if err != nil {
		return nil, fmt.Errorf("read the room's documents: %w", err)
	}
	defer rows.Close()
	out := []buyerDocument{}
	for rows.Next() {
		var doc buyerDocument
		var group string
		if err := rows.Scan(&doc.Id, &doc.AttachmentID, &group, &doc.Title, &doc.Position,
			&doc.Filename, &doc.ContentType, &doc.ByteSize); err != nil {
			return nil, fmt.Errorf("scan a room document: %w", err)
		}
		doc.GroupKey = crmcontracts.DealRoomDocumentGroup(group)
		out = append(out, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the room's documents: %w", err)
	}
	return out, nil
}

func buyerDocuments(docs []buyerDocument) []crmcontracts.BuyerRoomDocument {
	out := make([]crmcontracts.BuyerRoomDocument, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.BuyerRoomDocument)
	}
	return out
}

// BuyerDocuments lists what the buyer may read now.
func (s *Store) BuyerDocuments(ctx context.Context, sess Session) ([]crmcontracts.BuyerRoomDocument, error) {
	if sess.ID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []crmcontracts.BuyerRoomDocument
	err := s.tx(ctx, func(tx pgx.Tx) error {
		docs, err := visibleDocuments(ctx, tx, sess.RoomID, time.Now())
		if err != nil {
			return err
		}
		out = buyerDocuments(docs)
		return nil
	})
	return out, err
}

// BuyerDocumentFile is what a download needs: the file's own facts and the
// locator the attachment row holds now.
type BuyerDocumentFile struct {
	Filename    string
	ContentType *string
	ByteSize    *int64
	StorageKey  string
}

// BuyerDocumentLocator resolves one of the room's documents to its object.
//
// The lookup is the same one the list runs, so a buyer can only fetch bytes
// for something the list would have shown them: the document belongs to the
// session's room, its entry is live, and its file is still in the deal's Files
// area. A document removed from the room, or a file hidden from the deal, is
// simply not found — which is the same answer an id that never existed gets.
func (s *Store) BuyerDocumentLocator(ctx context.Context, sess Session, documentID ids.DealRoomDocumentID) (BuyerDocumentFile, error) {
	if sess.ID == ids.Nil {
		return BuyerDocumentFile{}, apperrors.ErrPermissionDenied
	}
	var out BuyerDocumentFile
	err := s.tx(ctx, func(tx pgx.Tx) error {
		docs, err := visibleDocuments(ctx, tx, sess.RoomID, time.Now())
		if err != nil {
			return err
		}
		wanted, ok := findVisible(docs, documentID)
		if !ok {
			return apperrors.ErrNotFound
		}
		err = tx.QueryRow(ctx,
			`SELECT a.storage_key FROM `+documentFrom+`
			  WHERE d.id = $1 AND d.room_id = $2 AND d.attachment_id = $3 AND `+inTheDealsFilesArea,
			documentID, sess.RoomID, wanted.AttachmentID).Scan(&out.StorageKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("locate deal room document: %w", err)
		}
		out.Filename, out.ContentType, out.ByteSize = wanted.Filename, wanted.ContentType, wanted.ByteSize
		return nil
	})
	return out, err
}

func findVisible(docs []buyerDocument, id ids.DealRoomDocumentID) (buyerDocument, bool) {
	for _, d := range docs {
		if ids.UUID(d.Id) == id.UUID {
			return d, true
		}
	}
	return buyerDocument{}, false
}

// NoteDocumentDelivered records that a live buyer seat actually received one
// document's bytes. The transport calls it AFTER the fetch succeeds.
//
// A preview seat records nothing: a seller looking at their own room as a
// buyer would drives exactly the buyer's code paths, and recording that would
// report the buyer opening documents the rep opened.
func (s *Store) NoteDocumentDelivered(
	ctx context.Context, sess Session, documentID ids.DealRoomDocumentID,
) error {
	if sess.Preview {
		return nil
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return recordEngagement(ctx, tx, sess.RoomID, sess.ParticipantID,
			&documentID, engagementDocumentDownloaded)
	})
}
