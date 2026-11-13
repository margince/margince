// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
)

const movedZonesPath = "data/moved-windows-zones.txt"

// cldrNewer names a newer CLDR windowsZones.xml to rewrite movedZonesPath
// from; data/README.md gives the command.
var cldrNewer = flag.String("cldr-newer", "", "path to a newer CLDR windowsZones.xml; rewrites "+movedZonesPath)

// movedZones lists the zones whose Windows zone newer gives differently from
// older, or not at all, sorted.
func movedZones(t *testing.T, older, newer []byte) []string {
	t.Helper()
	before, err := cldrMapping(older)
	if err != nil {
		t.Fatal(err)
	}
	after, err := cldrMapping(newer)
	if err != nil {
		t.Fatal(err)
	}
	var moved []string
	for zone, windows := range before {
		if after[zone] != windows {
			moved = append(moved, zone)
		}
	}
	slices.Sort(moved)
	return moved
}

func TestMovedWindowsZonesAreCLDRZones(t *testing.T) {
	if *cldrNewer != "" {
		newer, err := os.ReadFile(*cldrNewer)
		if err != nil {
			t.Fatal(err)
		}
		moved := movedZones(t, cldrWindowsZones, newer)
		if err := os.WriteFile(movedZonesPath, []byte(strings.Join(moved, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d moved zones to %s", len(moved), movedZonesPath)
		return
	}
	mapped, err := cldrMapping(cldrWindowsZones)
	if err != nil {
		t.Fatal(err)
	}
	listed := strings.Fields(string(cldrMovedZones))
	if !slices.IsSorted(listed) {
		t.Errorf("%s is not sorted; regenerate it as data/README.md says", movedZonesPath)
	}
	for _, zone := range listed {
		if _, ok := mapped[zone]; !ok {
			t.Errorf("%s names %s, which the copied windowsZones.xml does not map", movedZonesPath, zone)
		}
	}
}
