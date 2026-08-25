// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"errors"
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestLeadQueueCursorRoundTrip(t *testing.T) {
	want := leadQueueCursor{
		AsOf: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC), Rank: 1, Score: 73,
		CreatedAt: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC), ID: ids.NewV7(),
	}
	got, err := decodeLeadQueueCursor(encodeLeadQueueCursor(want))
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestLeadQueueCursorRejectsMalformedAndImpossibleValues(t *testing.T) {
	outOfRange := encodeLeadQueueCursor(leadQueueCursor{
		AsOf: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC), Rank: leadQueueRankInactive + 1, Score: 50,
		CreatedAt: time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC), ID: ids.NewV7(),
	})
	tests := []string{
		"not-base64",
		"e30", // {}
		outOfRange,
	}
	for _, token := range tests {
		if _, err := decodeLeadQueueCursor(token); !errors.As(err, new(*storekit.MalformedCursorError)) {
			t.Errorf("decode %q error = %v, want MalformedCursorError", token, err)
		}
	}
}
