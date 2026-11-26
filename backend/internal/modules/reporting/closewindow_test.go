// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestACustomCloseWindowNamesUpToTwelveMonthsThatEndAfterTheyStart(t *testing.T) {
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		window *crmcontracts.ReportingWindow
		valid  bool
	}{
		"missing":       {nil, false},
		"ends at start": {&crmcontracts.ReportingWindow{StartAt: start, EndAt: start}, false},
		"one quarter":   {&crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 3, 0)}, true},
		"twelve months": {&crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(1, 0, 0)}, true},
		"past a year":   {&crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(1, 0, 1)}, false},
	}
	for name, tc := range cases {
		_, err := CustomCloseWindow(tc.window)
		if tc.valid && err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
		if !tc.valid && !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("%s: got %v, want an invalid-argument refusal", name, err)
		}
	}
}
