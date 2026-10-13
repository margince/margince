// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestARunStopsWithAFixedSentenceAndNeverTheErrorText(t *testing.T) {
	for _, tc := range []struct {
		cause error
		want  string
	}{
		{errors.New(`ERROR: relation "contact" does not exist (SQLSTATE 42P01)`), CauseStopped},
		{fmt.Errorf("importing contact c-1: %w", context.DeadlineExceeded), CauseOverBudget},
	} {
		if got := StopCause(tc.cause); got != tc.want {
			t.Errorf("StopCause(%v) = %q, want %q", tc.cause, got, tc.want)
		}
	}
}
