// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

// The notification centre: one seat's whole notice history, and the one act
// that clears it.
//
// As against UnreadFor, which is the attention lane — what is still waiting,
// capped, newest first. A reader who has settled a notice can still need to
// find it ("what did that automation tell me on Tuesday"), and a lane that
// forgets the moment somebody clicks is a lane they learn not to clear.
//
// A rep's own stage moves are not news to the rep who made them, and the centre
// honours that filter rather than becoming the place those notices reappear.
//
// A MUTED CLASS IS THE OTHER WAY ROUND, and the difference is the point. The
// lane declines it and the badge does not count it — the reader asked not to be
// interrupted — but the centre LISTS it, because a preference governs delivery
// and never existence, and somewhere has to be the place a switched-off notice
// is still findable. Going to look is not being interrupted.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// centrePageBound caps one page of the centre, and is also its default.
//
// Not storekit.ClampLimit: its ceiling is the contract's list cap of 200 rows,
// which sizes a record list somebody exports. The centre is a panel somebody
// opens, and fifty lines is already more than one scroll of it.
const centrePageBound = 50

// CentreItem is one line of the centre: the notice, and whether its reader has
// already answered it.
//
// ReadAt rather than a bool, because the centre renders history: "you read this
// on Tuesday" is what tells a reader whether they are looking at something they
// have already dealt with, and a bool cannot say when.
type CentreItem struct {
	Notice
	ReadAt *time.Time
}

// CentrePage is one window of the centre, with the badge beside it.
//
// UnreadCount is the reader's whole unread LANE and not this page's share of it:
// a badge that fell as somebody scrolled would be counting the wrong thing.
//
// The lane and not the list, so it is the narrower of the two: a class the
// reader switched off is listed below and not counted here, because the count
// is the interruption they declined and the line is the record they did not.
type CentrePage struct {
	Items       []CentreItem
	NextCursor  string
	UnreadCount int
}

// ListFor answers the CALLING contact's notices, read and unread, newest first.
//
// The reader is the bound principal and not a parameter — another contact's
// history cannot be expressed — and a caller with no contact behind it is
// refused with the permission sentinel, which the centre renders as a withheld
// panel.
//
// Keyset over (created_at, id), never an offset: notices arrive while somebody
// is reading, and an offset page would show them a line twice for every one
// that landed above it.
func (s *Store) ListFor(ctx context.Context, limit int, cursor string) (CentrePage, error) {
	seat, err := actingSeat(ctx, "reading your notifications")
	if err != nil {
		return CentrePage{}, err
	}
	pageSize := boundedPageSize(limit)
	// One more than the page, so the answer knows whether to mint a token
	// without a second count over the remainder.
	args := []any{seat, pageSize + 1}
	keyset := ""
	if cursor != "" {
		// A token this package did not mint is the CALLER's mistake, so the
		// sentinel travels out unwrapped: httperr answers 422 for it, and a
		// wrap would land it in the 500 lane with an admin looking for an
		// outage that is not there.
		position, decodeErr := storekit.DecodeCursor(cursor)
		if decodeErr != nil {
			return CentrePage{}, decodeErr
		}
		args = append(args, position.CreatedAt, position.ID)
		keyset = storekit.SQLf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}

	var page CentrePage
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The badge is its own statement rather than a window over the page,
		// because a window can only count rows the page carried: on the second
		// page of a long history it would report the tail and on an empty page
		// nothing at all. Both statements run in one transaction, so the count
		// and the window describe the same reader — a notice landing between
		// them is the next open's business.
		muted, err := mutedFor(ctx, tx, seat)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT count(*) FROM notice
			 WHERE recipient_user_id = $1 AND read_at IS NULL AND %s`, inTheReadersLane(2)),
			seat, muted).Scan(&page.UnreadCount); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, kind, subject, body, target_type, target_id, created_at, origin, read_at,
			       coalesce(dedupe_key, '')
			  FROM notice
			 WHERE recipient_user_id = $1 AND `+notTheReadersOwnStageMove+keyset+`
			 ORDER BY created_at DESC, id DESC
			 LIMIT $2`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		items, err := scanCentreItems(rows)
		if err != nil {
			return err
		}
		page.Items, page.NextCursor, err = trimToPage(items, pageSize)
		return err
	}); err != nil {
		return CentrePage{}, fmt.Errorf("notices: listing your notifications: %w", err)
	}
	return page, nil
}

// MarkAllRead settles every unread notice the calling contact holds and answers
// how many of them the reader could SEE.
//
// The two numbers differ, and which one leaves this function is the whole point.
// The statement settles everything — self-made stage moves and muted classes
// included — because both are hidden from the badge, so leaving them unread
// would strand rows in the partial unread index that no act of the reader's
// could ever clear. The ANSWER counts the lines the reader's BADGE was
// carrying, because a figure a reader is ever shown may not exceed the one it
// replaces: "3 notifications marked read" over a badge that read two is a lie
// to them. The ledger entry keeps the true figure.
//
// ONE ledger entry, and no announcement. A seat clearing a month of notices is
// one act by one contact, and the per-notice alternative puts a hundred
// notice.read events on the bus for a single tap — a fan-out every consumer
// pays for to learn what one number already says. The cost of that ruling is
// stated where it is ratified (backend/gates/writeshape_test.go): a seat's own
// webhook subscription hears notice.read for a single settle and nothing for
// this one, so a subscriber tracking read state has to re-read the lane.
//
// Idempotent, like settling one notice twice: a second tap on a lane already
// clear moves no row and writes no entry for a change nobody made.
func (s *Store) MarkAllRead(ctx context.Context) (int, error) {
	seat, err := actingSeat(ctx, "settling your notifications")
	if err != nil {
		return 0, err
	}
	var settled, shown int
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// RETURNING the visibility predicate rather than counting twice: one
		// statement settles the rows and says, per row, whether the reader was
		// ever shown it. A second SELECT under the same predicate would be a
		// second copy of the question, and at READ COMMITTED it could answer
		// about a row this statement had just changed.
		muted, txErr := mutedFor(ctx, tx, seat)
		if txErr != nil {
			return txErr
		}
		rows, txErr := tx.Query(ctx, fmt.Sprintf(`
			UPDATE notice SET read_at = now()
			 WHERE recipient_user_id = $1 AND read_at IS NULL
			RETURNING %s`, inTheReadersLane(2)), seat, muted)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		for rows.Next() {
			var theBadgeWasCountingIt bool
			if err := rows.Scan(&theBadgeWasCountingIt); err != nil {
				return err
			}
			settled++
			if theBadgeWasCountingIt {
				shown++
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if settled == 0 {
			return nil
		}
		// The entity is the SEAT, the way this module already audits a change
		// to their notification settings: the act settled a set of notices and
		// no single one of them, so there is no notice id to name — and naming
		// the seat's id under entity_type "notice" would leave a ledger reader
		// resolving a notice that does not exist. What changed is a fact about
		// the contact.
		//
		// The images are the seat's unread standing on either side of the
		// act, which is the state this write moved: the statement's own
		// predicate IS the unread set, so the rows it touched are what the
		// seat held unread, and after it they hold none. The notice ids are
		// not the image — a month of them is an unbounded list in one ledger
		// row, and a reader asking what this entry covers is asking how much
		// moved.
		//
		// The count is the rows that MOVED and not the reader-facing figure:
		// the ledger is what an operator reads to know what the write did,
		// and it says so without a reader subtracting one image from the other.
		_, txErr = storekit.Audit(ctx, tx, "update", "user", seat,
			map[string]any{"unread": settled},
			map[string]any{"unread": 0, "read_all": true, "count": settled})
		return txErr
	}); err != nil {
		return 0, fmt.Errorf("notices: settling your notifications: %w", err)
	}
	return shown, nil
}

// boundedPageSize is how many lines one call may answer with. An absent or
// nonsense limit is the full page rather than one row: the centre's caller is a
// panel, and a zero it forgot to fill in means "show me the notifications".
func boundedPageSize(limit int) int {
	if limit < 1 || limit > centrePageBound {
		return centrePageBound
	}
	return limit
}

// scanCentreItems reads the window, including the one row past the page.
func scanCentreItems(rows pgx.Rows) ([]CentreItem, error) {
	items := []CentreItem{}
	for rows.Next() {
		var item CentreItem
		// Both halves are nullable and the table pairs them, so either
		// arriving alone is a row the constraint should have refused.
		var targetType *string
		var targetID *ids.UUID
		var dedupeKey string
		if err := rows.Scan(&item.ID, &item.Kind, &item.Subject, &item.Body,
			&targetType, &targetID, &item.CreatedAt, &item.Origin, &item.ReadAt, &dedupeKey); err != nil {
			return nil, err
		}
		if targetType != nil && targetID != nil {
			item.Target = Target{Type: *targetType, ID: *targetID}
		}
		// A notice asking for a decision opens the decision, not the record it
		// is about: the record answers nothing, and the decision names it.
		if approval, ok := approvalOfNoticeKey(item.Kind, dedupeKey); ok {
			item.Target = Target{Type: TargetApproval, ID: approval}
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// trimToPage cuts the read-ahead row off and mints the token that continues
// after the last one kept.
//
// The token ANSWERS an error rather than coming back empty, because a caller
// pairs it with "there is more": an empty token beside a remainder is a page
// the client can ask for and never receive. The failure is reachable —
// time.Time refuses an instant outside years 0000-9999 and timestamptz reaches
// year 294276, so one absurd-but-storable created_at is enough.
func trimToPage(items []CentreItem, limit int) ([]CentreItem, string, error) {
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	last := items[len(items)-1]
	next, err := storekit.EncodeCursor(last.CreatedAt, last.ID)
	if err != nil {
		return nil, "", err
	}
	return items, next, nil
}
