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
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// HeldSearchMax bounds a search, as the contract does.
const HeldSearchMax = 200

// HeldThreadQuery narrows and pages one seat's held threads.
type HeldThreadQuery struct {
	From, To *time.Time
	Reason   string
	Cursor   string
	Limit    *int
	// Q matches a readable message's subject or participants.
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

// cursor reads the query's cursor before any transaction opens. The first page
// has none, which is the zero cursor.
func (q HeldThreadQuery) cursor() (heldCursor, error) {
	if q.Cursor == "" {
		return heldCursor{}, nil
	}
	after, err := storekit.DecodeOpaque[heldCursor](q.Cursor)
	if err != nil || after.ID == ids.Nil || after.At.IsZero() {
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
// `a`. Every match goes through readable message content, so a filter cannot test a
// party of a message the reader may not read.
func (q HeldThreadQuery) filters(after heldCursor, arg func(any) int) string {
	var where strings.Builder
	if q.From != nil {
		where.WriteString(" AND " + occurredday.FromSQL("a", fmt.Sprintf("$%d", arg(*q.From))))
	}
	if q.To != nil {
		where.WriteString(" AND " + occurredday.BeforeSQL("a", fmt.Sprintf("$%d", arg(*q.To))))
	}
	if q.Reason != "" {
		fmt.Fprintf(&where, " AND v.kind=$%d", arg(q.Reason))
	}
	if q.Status != "" {
		fmt.Fprintf(&where, " AND v.status = $%d", arg(q.Status))
	}
	if term := strings.TrimSpace(q.Q); term != "" {
		pattern := arg("%" + storekit.EscapeLike(term) + "%")
		fmt.Fprintf(&where, ` AND (a.subject ILIKE $%[1]d
			OR EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id
			            AND (p.address ILIKE $%[1]d OR p.display_name ILIKE $%[1]d)))`, pattern)
	}
	if q.ContactID != nil {
		contact := arg(*q.ContactID)
		fmt.Fprintf(&where, ` AND EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND p.contact_id = $%d)`, contact)
	}
	if after.ID != ids.Nil {
		fmt.Fprintf(&where, " AND %s < ($%d, $%d, $%d)", heldOrder, arg(after.Pending), arg(after.At), arg(after.ID))
	}
	return where.String()
}

// heldThreadCountsTx counts the seat's whole list, whatever a page narrows to.
func heldThreadCountsTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) (HeldThreadCounts, error) {
	args := []any{seat, PendingMaxAttempts}
	bind := func(value any) int { args = append(args, value); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "held_message", bind)
	if err != nil {
		return HeldThreadCounts{}, err
	}
	var out HeldThreadCounts
	err = tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'pending'),
		       count(*) FILTER (WHERE status = 'unsure'),
		       count(*) FILTER (WHERE status = 'held'),
		       count(*) FILTER (WHERE status = 'held_by_owner'),
		       count(*) FILTER (WHERE status = 'pending' AND attempts >= $2)
		  FROM capture_thread_verdict v
		 WHERE user_id = $1 AND status NOT IN ('cleared', 'shared_by_owner') AND EXISTS (SELECT 1 FROM activity held_message WHERE `+HeldMessageSQL("v", "held_message")+` AND `+content+`)`, args...).
		Scan(&out.Pending, &out.Unsure, &out.Held, &out.HeldByOwner, &out.Stalled)
	if err != nil {
		return out, fmt.Errorf("capture: counting a seat's held threads: %w", err)
	}
	return out, nil
}

// HeldMessageSQL keeps a conversation searchable after its opening message is gone.
// Callers apply their content scope to the same eligible message set.
func HeldMessageSQL(verdict, message string) string {
	return message + `.kind='email' AND ` + message + `.archived_at IS NULL AND (` + message + `.id=` + verdict + `.first_activity_id OR ` + message + `.thread_key=` + verdict + `.thread_key)`
}
