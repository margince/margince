// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// A comment that cites a migration by number must cite one that exists.
//
// The citations were doing real work: `core 0217 retired row-level security`
// told a reader WHERE to go and see why a per-statement predicate replaced a
// policy. The baseline consolidation deleted those files, and sixty-nine of
// these references survived it — each one now sending a reader to look for
// something that is not there, which is worse than no citation at all because
// it reads like a lead.
//
// WHY A GATE AND NOT A SWEEP. The sweep is the easy half and it rots: the next
// consolidation, or any renumber, breaks every citation written since. This
// derives the answer from the tree instead — the versions the namespaces
// actually carry — so it is correct the day a namespace changes rather than the
// day somebody remembers to look.
//
// IT PINS A COUNT RATHER THAN DEMANDING ZERO, and the reason is a judgement
// worth stating. The consolidation left 189 of these across 133 files, almost
// all comment prose. Rewriting them mechanically would cost more than it bought
// — a citation is usually the only thing in a sentence saying WHERE the
// reasoning lives — and they are not unfollowable: the deleted migrations are
// still in git, so `core 0217` resolves through history even though no file in
// the tree carries it. What is worth preventing is the count GROWING, because a
// citation written today against a version that never existed is a dead end from
// birth. So the pin blocks new ones and leaves the backlog to #2197, where the
// sweep can be judged on its own.
//
// WHAT A FIX LOOKS LIKE is not "delete the number". CLAUDE.md rule 4 already
// says state the invariant so it stands alone, and that is it: the sentence
// keeps its claim and loses the pointer, or says explicitly that the migration
// is in history rather than in the tree.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// citation matches the shapes this tree uses to name a migration: `core 0217`,
// `core/0063`, `migration 0099`, `migration ` + "`0105`" + `, `custom/20260716120000`.
//
// Four digits or fourteen, never ten: a ten-digit unix-second stamp is the
// current naming and citing one is normal. What goes stale is a citation of the
// closed four-digit range, and of a custom stamp.
var citation = regexp.MustCompile(
	"(?i)\\b(?:core|custom|migration)s?[ /`]+([0-9]{4}|[0-9]{14})\\b")

// genWritten reports a file that `make gen` writes. A citation inside one is a
// citation in the SOURCE it was rendered from, and that source is scanned —
// reporting both would send somebody to fix the copy gen overwrites.
func genWritten(rel string) bool {
	return strings.HasSuffix(rel, "_gen.go") ||
		strings.HasPrefix(rel, "backend/internal/contracts/") ||
		strings.HasSuffix(rel, ".generated.json")
}

func TestEveryCitedMigrationExists(t *testing.T) {
	known := knownMigrationVersions(t)
	// A namespace that failed to load would leave `known` empty and every
	// citation below would read as dangling — hundreds of failures pointing at
	// the wrong cause.
	if len(known) == 0 {
		t.Fatal("no migration versions could be read from the embedded namespaces, so every " +
			"citation would report as dangling — fix the loader, not the citations")
	}

	out, err := exec.Command("git", "-C", "..", "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("listing tracked files: %v (this test must run inside the git worktree)", err)
	}
	var dangling []string
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" || !citationScanned[filepath.Ext(rel)] {
			continue
		}
		// This file states the citation shapes it looks for, so scanning it
		// fails on its own source. The exemption is the exact path, not the
		// basename: a second file by this name anywhere else is scanned.
		if rel == "backend/migrationcitations_test.go" || genWritten(rel) {
			continue
		}
		dangling = append(dangling, danglingCitations(t, rel, known)...)
	}

	switch {
	case len(dangling) > danglingCitationBacklog:
		// The added ones cannot be named — this counts, it does not diff against
		// a recorded set — so it says how to find them.
		t.Errorf("%d dangling migration citations, %d more than the %d this tree carries.\n"+
			"A citation written now against a version that never existed is a dead end from "+
			"birth: state the invariant instead, or say the migration is in git history rather "+
			"than in the tree. Run with -v to list every one and find yours.",
			len(dangling), len(dangling)-danglingCitationBacklog, danglingCitationBacklog)
		for _, d := range dangling {
			t.Log(d)
		}
	case len(dangling) < danglingCitationBacklog:
		t.Errorf("%d dangling migration citations, %d fewer than the %d pinned — thank you.\n"+
			"Lower danglingCitationBacklog to %d in this change, so the number keeps describing "+
			"the tree and the next addition still fails.",
			len(dangling), danglingCitationBacklog-len(dangling), danglingCitationBacklog, len(dangling))
	}
}

// danglingCitationBacklog is how many citations name a version no namespace
// carries, today.
//
// A pin, not a target. It exists so the count cannot GROW unnoticed; the backlog
// itself is #2197's, and it is prose rather than a defect — every one resolves
// through git history, which is where the migrations still are.
//
// Both directions fail on purpose. A number left above the tree is a gate with
// slack in it, and slack is how the next one rides in.
const danglingCitationBacklog = 189

// citationScanned are the extensions worth reading. Migration SQL is included:
// a migration may legitimately cite an earlier one, and that citation goes stale
// exactly like a comment's.
var citationScanned = map[string]bool{
	".go": true, ".md": true, ".yml": true, ".yaml": true,
	".sh": true, ".sql": true,
}

// danglingCitations returns one line per citation in rel naming a version no
// namespace carries.
func danglingCitations(t *testing.T, rel string, known map[string]bool) []string {
	t.Helper()
	// The working copy, not the committed blob: a stale citation must fail
	// before it is committed.
	body, err := os.ReadFile(filepath.Join("..", rel))
	if err != nil {
		// A tracked file can be absent mid-rebase or after `git rm`.
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading %s: %v", rel, err)
	}
	var found []string
	for i, line := range strings.Split(string(body), "\n") {
		for _, m := range citation.FindAllStringSubmatch(line, -1) {
			if known[m[1]] {
				continue
			}
			found = append(found, fmt.Sprintf("%s:%d cites migration %s: %s",
				rel, i+1, m[1], strings.TrimSpace(line)))
		}
	}
	return found
}

// knownMigrationVersions collects the versions every namespace carries, read as
// FILENAMES rather than through the migrations package.
//
// Not a preference: .go-arch-lint.yml refuses `root -> migrations`, and widening
// a component edge to serve a test would grant a production one. The version is
// the filename's prefix, so the directory answers the question directly.
//
// The namespace list is the directory listing, so a third namespace is covered
// the day it appears rather than when somebody remembers this file.
func knownMigrationVersions(t *testing.T) map[string]bool {
	t.Helper()
	namespaces, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("reading %s: %v", migrationsDir, err)
	}
	known := map[string]bool{}
	for _, ns := range namespaces {
		if !ns.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(migrationsDir, ns.Name()))
		if err != nil {
			t.Fatalf("reading %s/%s: %v", migrationsDir, ns.Name(), err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".up.sql") {
				continue
			}
			version, _, ok := strings.Cut(e.Name(), "_")
			if !ok {
				t.Fatalf("%s/%s/%s: want <version>_<name>.up.sql", migrationsDir, ns.Name(), e.Name())
			}
			known[version] = true
		}
	}
	return known
}

// migrationsDir is relative to this package, which sits at backend/.
const migrationsDir = "migrations"
