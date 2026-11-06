// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldSearchMax bounds a search, as the contract does.
const heldSearchMax = 200

// HeldThreadQuery narrows and pages one seat's held threads.
type HeldThreadQuery struct {
	Cursor string
	Limit  *int
	// Q matches the opening message's subject, and a party's address or name.
	// A thread whose message was erased is found by the addresses its ledger
	// row kept. Erasing a contact removes their address from that row.
	Q         string
	Status    string
	ContactID *ids.UUID
}

// HeldThreadPage is one page of the list, and the counts of the whole of it.
type HeldThreadPage struct {
	Threads []HeldThread
	Next    string
	Counts  HeldThreadCounts
}

// HeldThreadCounts counts the seat's held threads by status. Stalled is the
// pending ones asked about as often as the ledger ever will.
type HeldThreadCounts struct {
	Pending, Unsure, Held, HeldByOwner, Stalled int
}

// heldCursor is where a page stops: pending first, then newest, then the row.
type heldCursor struct {
	Pending bool      `json:"p"`
	At      time.Time `json:"t"`
	ID      ids.UUID  `json:"id"`
}

// heldOrder is the tuple the cursor compares against. The list's ORDER BY sorts
// the same three values by their aliases, each descending.
const heldOrder = `(v.status = 'pending', coalesce(a.occurred_at, v.created_at), v.id)`

// ledgerReadable admits a thread to a match on its ledger's addresses. Those
// carry no content gate, so a message the reader may not read matches nothing:
// a match would name its parties. An erased message has no gate left to hold.
const ledgerReadable = `(a.id IS NOT NULL OR present.id IS NULL)`

// checked refuses a query before any transaction opens, and reads its cursor.
// The first page has none, which is the zero cursor.
func (q HeldThreadQuery) checked() (heldCursor, error) {
	if utf8.RuneCountInString(q.Q) > heldSearchMax {
		return heldCursor{}, fmt.Errorf("a search is at most %d characters: %w", heldSearchMax, apperrors.ErrInvalidArgument)
	}
	if q.Cursor == "" {
		return heldCursor{}, nil
	}
	after, err := storekit.DecodeOpaque[heldCursor](q.Cursor)
	if err != nil || after.ID == ids.Nil {
		return heldCursor{}, &storekit.MalformedCursorError{}
	}
	return after, nil
}

// ensureContactReadable answers 404 for a contact the reader may not read, as
// heldThreadPartiesTx leaves one unnamed: no grant, out of scope, or archived.
func (q HeldThreadQuery) ensureContactReadable(ctx context.Context, tx pgx.Tx) error {
	if q.ContactID == nil {
		return nil
	}
	if !auth.ReadGranted(ctx, "contact") {
		return apperrors.ErrNotFound
	}
	return auth.EnsureVisibleLive(ctx, tx, "contact", *q.ContactID)
}

// filters renders the query's narrowing over the verdict `v`, the gated message
// `a` and the ungated one `present`. Every match goes through `a` or
// ledgerReadable, so a filter cannot test a party of a withheld message.
func (q HeldThreadQuery) filters(after heldCursor, arg func(any) int) string {
	var where strings.Builder
	if q.Status != "" {
		fmt.Fprintf(&where, " AND v.status = $%d", arg(q.Status))
	}
	if term := strings.TrimSpace(q.Q); term != "" {
		pattern := arg("%" + storekit.EscapeLike(term) + "%")
		fmt.Fprintf(&where, ` AND (a.subject ILIKE $%[1]d
			OR (`+ledgerReadable+` AND EXISTS (SELECT 1 FROM unnest(v.seen_addresses) seen WHERE seen ILIKE $%[1]d))
			OR EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id
			            AND (p.address ILIKE $%[1]d OR p.display_name ILIKE $%[1]d)))`, pattern)
	}
	if q.ContactID != nil {
		contact := arg(*q.ContactID)
		fmt.Fprintf(&where, ` AND (EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND p.contact_id = $%[1]d)
			OR (`+ledgerReadable+` AND EXISTS (SELECT 1 FROM unnest(v.seen_addresses) seen
			            JOIN contact_email ce ON lower(ce.email) = lower(seen)
			           WHERE ce.contact_id = $%[1]d AND ce.archived_at IS NULL)))`, contact)
	}
	if after.ID != ids.Nil {
		fmt.Fprintf(&where, " AND %s < ($%d, $%d, $%d)", heldOrder, arg(after.Pending), arg(after.At), arg(after.ID))
	}
	return where.String()
}

// heldThreadCountsTx counts the seat's whole list, whatever a page narrows to.
func heldThreadCountsTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) (HeldThreadCounts, error) {
	var out HeldThreadCounts
	err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'unsure'),
		       count(*) FILTER (WHERE status = 'held'),
		       count(*) FILTER (WHERE status = 'held_by_owner'),
		       count(*) FILTER (WHERE status = 'pending' AND attempts >= $2)
		  FROM capture_thread_verdict
		 WHERE user_id = $1 AND status NOT IN ('cleared', 'shared_by_owner')`, seat, PendingMaxAttempts).
		Scan(&out.Pending, &out.Unsure, &out.Held, &out.HeldByOwner, &out.Stalled)
	if err != nil {
		return out, fmt.Errorf("capture: counting a seat's held threads: %w", err)
	}
	return out, nil
}
