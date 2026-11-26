// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestEachRefusedCustomIntervalNamesItsReasonBesideTheSentence(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	calendar := Calendar{Timezone: "Europe/Berlin", FiscalStartMonth: 1}
	cases := []struct {
		interval *crmcontracts.ReportingWindow
		reason   string
	}{
		{nil, intervalMissing},
		{&crmcontracts.ReportingWindow{StartAt: at.AddDate(0, 0, -2), EndAt: at.AddDate(0, 0, -12)}, intervalEndBeforeStart},
		{&crmcontracts.ReportingWindow{StartAt: at.AddDate(-2, 0, 0), EndAt: at}, intervalLongerThanAYear},
		{&crmcontracts.ReportingWindow{StartAt: at.Add(time.Hour), EndAt: at.Add(2 * time.Hour)}, intervalAfterCutoff},
	}
	for _, tc := range cases {
		_, err := Interval(crmcontracts.ReportingSelection{Period: "custom", Interval: tc.interval}, calendar, at)
		fault, ok := httperr.Classify(err)
		if !ok || fault.Status != http.StatusBadRequest || fault.Code != codeIntervalInvalid {
			t.Fatalf("%s: want 400 %s, got %+v", tc.reason, codeIntervalInvalid, fault)
		}
		if fault.Details["reason"] != tc.reason || fault.Detail != err.Error() {
			t.Fatalf("%s: the reason or the sentence is lost: %+v", tc.reason, fault)
		}
	}
}
