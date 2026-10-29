// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// proposalsReadable answers, by position, whether p may read each proposal.
// That takes the grants deciding it needs, a seat it may be shown to, and the
// content of every activity it cites. One statement asks the whole batch.
func proposalsReadable(ctx context.Context, tx pgx.Tx, p principal.Principal, rows []row) ([]bool, error) {
	out := make([]bool, len(rows))
	cited := make([][]ids.UUID, len(rows))
	var asked []ids.UUID
	for i, a := range rows {
		if requireDecisionGrants(p, a) != nil || withheldFromOtherSeats(p, a) {
			continue
		}
		activities, checkable, err := citedActivities(a)
		if err != nil {
			return nil, err
		}
		out[i], cited[i] = checkable, activities
		asked = append(asked, activities...)
	}
	if len(asked) == 0 {
		return out, nil
	}
	readable, err := objectReadable(ctx, objectActivity)
	if err != nil {
		return nil, err
	}
	visible := map[ids.UUID]bool{}
	if readable {
		// Archived evidence remains readable; only the action target must be live.
		if visible, err = auth.ActivityContentSubset(ctx, tx, asked, false); err != nil {
			return nil, err
		}
	}
	for i := range rows {
		for _, id := range cited[i] {
			out[i] = out[i] && visible[id]
		}
	}
	return out, nil
}

// citedActivities is the activities a proposal's evidence cites. checkable is
// false when a citation names an activity without its id, which nobody can read.
func citedActivities(a row) (activities []ids.UUID, checkable bool, err error) {
	if len(a.Evidence) == 0 {
		return nil, true, nil
	}
	var evidence []evidenceJSON
	if err := json.Unmarshal(a.Evidence, &evidence); err != nil {
		return nil, false, fmt.Errorf("read proposal evidence: %w", err)
	}
	for _, item := range evidence {
		if item.SourceType == nil || *item.SourceType != "activity" {
			continue
		}
		if item.SourceID == nil {
			return nil, false, nil
		}
		activities = append(activities, *item.SourceID)
	}
	return activities, true, nil
}
