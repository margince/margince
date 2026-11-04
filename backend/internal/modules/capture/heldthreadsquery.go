// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Page sizes for the held list. A mailbox with years of mail holds thousands of
// threads, and each one loads its parties too.
const (
	heldThreadsPageDefault = 50
	heldThreadsPageMax     = 200
)

// heldStatuses are the ledger words the list holds, and so the ones a filter
// may name.
var heldStatuses = map[string]bool{VerdictPending: true, VerdictUnsure: true, VerdictHeld: true, VerdictHeldByOwner: true}

// HeldThreadQuery narrows and pages one seat's held threads.
type HeldThreadQuery struct {
	Cursor string
	Limit  int
	// Q matches the opening message's subject, and a party's address or name.
	// A thread whose message was erased is still found by the addresses its
	// ledger row kept.
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

// heldOrder is the tuple the list sorts by, every column descending, and the
// one its cursor compares against.
const (
	heldOrder           = `(v.status = 'pending')::int, coalesce(a.occurred_at, v.created_at), v.id`
	heldOrderDescending = `(v.status = 'pending')::int DESC, coalesce(a.occurred_at, v.created_at) DESC, v.id DESC`
)

func (q HeldThreadQuery) limit() int {
	if q.Limit <= 0 {
		return heldThreadsPageDefault
	}
	return min(q.Limit, heldThreadsPageMax)
}

// filters renders the query's narrowing over the verdict `v`, the gated
// message `a` and the ungated one `present`.
func (q HeldThreadQuery) filters(ctx context.Context, tx pgx.Tx, arg func(any) int) (string, error) {
	var where strings.Builder
	if q.Status != "" {
		if !heldStatuses[q.Status] {
			return "", fmt.Errorf("status %q is not one the held list holds: %w", q.Status, apperrors.ErrInvalidArgument)
		}
		fmt.Fprintf(&where, " AND v.status = $%d", arg(q.Status))
	}
	if term := strings.TrimSpace(q.Q); term != "" {
		pattern := arg("%" + storekit.EscapeLike(term) + "%")
		fmt.Fprintf(&where, ` AND (a.subject ILIKE $%[1]d
			OR EXISTS (SELECT 1 FROM unnest(v.seen_addresses) seen WHERE seen ILIKE $%[1]d)
			OR EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id
			            AND (p.address ILIKE $%[1]d OR p.display_name ILIKE $%[1]d)))`, pattern)
	}
	if q.ContactID != nil {
		// A contact the reader may not see is a missing row, so the filter
		// cannot confirm one exists.
		if err := auth.EnsureVisible(ctx, tx, "contact", *q.ContactID); err != nil {
			return "", err
		}
		contact := arg(*q.ContactID)
		fmt.Fprintf(&where, ` AND (EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = present.id AND p.contact_id = $%[1]d)
			OR EXISTS (SELECT 1 FROM unnest(v.seen_addresses) seen
			            JOIN contact_email ce ON lower(ce.email) = lower(seen)
			           WHERE ce.contact_id = $%[1]d AND ce.archived_at IS NULL))`, contact)
	}
	if q.Cursor != "" {
		after, err := storekit.DecodeOpaque[heldCursor](q.Cursor)
		if err != nil || after.ID == ids.Nil {
			return "", &storekit.MalformedCursorError{}
		}
		rank := 0
		if after.Pending {
			rank = 1
		}
		fmt.Fprintf(&where, " AND (%s) < ($%d, $%d, $%d)", heldOrder, arg(rank), arg(after.At), arg(after.ID))
	}
	return where.String(), nil
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
