// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// One pnpm version, and package.json's "packageManager" field is it.
//
// Before that field existed the version was whatever the environment reached
// for: the workflows asked pnpm/action-setup for the `11` range, the image
// build ran `corepack enable` with no manifest to read and got whatever npm
// called latest that morning, and a laptop ran whatever it had installed. So
// the toolchain a build used was chosen by an upstream release schedule rather
// than by this repository, and a pnpm major landed in the image with nothing in
// the tree having changed — which is how a green commit built red.
//
// The obligation is derived from the field rather than listed here: a version
// written anywhere else is a second source of the same answer, and the two
// disagree the first time one is bumped alone. pnpm/action-setup refuses that
// case outright (ERR_PNPM_BAD_PM_VERSION); corepack does not, and picks
// whichever it happened to read.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pnpmVersionLiteral matches a concrete pnpm version wherever it is written:
// the `pnpm@11.25.0` corepack spelling and the `version: 11` input
// pnpm/action-setup takes. Only package.json may carry one.
var (
	pnpmVersionLiteral = regexp.MustCompile(`pnpm@\d`)
	actionSetupStep    = regexp.MustCompile(`(?m)^(?P<indent>[ \t]*)- uses: pnpm/action-setup@`)
	packageManagerPin  = regexp.MustCompile(`^pnpm@\d+\.\d+\.\d+(\+[a-z0-9]+\.[0-9a-f]+)?$`)
)

func TestThePnpmVersionHasOneSource(t *testing.T) {
	t.Parallel()
	want := pinnedPackageManager(t)

	// Every step that installs pnpm in CI must take the version from the pin
	// rather than restate it. The scan is over the workflow directory, not a
	// list: a lane added tomorrow is covered the day it is written.
	t.Run("no workflow states a version of its own", func(t *testing.T) {
		files, err := filepath.Glob(filepath.Join(repoRoot, ".github", "workflows", "*.yml"))
		if err != nil {
			t.Fatalf("listing workflows: %v", err)
		}
		var steps int
		for _, file := range files {
			steps += assertActionSetupReadsThePin(t, file)
		}
		if steps == 0 {
			t.Fatal("no workflow installs pnpm — a scan that finds no step reports clean exactly like one that checked every step, so either the action moved or this pattern is stale")
		}
	})

	// The image build has no action to read the field for it: `corepack enable`
	// installs a shim, and a shim with nothing to read resolves latest. So every
	// stage that runs pnpm must have activated the pin first.
	t.Run("the image build activates the pin before it runs pnpm", func(t *testing.T) {
		assertDockerfileActivatesThePin(t)
	})

	// And nowhere else. A version written a second time is the failure this
	// gate exists for, whether it is a workflow input, a Dockerfile argument or
	// a line of prose that goes stale.
	t.Run("nothing else in the tree names a pnpm version", func(t *testing.T) {
		files := trackedFiles(t)
		if len(files) == 0 {
			t.Fatal("the index lists no file — a sweep over nothing reports the clean tree it never read")
		}
		for _, file := range files {
			if file.symlink {
				continue
			}
			assertNamesNoPnpmVersion(t, file.path, want)
		}
	})
}

// pinnedPackageManager reads the field every other reader resolves, and holds
// it to an EXACT version: corepack accepts a range, and a range puts the choice
// back with npm's release schedule, which is the whole defect.
func pinnedPackageManager(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "package.json"))
	if err != nil {
		t.Fatalf("reading the workspace root manifest: %v", err)
	}
	// Decoded through a map rather than a tagged struct: the field is npm's,
	// spelled the way npm spells it, and a struct tag naming it is the one
	// place in this tree that has to disagree with the repo's casing rule.
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("package.json does not parse: %v", err)
	}
	var pin string
	if raw, declared := manifest["packageManager"]; declared {
		if err := json.Unmarshal(raw, &pin); err != nil {
			t.Fatalf("package.json declares a packageManager that is not a string: %v", err)
		}
	}
	if !packageManagerPin.MatchString(pin) {
		t.Fatalf("package.json declares packageManager %q, want an exact pnpm@<major>.<minor>.<patch> (optionally with corepack's integrity suffix) — "+
			"a range or an empty field hands the choice back to whatever npm published this morning", pin)
	}
	return pin
}

// assertActionSetupReadsThePin reports every action-setup step in one workflow
// that carries a version of its own, and returns how many steps it saw so the
// caller can refuse a sweep that found none.
func assertActionSetupReadsThePin(t *testing.T, file string) int {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	lines := strings.Split(string(body), "\n")
	var steps int
	for i, line := range lines {
		match := actionSetupStep.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		steps++
		// The step's own block: everything indented past the `- `, which ends
		// at the next line at or above the step's own indentation.
		indent := match[1]
		for _, within := range lines[i+1:] {
			if strings.TrimSpace(within) == "" {
				continue
			}
			if !strings.HasPrefix(within, indent+" ") {
				break
			}
			if strings.HasPrefix(strings.TrimSpace(within), "version:") {
				t.Errorf("%s:%d gives pnpm/action-setup a version of its own (%s) — omit it and the action reads package.json's packageManager; both is ERR_PNPM_BAD_PM_VERSION",
					file, i+1, strings.TrimSpace(within))
				break
			}
		}
	}
	return steps
}

// assertDockerfileActivatesThePin walks the stages rather than looking for one
// known line: the obligation is that pnpm never runs on a version nobody chose,
// and a stage added later inherits it.
func assertDockerfileActivatesThePin(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repoRoot, "Dockerfile"))
	if err != nil {
		t.Fatalf("reading the Dockerfile: %v", err)
	}
	var activated bool
	var invocations int
	for i, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "FROM ") {
			activated = false
			continue
		}
		if strings.Contains(trimmed, "corepack prepare") && strings.Contains(trimmed, "packageManager") {
			activated = true
			continue
		}
		if !strings.Contains(trimmed, "pnpm ") {
			continue
		}
		invocations++
		if !activated {
			t.Errorf("Dockerfile:%d runs pnpm before the stage activates package.json's packageManager — corepack resolves latest here\n\t%s", i+1, trimmed)
		}
	}
	if invocations == 0 {
		t.Fatal("the Dockerfile runs no pnpm — a walk that finds nothing to check passes exactly like one that checked every stage, so either the build moved or this scan is stale")
	}
}

// assertNamesNoPnpmVersion allows the pin itself and nothing else, over the
// WHOLE index rather than a chosen set of file types: a stale version is stale
// wherever it is written — a workflow, a Dockerfile, a script, a how-to — and a
// filter in front of this sweep is a place for one to hide. Reading every
// tracked file costs tens of megabytes once per run, which is cheaper than the
// shape of failure a narrower scan has.
//
// The exemptions are exact paths: a second file of the same basename elsewhere
// is scanned like everything else.
func assertNamesNoPnpmVersion(t *testing.T, rel, pin string) {
	t.Helper()
	switch rel {
	case "package.json":
		// The pin itself.
		return
	case "backend/gates/pnpmversionpins_test.go":
		// This file names the pattern it bans.
		return
	case "pnpm-lock.yaml":
		// Generated by the pinned pnpm and rewritten wholesale; it records the
		// version that wrote it rather than choosing one.
		return
	}
	body, err := os.ReadFile(filepath.Join(repoRoot, rel))
	if err != nil {
		// Tracked but absent happens mid-rebase and after `git rm`; not this
		// gate's business.
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("reading %s: %v", rel, err)
	}
	for i, line := range strings.Split(string(body), "\n") {
		if pnpmVersionLiteral.MatchString(line) {
			t.Errorf("%s:%d names a pnpm version; package.json pins %s and every reader resolves it from there\n\t%s",
				rel, i+1, pin, strings.TrimSpace(line))
		}
	}
}
