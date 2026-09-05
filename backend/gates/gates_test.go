// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package gates holds the fitness gates: the tests whose subject is the tree
// rather than a function. They live in one directory, apart from the code they
// judge, because they are not that code's unit tests — a gate here asks whether
// the module as a whole still holds an invariant, and answering that means
// walking packages none of them import.
//
// Every gate names its subject relative to the MODULE root — "internal/modules",
// "api/crm.yaml", "../docs/reference/gate-inventory.md" — because that is where
// the subjects are and where these files used to sit. Moving the files did not
// move their subjects, so TestMain restores that working directory rather than
// rewriting 117 files' worth of path literals. Many of those literals are
// import-path fragments a gate compares against and not file paths at all, so a
// mechanical rewrite would have corrupted the comparisons it did not change.
package gates

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain puts every gate back at the module root before any of them runs.
//
// It VERIFIES the landing rather than trusting the chdir, because the failure is
// silent in the direction that matters: a gate that walks "internal/modules"
// from the wrong directory finds nothing, and a census of nothing reports the
// clean tree it never read. Refusing to run at all is the only outcome that
// cannot be mistaken for a pass.
// repoRoot is the repository, from the directory TestMain leaves this package
// in. It is declared HERE, in the one file with no build constraint, rather than
// beside the first gate that needed it: a const behind `//go:build !integration`
// is invisible to `make lint`, which sets that tag — so every gate that used it
// compiled under `go test` and failed to compile under the linter, in a file
// that never mentioned the tag.
const repoRoot = ".."

// describesRatherThanRenders is a frontend file, named relative to
// frontend/src, that talks ABOUT what renders rather than rendering: a test, a
// story, a test kit, or the generated contract. Shared by every gate that
// sweeps the frontend for a second rendering of something, so two gates cannot
// disagree about what counts as production source.
func describesRatherThanRenders(rel string) bool {
	return strings.HasPrefix(rel, "api/") ||
		strings.Contains(rel, ".test.") ||
		strings.Contains(rel, ".stories.") ||
		strings.Contains(rel, ".testkit.") ||
		strings.HasSuffix(rel, ".d.ts")
}

func TestMain(m *testing.M) {
	if err := os.Chdir(".."); err != nil {
		fmt.Fprintf(os.Stderr, "gates: reaching the module root from the package directory: %v\n", err)
		os.Exit(1)
	}
	// The evidence is go.mod's MODULE LINE, not merely a go.mod. A bare
	// existence check would be satisfied by backend/tools/ and by every
	// directory under extensions/, each of which carries its own — so it would
	// answer yes for several wrong landings and only look like a proof.
	declaration, err := os.ReadFile("go.mod")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gates: reading go.mod to confirm the module root: %v\n", err)
		os.Exit(1)
	}
	if !bytes.HasPrefix(declaration, []byte("module "+modulePath+"\n")) {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			wd = "an unreadable directory"
		}
		fmt.Fprintf(os.Stderr, "gates: %s declares a different module than %s, so every gate here would "+
			"walk a tree that is not the one it judges and report the empty result as clean\n",
			wd, modulePath)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
