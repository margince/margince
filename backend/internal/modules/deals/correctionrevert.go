// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Taking back a close-date correction the product made by itself.
//
// A SEPARATE VERB from the general restore path, for the same structural reason
// the stage reversal beside it is one — and here the general path does not merely
// lose information, it refuses outright. Two walls:
//
// rejectPastCloseDate refuses any expected_close_date before today on an open
// deal, and that is exactly what an undo of an overdue correction restores. The
// sweep rolled a date forward BECAUSE it had passed; putting it back means
// putting back a past date, which the ordinary update shape exists to prevent.
//
// close_date_provisional is audited but is not a writable deal field: it is
// absent from the update request and from agents.UpdatableFields, so the restore
// image filter marks it unspellable and refuses the whole entry rather than
// putting back the part it understands. Every one of the sweep's three tiers
// touches that flag.
//
// So a correction carries its own undo, and the narrow exception is stated here
// rather than as a flag on the general writer: this path may restore a past date
// because it restores an AUTHENTICATED PRIOR IMAGE — the value the deal actually
// held, read from the correction's own audit row, not a date a caller chose. The
// ordinary editor keeps refusing arbitrary past dates, which is the invariant
// that matters.
//
// A restored past date lands provisional and unresolved. That is not a
// concession: the deal genuinely does not have a date anybody stands behind, and
// saying so is more honest than either keeping the machine's guess or pretending
// the old date is a plan.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CorrectionReversalError refuses an undo that cannot be made.
//
// Its own type rather than a bare conflict, because the ways this declines are
// different situations for the reader: a correction already taken back
// (somebody got there first), and a field a person has since edited (their work
// would be overwritten). Both are 409s about the state of a record rather than
// anything malformed in the request.
type CorrectionReversalError struct{ Reason string }

// alreadyTakenBack is the refusal a second Undo gets. One spelling, because
// three paths reach it — the pre-check, the stamp losing its race, and the
// hand-restored case — and three wordings would read as three situations.
const alreadyTakenBack = "this correction has already been taken back"

func (e *CorrectionReversalError) Error() string { return e.Reason }

// RevertCorrection puts back every field one machine correction changed.
//
// THE WHOLE FIELD SET, from the correction's own audit row. A correction can
// move the date, the provisional flag and the forecast category together, and
// restoring one of three would leave the deal in a state no writer ever produced
// — a date nobody chose sitting under a category that was notched for a reason
// that no longer applies.
//
// ONE TRANSACTION for the restore, the audit row and the lifecycle stamp. Split
// apart, a failure between them leaves the deal restored while the correction
// still reads live, and the next sweep would take the reversal for a fresh
// correction to repeat.
func (s *Store) RevertCorrection(
	ctx context.Context, correctionID ids.UUID,
) (crmcontracts.Deal, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.Deal{}, err
	}
	// Resolved before the transaction opens: the catalog reads through its own
	// connection, and a second connection inside a transaction can deadlock
	// undetectably against a lock that transaction holds.
	active, err := s.activeColumns(ctx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	var out crmcontracts.Deal
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		correction, err := lockReversibleCorrection(ctx, tx, correctionID)
		if err != nil {
			return err
		}
		// VISIBILITY BEFORE ANY CONFLICT, the module's rule everywhere: asked
		// after the refusals below, a caller with the object grant but no row
		// scope could tell a live correction from an already-reversed one by the
		// shape of the 409 and learn that a deal exists.
		if err := auth.EnsureWritable(ctx, tx, dealTable, correction.DealID.UUID); err != nil {
			return err
		}
		if err := restoreCorrectedFields(ctx, tx, correction); err != nil {
			return err
		}
		// Read inside the transaction that just took write authority on this
		// deal. GetDeal would take a VISIBILITY probe instead, which a manual
		// read-share satisfies — the wrong question on a path that has just
		// changed the record.
		out, err = readDealForCaller(ctx, tx, correction.DealID, storekit.LiveOnly, active)
		return err
	})
	return out, err
}

// restoreCorrectedFields puts the deal back and stamps the correction, in the
// caller's transaction.
//
// Split from RevertCorrection so each function holds one question: that one
// decides WHETHER this correction may be taken back, this one performs it.
func restoreCorrectedFields(ctx context.Context, tx pgx.Tx, correction DealCorrection) error {
	before, err := priorImage(ctx, tx, correction)
	if err != nil {
		return err
	}
	lock, err := storekit.LockRow(ctx, tx, dealTable, correction.DealID.UUID, storekit.LiveOnly)
	if err != nil {
		return err
	}
	patch := storekit.NewPatch()
	if err := buildReversalPatch(ctx, tx, correction, before, patch); err != nil {
		return err
	}
	if patch.Empty() {
		// The deal already holds everything the correction changed away from —
		// a person put it back by hand. Nothing to restore, and the stamp still
		// records that this correction is done.
		return markReversedOrConflict(ctx, tx, correction)
	}
	if err := applyDealPatchLocked(ctx, tx, patch, lock); err != nil {
		return fmt.Errorf("restore the corrected fields: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "deal",
		correction.DealID.UUID, patch.Before(), patch.After())
	if err != nil {
		return fmt.Errorf("audit the reversal: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, correction.DealID.UUID,
		crmcontracts.PublicEventDealUpdated{ChangedFields: reversalFields(patch)}); err != nil {
		return fmt.Errorf("emit the reversal: %w", err)
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	stamped, err := markReversed(ctx, tx, correction.ID, auditID, by)
	if err != nil {
		return err
	}
	if !stamped {
		return &CorrectionReversalError{Reason: alreadyTakenBack}
	}
	return nil
}

// lockReversibleCorrection takes the correction row and refuses one already
// taken back.
//
// FOR UPDATE is what makes a repeated Undo idempotent rather than a toggle: two
// calls racing on one correction serialize here, the first stamps it reversed,
// and the second reads that stamp and refuses instead of restoring twice.
func lockReversibleCorrection(ctx context.Context, tx pgx.Tx, id ids.UUID) (DealCorrection, error) {
	var c DealCorrection
	err := tx.QueryRow(ctx, `
		SELECT id, deal_id, audit_log_id, correction, fields, applied_at,
		       reversed_at, reversed_by,
		       remaining_open_stages, asking, standing_close_date
		  FROM deal_correction
		 WHERE id = $1
		   FOR UPDATE`, id).
		Scan(&c.ID, &c.DealID, &c.AuditLogID, &c.Correction, &c.Fields, &c.AppliedAt,
			&c.ReversedAt, &c.ReversedBy,
			&c.Evidence.RemainingOpenStages, &c.Evidence.Asking, &c.Evidence.StandingCloseDate)
	if errors.Is(err, pgx.ErrNoRows) {
		return DealCorrection{}, apperrors.ErrNotFound
	}
	if err != nil {
		return DealCorrection{}, fmt.Errorf("read the correction to reverse: %w", err)
	}
	if c.Reversed() {
		return DealCorrection{}, &CorrectionReversalError{Reason: alreadyTakenBack}
	}
	return c, nil
}

// priorImage reads what the deal held before the correction, from the
// correction's OWN audit row.
//
// From the audit rather than from the caller, and that is the whole basis of the
// past-date exception above: this path may write a date the ordinary editor
// refuses because the value is one the deal demonstrably held, not one somebody
// asked for.
func priorImage(ctx context.Context, tx pgx.Tx, c DealCorrection) (map[string]json.RawMessage, error) {
	var raw []byte
	err := tx.QueryRow(ctx,
		`SELECT before FROM audit_log WHERE id = $1`, c.AuditLogID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &CorrectionReversalError{
			Reason: "the change this correction recorded is no longer in the record's history",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read the correction's prior image: %w", err)
	}
	var before map[string]json.RawMessage
	if err := json.Unmarshal(raw, &before); err != nil {
		return nil, fmt.Errorf("decode the correction's prior image: %w", err)
	}
	return before, nil
}

// buildReversalPatch puts each corrected field back, and refuses if a person has
// changed one since.
//
// The later-edit check is per FIELD rather than per record on purpose. A rep who
// renamed the deal after the correction still gets their undo — the name is not
// what was corrected — while a rep who re-dated it does not have that answer
// silently overwritten by a machine reversal.
func buildReversalPatch(
	ctx context.Context, tx pgx.Tx, c DealCorrection,
	before map[string]json.RawMessage, patch *storekit.Patch,
) error {
	current, err := currentCorrectedValues(ctx, tx, c)
	if err != nil {
		return err
	}
	after, err := correctionAfterImage(ctx, tx, c)
	if err != nil {
		return err
	}
	// The provisional flag is decided last, by markRestoredDateUnresolved: a
	// restored past date must stay marked, and restoring the prior value here
	// would then be immediately contradicted.
	pastDate, err := restoresAPastDate(ctx, tx, before)
	if err != nil {
		return err
	}
	for _, field := range c.Fields {
		if field == provisionalField && pastDate {
			continue
		}
		wanted, ok := before[field]
		if !ok {
			// The correction created a value where there was none; putting it
			// back means clearing it.
			wanted = json.RawMessage("null")
		}
		applied, ok := after[field]
		if ok && !jsonEqual(applied, current[field]) {
			return &CorrectionReversalError{Reason: fmt.Sprintf(
				"%s has been changed since this correction, so taking it back would overwrite that", field)}
		}
		if jsonEqual(wanted, current[field]) {
			continue
		}
		if err := setReversalField(patch, field, current[field], wanted); err != nil {
			return err
		}
	}
	return markRestoredDateUnresolved(ctx, tx, c, before, patch)
}

// markRestoredDateUnresolved keeps a restored PAST date visibly unresolved.
//
// The prior image is the whole truth about what the deal held, and restoring it
// faithfully would put back close_date_provisional = false alongside a date that
// has since gone by. That pair is the state INV-CLOSE-PAST exists to keep out of
// the forecast: a date in the past, unmarked, reading as something somebody
// stands behind.
//
// So this is the one place the reversal deliberately does NOT restore the prior
// value. The date comes back because it is what the deal really held; the flag
// stays set because nobody has confirmed it and the calendar has moved on. What
// the reader sees is honest — the machine's guess is gone, and what is left is
// visibly waiting for a real date rather than pretending to be one.
//
// A restored FUTURE date needs none of this: it is a plan that has not lapsed,
// and its prior flag is restored like any other field.
func markRestoredDateUnresolved(
	ctx context.Context, tx pgx.Tx, c DealCorrection,
	before map[string]json.RawMessage, patch *storekit.Patch,
) error {
	pastDate, err := restoresAPastDate(ctx, tx, before)
	if err != nil || !pastDate {
		return err
	}
	current, err := currentCorrectedValues(ctx, tx, c)
	if err != nil {
		return err
	}
	var provisional bool
	if raw := current[provisionalField]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &provisional); err != nil {
			return fmt.Errorf("decode the current provisional flag: %w", err)
		}
	}
	if provisional {
		return nil
	}
	patch.Set(provisionalField, false, true)
	return nil
}

// setReversalField assigns one restored value, in the column's own type.
//
// A restored close date is forced PROVISIONAL alongside itself: the date coming
// back is one the sweep found overdue, and a deal claiming an unmarked date in
// the past is the state INV-CLOSE-PAST exists to keep out of the forecast. What
// the reader gets instead is honest — a date nobody stands behind, visibly
// unresolved, waiting for a real one.
func setReversalField(patch *storekit.Patch, field string, current, wanted json.RawMessage) error {
	switch field {
	case closeDateField:
		restored, hasRestored, err := decodeAuditDate(wanted)
		if err != nil {
			return fmt.Errorf("decode the restored close date: %w", err)
		}
		now, hasNow, err := decodeAuditDate(current)
		if err != nil {
			return fmt.Errorf("decode the current close date: %w", err)
		}
		patch.SetDate(closeDateField, datePtr(now, hasNow), datePtr(restored, hasRestored))
	default:
		var restored, now any
		if err := json.Unmarshal(wanted, &restored); err != nil {
			return fmt.Errorf("decode the restored %s: %w", field, err)
		}
		if len(current) > 0 {
			if err := json.Unmarshal(current, &now); err != nil {
				return fmt.Errorf("decode the current %s: %w", field, err)
			}
		}
		patch.Set(field, now, restored)
	}
	return nil
}

// reversalFields names what the reversal changed, marked as a reversal so a
// consumer can tell it from a fresh correction moving the same column.
func reversalFields(patch *storekit.Patch) map[string]any {
	fields := map[string]any{"close_date_correction": "reversal"}
	for field, v := range patch.After() {
		fields[field] = v
	}
	return fields
}

// markReversedOrConflict stamps a correction whose fields a person already
// restored by hand.
func markReversedOrConflict(ctx context.Context, tx pgx.Tx, c DealCorrection) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	stamped, err := markReversed(ctx, tx, c.ID, c.AuditLogID, by)
	if err != nil {
		return err
	}
	if !stamped {
		return &CorrectionReversalError{Reason: alreadyTakenBack}
	}
	return nil
}

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

// correctionAfterImage is what the correction WROTE, used to tell "a person has
// edited this since" from "the value is simply what the correction made it".
func correctionAfterImage(ctx context.Context, tx pgx.Tx, c DealCorrection) (map[string]json.RawMessage, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT after FROM audit_log WHERE id = $1`, c.AuditLogID).Scan(&raw)
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

// restoresAPastDate reports whether the prior image puts a date back that the
// calendar has already passed — the case a reversal must leave visibly
// unresolved rather than presenting as a plan.
func restoresAPastDate(ctx context.Context, tx pgx.Tx, before map[string]json.RawMessage) (bool, error) {
	restored, has, err := decodeAuditDate(before[closeDateField])
	if err != nil || !has {
		return false, err
	}
	var today time.Time
	if err := tx.QueryRow(ctx, `SELECT (now())::date`).Scan(&today); err != nil {
		return false, fmt.Errorf("read today for the restored date: %w", err)
	}
	return restored.Before(today), nil
}

// provisionalField is the column saying nobody has confirmed the deal's date.
const provisionalField = "close_date_provisional"

// datePtr renders a decoded date as the pointer storekit.SetDate takes, where
// absent is nil.
func datePtr(at time.Time, has bool) *time.Time {
	if !has {
		return nil
	}
	return &at
}
