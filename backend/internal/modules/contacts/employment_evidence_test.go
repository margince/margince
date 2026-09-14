// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"
	"time"
)

func TestEmploymentEvidencePreservesPrecisionAndUnknownStatus(t *testing.T) {
	episodes, err := employmentEpisodes("job_history", []byte(`[
 {"company_name":"Example","job_title":"Lead","started_at":"2020-02","ended_at":"2026-09"},
 {"company_name":"Example","job_title":"Advisor","started_at":"2024-03-17"},
 {"company_name":"Example","job_title":"Invalid","ended_at":"not-a-date"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 3 || episodes[0].Started != "2020-02" || episodes[1].Started != "2024-03-17" {
		t.Fatalf("lost date precision: %+v", episodes)
	}
	during := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if episodes[0].effectiveStatus(during) != "unknown" || episodes[0].effectiveStatus(after) != "former" {
		t.Fatal("month departure used an invented exact day")
	}
	if episodes[1].effectiveStatus(after) != "unknown" {
		t.Fatal("missing departure invented current work")
	}
	if !episodes[2].invalidDates {
		t.Fatal("invalid date silently became unknown")
	}
}

func TestContradictoryDepartureNeedsReview(t *testing.T) {
	today := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	for _, end := range []string{"2026-08-20", "2026-09"} {
		if !(employmentEvidence{Status: "former", Ended: end}).requiresReview(today) {
			t.Errorf("former role with future departure %s escaped review", end)
		}
	}
	if (employmentEvidence{Status: "former", Ended: "2026-08"}).requiresReview(today) {
		t.Fatal("a former role in this month need not invent its departure day")
	}
}
