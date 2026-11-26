// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// CustomCloseWindow checks the window close_window=custom names, for a reading
// and for a saved report alike. Unlike the reporting interval it may lie in the
// future: it bounds expected close dates.
func CustomCloseWindow(window *crmcontracts.ReportingWindow) (crmcontracts.ReportingWindow, error) {
	if window == nil {
		return crmcontracts.ReportingWindow{}, fmt.Errorf("choose the expected-close window close_window=custom names: %w", apperrors.ErrInvalidArgument)
	}
	if !window.EndAt.After(window.StartAt) || window.EndAt.After(window.StartAt.AddDate(1, 0, 0)) {
		return crmcontracts.ReportingWindow{}, fmt.Errorf("choose an expected-close window of up to twelve months that ends after it starts: %w", apperrors.ErrInvalidArgument)
	}
	return *window, nil
}
