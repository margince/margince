// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A rule waiver in sonar-project.properties has to keep pointing at something,
// and there are two ways one stops — NEITHER of which announces itself.
//
// A waiver whose id is missing from `sonar.issue.ignore.multicriteria` is not
// applied at all: the scanner reads that list and nothing else. The entry sits
// in the file looking like an argument somebody accepted, while the finding it
// answers goes on counting against the quality gate.
//
// A waiver whose resourceKey names a path that has since been renamed, split or
// deleted covers nothing either. The finding simply moves to the new path and
// arrives as NEW code — which is how the decision deck's keyboard stop, argued
// for at length and waived, came back and reddened main's reliability rating
// weeks later: the file had been split into frame/stack/item and the waiver
// still named the file the `tabIndex` had left.
//
// Both obligations are read off the configuration itself and off `git ls-files`
// rather than restated here, so a waiver written tomorrow is in this census the
// moment it is committed.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sonarWaiver is one `sonar.issue.ignore.multicriteria.<id>` entry. The id is
// the key it is held under rather than a field, so there is one copy of it.
type sonarWaiver struct {
	rule     string
	resource string
}

var sonarWaiverKey = regexp.MustCompile(
	`^sonar\.issue\.ignore\.multicriteria\.([A-Za-z0-9]+)\.(ruleKey|resourceKey)=(.+)$`,
)

// sonarWaivers reads the scan configuration and parses it.
func sonarWaivers(t *testing.T) (map[string]*sonarWaiver, []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "sonar-project.properties"))
	if err != nil {
		t.Fatalf("reading the scan configuration: %v", err)
	}
	waivers, enrolled := parseSonarWaivers(string(raw))
	if len(waivers) == 0 || len(enrolled) == 0 {
		t.Fatal("sonar-project.properties declares no rule waivers — the gate found nothing to hold")
	}
	return waivers, enrolled
}

// parseSonarWaivers returns the waivers and, separately, the ids the scanner
// was told to apply. Separately BECAUSE the interesting failure is the two
// disagreeing; a parser that reconciled them here would have nothing to report.
//
// Takes the text rather than the path so the shapes this tree does not contain
// today can still be held — the continuation case below is one nothing in the
// real file exercises.
func parseSonarWaivers(text string) (map[string]*sonarWaiver, []string) {
	waivers := map[string]*sonarWaiver{}
	var enrolled []string
	// Backslash continuations joined first, the way sonarExclusions does it: the
	// multicriteria list is one line today, and a reformat onto several would
	// otherwise leave `\` parsing as a waiver id.
	joined := strings.ReplaceAll(text, "\\\n", "")
	for _, line := range strings.Split(joined, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "sonar.issue.ignore.multicriteria="); ok {
			for _, id := range strings.Split(after, ",") {
				if trimmed := strings.TrimSpace(id); trimmed != "" {
					enrolled = append(enrolled, trimmed)
				}
			}
			continue
		}
		match := sonarWaiverKey.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		waiver, ok := waivers[match[1]]
		if !ok {
			waiver = &sonarWaiver{}
			waivers[match[1]] = waiver
		}
		if match[2] == "ruleKey" {
			waiver.rule = strings.TrimSpace(match[3])
		} else {
			waiver.resource = strings.TrimSpace(match[3])
		}
	}
	return waivers, enrolled
}

// antMatcher compiles a resourceKey into the pattern SonarQube matches paths
// with: `**` crosses directories, `*` and `?` stay inside one segment.
func antMatcher(pattern string) *regexp.Regexp {
	var expr strings.Builder
	expr.WriteString("^")
	// No post statement: each arm advances by what it consumed, and a loop that
	// reads `i++` in its header while the body also moves `i` is the shape that
	// gets miscounted by whoever adds the next arm.
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			expr.WriteString("(?:[^/]+/)*")
			i += len("**/")
		case strings.HasPrefix(pattern[i:], "/**"):
			// The separator is required, not optional: `scripts/**` names what is
			// UNDER scripts, and a waiver whose whole subtree has gone should fail
			// here rather than match the bare directory name and pass.
			expr.WriteString("/.*")
			i += len("/**")
		case pattern[i] == '*':
			expr.WriteString("[^/]*")
			i++
		case pattern[i] == '?':
			expr.WriteString("[^/]")
			i++
		default:
			expr.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			i++
		}
	}
	expr.WriteString("$")
	return regexp.MustCompile(expr.String())
}

// TestEverySonarWaiverIsEnrolledAndComplete: a waiver the multicriteria list
// does not name is dead configuration, and one missing either half of its pair
// is refused by the scanner outright. Both read as an accepted argument to the
// next author who opens the file.
func TestEverySonarWaiverIsEnrolledAndComplete(t *testing.T) {
	t.Parallel()
	waivers, enrolled := sonarWaivers(t)

	named := map[string]bool{}
	for _, id := range enrolled {
		if named[id] {
			t.Errorf("sonar.issue.ignore.multicriteria names %s twice", id)
		}
		named[id] = true
		if _, ok := waivers[id]; !ok {
			t.Errorf("sonar.issue.ignore.multicriteria names %s, which declares no ruleKey or resourceKey", id)
		}
	}
	for id, waiver := range waivers {
		if !named[id] {
			t.Errorf("%s declares a waiver (%s on %s) that sonar.issue.ignore.multicriteria does not name, so the scan never applies it", id, waiver.rule, waiver.resource)
		}
		if waiver.rule == "" || waiver.resource == "" {
			t.Errorf("%s declares ruleKey %q and resourceKey %q — the scanner needs both", id, waiver.rule, waiver.resource)
		}
	}
}

// TestEverySonarWaiverNamesSomethingInTheTree: a resourceKey that matches no
// tracked path waives nothing. The finding it was written for has moved, and it
// comes back as new code on whatever path now holds it.
func TestEverySonarWaiverNamesSomethingInTheTree(t *testing.T) {
	t.Parallel()
	waivers, _ := sonarWaivers(t)
	// The package's own listing of committed paths: the analysis runs on a fresh
	// checkout, so what is committed is exactly what it indexes.
	paths := trackedFiles(t)
	if len(paths) == 0 {
		t.Fatal("git reported no tracked file at all — a census over an empty tree reports PASS exactly like a clean one")
	}

	for id, waiver := range waivers {
		if waiver.resource == "" {
			continue // Reported by the completeness gate above.
		}
		matcher := antMatcher(waiver.resource)
		matched := false
		for _, file := range paths {
			if matcher.MatchString(file.path) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("%s waives %s on %q, which matches no tracked file — the rule it answers now fires somewhere else", id, waiver.rule, waiver.resource)
		}
	}
}

// TestTheResourceKeyMatcherIsNeitherBlindNorGreedy: the census above is only as
// good as this translation. A pattern compiled too loosely — `**` allowed to
// stand for nothing, say, or `*` allowed to cross a slash — matches some path
// in a tree this size whatever it was written to name, and the gate then
// reports PASS over every stale waiver at once. So the greedy direction is
// asserted here alongside the ordinary one.
func TestTheResourceKeyMatcherIsNeitherBlindNorGreedy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pattern string
		matches []string
		misses  []string
	}{{
		pattern: "scripts/dev.sh",
		matches: []string{"scripts/dev.sh"},
		misses:  []string{"scripts/dev.sh.bak", "other/scripts/dev.sh", "scripts/devxsh"},
	}, {
		pattern: "scripts/**",
		matches: []string{"scripts/dev.sh", "scripts/lib/testdb.sh"},
		misses:  []string{"scripts", "frontend/scripts/build.mjs"},
	}, {
		pattern: "**/*_test.go",
		matches: []string{"a_test.go", "backend/gates/a_test.go"},
		misses:  []string{"backend/gates/a.go", "a_test.golden"},
	}, {
		pattern: "extensions/*/go.mod",
		matches: []string{"extensions/one/go.mod"},
		// `*` stays inside one segment: a nested unit is a different path, and a
		// waiver written for the tier's top level must not silently cover it.
		misses: []string{"extensions/one/two/go.mod", "extensions/go.mod"},
	}, {
		pattern: "**/*.png",
		matches: []string{"logo.png", "frontend/src/a/logo.png"},
		misses:  []string{"logo.png.license", "logopng"},
	}} {
		t.Run(tc.pattern, func(t *testing.T) {
			t.Parallel()
			matcher := antMatcher(tc.pattern)
			for _, path := range tc.matches {
				if !matcher.MatchString(path) {
					t.Errorf("%q should match %q, and does not", tc.pattern, path)
				}
			}
			for _, path := range tc.misses {
				if matcher.MatchString(path) {
					t.Errorf("%q must not match %q — a matcher this loose makes the census vacuous", tc.pattern, path)
				}
			}
		})
	}
}

// TestTheWaiverParserReadsTheShapesAPropertiesFileCanTake: the two censuses
// above are only as good as this parse, and a parse that quietly recognised
// FEWER waivers than the file holds would pass both of them over the ones it
// missed. The continuation case is here because nothing in the real file
// exercises it — the list is one line today, and the day somebody wraps it is
// the day an unrecognised `\` would otherwise become a waiver id.
func TestTheWaiverParserReadsTheShapesAPropertiesFileCanTake(t *testing.T) {
	t.Parallel()
	const wrapped = `# A comment naming sonar.issue.ignore.multicriteria.e9.ruleKey is prose.
sonar.issue.ignore.multicriteria=e1,\
  e2
sonar.issue.ignore.multicriteria.e1.ruleKey=go:S1
sonar.issue.ignore.multicriteria.e1.resourceKey=backend/**
sonar.issue.ignore.multicriteria.e2.ruleKey=typescript:S2
sonar.issue.ignore.multicriteria.e2.resourceKey=frontend/src/a.ts
`
	waivers, enrolled := parseSonarWaivers(wrapped)

	if got := strings.Join(enrolled, ","); got != "e1,e2" {
		t.Errorf("a wrapped multicriteria list read as %q, want %q", got, "e1,e2")
	}
	if len(waivers) != 2 {
		t.Fatalf("read %d waiver(s) from a file holding 2: %v", len(waivers), waivers)
	}
	if waivers["e2"].rule != "typescript:S2" || waivers["e2"].resource != "frontend/src/a.ts" {
		t.Errorf("e2 read as %+v", *waivers["e2"])
	}

	// A waiver missing half its pair is reported as such rather than dropped: a
	// parser that skipped it would hand the completeness census nothing to fail.
	half, _ := parseSonarWaivers("sonar.issue.ignore.multicriteria.e1.ruleKey=go:S1\n")
	if len(half) != 1 || half["e1"].resource != "" {
		t.Errorf("a waiver with no resourceKey read as %v, want one entry with an empty resource", half)
	}
}
