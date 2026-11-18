// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
)

func TestACapturedDueDateOutsideTheWindowFilesAnUndatedTask(t *testing.T) {
	ancient := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := sensibleDue(ancient); !got.IsZero() {
		t.Errorf("sensibleDue(1900) = %v, want no due date", got)
	}
	soon := clocktest.Now(t).Add(72 * time.Hour)
	if got := sensibleDue(soon); !got.Equal(soon) {
		t.Errorf("sensibleDue(soon) = %v, want it kept", got)
	}
	if got := sensibleDue(time.Time{}); !got.IsZero() {
		t.Errorf("sensibleDue(zero) = %v, want zero", got)
	}
}
