// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestARunStopsWithAFixedSentenceAndNeverTheErrorText(t *testing.T) {
	driver := errors.New(`ERROR: relation "contact" does not exist (SQLSTATE 42P01)`)
	late := fmt.Errorf("importing contact c-1: %w", context.DeadlineExceeded)
	for _, tc := range []struct {
		status string
		cause  error
		want   string
	}{
		{StatusRunning, driver, CauseStopped},
		{StatusRunning, late, CauseOverBudget},
		{StatusValidating, driver, CauseTestRunStopped},
		{StatusValidating, late, CauseValidateOverBudget},
		{StatusValidating, fmt.Errorf("reading: %w", apperrors.ErrPermissionDenied), CausePermission},
	} {
		if got := StopCause(tc.status, tc.cause); got != tc.want {
			t.Errorf("StopCause(%s, %v) = %q, want %q", tc.status, tc.cause, got, tc.want)
		}
	}
}
