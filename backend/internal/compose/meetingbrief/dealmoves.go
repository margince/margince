// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// What happened to the DEAL since the reader last spoke to this room.
//
// "Since you last spoke" used to read only what people SAID — captured claims
// and conversations. The three things that most often changed while nobody was
// talking are not sentences: the deal moved stage, the offer was revised, and
// the buyer worked the Deal Room. A rep who walks into a meeting not knowing
// the offer was re-issued on Tuesday is exactly the failure the section exists
// to prevent, and the section was silent about it while looking complete.
//
// Every read here is bounded by the deal the brief already resolved and gated
// by the caller's own authority over it: the deal-room half runs only for a
// caller who may read a deal room, and answers empty rather than refusing, so a
// rep without that grant gets a brief missing that line rather than no brief.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// DealMoveIn is one thing that happened to the deal itself, already worded.
// The brief only ever renders these in date order, so the reader is the text
// plus what to cite; nothing downstream needs the parts back.
type DealMoveIn struct {
	At   time.Time
	Text string
	// DealID is what the sentence cites: these moves are facts about the deal,
	// and the deal page is where a reader goes to see them.
	DealID string
}

// dealMovesCap bounds the section's share of the brief. Five is what fits in
// the "since you last spoke" block beside the claims and conversations without
// pushing them off the screen.
const dealMovesCap = 5

// readDealMoves gathers the deal's own movements strictly after `since` and not
// after `now`. A future-dated row has not happened yet, exactly as the claim
// filter says.
func (s *Service) readDealMoves(
	ctx context.Context, tx pgx.Tx, dealID ids.UUID, since, now time.Time,
) ([]DealMoveIn, bool, error) {
	if dealID == (ids.UUID{}) {
		return nil, false, nil
	}
	moves, err := readStageMoves(ctx, tx, dealID, since, now)
	if err != nil {
		return nil, false, err
	}
	offers, err := readOfferMoves(ctx, tx, dealID, since, now)
	if err != nil {
		return nil, false, err
	}
	room, roomHidden, err := readRoomMoves(ctx, tx, dealID, since, now)
	if err != nil {
		return nil, false, err
	}
	// Built into its own slice rather than appended onto `moves`: appending to
	// one read's slice and calling the result something else shares that
	// read's backing array, and the sort below would then write through it.
	all := make([]DealMoveIn, 0, len(moves)+len(offers)+len(room))
	all = append(all, moves...)
	all = append(all, offers...)
	all = append(all, room...)
	return newestFirst(all), roomHidden, nil
}

// readStageMoves reads where the deal went. The stage NAME is read through the
// join rather than restated, so a renamed stage reads correctly in an old
// brief.
func readStageMoves(
	ctx context.Context, tx pgx.Tx, dealID ids.UUID, since, now time.Time,
) ([]DealMoveIn, error) {
	rows, err := tx.Query(ctx, `
		SELECT h.changed_at, coalesce(f.name, ''), coalesce(t.name, '')
		  FROM deal_stage_history h
		  LEFT JOIN stage f ON f.id = h.from_stage_id
		  LEFT JOIN stage t ON t.id = h.to_stage_id
		 WHERE h.deal_id = $1 AND h.changed_at > $2 AND h.changed_at <= $3
		 ORDER BY h.changed_at DESC LIMIT $4`, dealID, since, now, dealMovesCap)
	if err != nil {
		return nil, fmt.Errorf("read the deal's stage moves: %w", err)
	}
	defer rows.Close()
	var out []DealMoveIn
	for rows.Next() {
		var at time.Time
		var from, to string
		if err := rows.Scan(&at, &from, &to); err != nil {
			return nil, fmt.Errorf("scan a stage move: %w", err)
		}
		text := fmt.Sprintf("Since then the deal moved to %s.", to)
		if from != "" {
			text = fmt.Sprintf("Since then the deal moved from %s to %s.", from, to)
		}
		out = append(out, DealMoveIn{At: at, Text: text, DealID: dealID.String()})
	}
	return out, rowsErr(rows, "the deal's stage moves")
}

// readOfferMoves reads what was quoted. The revision number is the fact a rep
// needs — "they are looking at revision 3" — and the status says whether it
// actually went out.
func readOfferMoves(
	ctx context.Context, tx pgx.Tx, dealID ids.UUID, since, now time.Time,
) ([]DealMoveIn, error) {
	rows, err := tx.Query(ctx, `
		SELECT o.updated_at, o.offer_number, o.revision, o.status
		  FROM offer o
		 WHERE o.deal_id = $1 AND o.updated_at > $2 AND o.updated_at <= $3
		   AND o.status <> 'draft'
		 ORDER BY o.updated_at DESC LIMIT $4`, dealID, since, now, dealMovesCap)
	if err != nil {
		return nil, fmt.Errorf("read the deal's offers: %w", err)
	}
	defer rows.Close()
	var out []DealMoveIn
	for rows.Next() {
		var at time.Time
		var number, status string
		var revision int
		if err := rows.Scan(&at, &number, &revision, &status); err != nil {
			return nil, fmt.Errorf("scan an offer: %w", err)
		}
		out = append(out, DealMoveIn{
			At:     at,
			Text:   fmt.Sprintf("Since then offer %s revision %d was %s.", number, revision, status),
			DealID: dealID.String(),
		})
	}
	return out, rowsErr(rows, "the deal's offers")
}

// readRoomMoves reads what the buyer did in the Deal Room: their comments and
// their decisions on documents.
//
// It runs only for a caller holding deal_room read, and SAYS SO when it does
// not: a rep without that grant gets a brief with no room line rather than a
// refusal, but a silent one would read exactly like a deal whose buyer has done
// nothing. The bool is that admission, and the brief renders it.
func readRoomMoves(
	ctx context.Context, tx pgx.Tx, dealID ids.UUID, since, now time.Time,
) ([]DealMoveIn, bool, error) {
	if err := auth.Require(ctx, "deal_room", principal.ActionRead); err != nil {
		return nil, true, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT c.created_at, 'comment', coalesce(p.full_name, '')
		  FROM deal_room_comment c
		  JOIN deal_room_thread th ON th.id = c.thread_id
		  JOIN deal_room r ON r.id = th.room_id
		  LEFT JOIN deal_room_participant p ON p.id = c.participant_id
		 WHERE r.deal_id = $1 AND c.created_at > $2 AND c.created_at <= $3
		   AND c.participant_id IS NOT NULL
		UNION ALL
		SELECT d.created_at, d.kind, coalesce(p.full_name, '')
		  FROM deal_room_decision d
		  JOIN deal_room r ON r.id = d.room_id
		  LEFT JOIN deal_room_participant p ON p.id = d.participant_id
		 WHERE r.deal_id = $1 AND d.created_at > $2 AND d.created_at <= $3
		 ORDER BY 1 DESC LIMIT $4`, dealID, since, now, dealMovesCap)
	if err != nil {
		return nil, false, fmt.Errorf("read the deal room's activity: %w", err)
	}
	defer rows.Close()
	var out []DealMoveIn
	for rows.Next() {
		var at time.Time
		var kind, who string
		if err := rows.Scan(&at, &kind, &who); err != nil {
			return nil, false, fmt.Errorf("scan a deal room act: %w", err)
		}
		out = append(out, DealMoveIn{At: at, Text: roomMoveLine(kind, who), DealID: dealID.String()})
	}
	return out, false, rowsErr(rows, "the deal room's activity")
}

// roomMoveLine words one act of the buyer's. An unnamed participant reads as
// "the buyer" rather than as an empty name.
func roomMoveLine(kind, who string) string {
	if who == "" {
		who = "the buyer"
	}
	switch kind {
	case "confirm_version":
		return fmt.Sprintf("Since then %s confirmed a document in the Deal Room.", who)
	case "request_changes":
		return fmt.Sprintf("Since then %s asked for changes to a Deal Room document.", who)
	default:
		return fmt.Sprintf("Since then %s commented in the Deal Room.", who)
	}
}

// newestFirst orders the merged moves and cuts them to the section's share.
// Each read is already capped, so this bounds the UNION of three reads rather
// than any one of them.
func newestFirst(moves []DealMoveIn) []DealMoveIn {
	for i := 1; i < len(moves); i++ {
		for j := i; j > 0 && moves[j].At.After(moves[j-1].At); j-- {
			moves[j], moves[j-1] = moves[j-1], moves[j]
		}
	}
	if len(moves) > dealMovesCap {
		return moves[:dealMovesCap]
	}
	return moves
}

// rowsErr names which read failed while iterating, so a caller reading the log
// knows which of the three it was.
func rowsErr(rows pgx.Rows, what string) error {
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read %s: %w", what, err)
	}
	return nil
}
