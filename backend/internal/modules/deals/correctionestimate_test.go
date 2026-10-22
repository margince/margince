// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import "testing"

func TestEstimateBasisRequiresRecordedHistory(t *testing.T) {
	for _, invalid := range []string{
		`{}`, `not json`, `null`,
		`{"close_date_sample_count":19,"close_date_median_stage_days":4,"close_date_remaining_stages":2}`,
		`{"close_date_sample_count":20,"close_date_median_stage_days":0,"close_date_remaining_stages":2}`,
		`{"close_date_sample_count":20,"close_date_median_stage_days":4,"close_date_remaining_stages":0}`,
		`{"close_date_sample_count":"20","close_date_median_stage_days":4,"close_date_remaining_stages":2}`,
	} {
		if got := correctionEstimateBasis([]byte(invalid)); got != nil {
			t.Errorf("invalid evidence %s supplied a basis: %+v", invalid, got)
		}
	}
	got := correctionEstimateBasis([]byte(`{"close_date_sample_count":20,"close_date_median_stage_days":4.5,"close_date_remaining_stages":2}`))
	if got == nil || got.WonDeals != 20 || got.MedianDaysPerStage != 4.5 || got.RemainingStages != 2 {
		t.Fatalf("recorded basis = %+v", got)
	}
}
