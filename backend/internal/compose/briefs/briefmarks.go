// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// What the rep does with a queue item: acted, dismissed, snoozed.
//
// Split from the store's assembly and read paths because it is a different
// concept — those build and serve a run, these record a person's answer to one
// — and because one file holding both crossed the length ceiling once lineage
// gave the read something more to carry.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The field names an audit image of a brief item carries. Named because three
// writers spell them and a typo in one produces an audit row whose before and
// after describe different fields — which reads as a change nobody made.
const (
	auditFieldState        = "state"
	auditFieldStateAt      = "state_at"
	auditFieldSnoozedUntil = "snoozed_until"
	auditFieldReopenOn     = "reopen_on"
	auditFieldReopenRef    = "reopen_ref"
)

// snooze is what a rep is waiting for, carried as one value because the three
// fields are one decision: a condition, the instant it may store, and the row it
// may name. Passing them separately is how a caller writes two of the three and
// hits a CHECK the client cannot read.
type snooze struct {
	on    values.ReopenCondition
	until *time.Time
	ref   *ids.UUID
}

// MarkActed records that the rep acted on a queue item; the next run's
// candidate filter drops the deal until it materially changes.
func (e *BriefEngine) MarkActed(ctx context.Context, itemID ids.UUID, now time.Time) (BriefRunItem, error) {
	return e.markItem(ctx, itemID, briefStateActed, snooze{}, now)
}

// MarkDismissed records that the rep dismissed a queue item; the deal
// does not reappear unless a new linked activity arrives after the mark.
func (e *BriefEngine) MarkDismissed(ctx context.Context, itemID ids.UUID, now time.Time) (BriefRunItem, error) {
	return e.markItem(ctx, itemID, briefStateDismissed, snooze{}, now)
}

// MarkSnoozed hides a queue item until its reopen condition is met, after which
// it re-surfaces as actionable.
//
// A `time` snooze needs the instant and the transport validates it lies ahead;
// `reply` and `meeting` wait on the world instead, and the re-surface read
// decides when they are over. The shape is checked here rather than left to the
// database, so a caller that names a meeting without naming which one reads a
// validation error instead of a constraint violation.
func (e *BriefEngine) MarkSnoozed(
	ctx context.Context, itemID ids.UUID, on values.ReopenCondition,
	until *time.Time, ref *ids.UUID, now time.Time,
) (BriefRunItem, error) {
	held := snooze{on: on, ref: ref}
	if on.WantsInstant() {
		if until == nil {
			return BriefRunItem{}, &values.ParseError{
				Field: "snoozed_until", Code: "snooze_needs_a_moment",
				Message: "a snooze that waits on the clock names the moment it lifts",
			}
		}
		utc := until.UTC()
		held.until = &utc
	} else if until != nil {
		return BriefRunItem{}, &values.ParseError{
			Field: "snoozed_until", Code: "snooze_has_no_moment",
			Message: "a snooze waiting on a reply or a meeting lifts when that happens, not on a date",
		}
	}
	if on.NeedsReference() != (ref != nil) {
		return BriefRunItem{}, &values.ParseError{
			Field: "reopen_ref", Code: "reopen_ref_shape",
			Message: "only a snooze waiting on a meeting names the meeting it waits for",
		}
	}
	return e.markItem(ctx, itemID, briefStateSnoozed, held, now)
}

// markItem is the one acted/dismissed/snoozed transition: only the run's
// owner may mark, only an actionable item transitions (a second mark is a
// conflict, not a silent overwrite), and the write is audited in the
// same transaction. An expired snooze counts as actionable — a rep who
// marks straight from a stale screen must not read differently from one
// who re-opened the brief and had the item re-surfaced first. The brief
// is per-rep personal queue state — the object gate is the deal-read
// grant the brief itself rides on, and the real authority is run
// ownership.
func (e *BriefEngine) markItem(ctx context.Context, itemID ids.UUID, state string, held snooze, now time.Time) (BriefRunItem, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return BriefRunItem{}, err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return BriefRunItem{}, err
	}

	var item BriefRunItem
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		owner, err := lockItemForMark(ctx, tx, itemID, &item)
		if err != nil {
			return err
		}
		if owner != userID {
			// Another rep's brief: existence-hiding, like every row-scope miss.
			return apperrors.ErrNotFound
		}
		// The meeting a snooze names must exist, be a meeting, and be one this
		// rep may read. Checked inside the transaction so the row cannot be
		// archived between the check and the write.
		if held.ref != nil {
			if err := activities.EnsureMeetingReference(ctx, tx, *held.ref); err != nil {
				return err
			}
		}
		actionable, err := briefItemActionable(ctx, tx, item, now)
		if err != nil {
			return err
		}
		if !actionable {
			return apperrors.ErrConflict
		}

		markedAt := now.UTC()
		// NULL rather than the empty string when the mark is not a snooze: the
		// column's CHECK pairs its presence with the snoozed state, and an
		// empty string is present.
		var storedOn *string
		if held.on != "" {
			on := string(held.on)
			storedOn = &on
		}
		if _, err := tx.Exec(ctx, `
			UPDATE brief_item SET state = $2, state_at = $3, snoozed_until = $4,
			       reopen_on = $5, reopen_ref = $6 WHERE id = $1`,
			itemID, state, markedAt, held.until, storedOn, held.ref); err != nil {
			return err
		}
		before := map[string]any{
			auditFieldState: item.State, auditFieldStateAt: item.StateAt,
			auditFieldSnoozedUntil: item.SnoozedUntil,
			auditFieldReopenOn:     item.ReopenOn, auditFieldReopenRef: item.ReopenRef,
		}
		after := map[string]any{
			auditFieldState: state, auditFieldStateAt: markedAt,
			auditFieldSnoozedUntil: held.until,
			auditFieldReopenOn:     held.on, auditFieldReopenRef: held.ref,
		}
		if _, err := storekit.Audit(ctx, tx, "update", "brief_item", itemID, before, after); err != nil {
			return err
		}
		item.State = state
		item.StateAt = &markedAt
		item.SnoozedUntil = held.until
		item.ReopenOn = held.on
		item.ReopenRef = held.ref
		return nil
	})
	if err != nil {
		return BriefRunItem{}, err
	}
	return item, nil
}

// Unanswered says whether this item is still waiting on the rep, as a run READ
// hands it back.
//
// It takes no instant, unlike briefItemActionable below, and the difference is
// which question is being asked. That one guards a MARK against a stale screen,
// so it must admit a snooze whose window has passed but whose row a read has
// not yet flipped. This one describes an item LatestRun has already returned,
// and that read resurfaces expired snoozes inside its own transaction — so a
// still-snoozed item here is genuinely still set aside, and re-deciding that
// against a second clock could only disagree with the read that produced it.
//
// Exported because the worklist lane asks it. What a brief state means belongs
// to this package; a lane spelling `state == "new"` for itself would be a
// second copy of that vocabulary.
func Unanswered(item BriefRunItem) bool {
	return item.State == briefStateNew
}

// briefItemActionable says whether a rep may still mark this item: fresh, or
// snoozed past whatever it was waiting for (the re-surface may not have been
// materialized by a read yet, but the item is already actionable again).
//
// It reads the database because two of the three conditions are answered by
// rows rather than by the clock — a reply that arrived, a meeting that ended —
// and asking here is what keeps ONE answer to "is this snooze over". The
// re-surface read calls the same SQL through briefSnoozeLiftedSQL.
func briefItemActionable(ctx context.Context, tx pgx.Tx, item BriefRunItem, now time.Time) (bool, error) {
	if item.State == briefStateNew {
		return true, nil
	}
	if item.State != briefStateSnoozed {
		return false, nil
	}
	switch item.ReopenOn {
	case values.ReopenOnTime:
		return item.SnoozedUntil != nil && !now.UTC().Before(*item.SnoozedUntil), nil
	case values.ReopenOnReply, values.ReopenOnMeeting:
		var lifted bool
		if err := tx.QueryRow(ctx, `SELECT `+briefSnoozeLiftedSQL("$1", "$2", "$3", "$4", "$5"),
			item.DealID, string(item.ReopenOn), item.ReopenRef, item.StateAt, now.UTC()).Scan(&lifted); err != nil {
			return false, fmt.Errorf("brief: reading whether item %s is still set aside: %w", item.ID, err)
		}
		return lifted, nil
	default:
		// A snoozed row with no condition cannot exist — the column's CHECK
		// pairs the two — so reaching here means the constraint was dropped
		// rather than that a rep is waiting for something unnameable.
		return false, fmt.Errorf("brief: item %s is snoozed waiting for %q, which is not a condition", item.ID, item.ReopenOn)
	}
}

// lockItemForMark reads one queue item under FOR UPDATE and fills it in,
// returning the id of the rep whose run it belongs to.
//
// Read back with the mark, because the client replaces its cached item with the
// response wholesale: dropping the lineage here makes the "you dismissed it"
// line vanish the moment the rep acts on the very item it was explaining.
//
// Split from markItem because that function is the TRANSITION — who may mark,
// whether the item is still actionable, and the write itself — and fifteen scan
// targets in the middle of it hid which of those questions each branch was
// answering.
func lockItemForMark(ctx context.Context, tx pgx.Tx, itemID ids.UUID, item *BriefRunItem) (ids.UUID, error) {
	var owner ids.UUID
	var featuresRaw []byte
	var dismissedOn, returnedWith *time.Time
	var wasOn *string
	err := tx.QueryRow(ctx, `
		SELECT bi.id, bi.deal_id, bi.rank, bi.composite, bi.feature_vector, bi.evidence_ids, bi.state, bi.state_at, bi.snoozed_until, coalesce(bi.finding, ''),
		       bi.reopen_on, bi.reopen_ref, bi.returned_after_dismissal_on, bi.returned_with_activity_at, br.user_id
		FROM brief_item bi
		JOIN brief_run br ON br.id = bi.brief_run_id
		WHERE bi.id = $1
		FOR UPDATE OF bi`, itemID).Scan(&item.ID, &item.DealID, &item.Rank, &item.Composite, &featuresRaw,
		&item.EvidenceIDs, &item.State, &item.StateAt, &item.SnoozedUntil, &item.Finding,
		&wasOn, &item.ReopenRef, &dismissedOn, &returnedWith, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ids.UUID{}, err
	}
	// The CHECK keeps the pair whole, so one non-null half means both are.
	if dismissedOn != nil && returnedWith != nil {
		item.Lineage = &ItemLineage{DismissedOn: *dismissedOn, ReturnedWith: *returnedWith}
	}
	if wasOn != nil {
		item.ReopenOn = values.ReopenCondition(*wasOn)
	}
	if err := json.Unmarshal(featuresRaw, &item.Features); err != nil {
		return ids.UUID{}, fmt.Errorf("brief: item %s carries an unreadable feature vector: %w", item.ID, err)
	}
	// The mark's twin of the join in readRunItems: the item names a deal that
	// may have moved since the run was assembled, and a mark is a read-back as
	// much as a write. Checked HERE, beside the read that produced the
	// reference, rather than in the caller — a persisted reference is
	// re-checked when it is served, and a function that hands one back without
	// checking is one refactor away from a caller that forgets to.
	//
	// Before actionability, so a deal the rep can no longer read answers
	// not-found rather than disclosing the item's state through a conflict.
	if err := auth.EnsureVisible(ctx, tx, "deal", item.DealID); err != nil {
		return ids.UUID{}, err
	}
	return owner, nil
}
