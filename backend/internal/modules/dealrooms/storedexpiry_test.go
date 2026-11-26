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
		standing  *time.Time
		createdAt time.Time
		now       time.Time
		stored    *time.Time
	}{
		{"no expiry stays unbounded", nil, nil, created, now, nil},
		{"a future instant is kept", at(48 * time.Hour), nil, created, now, at(48 * time.Hour)},
		{"a past instant after creation is kept", at(time.Hour), nil, created, now, at(time.Hour)},
		{"an instant before creation becomes the time of the call", at(-24 * time.Hour), nil, created, now, &now},
		{"the creation instant itself becomes the time of the call", at(0), nil, created, now, &now},
		{"a call on the creation tick lands one microsecond after it", at(-time.Hour), nil, created, created, at(time.Microsecond)},
		{"a call clock behind creation still lands after it", at(-time.Hour), nil, created, created.Add(-time.Second), at(time.Microsecond)},
		{"a repeat of a cut short keeps the expiry that already passed", at(-24 * time.Hour), at(time.Hour), created, now, at(time.Hour)},
		{"a standing expiry at the time of the call counts as passed", at(-24 * time.Hour), &now, created, now, &now},
		{"a standing expiry still ahead is cut short to the time of the call", at(-24 * time.Hour), at(48 * time.Hour), created, now, &now},
		{"a standing expiry does not hold back an instant after creation", at(time.Minute), at(time.Hour), created, now, at(time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := storedExpiry(tc.want, tc.standing, tc.createdAt, tc.now)
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

func TestOnlyALaterOrRemovedExpiryGrantsMoreAccess(t *testing.T) {
	early := time.Date(2026, time.November, 9, 9, 0, 0, 0, time.UTC)
	late := early.Add(24 * time.Hour)
	for _, tc := range []struct {
		name           string
		standing, next *time.Time
		grantsNoMore   bool
	}{
		{"an unchanged expiry", &early, &early, true},
		{"an earlier expiry", &late, &early, true},
		{"a first expiry", nil, &early, true},
		{"no bound before or after", nil, nil, true},
		{"a later expiry", &early, &late, false},
		{"a removed bound", &early, nil, false},
	} {
		if got := grantsNoMoreAccess(tc.standing, tc.next); got != tc.grantsNoMore {
			t.Errorf("%s: grantsNoMoreAccess = %v, want %v", tc.name, got, tc.grantsNoMore)
		}
	}
}
