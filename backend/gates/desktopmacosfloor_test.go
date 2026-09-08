// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// Every macOS build script pins the bundle's OS floor before it compiles
// anything.
//
// A compiler on macOS stamps its output with the deployment target it was
// given, and clang's default is the OS of the machine doing the build. A build
// step that does not source desktop/build/macos-target.sh therefore produces
// binaries that refuse to launch on any Mac older than the builder's — a floor
// nobody chose, that moves whenever the build machine takes an OS update, and
// that is invisible to the builder, whose own Mac always satisfies it.
//
// This gate exists because the desktop bundle shipped for three weeks with its
// Go half unpinned and looked correct the whole time. Go's own linker stamps
// the floor its toolchain supports, which is the same number macos-target.sh
// declares, so the omission was masked — until a dependency arrived carrying a
// darwin-only cgo file, which handed the link to clang and let clang's default
// through. The bundle's floor had been resting on a transitive dependency's
// build tags rather than on a decision.
//
// The lane that would have caught it runs only on a macOS runner, and only for
// pull requests touching desktop/**. The change that broke it touched
// backend/go.mod. This gate is the cheap half of the answer: it runs on every
// pull request, on Linux, and reads the scripts rather than their output.
//
// WHAT IT CANNOT SEE, and why H1 rather than H3. The corpus is a total list —
// every *.sh in the directory, no skip-list — but whether a script pins the
// floor is decided by a regex over shell text, and text can lie about what
// runs. Three ways this passes something it should not:
//
//   a script that sources the floor and then clears or overrides
//   MACOSX_DEPLOYMENT_TARGET before compiling; a top-level source placed AFTER
//   a top-level compiler call; and any spelling of the source command this
//   regex does not cover, which shows up as a false FAILURE rather than a false
//   pass and is the safe direction to be wrong in.
//
// The source must sit in column ONE, which is what rules out the near misses
// worth ruling out: a source buried in a function body or a conditional branch
// is indented in this tree, so it cannot satisfy this. Combined with the shape
// every script here has — compilers inside functions, one `main "$@"` on the
// last line — a column-one source necessarily runs before any compiler. That
// shape is not itself gated, which is the residual hole and the reason for the
// paragraph above.
//
// It also judges only the macOS lane: the Windows .ps1 scripts carry no
// equivalent stamp, so they have nothing to pin.
//
// None of this is the last line of defence. What the scripts actually PRODUCE
// is held by assert_min_os inside the bundle build, reading the Mach-O of every
// binary on a real Mac — an H3 check on the output, which this one only tries to
// beat to the answer by twenty minutes and one macOS runner.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	// The script that declares the floor, and the directory holding the lane
	// that sources it — relative to the module root TestMain lands every gate
	// at. macos-target.sh is excluded from the corpus below because it is the
	// subject, not a subscriber.
	macOSFloorScript = "macos-target.sh"
	macOSBuildDir    = "../desktop/build"

	// Below the four scripts present when this gate was written, so a glob that
	// stops matching after a rename fails here rather than certifying an empty
	// directory. It is a floor on the SCAN, not a claim about how many build
	// steps the lane ought to have.
	macOSBuildScriptFloor = 3
)

// sourcesFloor matches a shell source of the floor script, in either spelling
// bash accepts, anchored to COLUMN ONE.
//
// The anchor is the check's teeth: an indented source is inside a function body
// or a branch, and either can go unexecuted while reading exactly like a script
// that pins the floor. Every script in this lane sources at column one already,
// so the anchor costs nothing and closes the near miss.
//
// The path is deliberately loose: which variable holds the script's directory is
// the script's business, and pinning it here would fail on a rename that changed
// nothing that matters.
var sourcesFloor = regexp.MustCompile(`^(\.|source)\s+\S*` + regexp.QuoteMeta(macOSFloorScript))

// exportsDeploymentTarget matches the floor script's own export. Checked so
// that gutting macos-target.sh cannot leave every script dutifully sourcing a
// file that no longer sets anything — the failure this gate would otherwise
// certify as clean.
var exportsDeploymentTarget = regexp.MustCompile(`(?m)^export MACOSX_DEPLOYMENT_TARGET=`)

func TestEveryMacOSBuildScriptPinsTheDeploymentTarget(t *testing.T) {
	t.Parallel()

	floor, err := os.ReadFile(filepath.Join(macOSBuildDir, macOSFloorScript))
	if err != nil {
		t.Fatalf("reading the script that declares the macOS floor: %v", err)
	}
	if !exportsDeploymentTarget.Match(floor) {
		t.Fatalf("%s/%s no longer exports MACOSX_DEPLOYMENT_TARGET, so every script that sources "+
			"it is pinning nothing and the bundle's OS floor is again the build machine's",
			macOSBuildDir, macOSFloorScript)
	}

	scripts, err := filepath.Glob(filepath.Join(macOSBuildDir, "*.sh"))
	if err != nil {
		t.Fatalf("listing the macOS build scripts: %v", err)
	}

	// Two lists, both read from the tree: every script in the lane, and those
	// that pin the floor. There is no waiver map and no compiler-detection
	// heuristic — a heuristic is what goes short when the next script compiles
	// by a spelling it does not recognise, and sourcing costs a script that
	// ships nothing exactly nothing. Every script in this directory pins the
	// floor; that an exemption has never been needed is why none exists.
	var inLane, pinned []string
	for _, script := range scripts {
		name := filepath.Base(script)
		if name == macOSFloorScript {
			continue
		}
		inLane = append(inLane, name)

		body, err := os.ReadFile(script)
		if err != nil {
			t.Fatalf("reading %s: %v", script, err)
		}
		if pinsFloor(string(body)) {
			pinned = append(pinned, name)
		}
	}

	if len(inLane) < macOSBuildScriptFloor {
		t.Fatalf("found %d script(s) under %s, fewer than the %d this gate was written against — "+
			"the glob has stopped matching the lane it judges, and an empty scan reports clean",
			len(inLane), macOSBuildDir, macOSBuildScriptFloor)
	}

	// missingFrom is this package's shared list diff, and takes what is HELD
	// first: the scripts that pin, then the lane they are drawn from.
	unpinned := missingFrom(pinned, inLane)
	if len(unpinned) > 0 {
		t.Fatalf("%d of %d macOS build script(s) do not source %s: %s\n"+
			"Anything a script in this directory compiles is stamped with the build machine's OS "+
			"unless the floor is sourced first, and will then refuse to launch on an older Mac. "+
			"Add `. \"$HERE/%s\"` near the top, the way build-postgres.sh and build-valkey.sh do.",
			len(unpinned), len(inLane), macOSFloorScript, strings.Join(unpinned, ", "), macOSFloorScript)
	}
}

// pinsFloor reports whether a script sources the floor from a line bash runs on
// the way in.
//
// Comment lines go first, because this file's own failure message spells the
// source command and several of these scripts explain the target in prose — a
// gate that read either would certify a script that only talks about pinning.
// The column-one anchor in sourcesFloor does the rest; pinsFloorCases covers
// the near misses.
func pinsFloor(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if sourcesFloor.MatchString(line) {
			return true
		}
	}
	return false
}

// pinsFloorCases are the near misses, each a line that reads like a script
// pinning the floor and is not one. They exist because the predicate is a regex
// and a regex is exactly the thing that quietly says yes: the gate's value is
// what it REFUSES, and nothing else in this file demonstrates a refusal.
var pinsFloorCases = []struct {
	name string
	body string
	want bool
}{
	{"a top-level source pins it", ". \"$HERE/macos-target.sh\"", true},
	{"and so does the source keyword", "source \"$HERE/macos-target.sh\"", true},
	{"a bare relative path is still a source", ". ./macos-target.sh", true},
	{
		// The case that made the anchor worth having: bash runs this only if
		// something calls the function, and nothing here checks that anything
		// does.
		name: "a source inside a function body is not a pin",
		body: "setup() {\n  . \"$HERE/macos-target.sh\"\n}\n",
		want: false,
	},
	{
		name: "nor is one inside a branch that may never be taken",
		body: "if [[ -n \"${RELEASE:-}\" ]]; then\n  . \"$HERE/macos-target.sh\"\nfi\n",
		want: false,
	},
	{
		// Every script in this lane discusses the deployment target, and this
		// file's own failure message spells the command.
		name: "a comment describing the pin is not a pin",
		body: "# Source it first: . \"$HERE/macos-target.sh\"\n",
		want: false,
	},
	{"an indented comment is not a pin either", "  # . \"$HERE/macos-target.sh\"\n", false},
	{"naming the file without sourcing it is not a pin", "echo macos-target.sh\n", false},
	{"a script that never mentions it does not pin it", "set -euo pipefail\ngo build ./...\n", false},
}

func TestPinsFloorRefusesASourceBashMayNeverRun(t *testing.T) {
	t.Parallel()

	for _, tc := range pinsFloorCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := pinsFloor(tc.body); got != tc.want {
				t.Errorf("pinsFloor(%q) = %t, want %t", tc.body, got, tc.want)
			}
		})
	}
}
