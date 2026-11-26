// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CitedMomentsEvidenceKey holds when each cited activity happened as the
// change was written. A meeting moved later leaves the receipt's date alone.
const CitedMomentsEvidenceKey = "cited_activity_moments"

// CitedMoment is when one cited activity happened, and how much of that was
// known, at the change's write.
type CitedMoment struct {
	At        time.Time `json:"occurred_at"`
	Precision string    `json:"occurred_precision"`
}

// Cited lists the activity ids the change's evidence cites.
func (c VerdictChange) Cited() []string {
	cited, ok := c.Evidence[CitedActivitiesEvidenceKey].([]string)
	if !ok {
		return nil
	}
	return cited
}

// lockCitedMoments holds every activity the changes cite, sorted by id after
// the deal's own lock, and reads when each happened. A meeting moved
// concurrently waits for this transaction, so the evidence check and the
// recorded moments see one row.
func lockCitedMoments(ctx context.Context, tx pgx.Tx, changes []VerdictChange) (map[string]CitedMoment, error) {
	var cited []ids.UUID
	for _, change := range changes {
		for _, raw := range change.Cited() {
			if id, err := ids.Parse(raw); err == nil && !id.IsZero() && !slices.Contains(cited, id) {
				cited = append(cited, id)
			}
		}
	}
	moments := map[string]CitedMoment{}
	if len(cited) == 0 {
		return moments, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id, occurred_at, occurred_precision FROM activity
		 WHERE id = ANY($1) AND archived_at IS NULL AND restricted_at IS NULL
		 ORDER BY id FOR SHARE`, cited)
	if err != nil {
		return nil, fmt.Errorf("deals: holding the cited activities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var moment CitedMoment
		if err := rows.Scan(&id, &moment.At, &moment.Precision); err != nil {
			return nil, fmt.Errorf("deals: holding the cited activities: %w", err)
		}
		moments[id.String()] = moment
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deals: holding the cited activities: %w", err)
	}
	return moments, nil
}

// verdictEvidence is the audit evidence of one verdict change: its kind, whose
// grants wrote it and the change's evidence. It adds the moments of the
// activities the change cites, from the verdict's one locked read.
func verdictEvidence(in VerdictInput, change VerdictChange) map[string]any {
	evidence := map[string]any{CorrectionKindEvidenceKey: change.Correction, basisOwnerKey: in.Owner.String()}
	maps.Copy(evidence, change.Evidence)
	moments := map[string]CitedMoment{}
	for _, id := range change.Cited() {
		if moment, ok := in.moments[id]; ok {
			moments[id] = moment
		}
	}
	if len(moments) > 0 {
		evidence[CitedMomentsEvidenceKey] = moments
	}
	return evidence
}

// citedMoments reads the moments a change's evidence recorded, none for a
// change written before they were recorded or evidence the reader may not read.
func citedMoments(evidence []byte) map[ids.UUID]CitedMoment {
	var fields map[string]json.RawMessage
	if len(evidence) == 0 || json.Unmarshal(evidence, &fields) != nil {
		return nil
	}
	var recorded map[string]CitedMoment
	if json.Unmarshal(fields[CitedMomentsEvidenceKey], &recorded) != nil {
		return nil
	}
	out := make(map[ids.UUID]CitedMoment, len(recorded))
	for raw, moment := range recorded {
		if id, err := ids.Parse(raw); err == nil && !id.IsZero() {
			out[id] = moment
		}
	}
	return out
}
