// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
)

// An imported day cell is previewed as the date it names. A timed cell keeps
// its instant, so the reviewer never reads the day's anchor as a time.
func TestAnImportPreviewShowsADayCellAsADate(t *testing.T) {
	at := time.Date(2025, 3, 9, 0, 0, 0, 0, time.UTC)
	for precision, want := range map[string]string{activities.PrecisionDay: previewValueDate, "": previewValueText} {
		raw, err := json.Marshal(previewOccurredAt(activities.LogActivityInput{OccurredAt: &at, OccurredPrecision: precision}))
		if err != nil {
			t.Fatal(err)
		}
		value, err := approvalPreviewReader{}.value(context.Background(), creationValueField(activityTargetOccurredAt), raw)
		if err != nil {
			t.Fatalf("previewing %s: %v", raw, err)
		}
		if value.Kind != want {
			t.Errorf("precision %q previews %s as %q, want %q", precision, raw, value.Kind, want)
		}
	}
}
