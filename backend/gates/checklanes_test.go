// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// `make check` runs every leg its two halves, check-backend and check-fe, run.
//
// CI runs the two halves as separate jobs. `make check` runs the same legs in
// another shape: the writers first, then lanes side by side. A leg that only a
// half reaches runs in CI and never on a laptop. The laptop run is the one the
// rulebook asks for before every push.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const mergeGate = "check"

var mergeGateHalves = []string{"check-backend", "check-fe"}

// legsUnder walks a target over its prerequisites and sub-makes and returns
// every target it reaches. A sub-make into another directory is recorded as
// "<dir>/<target>" and not followed, because that directory has its own Makefile.
func legsUnder(targets map[string]*recipeTarget, vars map[string]string, root string) map[string]bool {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		target := targets[name]
		if target == nil {
			return
		}
		next := slices.Clone(target.pulls)
		for _, recipe := range target.recipes {
			for _, sub := range subMakes(expandRefs(recipe, vars)) {
				if sub.dir == "" {
					next = append(next, sub.targets...)
					continue
				}
				for _, remote := range sub.targets {
					seen[sub.dir+"/"+remote] = true
				}
			}
		}
		for _, name := range next {
			walk(name)
		}
	}
	walk(root)
	return seen
}

// legsTheGateMisses answers which legs a half reaches that the gate does not.
func legsTheGateMisses(t *testing.T, makefile, gate string) []string {
	t.Helper()
	targets, vars := parseRecipes(t, makefile)
	whole := legsUnder(targets, vars, gate)
	var missing []string
	for _, half := range mergeGateHalves {
		if targets[half] == nil {
			t.Fatalf("%s declares no %q target, so this gate was about to compare nothing", makefile, half)
		}
		for leg := range legsUnder(targets, vars, half) {
			if leg != half && !whole[leg] {
				missing = append(missing, half+" → "+leg)
			}
		}
	}
	slices.Sort(missing)
	return missing
}

func TestMakeCheckRunsEveryLegItsTwoHalvesRun(t *testing.T) {
	t.Parallel()
	targets, vars := parseRecipes(t, rootMakefile)

	// The walk must follow both kinds of edge, or the comparison below holds
	// over a graph too small to miss anything.
	for half, leg := range map[string]string{"check-backend": "backend/check", "check-fe": "fe-unit"} {
		if !legsUnder(targets, vars, half)[leg] {
			t.Fatalf("the walk from `make %s` does not reach %s, which its recipe runs — this gate stopped following its sub-makes", half, leg)
		}
	}
	for _, leg := range legsTheGateMisses(t, rootMakefile, mergeGate) {
		t.Errorf("%s: `make check` does not reach this leg, so it runs in CI and never in the local merge gate. "+
			"Run it in one of the lanes, or as a writer before them", leg)
	}
}

func TestTheLegCheckFailsWhenTheMergeGateDropsALeg(t *testing.T) {
	t.Parallel()
	body, err := gatekit.ReadFile(rootMakefile)
	if err != nil {
		t.Fatalf("reading %s: %v", rootMakefile, err)
	}
	// A merge gate that runs the backend half and forgot the frontend one.
	planted := string(body) + "\n" + mergeGate + "-planted:\n\t$(MAKE) check-backend\n"
	path := filepath.Join(t.TempDir(), "Makefile")
	if err := os.WriteFile(path, []byte(planted), 0o600); err != nil {
		t.Fatalf("writing the fixture Makefile: %v", err)
	}
	missing := legsTheGateMisses(t, path, mergeGate+"-planted")
	if !slices.Contains(missing, "check-fe → fe-unit") || slices.Contains(missing, "check-backend → backend/check") {
		t.Fatalf("a gate that runs only check-backend misses %q; want check-fe's legs and none of check-backend's", missing)
	}
}
