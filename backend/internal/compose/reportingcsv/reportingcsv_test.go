// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportingcsv_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/reportingcsv"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestMajorWritesTheDigitsEachCurrencyHas(t *testing.T) {
	for currency, want := range map[string]string{"EUR": "17100.55", "JPY": "1710055", "KWD": "1710.055"} {
		minor := float64(1710055)
		if got := reportingcsv.Major(&minor, currency); got != want {
			t.Errorf("%s: %q, want %q", currency, got, want)
		}
	}
	if reportingcsv.Major(nil, "EUR") != "" {
		t.Error("no value must stay an empty cell")
	}
}

func TestFilenameNamesScopePeriodAndLocalDay(t *testing.T) {
	name := "Müller / Team"
	var frame crmcontracts.ReportingContext
	frame.Scope = crmcontracts.ReportingScope{Kind: "team", Label: &name}
	frame.PeriodKind = "this_quarter"
	frame.Timezone = "Europe/Berlin"
	frame.EvaluatedAt = time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	if got, want := reportingcsv.Filename(frame), "reporting-müller-team-this-quarter-2027-01-01.csv"; got != want {
		t.Fatalf("filename %q, want %q", got, want)
	}
}

func TestLabelsSpeakEveryLanguage(t *testing.T) {
	for id, phrase := range reportingcsv.Labels {
		if phrase.En == "" || phrase.De == "" || phrase.Vi == "" {
			t.Errorf("%s has a missing language: %+v", id, phrase)
		}
	}
}
