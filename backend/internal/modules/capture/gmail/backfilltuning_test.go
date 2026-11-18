// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import "testing"

func TestBackfillTuningKeepsTheBuiltInsWhenUnset(t *testing.T) {
	t.Parallel()
	var tuning BackfillTuning
	workers, lookahead := tuning.walkSize()
	if tuning.pageSize() != backfillPageSize || workers != backfillFetchWorkers || lookahead != 2*backfillFetchWorkers {
		t.Fatalf("zero tuning gave page %d, workers %d, lookahead %d", tuning.pageSize(), workers, lookahead)
	}
}

func TestBackfillTuningAppliesWhatIsSetAndNeverLooksAheadLessThanItsWorkers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                       string
		tuning                     BackfillTuning
		page, workers, lookaheadIs int
	}{
		{"all set", BackfillTuning{PageSize: 250, FetchWorkers: 8, Lookahead: 20}, 250, 8, 20},
		{"lookahead follows the workers", BackfillTuning{FetchWorkers: 6}, backfillPageSize, 6, 12},
		{"lookahead under the workers is raised", BackfillTuning{FetchWorkers: 6, Lookahead: 2}, backfillPageSize, 6, 6},
	} {
		workers, lookahead := tc.tuning.walkSize()
		if tc.tuning.pageSize() != tc.page || workers != tc.workers || lookahead != tc.lookaheadIs {
			t.Errorf("%s: page %d, workers %d, lookahead %d; want %d, %d, %d", tc.name,
				tc.tuning.pageSize(), workers, lookahead, tc.page, tc.workers, tc.lookaheadIs)
		}
	}
}
