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

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The field names an audit image of a brief item carries. Named because three
// writers spell them and a typo in one produces an audit row whose before and
// after describe different fields — which reads as a change nobody made.
const (
	auditFieldState        = "state"
	auditFieldStateAt      = "state_at"
	auditFieldSnoozedUntil = "snoozed_until"
)

// MarkActed records that the rep acted on a queue item; the next run's
// candidate filter drops the deal until it materially changes.
func (e *BriefEngine) MarkActed(ctx context.Context, itemID ids.UUID, now time.Time) (BriefRunItem, error) {
	return e.markItem(ctx, itemID, briefStateActed, nil, now)
}

// MarkDismissed records that the rep dismissed a queue item; the deal
// does not reappear unless a new linked activity arrives after the mark.
func (e *BriefEngine) MarkDismissed(ctx context.Context, itemID ids.UUID, now time.Time) (BriefRunItem, error) {
	return e.markItem(ctx, itemID, briefStateDismissed, nil, now)
}

// MarkSnoozed hides a queue item until the given instant (A77/AC-home-6),
// after which it re-surfaces as actionable. The transport validates that
// `until` lies in the future; this transition only records it.
func (e *BriefEngine) MarkSnoozed(ctx context.Context, itemID ids.UUID, until, now time.Time) (BriefRunItem, error) {
	untilUTC := until.UTC()
	return e.markItem(ctx, itemID, briefStateSnoozed, &untilUTC, now)
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
func (e *BriefEngine) markItem(ctx context.Context, itemID ids.UUID, state string, snoozedUntil *time.Time, now time.Time) (BriefRunItem, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return BriefRunItem{}, err
	}
	userID, err := briefUser(ctx)
	if err != nil {
		return BriefRunItem{}, err
	}

	var item BriefRunItem
	err = database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var owner ids.UUID
		row := tx.QueryRow(ctx, `
			SELECT bi.id, bi.deal_id, bi.rank, bi.composite, bi.feature_vector, bi.evidence_ids, bi.state, bi.state_at, bi.snoozed_until, coalesce(bi.finding, ''),
			       bi.returned_after_dismissal_on, bi.returned_with_activity_at, br.user_id
			FROM brief_item bi
			JOIN brief_run br ON br.id = bi.brief_run_id
			WHERE bi.id = $1
			FOR UPDATE OF bi`, itemID)
		var featuresRaw []byte
		// Read back with the mark, because the client replaces its cached item
		// with this response wholesale: dropping the lineage here makes the
		// "you dismissed it" line vanish the moment the rep acts on the very
		// item it was explaining.
		var dismissedOn, returnedWith *time.Time
		err := row.Scan(&item.ID, &item.DealID, &item.Rank, &item.Composite, &featuresRaw,
			&item.EvidenceIDs, &item.State, &item.StateAt, &item.SnoozedUntil, &item.Finding,
			&dismissedOn, &returnedWith, &owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		// The CHECK keeps the pair whole, so one non-null half means both are.
		if dismissedOn != nil && returnedWith != nil {
			item.Lineage = &ItemLineage{DismissedOn: *dismissedOn, ReturnedWith: *returnedWith}
		}
		if err := json.Unmarshal(featuresRaw, &item.Features); err != nil {
			return fmt.Errorf("brief: item %s carries an unreadable feature vector: %w", item.ID, err)
		}
		if owner != userID {
			// Another rep's brief: existence-hiding, like every row-scope miss.
			return apperrors.ErrNotFound
		}
		// The mark's twin of the join in readRunItems: the item names a deal
		// that may have moved since the run was assembled, and a mark is a
		// read-back as much as a write. Checked BEFORE actionability, so a
		// deal the rep can no longer read answers not-found rather than
		// disclosing the item's state through a conflict.
		if err := auth.EnsureVisible(ctx, tx, "deal", item.DealID); err != nil {
			return err
		}
		if !briefItemActionable(item, now) {
			return apperrors.ErrConflict
		}

		markedAt := now.UTC()
		if _, err := tx.Exec(ctx, `
			UPDATE brief_item SET state = $2, state_at = $3, snoozed_until = $4 WHERE id = $1`,
			itemID, state, markedAt, snoozedUntil); err != nil {
			return err
		}
		before := map[string]any{
			auditFieldState: item.State, auditFieldStateAt: item.StateAt,
			auditFieldSnoozedUntil: item.SnoozedUntil,
		}
		after := map[string]any{
			auditFieldState: state, auditFieldStateAt: markedAt,
			auditFieldSnoozedUntil: snoozedUntil,
		}
		if _, err := storekit.Audit(ctx, tx, "update", "brief_item", itemID, before, after); err != nil {
			return err
		}
		item.State = state
		item.StateAt = &markedAt
		item.SnoozedUntil = snoozedUntil
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

// briefItemActionable says whether a rep may still mark this item: fresh,
// or snoozed past its snoozed_until (the re-surface may not have been
// materialized by a read yet, but the item is already actionable again).
func briefItemActionable(item BriefRunItem, now time.Time) bool {
	if item.State == briefStateNew {
		return true
	}
	return item.State == briefStateSnoozed && item.SnoozedUntil != nil && !now.UTC().Before(*item.SnoozedUntil)
}
