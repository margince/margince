// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apideprecation

import (
	"testing"
	"time"
)

func instant(t *testing.T, raw string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestNoticeUsesThePublicationDateAndCalendarMonths(t *testing.T) {
	for _, tc := range []struct{ announced, want string }{
		{"2026-01-31T12:00:00Z", "2026-04-30T12:00:00Z"},
		{"2026-11-29T23:00:00-05:00", "2027-02-28T04:00:00Z"},
		{"2026-11-30T04:00:00Z", "2027-02-28T04:00:00Z"},
		{"2027-11-30T12:00:00Z", "2028-02-29T12:00:00Z"},
		{"2026-11-30T12:00:00+07:00", "2027-02-28T12:00:00+07:00"},
	} {
		if got := EarliestRemoval(instant(t, tc.announced)); !got.Equal(instant(t, tc.want)) {
			t.Errorf("%s expires at %s, want %s", tc.announced, got, tc.want)
		}
	}
}

func TestPendingAndPublishedNoticesHaveDifferentHeaders(t *testing.T) {
	pending, err := (Notice{}).Headers()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Get("Link") != PolicyLink || pending.Get("Deprecation") != "" || pending.Get("Sunset") != "" {
		t.Fatal("pending notice invented dates or omitted its policy")
	}
	n := Notice{Release: "1970.123", AnnouncedAt: "2026-01-31T12:00:00Z", SunsetAt: "2026-04-30T12:00:00Z"}
	headers, err := n.Headers()
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Deprecation") != "@1769860800" || headers.Get("Sunset") != "Thu, 30 Apr 2026 12:00:00 GMT" {
		t.Fatalf("dated headers=%v", headers)
	}
	n.SunsetAt = "2026-04-30T11:59:59Z"
	if _, err := n.Headers(); err == nil {
		t.Fatal("early sunset accepted")
	}
}

func TestRemovalRequiresANoticeWindowAndANonPatchRelease(t *testing.T) {
	n := Notice{Release: "v1.0.0", AnnouncedAt: "2026-01-31T12:00:00Z"}
	at := instant(t, "2026-04-30T12:00:00Z")
	for _, release := range []string{"", "v1.0.1", "v1.0.0", "v1.1.0-rc.1", "v1.1", "1970.124"} {
		if err := n.CheckRemoval(release, at); err == nil {
			t.Errorf("removal in %q accepted", release)
		}
	}
	if err := n.CheckRemoval("v1.1.0", at); err != nil {
		t.Fatal(err)
	}
	if err := n.CheckRemoval("v1.1.0", at.Add(-time.Nanosecond)); err == nil {
		t.Fatal("removal before the full window accepted")
	}
	if err := (Notice{}).CheckRemoval("v1.1.0", at); err == nil {
		t.Fatal("unannounced removal accepted")
	}
	n.SunsetAt = "2026-05-01T12:00:00Z"
	if err := n.CheckRemoval("v1.1.0", at); err == nil {
		t.Fatal("published later sunset ignored")
	}
}
