// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

//go:build !integration

package backendarch

// Every config file the Makefiles name is a config file that exists.
//
// This gate exists because a retirement kept missing one. Deleting
// config/ai-routing*.yaml left four references behind in four rounds — a help
// string, a cache comment, a Go doc comment, and a docs table — and the help
// string was the worst of them: `make ai-probe` advertised
// `--ai-routing ../config/ai-routing.example.yaml` for a subcommand whose flag
// had been removed, so following the documented invocation returned
// "flag provided but not defined". Nothing failed. A Makefile's prose is not
// compiled, and no gate read it.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// gitIgnored reports whether the repo declares this path as generated rather
// than shipped.
func gitIgnored(t *testing.T, path string) bool {
	t.Helper()
	// check-ignore exits 0 when the path IS ignored, 1 when it is not; any
	// other failure means the question went unanswered and the gate must not
	// treat that as "ignored", which would be the silent pass.
	cmd := exec.Command("git", "check-ignore", "-q", path)
	if err := cmd.Run(); err == nil {
		return true
	} else if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
		return false
	} else {
		t.Fatalf("git check-ignore %s did not answer: %v", path, err)
		return false
	}
}

// configPath matches a config/ path written anywhere in a Makefile — recipe,
// variable or help string alike, since a help string is an instruction a reader
// will type.
var configPath = regexp.MustCompile(`(?:\.\./)?config/[A-Za-z0-9._*-]+\.(?:ya?ml|json)`)

func TestEveryConfigPathAMakefileNamesExists(t *testing.T) {
	makefiles := []string{"Makefile", "../Makefile"}
	checked := 0
	for _, mk := range makefiles {
		raw, err := os.ReadFile(mk)
		if err != nil {
			t.Fatalf("read %s: %v", mk, err)
		}
		for _, match := range configPath.FindAllString(string(raw), -1) {
			// A glob is a pattern over a directory rather than one file; it is
			// checked by whether it matches anything at all.
			path := filepath.Join("..", strings.TrimPrefix(match, "../"))
			checked++
			if strings.Contains(match, "*") {
				hits, err := filepath.Glob(path)
				if err != nil {
					t.Errorf("%s names %q, which is not a valid pattern: %v", mk, match, err)
					continue
				}
				if len(hits) == 0 {
					t.Errorf("%s names %q and nothing matches it — a pattern over files that were deleted", mk, match)
				}
				continue
			}
			if _, err := os.Stat(path); err == nil {
				continue
			}
			// A gitignored path is one the tooling CREATES — config/margince.yaml
			// is written from the example on first `make dev` — so its absence in
			// a fresh checkout is the design rather than a broken reference.
			// Read from gitignore rather than an exception list here, which would
			// be this gate carrying its own copy of what the repo already
			// declares.
			if gitIgnored(t, path) {
				continue
			}
			t.Errorf("%s names %q, which does not exist and is not gitignored. A Makefile's prose is an "+
				"instruction a reader will type, and a help string naming a deleted file sends them at a "+
				"command that cannot run.", mk, match)
		}
	}
	// NOT a tolerated zero: both Makefiles name config paths today, so finding
	// none means the pattern stopped matching and this gate reads an empty set
	// while still reporting PASS.
	if checked == 0 {
		t.Fatal("no config/ path found in either Makefile — the pattern has gone blind")
	}
}
