// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the rail names a site as, for every shape an override URL can arrive
// in.

import "testing"

func TestSiteHostOfNamesTheSiteAReaderWouldRecognise(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want string
	}{
		{"mixed case host and leading www", "https://WWW.Example.com/path", "example.com"},
		{"plain host, no www", "https://zenloop.com", "zenloop.com"},
		{"mixed case host, path and www", "https://www.Zenloop.COM/about", "zenloop.com"},
		{"not a URL at all", "://bad", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := siteHostOf(c.url); got != c.want {
				t.Errorf("siteHostOf(%q) = %q, want %q", c.url, got, c.want)
			}
		})
	}
}
