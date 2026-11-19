// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "testing"

func TestBackfillTuningKeepsTheConnectorsOwnValuesWhenUnset(t *testing.T) {
	t.Parallel()
	page, workers, lookahead := BackfillTuning{}.Resolve(100, 2)
	if page != 100 || workers != 2 || lookahead != 4 {
		t.Fatalf("zero tuning gave page %d, workers %d, lookahead %d; want 100, 2, 4", page, workers, lookahead)
	}
}

func TestBackfillTuningAppliesWhatIsSetAndNeverLooksAheadLessThanItsWorkers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                     string
		tuning                   BackfillTuning
		page, workers, lookahead int
	}{
		{"all set", BackfillTuning{PageSize: 250, FetchWorkers: 8, Lookahead: 20}, 250, 8, 20},
		{"lookahead follows the workers", BackfillTuning{FetchWorkers: 6}, 100, 6, 12},
		{"lookahead under the workers is raised", BackfillTuning{FetchWorkers: 6, Lookahead: 2}, 100, 6, 6},
	} {
		page, workers, lookahead := tc.tuning.Resolve(100, 4)
		if page != tc.page || workers != tc.workers || lookahead != tc.lookahead {
			t.Errorf("%s: got %d, %d, %d; want %d, %d, %d", tc.name, page, workers, lookahead, tc.page, tc.workers, tc.lookahead)
		}
	}
}
