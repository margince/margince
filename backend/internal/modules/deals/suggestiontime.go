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

// suggestionSourceTimes says whether every item still cites its source as
// stored: a zone change moves no stamp, so it never reads as changed evidence.
// It answers where each item is placed (occurredday.StartSQL), in order.
func suggestionSourceTimes(ctx context.Context, tx pgx.Tx, evidence []SuggestionEvidence) ([]time.Time, bool, error) {
	placed := make([]time.Time, 0, len(evidence))
	for _, item := range evidence {
		var query string
		switch item.Kind {
		case EvidenceSignal:
			query = `SELECT ` + storekit.CitedActivityStamp("s.evidence") + `, ` + storekit.CitedActivityTime("s.evidence") + `
		 FROM signal s WHERE s.id = $1 AND s.archived_at IS NULL`
		case EvidenceMeeting:
			query = `SELECT a.occurred_at, ` + occurredday.StartSQL("a") + ` FROM activity a WHERE a.id = $1 AND a.archived_at IS NULL`
		case EvidenceAttachment:
			query = `SELECT a.occurred_at, ` + occurredday.StartSQL("a") + ` FROM attachment at JOIN activity a ON a.id = at.activity_id
 WHERE at.id = $1 AND at.archived_at IS NULL AND a.archived_at IS NULL`
		default:
			return nil, false, nil
		}
		var stamp, at *time.Time
		err := tx.QueryRow(ctx, query, item.ref()).Scan(&stamp, &at)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("deals: reading suggestion source time: %w", err)
		}
		if stamp == nil || at == nil || !stamp.Equal(item.OccurredAt) {
			return nil, false, nil
		}
		placed = append(placed, *at)
	}
	return placed, true, nil
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
	_, stands, err := suggestionSourceTimes(ctx, tx, evidence)
	return stands, err
}

func (s *Store) supersedeIfBuyerChanged(ctx context.Context, tx pgx.Tx, current Suggestion) (bool, error) {
	valid, err := s.suggestionEffects.BuyerStillVerified(ctx, tx, current, s.clock())
	if err != nil || valid {
		return false, err
	}
	return true, supersedeTx(ctx, tx, current.ID, nil)
}

// evidenceStandsTx says whether the draft's evidence still stands: each item
// is at its source's time and placed past the company's floor. It answers the
// newest item's time with it.
func evidenceStandsTx(ctx context.Context, tx pgx.Tx, d SuggestionDraft) (time.Time, bool, error) {
	placed, current, err := suggestionSourceTimes(ctx, tx, d.Evidence)
	if err != nil || !current {
		return time.Time{}, false, err
	}
	floor, err := suggestionFloorTx(ctx, tx, d.CompanyID)
	if err != nil {
		return time.Time{}, false, err
	}
	through, past := evidenceThrough(d.Evidence, placed, floor)
	return through, past, nil
}

// evidenceThrough is the newest item's time, and whether every item is placed
// after floor. A nil floor admits every item.
func evidenceThrough(evidence []SuggestionEvidence, placed []time.Time, floor *time.Time) (time.Time, bool) {
	through := evidence[0].OccurredAt
	for i, e := range evidence {
		if floor != nil && !placed[i].After(*floor) {
			return time.Time{}, false
		}
		if e.OccurredAt.After(through) {
			through = e.OccurredAt
		}
	}
	return through, true
}
