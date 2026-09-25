// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A gate runs in a lane that every path it reads can trigger.
//
// The gates here are Go tests, and CI decides whether to run them from a
// paths-filter. A gate that reads a file outside that filter's scope is one a
// pull request can break without ever running it: the tree goes red on `main`,
// and the failure then surfaces on the next unrelated change that does trigger
// the lane, where it reads as that change's fault.
//
// It is not hypothetical. A frontend-only pull request added a retired word to a
// comment, the vocabulary gate reads frontend comments, and the lane holding it
// never ran. Three sessions found the red independently on unrelated branches
// and two spent a full integration lane ruling out their own diff.
//
// The filter already carried one frontend file for exactly this reason, with a
// comment describing this bug. One path is a point fix; this is the rule.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// candidatePath finds string literals that could name a file: no whitespace,
// and either a separator or an extension. Prose is excluded by the space rule —
// a failure message mentioning `docs/` reads as a sentence, not a path.
//
// Which of these ARE paths is decided by the tree rather than by a list of
// prefixes: the first version enumerated seven directories and silently missed
// `.github/labels.yml` and the root `package.json`, both read by gates and
// neither covered. A census that names its own subject can only ever find what
// its author already thought of.
var candidatePath = regexp.MustCompile(`"([A-Za-z0-9_.][A-Za-z0-9_./*-]*(?:/[A-Za-z0-9_./*-]*|\.[A-Za-z]{2,4}))"`)

// coveredBy reports whether a paths-filter pattern matches this path. The two
// shapes the filter uses: a `**` tree and an exact file.
func coveredBy(pattern, path string) bool {
	if tree, ok := strings.CutSuffix(pattern, "/**"); ok {
		return path == tree || strings.HasPrefix(path, tree+"/")
	}
	return pattern == path
}

func TestEveryGateRunsInALaneItsSubjectTriggers(t *testing.T) {
	t.Parallel()

	// The gates suite runs with backend/ as its working directory, so the tree
	// this judges is one level up.
	const repoRoot = ".."
	scope := backendFilterPatterns(t)
	// The suite runs with backend/ as its working directory, which is why the
	// workflow above is reached as ../.github — the gates are a directory down
	// from here, not the one the process is in.
	const dir = "gates"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	read := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		for _, match := range candidatePath.FindAllStringSubmatch(string(body), -1) {
			path := match[1]
			path, reads := readsFromTheTree(repoRoot, path)
			if !reads {
				continue
			}
			read++
			if covered(scope, path) {
				continue
			}
			t.Errorf("%s reads %q, which no pattern in ci.yml's `backend` filter covers.\n"+
				"A pull request touching only that path skips this lane, so it can break this "+
				"gate and merge green — and the red then lands on the next unrelated change "+
				"that does trigger the lane.\nAdd the tree to the `backend` scope (not "+
				"`backend_db`: a Go gate needs no Postgres shard).", entry.Name(), path)
		}
	}
	// The census must not pass by reading nothing. Forty-eight gates reach
	// outside backend/ today; a walk that stopped recognising the literal, or
	// resolved the wrong directory, would report a clean tree having judged none
	// of them.
	if read < 20 {
		t.Fatalf("this census found %d outside-backend path(s) across the gates and expects at "+
			"least 20 — it has stopped recognising the reads rather than the tree having lost them",
			read)
	}
}

// readsFromTheTree resolves a literal to something that EXISTS outside
// backend/, which is what separates a path a gate opens from an import path, a
// format string or a word that happens to carry a dot.
//
// `./` is relative to the suite's own directory and therefore already inside
// backend/**. `../` reaches the repository root, which is where this census
// judges from anyway. A glob is resolved by matching it: a gate naming a
// pattern reads whatever the pattern matches today.
func readsFromTheTree(root, path string) (string, bool) {
	switch {
	case strings.HasPrefix(path, "./"):
		return "", false
	case strings.HasPrefix(path, "../"):
		path = strings.TrimPrefix(path, "../")
	case strings.HasPrefix(path, "backend/"), path == "backend":
		return "", false
	}
	// After normalisation, not before: the literal that reaches here is
	// "../.git", and a check ahead of the prefix strip never sees it. The
	// repository's own metadata is named by gates locating the root — not a
	// source path, and no pull request edits it.
	if path == "" || path == "." || path == ".git" || strings.HasPrefix(path, ".git/") {
		return "", false
	}
	if strings.ContainsAny(path, "*?") {
		matches, err := filepath.Glob(filepath.Join(root, path))
		return path, err == nil && len(matches) > 0
	}
	_, err := os.Stat(filepath.Join(root, path))
	return path, err == nil
}

func covered(scope []string, path string) bool {
	for _, pattern := range scope {
		if coveredBy(pattern, path) {
			return true
		}
	}
	return false
}

// backendFilterPatterns reads the `backend` scope out of ci.yml, including the
// backend_db anchor it derives from, so this gate judges the filter the lane
// actually uses rather than a copy of it.
func backendFilterPatterns(t *testing.T) []string {
	t.Helper()
	const workflow = "../.github/workflows/ci.yml"
	body, err := os.ReadFile(filepath.Clean(workflow))
	if err != nil {
		t.Fatalf("reading %s: %v", workflow, err)
	}
	text := string(body)
	start := strings.Index(text, "backend_db: &backend_db")
	end := strings.Index(text, "\n            frontend:")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("%s no longer declares `backend_db: &backend_db` followed by a `frontend:` "+
			"scope; this gate can no longer read which paths trigger the backend lane", workflow)
	}
	var patterns []string
	for _, line := range strings.Split(text[start:end], "\n") {
		line = strings.TrimSpace(line)
		quoted := strings.TrimPrefix(line, "- ")
		if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
			continue
		}
		patterns = append(patterns, strings.Trim(quoted, "'"))
	}
	if len(patterns) < 10 {
		t.Fatalf("the backend scope parsed to %d pattern(s), which is fewer than it has — the "+
			"shape this gate reads has changed and it would judge every path uncovered",
			len(patterns))
	}
	return patterns
}
