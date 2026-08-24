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
// Every read here is gated by the caller's own authority, twice over. The deal
// must be one they may READ and one their row scope admits — a brief must never
// tell a rep the stage or the price of a deal they cannot open. The Deal Room
// half additionally needs the deal_room grant, and answers "hidden" rather than
// refusing, so a rep without it gets a brief missing that line and a sentence
// saying so, rather than no brief.
//
// The reads run inside the caller's workspace transaction, which is what binds
// the tenant predicate here (database.WithWorkspaceTx); the deal id itself is
// resolved by the gated meeting read above, never taken from a request.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
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

// dealObject is the RBAC object and the row-scoped table name — the same
// string for both, as the platform's own gates spell it.
const dealObject = "deal"

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
	// The deal gate, before a single statement. The brief's own gates cover the
	// activity and the people in the room; nothing until now asked whether this
	// reader may read the DEAL these sentences are about.
	if err := auth.Require(ctx, dealObject, principal.ActionRead); err != nil {
		return nil, false, err
	}
	if err := auth.EnsureVisible(ctx, tx, dealObject, dealID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			// A deal outside this reader's row scope is not theirs to be told
			// about. The section simply has no deal half, exactly as it has
			// none for a meeting about no deal at all.
			return nil, false, nil
		}
		// Anything else is the probe itself failing, and a failed probe is not
		// a permission answer: reporting it as "no deal facts" would hide a
		// broken read behind a silence that looks deliberate.
		return nil, false, err
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
		 ORDER BY h.changed_at DESC, h.id DESC LIMIT $4`, dealID, since, now, dealMovesCap)
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
		 ORDER BY o.updated_at DESC, o.id DESC LIMIT $4`, dealID, since, now, dealMovesCap)
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

// readRoomMoves reads what the buyer said in the Deal Room.
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
	// The room follows its deal, and the deal's visibility was proved by the
	// caller above. Nothing further to check here: a reader who may see the
	// deal and holds the room grant may read what happened in its room, which
	// is the same rule the room's own store applies.
	rows, err := tx.Query(ctx, `
		SELECT c.created_at, coalesce(p.full_name, '')
		  FROM deal_room_comment c
		  JOIN deal_room r ON r.id = c.room_id
		  LEFT JOIN deal_room_participant p ON p.id = c.author_participant_id
		 WHERE r.deal_id = $1 AND c.created_at > $2 AND c.created_at <= $3
		   -- The BUYER's side only. A seller's own comment is their own act,
		   -- and reporting it back to them as something that changed while
		   -- they were away is noise in the one section that must not have any.
		   AND c.author_participant_id IS NOT NULL
		 ORDER BY 1 DESC, 2 DESC LIMIT $4`, dealID, since, now, dealMovesCap)
	if err != nil {
		return nil, false, fmt.Errorf("read the deal room's activity: %w", err)
	}
	defer rows.Close()
	var out []DealMoveIn
	for rows.Next() {
		var at time.Time
		var who string
		if err := rows.Scan(&at, &who); err != nil {
			return nil, false, fmt.Errorf("scan a deal room act: %w", err)
		}
		out = append(out, DealMoveIn{At: at, Text: roomMoveLine(who), DealID: dealID.String()})
	}
	return out, false, rowsErr(rows, "the deal room's activity")
}

// roomMoveLine words one comment of the buyer's. An unnamed participant reads
// as "the buyer" rather than as an empty name.
func roomMoveLine(who string) string {
	if who == "" {
		who = "the buyer"
	}
	return fmt.Sprintf("Since then %s commented in the Deal Room.", who)
}

// newestFirst orders the merged moves and cuts them to the section's share.
// Each read is already capped, so this bounds the UNION of three reads rather
// than any one of them.
func newestFirst(moves []DealMoveIn) []DealMoveIn {
	// Tied timestamps break on the text, so two reads of one unchanged deal
	// render the same order. Without it a stage move and an offer stamped in
	// the same transaction could swap places between requests, and one of them
	// could fall in and out of the top five.
	for i := 1; i < len(moves); i++ {
		for j := i; j > 0 && laterThan(moves[j], moves[j-1]); j-- {
			moves[j], moves[j-1] = moves[j-1], moves[j]
		}
	}
	if len(moves) > dealMovesCap {
		return moves[:dealMovesCap]
	}
	return moves
}

// laterThan orders two moves: newest first, and on a tie by text so the order
// is the same on every read.
func laterThan(a, b DealMoveIn) bool {
	if a.At.Equal(b.At) {
		return a.Text > b.Text
	}
	return a.At.After(b.At)
}

// rowsErr names which read failed while iterating, so a caller reading the log
// knows which of the three it was.
func rowsErr(rows pgx.Rows, what string) error {
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read %s: %w", what, err)
	}
	return nil
}

// ReadDealMovesForTest exposes the three reads to the integration lane.
//
// They are unexported methods on Service, which the lane cannot build without
// the whole assembly around it; what has to be checked is narrower and lower —
// that every column these statements name exists. A test that rebuilt the SQL
// would check its own copy, and a copy stays green through the change that
// breaks the original.
func ReadDealMovesForTest(
	ctx context.Context, tx pgx.Tx, dealID ids.UUID, since, now time.Time,
) ([]DealMoveIn, bool, error) {
	return (&Service{}).readDealMoves(ctx, tx, dealID, since, now)
}
