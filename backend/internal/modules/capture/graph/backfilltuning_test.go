// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestTheGraphWalkKeepsItsBuiltInsUntilTuned(t *testing.T) {
	t.Parallel()
	page, walk := New(nil, nil).walk()
	if page != backfillPageSize || walk.Workers != 2 || walk.Lookahead != 4 {
		t.Fatalf("untuned walk: page %d, workers %d, lookahead %d; want %d, 2, 4", page, walk.Workers, walk.Lookahead, backfillPageSize)
	}
}

func TestTheGraphWalkNeverUsesMoreRequestsThanMicrosoftAllowsPerMailbox(t *testing.T) {
	t.Parallel()
	tuned := New(nil, nil).WithBackfillTuning(connector.BackfillTuning{PageSize: 250, FetchWorkers: 12, Lookahead: 30})
	page, walk := tuned.walk()
	if page != 250 || walk.Workers != maxBackfillFetchWorkers || walk.Lookahead != 30 {
		t.Fatalf("tuned walk: page %d, workers %d, lookahead %d; want 250, %d, 30", page, walk.Workers, walk.Lookahead, maxBackfillFetchWorkers)
	}
}
