// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

//go:build !integration

package backendarch

// A comment that cites a migration by number must cite one that exists.
//
// The citations do real work: `core 0217 retired row-level security` tells a
// reader WHERE to go and see why a per-statement predicate replaced a policy.
// The baseline consolidation deleted those files and the references survived it,
// each one now sending a reader to look for something that is not there — which
// is worse than no citation at all, because it reads like a lead.
//
// WHY A GATE AND NOT A SWEEP. The sweep is the easy half and it rots: the next
// consolidation, or any renumber, breaks every citation written since. This
// derives the answer from the tree instead — the versions the namespaces
// actually carry — so it is correct the day a namespace changes rather than the
// day somebody remembers to look.
//
// A RATCHET, NOT A COUNT. The backlog is recorded per file in
// danglingcitations.txt and diffed, so a failure NAMES the citation that arrived
// rather than reporting a number and asking the author to find their own. A
// single total was the first shape this gate had, and it had a hole worth
// stating: STATUS.md alone carries dozens of these and is edited at the end of
// every session, so deleting a citation there funded a new dead one anywhere
// else and the total netted to green.
//
// WHAT A FIX LOOKS LIKE is not "delete the number". CLAUDE.md rule 4 says state
// the invariant so it stands alone, and that is it: the sentence keeps its claim
// and loses the pointer, or says explicitly that the migration is in history
// rather than in the tree.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

// citation matches the shapes this tree uses to name a migration: `core 0217`,
// `core/0063`, `migration 0099`, `migration ` + "`0105`" + `,
// `custom/20260716120000`, and the FILENAME form `core/0012_audit_log.up.sql`.
//
// Three widths, because the tree uses three: the closed four-digit range, the
// ten-digit unix-second stamps every current core migration carries, and
// custom's fourteen-digit stamps. Excluding ten was this gate's first blind spot
// — "the format is current" is not "the citation resolves", and ten-digit
// migrations are deleted and renumbered like any other.
//
// The version ends at `_` OR a word boundary, which is the second blind spot and
// was the larger one: `_` is an ASCII word character, so a trailing \b never
// matched the filename form at all — the most natural way to cite a migration,
// and 88 of them in the tree when this was measured.
//
// The namespace is CAPTURED, not discarded: `core 0001` must be checked against
// CORE's versions. Merged into one set it passed because custom happens to carry
// a 0001 too, which is a false negative in the case a citation is most specific
// about.
var citation = regexp.MustCompile(
	"(?i)\\b(core|custom|migration)s?[ /`]+([0-9]{4}|[0-9]{10}|[0-9]{14})(?:_|\\b)")

// migrationsRoot holds the namespace directories, relative to this package.
const migrationsRoot = "migrations"

// citationRegisterPath records the citations this tree carries today, one per line.
const citationRegisterPath = "danglingcitations.txt"

// generatedByGen reports a file `make gen` writes. A citation inside one is a
// citation in the SOURCE it was rendered from, and that source is scanned —
// reporting both would send somebody to fix the copy gen overwrites.
//
// The frontend artifacts are here for that reason: schema.d.ts is rendered from
// api/crm.yaml, which this gate reads.
func generatedByGen(rel string) bool {
	switch {
	case strings.HasSuffix(rel, "_gen.go"),
		strings.HasSuffix(rel, ".generated.json"),
		strings.HasPrefix(rel, "backend/internal/contracts/"),
		rel == "frontend/src/api/schema.d.ts",
		rel == "frontend/src/api/public-events.ts":
		return true
	}
	return false
}

// updateCitationRegister rewrites the register from the tree, the same way
// identity's matrix fixture and the head catalog are regenerated. Without it the
// only way to record a legitimate new entry is to hand-copy a line out of a
// failure message, which is how a register acquires a typo nobody can see.
var updateCitationRegister = flag.Bool("update-citations", false,
	"rewrite danglingcitations.txt from the tree")

func TestEveryCitedMigrationExists(t *testing.T) {
	known := knownMigrationVersions(t)
	// A namespace that failed to load would leave `known` empty and every
	// citation below would read as dangling — hundreds of failures pointing at
	// the wrong cause.
	if len(known) == 0 {
		t.Fatal("no migration versions could be read from the namespace directories, so every " +
			"citation would report as dangling — fix the loader, not the citations")
	}

	found := scanTreeForCitations(t, known)
	if *updateCitationRegister {
		writeCitationRegister(t, found)
		return
	}
	recorded := readCitationRegister(t)

	var arrived []string
	for _, c := range found {
		if !recorded[c.key()] {
			arrived = append(arrived, c.String())
		}
	}
	// NAMED, not counted. The whole reason for a per-file register is that the
	// author of a new dead-end learns which line is theirs.
	for _, a := range arrived {
		t.Errorf("new dangling migration citation:\n  %s\n"+
			"A citation written now against a version that never existed is a dead end from "+
			"birth. State the invariant instead, or say the migration is in git history rather "+
			"than in the tree. If it is genuinely unavoidable, add the line to %s in this change.",
			a, citationRegisterPath)
	}

	// The other direction: a register line whose citation is gone. Left in, the
	// register stops describing the tree and quietly licenses a re-addition at
	// the same spot.
	present := map[string]bool{}
	for _, c := range found {
		present[c.key()] = true
	}
	var stale []string
	for key := range recorded {
		if !present[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, s := range stale {
		t.Errorf("%s records %q, which is no longer a dangling citation — thank you.\n"+
			"Delete that line in this change, so the register keeps describing the tree.",
			citationRegisterPath, s)
	}
}

// citationRef is one dangling citation: where it is, and what it names.
type citationRef struct {
	path      string
	line      int
	namespace string
	version   string
	excerpt   string
}

// key identifies a citation for the register WITHOUT its line number, so
// editing a file above one does not churn the register or read as a new
// citation. Two citations of the same version in one file collapse to one
// entry, which is the right granularity: the register answers "does this file
// still cite this missing version", not "how often".
func (c citationRef) key() string {
	return fmt.Sprintf("%s %s %s", c.path, c.namespace, c.version)
}

func (c citationRef) String() string {
	return fmt.Sprintf("%s:%d cites %s %s — %s", c.path, c.line, c.namespace, c.version, c.excerpt)
}

// scanTreeForCitations reads every tracked text file and reports the citations that name a
// version no namespace carries.
//
// EVERY tracked file, with no extension allowlist. The first shape of this gate
// had one, and it dropped twenty citations — nineteen in frontend .ts/.tsx and
// one in the Makefile — so the invariant was gated on one side of the wire only
// while claiming to hold the tree. An unmeasured prefilter in front of a census
// is what CLAUDE.md rule 8 forbids by name; binary files are skipped by looking
// at the bytes rather than by trusting a suffix.
func scanTreeForCitations(t *testing.T, known map[string]map[string]bool) []citationRef {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("listing tracked files: %v (this test must run inside the git worktree)", err)
	}

	var found []citationRef
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" || generatedByGen(rel) {
			continue
		}
		// This file states the citation shapes it looks for, so scanning it
		// fails on its own source. The exact path, not the basename: a second
		// file by this name anywhere else is scanned.
		if rel == "backend/migrationcitations_test.go" || rel == "backend/"+citationRegisterPath {
			continue
		}
		body, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			// A tracked file can be absent mid-rebase or after `git rm`.
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("reading %s: %v", rel, err)
		}
		if !utf8.Valid(body) {
			continue
		}
		found = append(found, danglingCitationsIn(rel, string(body), known)...)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].key() < found[j].key() })
	return found
}

func danglingCitationsIn(rel, body string, known map[string]map[string]bool) []citationRef {
	var found []citationRef
	for i, line := range strings.Split(body, "\n") {
		for _, m := range citation.FindAllStringSubmatch(line, -1) {
			namespace, version := strings.ToLower(m[1]), m[2]
			if namespaceCarries(known, namespace, version) {
				continue
			}
			found = append(found, citationRef{
				path: rel, line: i + 1, namespace: namespace, version: version,
				excerpt: citationExcerpt(line),
			})
		}
	}
	return found
}

// carries reports whether the cited version exists in the namespace the citation
// named. An UNQUALIFIED citation ("migration 0099") names no namespace, so the
// union is the honest pool for it — narrowing that to one namespace would invent
// a claim the text never made.
func namespaceCarries(known map[string]map[string]bool, namespace, version string) bool {
	if versions, ok := known[namespace]; ok {
		return versions[version]
	}
	for _, versions := range known {
		if versions[version] {
			return true
		}
	}
	return false
}

// knownMigrationVersions collects the versions every namespace carries, read as
// FILENAMES rather than through the migrations package.
//
// Not a preference: .go-arch-lint.yml refuses `root -> migrations`, and widening
// a component edge to serve a test would grant a production one. The version is
// the filename's prefix, so the directory answers the question directly.
//
// The namespace list is the directory listing, so a third namespace is covered
// the day it appears. `testdata` sits beside core/ and custom/ and is NOT one —
// check-migration-versions.sh rules it out for the same reason, and admitting it
// would let a fixture migration make its version "known" and mask real dangling
// citations.
func knownMigrationVersions(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(migrationsRoot)
	if err != nil {
		t.Fatalf("reading %s: %v", migrationsRoot, err)
	}
	known := map[string]map[string]bool{}
	for _, ns := range entries {
		if !ns.IsDir() || ns.Name() == "testdata" {
			continue
		}
		versions := migrationVersionsIn(t, filepath.Join(migrationsRoot, ns.Name()))
		if len(versions) > 0 {
			known[ns.Name()] = versions
		}
	}
	return known
}

func migrationVersionsIn(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	versions := map[string]bool{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		version, _, ok := strings.Cut(e.Name(), "_")
		if !ok {
			t.Fatalf("%s/%s: want <version>_<name>.up.sql", dir, e.Name())
		}
		versions[version] = true
	}
	return versions
}

// readCitationRegister reads the recorded backlog. Comments and blank lines are ignored
// so the file can explain itself.
func readCitationRegister(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(citationRegisterPath)
	if err != nil {
		t.Fatalf("reading %s: %v", citationRegisterPath, err)
	}
	recorded := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		recorded[trimmed] = true
	}
	return recorded
}

// writeCitationRegister rewrites the register, header and all.
//
// The header is written here rather than preserved from the file, so a
// regeneration cannot silently drop the explanation of what the list is for —
// which is the part that stops the next reader treating it as a permission.
func writeCitationRegister(t *testing.T, found []citationRef) {
	t.Helper()
	var b strings.Builder
	b.WriteString(citationRegisterHeader)
	seen := map[string]bool{}
	for _, c := range found {
		if seen[c.key()] {
			continue
		}
		seen[c.key()] = true
		b.WriteString(c.key())
		b.WriteString("\n")
	}
	if err := os.WriteFile(citationRegisterPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writing %s: %v", citationRegisterPath, err)
	}
	t.Logf("%s rewritten: %d entries", citationRegisterPath, len(seen))
}

// citationRegisterHeader is what the register explains about itself. It is
// written on every regeneration rather than preserved from the file, so a
// rewrite cannot silently drop the part that stops the next reader treating
// the list as a permission.
const citationRegisterHeader = `# THE DANGLING-CITATION REGISTER — a record, not a permission.
#
# Every line is a file that cites a migration version no namespace carries, in
# the form: <path> <namespace> <version>. A namespace of "migration" means the
# citation named none, so it was checked against every namespace at once.
#
# The migrations these name were mostly deleted by the 2026-08 baseline
# consolidation, and MOST of them still resolve through git history — so they are
# stale prose rather than defects, and rewriting them mechanically would cost
# more than it buys: a citation is usually the only thing in a sentence saying
# where the reasoning lives. Two on this list named versions that never existed
# on ANY ref, and were repaired in the change that added this file.
#
# What the register prevents is the backlog GROWING. A citation written today
# against a version that never existed is a dead end from birth.
#
# Per FILE, not a total, and that is the point: a total let a deletion anywhere
# fund a new dead citation elsewhere and net to green — STATUS.md alone carries
# dozens and is edited at the end of every session.
#
# Line numbers are deliberately absent: editing a file above a citation is not a
# new citation, and a register that churned on every edit would be ignored.
#
# TO FIX ONE: state the invariant so the sentence stands alone, or say the
# migration is in git history rather than in the tree — then DELETE its line
# here. The gate fails on a line whose citation is gone, so the register cannot
# drift from the tree in either direction.
#
# TO REGENERATE: go test ./... -run TestEveryCitedMigrationExists -update-citations
`

// citationExcerpt trims a cited line to something readable. STATUS.md holds single-line
// paragraphs thousands of characters long, and one per finding buries the very
// list this output exists to be read as. Runes, not bytes: these lines are full
// of em-dashes, and slicing mid-rune prints replacement characters.
func citationExcerpt(line string) string {
	const width = 100
	trimmed := strings.TrimSpace(line)
	if utf8.RuneCountInString(trimmed) <= width {
		return trimmed
	}
	return string([]rune(trimmed)[:width]) + "…"
}
