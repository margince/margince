// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package buildinfo

import "testing"

func TestVersionLineNamesTheStampedReleaseAndRevision(t *testing.T) {
	stamp(t, "v1.2.0", "4e1b736eb")
	if got, want := VersionLine("margince-api"), "margince-api v1.2.0 (revision 4e1b736eb)"; got != want {
		t.Errorf("VersionLine = %q, want %q", got, want)
	}
}

func TestVersionLineSaysDevForAFactTheBuildDidNotStamp(t *testing.T) {
	stamp(t, "", "")
	if got, want := VersionLine("margince-worker"), "margince-worker dev (revision dev)"; got != want {
		t.Errorf("VersionLine = %q, want %q", got, want)
	}
}

func TestAsksForVersionOnlyWhenItIsTheWholeCommandLine(t *testing.T) {
	cases := map[string]struct {
		args []string
		want bool
	}{
		"alone":          {[]string{"--version"}, true},
		"no arguments":   {nil, false},
		"with a command": {[]string{"up", "--version"}, false},
		"single dash":    {[]string{"-version"}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := AsksForVersion(tc.args); got != tc.want {
				t.Errorf("AsksForVersion(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func stamp(t *testing.T, release, revision string) {
	t.Helper()
	savedRelease, savedRevision := ReleaseVersion, Revision
	ReleaseVersion, Revision = release, revision
	t.Cleanup(func() { ReleaseVersion, Revision = savedRelease, savedRevision })
}
