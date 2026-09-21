// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Reading an audit image back into values a patch can write.
//
// Split from the reversal itself because it answers a different question: that
// file decides WHAT to put back, this one turns stored jsonb into the Go types
// the columns take. The awkwardness lives here — a DATE column is rendered
// "2026-08-27" rather than as an instant, which time.Time's own JSON decoder
// will not accept — so the reversal beside it reads as the decision it is.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if err := json.Unmarshal(nullIfEmpty(a), &x); err != nil {
		return false
	}
	if err := json.Unmarshal(nullIfEmpty(b), &y); err != nil {
		return false
	}
	return fmt.Sprintf("%v", x) == fmt.Sprintf("%v", y)
}

func nullIfEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return raw
}

// currentCorrectedValues reads what the deal holds NOW in the columns the
// correction touched, as json so it compares like-for-like against the audit
// images.
func currentCorrectedValues(ctx context.Context, tx pgx.Tx, c DealCorrection) (map[string]json.RawMessage, error) {
	var raw []byte
	if err := tx.QueryRow(ctx, `
		SELECT to_jsonb(d) FROM deal d WHERE d.id = $1`, c.DealID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("read the deal's current values: %w", err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, fmt.Errorf("decode the deal's current values: %w", err)
	}
	out := make(map[string]json.RawMessage, len(c.Fields))
	for _, field := range c.Fields {
		out[field] = row[field]
	}
	return out, nil
}

// correctionAfterImage is what the correction WROTE, used to tell "a contact has
// edited this since" from "the value is simply what the correction made it".
func correctionAfterImage(ctx context.Context, tx pgx.Tx, c DealCorrection) (map[string]json.RawMessage, error) {
	var raw []byte
	// Bound to the record type it means. The id is the deal's own audit row —
	// recordCorrection stores the id storekit wrote for the deal update in the
	// same transaction — so this predicate refuses nothing that should arrive.
	// What it buys is that a read by a bare id off a stored column cannot
	// return another record's image if that id is ever wrong: the correction
	// would otherwise compare a deal against something that is not one, and
	// the audit trail is one table for every record the product keeps.
	err := tx.QueryRow(ctx,
		`SELECT after FROM audit_log WHERE id = $1 AND entity_type = 'deal'`,
		c.AuditLogID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &CorrectionReversalError{
			Reason: "the change this correction recorded is no longer in the record's history",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read the correction's applied image: %w", err)
	}
	var after map[string]json.RawMessage
	if err := json.Unmarshal(raw, &after); err != nil {
		return nil, fmt.Errorf("decode the correction's applied image: %w", err)
	}
	return after, nil
}

// decodeAuditDate reads a close date out of an audit image.
//
// expected_close_date is a DATE column, so to_jsonb and the audit writer render
// it as "2026-08-27" rather than an RFC 3339 instant — and time.Time's own JSON
// decoder only accepts the latter. Reading it as a plain string and parsing the
// date layout is what makes a restore of a date-typed column work at all.
// The second return says whether there WAS a date, rather than a nil pointer
// standing for it: a deal with no close date is the ordinary case, not a fault,
// and every caller here has to branch on it anyway.
func decodeAuditDate(raw json.RawMessage) (time.Time, bool, error) {
	if len(raw) == 0 {
		return time.Time{}, false, nil
	}
	var text *string
	if err := json.Unmarshal(raw, &text); err != nil {
		return time.Time{}, false, err
	}
	if text == nil {
		return time.Time{}, false, nil
	}
	parsed, err := time.Parse(time.DateOnly, *text)
	if err != nil {
		// An instant is accepted too, so a column that ever carries one does
		// not fail here for a reason a reader would find baffling.
		parsed, err = time.Parse(time.RFC3339, *text)
		if err != nil {
			return time.Time{}, false, err
		}
	}
	return parsed, true, nil
}

// datePtr renders a decoded date as the pointer storekit.SetDate takes, where
// absent is nil.
func datePtr(at time.Time, has bool) *time.Time {
	if !has {
		return nil
	}
	return &at
}
