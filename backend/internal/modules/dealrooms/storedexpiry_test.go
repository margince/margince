// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"testing"
	"time"
)

func TestAnExpiryIsStoredAfterTheRoomWasCreated(t *testing.T) {
	created := time.Date(2026, time.November, 10, 9, 0, 0, 0, time.UTC)
	now := created.Add(2 * time.Hour)
	at := func(d time.Duration) *time.Time { v := created.Add(d); return &v }

	for _, tc := range []struct {
		name      string
		want      *time.Time
		createdAt time.Time
		now       time.Time
		stored    *time.Time
	}{
		{"no expiry stays unbounded", nil, created, now, nil},
		{"a future instant is kept", at(48 * time.Hour), created, now, at(48 * time.Hour)},
		{"a past instant after creation is kept", at(time.Hour), created, now, at(time.Hour)},
		{"an instant before creation becomes the time of the call", at(-24 * time.Hour), created, now, &now},
		{"the creation instant itself becomes the time of the call", at(0), created, now, &now},
		{"a call on the creation tick lands one microsecond after it", at(-time.Hour), created, created, at(time.Microsecond)},
		{"a call clock behind creation still lands after it", at(-time.Hour), created, created.Add(-time.Second), at(time.Microsecond)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storedExpiry(tc.want, tc.createdAt, tc.now)
			switch {
			case tc.stored == nil && got != nil:
				t.Fatalf("stored %v, want no expiry", *got)
			case tc.stored != nil && (got == nil || !got.Equal(*tc.stored)):
				t.Fatalf("stored %v, want %v", got, *tc.stored)
			case got != nil && !got.After(tc.createdAt):
				t.Fatalf("stored %v, not after creation at %v", *got, tc.createdAt)
			}
		})
	}
}
