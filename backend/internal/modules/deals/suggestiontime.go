// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

func suggestionSourceTimes(ctx context.Context, tx pgx.Tx, evidence []SuggestionEvidence) (bool, error) {
	for _, item := range evidence {
		var sourceTime *time.Time
		args := []any{item.ref()}
		var query string
		switch item.Kind {
		case EvidenceSignal:
			query = fmt.Sprintf(`SELECT `+storekit.CitedActivityTime("s.evidence")+`
		 FROM signal s WHERE s.id = $%d AND s.archived_at IS NULL`, len(args))
		case EvidenceMeeting:
			query = fmt.Sprintf(`SELECT occurred_at FROM activity WHERE id=$%d AND archived_at IS NULL`, len(args))
		case EvidenceAttachment:
			query = fmt.Sprintf(`SELECT a.occurred_at FROM attachment at JOIN activity a ON a.id=at.activity_id
 WHERE at.id=$%d AND at.archived_at IS NULL AND a.archived_at IS NULL`, len(args))
		default:
			return false, nil
		}
		err := tx.QueryRow(ctx, query, args...).Scan(&sourceTime)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("deals: reading suggestion source time: %w", err)
		}
		if sourceTime == nil || !sourceTime.Equal(item.OccurredAt) {
			return false, nil
		}
	}
	return true, nil
}

func supersedeIfEvidenceChanged(ctx context.Context, tx pgx.Tx, current Suggestion) (bool, error) {
	valid, err := suggestionEvidenceCurrentTx(ctx, tx, current)
	if err != nil || valid {
		return false, err
	}
	return true, supersedeTx(ctx, tx, current.ID, nil)
}

func suggestionEvidenceCurrentTx(ctx context.Context, tx pgx.Tx, current Suggestion) (bool, error) {
	evidence := make([]SuggestionEvidence, 0, len(current.Evidence))
	for _, item := range current.Evidence {
		evidence = append(evidence, item.SuggestionEvidence)
	}
	return suggestionSourceTimes(ctx, tx, evidence)
}

func (s *Store) supersedeIfBuyerChanged(ctx context.Context, tx pgx.Tx, current Suggestion) (bool, error) {
	valid, err := s.suggestionEffects.BuyerStillVerified(ctx, tx, current, s.clock())
	if err != nil || valid {
		return false, err
	}
	return true, supersedeTx(ctx, tx, current.ID, nil)
}

// placed is when the item counts as having happened against the floor: a day
// entry when its date begins in zone, as occurredday.StartSQL places it.
func (e SuggestionEvidence) placed(zone *time.Location) time.Time {
	if e.DateOnly {
		return occurredday.Begins(e.OccurredAt, zone)
	}
	return e.OccurredAt
}

// evidenceStandsTx says whether the draft's evidence still stands: each item
// is at its source's time and past the company's floor. It answers the newest
// item's time with it.
func evidenceStandsTx(ctx context.Context, tx pgx.Tx, d SuggestionDraft, zone *time.Location) (time.Time, bool, error) {
	if zone == nil {
		return time.Time{}, false, errors.New("deals: recording a suggestion needs the installation zone")
	}
	current, err := suggestionSourceTimes(ctx, tx, d.Evidence)
	if err != nil || !current {
		return time.Time{}, false, err
	}
	floor, err := suggestionFloorTx(ctx, tx, d.CompanyID)
	if err != nil {
		return time.Time{}, false, err
	}
	through, past := evidenceThrough(d.Evidence, floor, zone)
	return through, past, nil
}

// evidenceThrough is the newest item's time, and whether every item falls
// after floor where zone places it. A nil floor admits every item.
func evidenceThrough(evidence []SuggestionEvidence, floor *time.Time, zone *time.Location) (time.Time, bool) {
	through := evidence[0].OccurredAt
	for _, e := range evidence {
		if floor != nil && !e.placed(zone).After(*floor) {
			return time.Time{}, false
		}
		if e.OccurredAt.After(through) {
			through = e.OccurredAt
		}
	}
	return through, true
}
