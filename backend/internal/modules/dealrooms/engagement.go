// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// What a buyer did in the room, and what the seller is told about it.
//
// `deal_room_session.last_seen_at` already says WHETHER somebody has been here.
// This says WHAT they did — signed in, downloaded this document — which is the
// question a rep asks before a call: did they read the contract.
//
// Two rules hold the whole file.
//
// A PREVIEW IS NEVER ENGAGEMENT. A seller looking at their own room as a buyer
// would drives exactly the buyer's code paths, which is the point of it; if
// that were recorded, the Access panel would report the buyer opening documents
// the rep opened, and a rep would ring a buyer about a document nobody outside
// the company has seen. Every write here refuses a preview seat.
//
// THE RECORD COMMITS WITH THE ACT IT RECORDS. The write rides the same
// transaction as the read that earned it, so the seller's panel and the buyer's
// download can never disagree about whether the file was taken. A separate
// best-effort write would fail silently and leave a rep ringing a buyer about a
// document the record says they never opened.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// The acts this table records. A sign-in is about the room; a download is
// about one document. The schema's CHECK holds the pairing.
const (
	engagementSignedIn           = "signed_in"
	engagementDocumentDownloaded = "document_downloaded"
)

// recordEngagement writes one act inside the caller's transaction.
//
// Not audited, and deliberately: the audit log is the record of what CHANGED,
// and a buyer reading their own room changes nothing. The act is already
// covered by the session audit row on the way in; this table is the seller's
// reading of it, not a second ledger.
func recordEngagement(
	ctx context.Context, tx pgx.Tx,
	roomID ids.DealRoomID, participantID ids.DealRoomParticipantID,
	documentID *ids.DealRoomDocumentID, kind string,
) error {
	var doc *ids.UUID
	if documentID != nil {
		id := documentID.UUID
		doc = &id
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO deal_room_engagement (room_id, participant_id, document_id, kind)
		 VALUES ($1, $2, $3, $4)`, roomID, participantID, doc, kind); err != nil {
		return fmt.Errorf("record deal room engagement: %w", err)
	}
	return nil
}

// noteBuyerEngagement records one act of a live buyer seat inside the caller's
// transaction. A preview seat writes nothing: see the file header.
func noteBuyerEngagement(
	ctx context.Context, tx pgx.Tx, sess Session,
	documentID *ids.DealRoomDocumentID, kind string,
) error {
	if sess.Preview {
		return nil
	}
	return recordEngagement(ctx, tx, sess.RoomID, sess.ParticipantID, documentID, kind)
}

// participantEngagement is what the Access panel says about one seat beyond
// "they have been here": how many documents they took, and when they last did
// anything at all.
type participantEngagement struct {
	Downloads int
	Documents []string
}

// engagementByParticipant reads one room's activity, grouped by seat.
//
// One query for the whole roster rather than one per seat: the panel draws
// every participant at once, and a per-row read turns a roster of ten into ten
// round trips for a number.
func engagementByParticipant(
	ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID,
) (map[ids.UUID]participantEngagement, error) {
	rows, err := tx.Query(ctx, `
		SELECT e.participant_id, count(*) FILTER (WHERE e.kind = $2),
		       coalesce(array_agg(DISTINCT d.title)
		                FILTER (WHERE e.kind = $2 AND d.title IS NOT NULL), '{}')
		  FROM deal_room_engagement e
		  LEFT JOIN deal_room_document d ON d.id = e.document_id
		 WHERE e.room_id = $1
		 GROUP BY e.participant_id`, roomID, engagementDocumentDownloaded)
	if err != nil {
		return nil, fmt.Errorf("read deal room engagement: %w", err)
	}
	defer rows.Close()
	out := map[ids.UUID]participantEngagement{}
	for rows.Next() {
		var id ids.UUID
		var seen participantEngagement
		if err := rows.Scan(&id, &seen.Downloads, &seen.Documents); err != nil {
			return nil, fmt.Errorf("scan deal room engagement: %w", err)
		}
		out[id] = seen
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read deal room engagement: %w", err)
	}
	return out, nil
}

// withEngagement stamps the roster's rows with what each seat has done.
func withEngagement(
	participants []crmcontracts.DealRoomParticipant,
	seen map[ids.UUID]participantEngagement,
) []crmcontracts.DealRoomParticipant {
	for i := range participants {
		act, ok := seen[ids.UUID(participants[i].Id)]
		if !ok {
			continue
		}
		downloads := act.Downloads
		participants[i].DownloadCount = &downloads
		if len(act.Documents) > 0 {
			docs := act.Documents
			participants[i].DocumentsDownloaded = &docs
		}
	}
	return participants
}
