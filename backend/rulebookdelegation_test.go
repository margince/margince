// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

//go:build !integration

package backendarch

// AGENTS.md is the rulebook — at the root, and in any directory that needs one of
// its own. The CLAUDE.md beside it contains one line, `@AGENTS.md`, and nothing
// else. It exists only because Claude Code reads CLAUDE.md and never AGENTS.md.
//
// Keeping the shim empty is the whole rule, and it is stricter than it looks
// because the alternative fails silently. A rule written into a CLAUDE.md binds
// Claude Code and no other harness: `cli/craft` walks up for the nearest
// AGENTS.md and never opens CLAUDE.md, so that rule cannot reach the gate, and
// neither file looks wrong. An empty shim cannot hold a rule at all, which is
// why the assertion is "nothing but the import" rather than a heading census or
// a size ceiling — those admit the paragraph that is not quite a rule yet, and
// they need a hand-written list of what a rule looks like, which is itself a
// second copy of the rulebook's table of contents.
//
// Both assertions derive their subjects from the tree: the pairs come from
// walking for AGENTS.md. A directory that grows a rulebook tomorrow is enrolled
// the moment it exists, and there is no list here to go short.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot is where the rulebooks live, relative to this package.
const repoRoot = ".."

// importLine matches the one line a shim is allowed to contain.
var importLine = regexp.MustCompile(`^@AGENTS\.md$`)

// skipRulebookDir keeps the walk to the rulebooks this repository ships.
// Worktrees hold whole checkouts of their own, including their own copies of
// these files; node_modules holds other people's.
func skipRulebookDir(name string) bool {
	switch name {
	case "node_modules", ".git", ".tmp", "worktrees", "build", "dist", "coverage":
		return true
	}
	return false
}

// rulebookDirs returns every directory holding an AGENTS.md.
func rulebookDirs(t *testing.T) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipRulebookDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "AGENTS.md" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking for rulebooks: %v", err)
	}
	if len(dirs) == 0 {
		t.Fatal("no AGENTS.md found anywhere — this gate is reading a tree shape that is gone")
	}
	return dirs
}

// TestEveryRulebookHasAClaudeShim covers the direction that produces no error at
// all: an AGENTS.md with no CLAUDE.md beside it is a rulebook Claude Code never
// opens, so a Claude session in that directory works without those rules and
// nothing says so.
func TestEveryRulebookHasAClaudeShim(t *testing.T) {
	for _, dir := range rulebookDirs(t) {
		shim := filepath.Join(dir, "CLAUDE.md")
		if _, err := os.Stat(shim); err != nil {
			if os.IsNotExist(err) {
				t.Errorf("%s/AGENTS.md has no CLAUDE.md beside it, so Claude Code reads none of it — it loads "+
					"CLAUDE.md and never AGENTS.md. Create %s containing exactly one line: @AGENTS.md", dir, shim)
				continue
			}
			t.Fatalf("stat %s: %v", shim, err)
		}
	}
}

// TestEveryClaudeShimIsNothingButTheImport is the rule stated as one assertion.
//
// The import must also be a line of its own with no backticks and no fence
// around it, because Claude Code's import parser skips code spans and fenced
// blocks — a shim reading `@AGENTS.md` in backticks, or inside a fence, looks
// correct in a diff and imports nothing at all. A file that is allowed to hold
// only that line cannot hold a fence to hide it in, which is the second reason
// this shape is the one to gate.
func TestEveryClaudeShimIsNothingButTheImport(t *testing.T) {
	for _, dir := range rulebookDirs(t) {
		path := filepath.Join(dir, "CLAUDE.md")
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue // reported by TestEveryRulebookHasAClaudeShim
			}
			t.Fatalf("read %s: %v", path, err)
		}
		imports := 0
		for i, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			switch {
			case trimmed == "":
				continue
			case importLine.MatchString(trimmed):
				imports++
			default:
				t.Errorf("%s:%d holds content other than the import:\n  %s\n"+
					"A CLAUDE.md is one line, @AGENTS.md, and nothing else. Anything written here binds Claude Code "+
					"alone — Codex does not read it, and `cli/craft` feeds the nearest AGENTS.md into its gate prompt "+
					"and never opens this file. Put it in %s/AGENTS.md, or in a skill or a `.claude/rules/` file with "+
					"a `paths:` glob if it is a procedure rather than a rule.", path, i+1, trimmed, dir)
			}
		}
		if imports != 1 {
			t.Errorf("%s contains %d lines reading exactly `@AGENTS.md`, want 1. Claude Code skips imports inside "+
				"code spans and fenced blocks, so a backticked or fenced one loads nothing while reading correctly "+
				"in a diff.", path, imports)
		}
	}
}
