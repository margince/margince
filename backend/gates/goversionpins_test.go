// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// One Go version, pinned in several places, and they have to agree.
//
// The pins exist for different readers: go.mod is what the compiler and CI
// resolve (every workflow job uses `go-version-file: backend/go.mod`),
// .tool-versions is what a developer's asdf/mise shell installs, and the
// extension how-to is what somebody copies when they start a new unit. Nothing
// made them move together, so a security bump updated the modules and left the
// developer pin a patch release behind — which is how a machine keeps building
// with the vulnerable toolchain the bump was for.
//
// This derives the answer from backend/go.mod rather than holding a list: the
// product module is the pin CI actually reads, so it is the one that decides.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// goDirective matches the `go 1.26.6` line a module or workspace file carries.
var goDirective = regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)$`)

func TestEveryGoVersionPinMatchesTheProductModule(t *testing.T) {
	t.Parallel()
	want := goVersionOf(t, "go.mod")
	if strings.Count(want, ".") != 2 {
		t.Fatalf("backend/go.mod pins %q; a patch version is what the other pins have to match", want)
	}

	// Each of these is read by somebody the others are not: the workspace by
	// every bare go command, .tool-versions by a developer's shell, the
	// how-to by whoever writes the next extension.
	t.Run("the workspace file", func(t *testing.T) {
		if got := goVersionOf(t, "../go.work"); got != want {
			t.Errorf("go.work pins go %s, backend/go.mod pins %s", got, want)
		}
	})

	for _, module := range append([]string{
		"tools/go.mod",
		"../composition/go.mod",
	}, extensionModules(t)...) {
		t.Run(module, func(t *testing.T) {
			if got := goVersionOf(t, module); got != want {
				t.Errorf("%s pins go %s, backend/go.mod pins %s", module, got, want)
			}
		})
	}

	t.Run("the developer toolchain pin", func(t *testing.T) {
		pinned := readFile(t, "../.tool-versions")
		if !strings.Contains(pinned, "golang "+want+"\n") {
			t.Errorf("`.tool-versions` does not pin golang %s:\n%s\n\n"+
				"A developer's shell installs what this file says, so a stale pin here "+
				"keeps building with the toolchain the bump replaced.", want, pinned)
		}
	})

	// The how-to is a template somebody copies verbatim into a new module, so a
	// stale version there is a stale pin in every extension written from it.
	t.Run("the extension how-to", func(t *testing.T) {
		doc := readFile(t, "../docs/how-to/add-an-extension.md")
		if strings.Contains(doc, "go 1.") && !strings.Contains(doc, "go "+want) {
			t.Errorf("docs/how-to/add-an-extension.md does not show go %s; "+
				"the template is copied into new modules verbatim", want)
		}
	})
}

// extensionModules scans the tier for its unit modules rather than listing
// them here. A list is a second copy of the tier: the unit added this morning
// is not on it, and a pin nobody checks reads exactly like one that agrees. It
// fails on an empty tier for the reason every census in this tree does —
// reading nothing and reporting a clean sweep is the failure a version pin
// cannot afford.
func extensionModules(t *testing.T) []string {
	t.Helper()
	found, err := filepath.Glob("../extensions/*/go.mod")
	if err != nil {
		t.Fatalf("scanning the extension tier: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("the extension tier holds no go.mod — a scan that finds no module passes exactly like one that checked every module, so either the tier moved or this glob is stale")
	}
	sort.Strings(found)
	return found
}

func goVersionOf(t *testing.T, path string) string {
	t.Helper()
	match := goDirective.FindStringSubmatch(readFile(t, path))
	if match == nil {
		t.Fatalf("%s carries no `go <version>` directive", path)
	}
	return match[1]
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(body)
}
