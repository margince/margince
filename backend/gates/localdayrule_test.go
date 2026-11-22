// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// One shared rule answers "which day is it for this reader". A file that picks
// a surface through compose/dayrule or occurredday.Personal must not also work
// out the installation's day another way. Otherwise one person gets two
// answers. The files that pick a surface are read from the code. A new reader
// that adopts the rule is held to it with no list to update.

import (
	"strings"
	"testing"
)

// installationOnlyDay is every spelling that resolves a day or zone from the
// installation alone, bypassing the surface choice.
var installationOnlyDay = []string{
	"identity.ZoneAppliedTx(", "identity.TimezoneAppliedTx(", "identity.TimezoneOf(",
	"occurredday.InstallationZoneSQL", "occurredday.TodaySQL(",
}

// personalPackages hold lists that belong to one person. Nothing in them may
// resolve the installation's day directly, even in a file that does not yet
// import the rule.
var personalPackages = []string{"internal/compose/briefs/"}

// minimumRuleReaders fails the census when the walk finds fewer files that
// pick a surface than the routing needs. A walk that finds too few reads a
// smaller tree and reports a pass.
const minimumRuleReaders = 8

func TestEveryReaderOfALocalDayPicksItsSurfaceThroughTheDayRule(t *testing.T) {
	picksSurface := 0
	walkHandWrittenGoFiles(t, "internal", func(path, text string) {
		if strings.HasSuffix(path, "_test.go") {
			return
		}
		if strings.Contains(path, "/compose/dayrule/") {
			return
		}
		usesRule := strings.Contains(text, "compose/dayrule\"") || strings.Contains(text, "occurredday.Personal")
		if usesRule {
			picksSurface++
		}
		inPersonalPackage := false
		for _, pkg := range personalPackages {
			inPersonalPackage = inPersonalPackage || strings.Contains(path, pkg)
		}
		if !usesRule && !inPersonalPackage {
			return
		}
		for _, spelling := range installationOnlyDay {
			if strings.Contains(text, spelling) {
				t.Errorf("%s computes the installation's day with %s; pick a surface through dayrule.Zone or dayrule.DayAt instead", path, spelling)
			}
		}
	})
	if picksSurface < minimumRuleReaders {
		t.Fatalf("found %d files that pick a surface through the day rule, want at least %d: the census is too short", picksSurface, minimumRuleReaders)
	}
}
