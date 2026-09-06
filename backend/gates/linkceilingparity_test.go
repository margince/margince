// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The per-activity link ceiling is one number, wherever it is spelled.
//
// It is spelled four times: `activities.maxActivityLinks` owns it, two sibling
// modules mirror it because a module never imports a sibling, and the trigger
// that makes it hold against two writers at once carries it in SQL. Nothing
// tied them together, so one moving left the others silently disagreeing — and
// which ceiling applied depended on which writer ran, which is exactly the
// defect the trigger was added to end.
//
// The mirrors are FOUND rather than listed. Each one names the owner in its own
// doc comment, because that is how a reader is told it is a mirror, so a fourth
// written the same way is held by this gate the day it is written. A floor
// underneath says the search still finds the two that exist: a census that
// stops seeing its subject reports a pass.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	// ceilingOwner is the declaration the others mirror.
	ceilingOwner     = "backend/internal/modules/activities/activitylinks.go"
	ceilingOwnerName = "maxActivityLinks"
	// ceilingTrigger carries the number in SQL, where it is enforced for every
	// writer at once. Read from the migration rather than the head catalog: the
	// catalog records that the trigger exists, never what it says.
	ceilingTrigger = "backend/migrations/core/" +
		"1788662190_a_link_ceiling_that_two_writers_cannot_exceed.up.sql"
	// ceilingMirrors is how many sibling modules carry a copy today. A floor,
	// not a target: fewer means the search stopped finding them.
	ceilingMirrors = 2
)

// triggerCeiling reads the number the trigger refuses past.
var triggerCeiling = regexp.MustCompile(`filed\s*>=\s*(\d+)`)

// mirrorConst matches the name a mirrored ceiling is given. Loose on purpose —
// the three that exist are spelled three different ways, and a fourth author
// will spell it a fourth.
var mirrorConst = regexp.MustCompile(`(?i)^max.*link`)

func TestTheLinkCeilingHasOneValue(t *testing.T) {
	t.Parallel()

	want, ok := intConstIn(t, ceilingOwner, ceilingOwnerName)
	if !ok {
		t.Fatalf("%s no longer declares %s as an integer constant — this gate is reading a shape "+
			"that is gone, and every mirror below would then be compared against nothing",
			ceilingOwner, ceilingOwnerName)
	}

	mirrors := 0
	root := filepath.Join(repoRoot, "backend", "internal")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel := filepath.ToSlash(mustRel(t, path))
		if rel == ceilingOwner {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// A file that names the owner is a file claiming to mirror it. That is
		// the claim this gate checks, and it is the one every mirror makes.
		if !strings.Contains(string(raw), ceilingOwnerName) {
			return nil
		}
		for name, value := range intConstsIn(t, path) {
			if !mirrorConst.MatchString(name) {
				continue
			}
			mirrors++
			if value != want {
				t.Errorf("%s declares %s = %d and %s owns %d — one ceiling moved and the other did "+
					"not, so which one applies depends on which writer ran",
					rel, name, value, ceilingOwnerName, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if mirrors < ceilingMirrors {
		t.Errorf("found %d mirrored ceiling(s), want at least %d: the search reads a file that "+
			"names %s and takes its max…link constant, so fewer means a mirror stopped saying "+
			"what it mirrors — and an unchecked copy is how the number drifts",
			mirrors, ceilingMirrors, ceilingOwnerName)
	}

	sql, err := os.ReadFile(filepath.Join(repoRoot, ceilingTrigger))
	if err != nil {
		t.Fatalf("reading the ceiling trigger: %v", err)
	}
	match := triggerCeiling.FindStringSubmatch(string(sql))
	if match == nil {
		t.Fatalf("%s no longer refuses past a number this gate can read — the SQL is where the "+
			"ceiling actually holds, and an unread one is the copy nothing checks", ceilingTrigger)
	}
	inSQL, convErr := strconv.Atoi(match[1])
	if convErr != nil {
		t.Fatalf("%s: %q is not a count", ceilingTrigger, match[1])
	}
	if inSQL != want {
		t.Errorf("the trigger refuses past %d and %s owns %d — the database and the callers "+
			"disagree about the same ceiling, and the database is the one that wins",
			inSQL, ceilingOwnerName, want)
	}
}

// intConstIn answers one named integer constant from one file.
func intConstIn(t *testing.T, rel, name string) (int, bool) {
	t.Helper()
	value, ok := intConstsIn(t, filepath.Join(repoRoot, rel))[name]
	return value, ok
}

// intConstsIn answers every integer constant a file declares, by name.
func intConstsIn(t *testing.T, path string) map[string]int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	out := map[string]int{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		literal, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.INT {
			return true
		}
		value, convErr := strconv.Atoi(literal.Value)
		if convErr != nil {
			return true
		}
		out[spec.Names[0].Name] = value
		return true
	})
	return out
}

func mustRel(t *testing.T, path string) string {
	t.Helper()
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		t.Fatalf("relativising %s: %v", path, err)
	}
	return rel
}
