// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"testing"
	"time"
)

// A digest is labelled with the installation's day at the instant it is built,
// and is read back under that label. 23:30 on the 19th in Los Angeles is the
// 20th in UTC, and 06:30 on the 20th in Auckland is the 19th in UTC.
func TestADigestIsLabelledWithTheInstallationDay(t *testing.T) {
	for _, tc := range []struct {
		name, zone string
		at         time.Time
		day        string
	}{
		{"west of UTC", "America/Los_Angeles", time.Date(2026, 8, 20, 6, 30, 0, 0, time.UTC), "2026-08-19"},
		{"east of UTC", "Pacific/Auckland", time.Date(2026, 8, 19, 18, 30, 0, 0, time.UTC), "2026-08-20"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := setupBackfillWire(t)
			b.env.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, tc.zone)
			if err := b.registry.BuildDigests(b.human, tc.at); err != nil {
				t.Fatalf("BuildDigests: %v", err)
			}
			status, digest := b.readDigest(t, nil)
			if status != http.StatusOK {
				t.Fatalf("digest after the build -> %d, want 200", status)
			}
			if got := digest.Date.Format(time.DateOnly); got != tc.day {
				t.Errorf("the digest is dated %s, want %s", got, tc.day)
			}
		})
	}
}
