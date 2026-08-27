// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

//go:build !integration

package gates

// No file acquires a citation of a migration version that does not exist.
//
// The ideal behind that is simpler — every citation should resolve — but the
// backlog is recorded rather than swept, so what this HOLDS is the narrower
// claim: the citations already registered stay, and a file gains no citation of
// a version it does not already cite.
//
// The citations do real work: `custom/20260716120000 is the fork-owned seam`
// tells a reader WHERE to go and see why. When the file behind a citation is
// gone, the reference sends a reader looking for something that is not there,
// which is worse than no citation at all because it reads like a lead.
//
// WHAT THIS DETECTOR CANNOT SEE, first, because an H1 gate over prose always
// leaks and one that hides its leaks is worse than none:
//
//   - a citation split across MORE than two lines;
//   - a separator this does not know (`core-0217`), or a version zero-padded
//     past the widths the tree uses (`core 00217`);
//   - a number reached by prose alone — "the migration after 0007";
//   - a citation on a line that is not valid UTF-8.
//
// Those are gaps, not decisions, and the detector test below plants each one as
// a `want: nil` row so widening the pattern is a one-line flip rather than an
// archaeology exercise. The sibling detector states its own limits the same way
// (uniquenessclaimsdetector_test.go).
//
// WHY A GATE AND NOT A SWEEP. The sweep is the easy half and it rots: the next
// consolidation, or any renumber, breaks every citation written since. The
// answer is derived from the tree — the versions the namespaces actually carry
// — so it is correct the day a namespace changes rather than the day somebody
// remembers to look.
//
// A RATCHET, NOT A COUNT. The backlog is recorded per file in
// danglingcitations.txt and diffed, so a failure NAMES the citation that
// arrived. A count also lets a deletion anywhere fund an addition elsewhere:
// the two net, and the gate reports green over a tree that got worse.
//
// WHAT A FIX LOOKS LIKE is not "delete the number". CLAUDE.md rule 4 says state
// the invariant so it stands alone, and that is it: the sentence keeps its claim
// and loses the pointer, or says explicitly that the migration is in git history
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

var (
	// namespacedCitation matches a namespace word followed by a version:
	// `core 0217`, `core/0063`, `migration 0099`, `custom/20260716120000`, and
	// the filename form `core/0012_audit_log.up.sql`.
	//
	// Three widths, because the tree uses three: the closed four-digit range
	// (core/0001_baseline is still one), the ten-digit unix-second stamps most
	// current migrations carry, and custom's fourteen-digit stamps.
	//
	// The version ends at `_` OR a word boundary. `_` is an ASCII word
	// character, so a trailing \b alone never matches the filename form.
	//
	// The namespace is CAPTURED, not discarded: `core 0001` must be checked
	// against CORE's versions. Merged into one set it passes because custom
	// carries a 0001 too, which is a false negative in the case a citation is
	// most specific about.
	namespacedCitation = regexp.MustCompile(
		"(?i)\\b(core|custom|migration)s?[ /`]+([0-9]{4}|[0-9]{10}|[0-9]{14})(?:_|\\b)")

	// listedVersion consumes the SECOND and later versions of a list written
	// under one namespace word — `core 0007, 0131` and `core 0008/0038`. Only
	// the version adjacent to the namespace matches the pattern above, so a list
	// otherwise hid every version but its first.
	listedVersion = regexp.MustCompile("^[\\s,+/&–-]+`?([0-9]{4}|[0-9]{10}|[0-9]{14})`?(?:_|\\b)")

	// bareFilename matches a migration filename with no namespace word in front
	// of it, which is how a file's own header usually names one.
	bareFilename = regexp.MustCompile(
		`\b([0-9]{4}|[0-9]{10}|[0-9]{14})_[a-z0-9_]+\.(?:up|down)\.sql\b`)

	// commentContinuation is the prefix a wrapped comment line carries.
	// Stripping it is what lets a citation split across two lines be seen: this
	// tree wraps at about 78 columns, so a namespace word and its version land
	// on different lines routinely.
	commentContinuation = regexp.MustCompile(`^\s*(?://+|#+|--+|\*+)?\s*`)

	// embedLine finds the namespaces the product actually embeds.
	embedLine = regexp.MustCompile(`(?m)^//go:embed\s+(.+)$`)
)

const (
	// migrationsRoot holds the namespace directories, relative to this package.
	migrationsRoot = "migrations"

	// namespaceOwner declares which of them are real. The embed line is the
	// product's own answer, and reading it as TEXT is deliberate: .go-arch-lint
	// refuses `root -> migrations`, but nothing refuses reading the file.
	//
	// Not the directory listing. Any directory that happened to sit under
	// migrations/ — a scratch copy, an unpacked dump — otherwise became a
	// namespace and its versions became "known", which silently converts real
	// dangling citations into passing ones with no failing assertion.
	namespaceOwner = "migrations/migrations.go"

	// citationRegisterPath records the citations this tree carries today.
	citationRegisterPath = "danglingcitations.txt"

	// scannedFloor is far below the tree's real file count, so it catches a
	// census that read almost nothing rather than a tree that grew or shrank.
	scannedFloor = 1000
)

// updateCitationRegister PRUNES the register; it never adds.
//
// One-way on purpose. A regeneration that absorbed whatever it found would turn
// the sanctioned command into the way a new dead-end citation is laundered
// green, leaving added lines in a long file as the only signal. Removing an
// entry cannot hide anything, so that half is safe to automate; adding one is a
// judgement somebody has to make by hand, which is the friction the register
// exists to carry.
var updateCitationRegister = flag.Bool("update-citations", false,
	"prune danglingcitations.txt of entries the tree no longer has")

func TestEveryCitedMigrationExists(t *testing.T) {
	known := knownMigrationVersions(t)
	found, unread, scanned, skipped := scanTreeForCitations(t, known)
	// A census that read a fraction of the tree must not report the same word
	// for it. Under a sparse checkout every skip-worktree file vanishes with no
	// signal.
	if scanned < scannedFloor {
		t.Fatalf("only %d file(s) were read (%d skipped) — this census covered almost nothing, "+
			"which reports PASS while checking a fraction of the tree", scanned, skipped)
	}

	recorded := readCitationRegister(t, citationRegisterPath)
	present := map[string]bool{}
	for _, c := range found {
		present[c.key()] = true
	}

	if *updateCitationRegister {
		pruneCitationRegister(t, recorded, present, unread)
		return
	}

	for _, c := range found {
		if recorded[c.key()] {
			continue
		}
		t.Errorf("new dangling migration citation:\n  %s\n"+
			"A citation written now against a version that never existed is a dead end from "+
			"birth. State the invariant instead, or say the migration is in git history rather "+
			"than in the tree.\nIf it is genuinely unavoidable, add the line to %s BY HAND — "+
			"`-update-citations` only prunes, because a command that absorbed new citations "+
			"would be how a dead end gets laundered green.", c, citationRegisterPath)
	}

	var stale []string
	for key := range recorded {
		if present[key] || unread[registerEntryPath(key)] {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)
	for _, s := range stale {
		t.Errorf("%s records %q, which is no longer a dangling citation — thank you.\n"+
			"Run `go test . -run TestEveryCitedMigrationExists -update-citations` from backend/ "+
			"to prune it, so the register keeps describing the tree.", citationRegisterPath, s)
	}
}

// TestTheCitationDetectorSeesWhatItClaims pins the detector's reach, including
// the shapes it is known NOT to reach.
//
// The register is no substitute for this: it protects a width or a terminator
// only where a (file, version) pair has no OTHER citation propping it up, so a
// silently narrowed pattern can look green for a long time. The `nil` rows are
// the honest half — each is a real shape this gate cannot see, and flipping one
// is how a future widening announces itself.
func TestTheCitationDetectorSeesWhatItClaims(t *testing.T) {
	known := map[string]map[string]bool{
		"core":   {"0001": true, "1787320004": true},
		"custom": {"20260716120000": true},
	}
	for _, tc := range []struct {
		name string
		text string
		want []string
	}{
		{"namespace and space", "core 0217 retired it", []string{"0217"}},
		{"namespace and slash", "see core/0063 for why", []string{"0063"}},
		{"unqualified", "migration 0099 added it", []string{"0099"}},
		{"backticked", "migration `0105` did", []string{"0105"}},
		{"filename form", "migrations/core/0012_audit_log.up.sql", []string{"0012"}},
		{"bare filename, no namespace word", "0008_activity.up.sql is where", []string{"0008"}},
		{"fourteen digits", "custom/20260101000000 seeded", []string{"20260101000000"}},
		{"ten digits", "core 1799999999 did", []string{"1799999999"}},
		{"a version that EXISTS is not reported", "core 0001 is the baseline", nil},
		{"a custom version cited as core", "core/20260716120000", []string{"20260716120000"}},
		{"list, comma", "(core 0007, 0131)", []string{"0007", "0131"}},
		{"list, slash", "core 0007/0131", []string{"0007", "0131"}},
		{"wrapped across two lines", "see core\n// 0217 (ADR-0091)", []string{"0217"}},

		// KNOWN GAPS. Each is a real shape in the wild that this detector does
		// not reach; the rows exist so widening it is a one-line flip.
		{"GAP: split across three lines", "see core\n//\n// 0217", nil},
		{"GAP: hyphen separator", "core-0217 did it", nil},
		{"GAP: zero-padded past the widths", "core 00217", nil},
		{"GAP: reached by prose", "the migration after 0007", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, c := range danglingCitationsIn("probe.go", tc.text, known) {
				got = append(got, c.version)
			}
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("detector reported %v, want %v\n  text: %q", got, want, tc.text)
			}
		})
	}
}

// TestTheCitationRegisterIsSortedAndUnique keeps the file diffable.
//
// Nothing else holds it: a hand-added entry lands wherever the author typed it,
// and the next prune rewrites the whole file sorted — so an unsorted register
// produces a diff full of moves that hides the one line that matters.
func TestTheCitationRegisterIsSortedAndUnique(t *testing.T) {
	entries := registerEntries(t, citationRegisterPath)
	seen := map[string]bool{}
	for i, e := range entries {
		if seen[e] {
			t.Errorf("%s lists %q twice", citationRegisterPath, e)
		}
		seen[e] = true
		if i > 0 && entries[i-1] > e {
			t.Errorf("%s is not sorted: %q follows %q.\nRun `go test . -run "+
				"TestEveryCitedMigrationExists -update-citations` from backend/ to rewrite it.",
				citationRegisterPath, e, entries[i-1])
		}
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
// editing a file above one is not a new citation and does not churn the file.
//
// The cost, stated because it is a real blind spot rather than a simplification:
// in a file that ALREADY has an entry for a version, a second citation of that
// same version is invisible. The register holds "this file cites this missing
// version", not "how often".
func (c citationRef) key() string {
	return fmt.Sprintf("%s %s %s", c.path, c.namespace, c.version)
}

func (c citationRef) String() string {
	return fmt.Sprintf("%s:%d cites %s %s — %s", c.path, c.line, c.namespace, c.version, c.excerpt)
}

// scanTreeForCitations reads every tracked text file and reports the citations
// naming a version no namespace carries. It returns what it read and what it
// skipped, so the caller can refuse a census that covered almost nothing.
//
// EVERY tracked file, with no extension allowlist: an unmeasured prefilter in
// front of a census is what CLAUDE.md rule 8 forbids by name.
func scanTreeForCitations(t *testing.T, known map[string]map[string]bool) (found []citationRef, unread map[string]bool, scanned, skipped int) {
	t.Helper()
	// Paths the scan did not read. A prune consults it, because an entry whose
	// file was merely unreadable is not an entry the tree has stopped carrying
	// — deleting it would quietly empty the register for whatever happened to
	// be missing at that moment.
	unread = map[string]bool{}
	for _, f := range trackedFiles(t) {
		// A symlink is skipped rather than followed. os.ReadFile follows one,
		// and a tracked symlink pointing outside the worktree would make this
		// gate an arbitrary-file read whose contents it then PRINTS — in a
		// public repository whose CI logs are public.
		if f.symlink || citationScanExempt(f.path) {
			skipped++
			unread[f.path] = true
			continue
		}
		body, err := os.ReadFile(filepath.Join("..", f.path))
		if err != nil {
			// A tracked path can be absent mid-rebase, and a gitlink
			// (submodule) reads as a directory.
			if os.IsNotExist(err) || strings.Contains(err.Error(), "is a directory") {
				skipped++
				unread[f.path] = true
				continue
			}
			t.Fatalf("reading %s: %v", f.path, err)
		}
		scanned++
		found = append(found, danglingCitationsIn(f.path, string(body), known)...)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].key() < found[j].key() })
	return found, unread, scanned, skipped
}

// citationScanExempt reports a path this gate does not read.
//
// The generated ones are exempt because a citation inside them is a citation in
// the SOURCE they were rendered from, and that source is scanned — reporting
// both would send somebody to fix the copy `make gen` overwrites. `_gen.go` is
// ANCHORED to the directories that hold generated Go rather than accepted as a
// bare suffix: a suffix alone makes `_gen.go` a general-purpose invisibility
// cloak any new file can put on.
func citationScanExempt(rel string) bool {
	switch {
	case rel == "backend/gates/migrationcitations_test.go",
		rel == "backend/"+citationRegisterPath,
		rel == "frontend/src/api/schema.d.ts",
		rel == "frontend/src/api/public-events.ts",
		strings.HasPrefix(rel, "backend/internal/contracts/"),
		strings.HasSuffix(rel, ".generated.json"):
		return true
	case strings.HasSuffix(rel, "_gen.go"):
		return strings.HasPrefix(rel, "backend/internal/") || strings.HasPrefix(rel, "extensions/")
	}
	return false
}

// trackedFile is one entry from the index, with the one mode bit this gate
// cares about.
type trackedFile struct {
	path    string
	symlink bool
}

// trackedFiles reads the index. `-s` carries the mode, which is how a symlink is
// told from a file without stat-ing it; `-z` makes the output NUL-delimited, so
// no filename can be misread.
func trackedFiles(t *testing.T) []trackedFile {
	t.Helper()
	out, err := exec.Command("git", "-C", "..", "ls-files", "-sz").Output()
	if err != nil {
		t.Fatalf("listing tracked files: %v (this test must run inside the git worktree)", err)
	}
	var files []trackedFile
	for _, row := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if row == "" {
			continue
		}
		// <mode> <sha> <stage>\t<path>
		meta, path, ok := strings.Cut(row, "\t")
		if !ok {
			continue
		}
		files = append(files, trackedFile{path: path, symlink: strings.HasPrefix(meta, "120000")})
	}
	return files
}

// danglingCitationsIn reports the citations in one file naming a version no
// namespace carries.
//
// Each line is matched joined to the NEXT one, with a comment prefix stripped,
// because this tree wraps at about 78 columns and a citation whose namespace
// word and version land on different lines is otherwise invisible. The finding
// is reported on the line the namespace word sits on.
func danglingCitationsIn(rel, body string, known map[string]map[string]bool) []citationRef {
	lines := strings.Split(body, "\n")
	var found []citationRef
	seen := map[string]bool{}

	add := func(i int, namespace, version, text string) {
		// UTF-8 is judged per LINE. Whole-file validity let one stray byte
		// anywhere hide every citation in the file — invisible in a diff, and
		// strictly worse than the suffix allowlist this gate refuses.
		if !utf8.ValidString(text) || carriesVersion(known, namespace, version) {
			return
		}
		ref := citationRef{
			path: rel, line: i + 1, namespace: namespace, version: version,
			excerpt: citationExcerpt(text),
		}
		if seen[ref.key()] {
			return
		}
		seen[ref.key()] = true
		found = append(found, ref)
	}

	for i, line := range lines {
		window := line
		if i+1 < len(lines) {
			window = line + " " + commentContinuation.ReplaceAllString(lines[i+1], "")
		}
		// Where a namespaced match consumed the version, so the filename pass
		// below does not report the same citation a second time under a
		// different namespace: `core/0012_audit_log.up.sql` matches BOTH
		// patterns, and two register entries for one citation would make the
		// register wrong about the tree in a way pruning cannot repair.
		var claimed [][2]int
		for _, m := range namespacedCitation.FindAllStringSubmatchIndex(window, -1) {
			namespace := strings.ToLower(window[m[2]:m[3]])
			claimed = append(claimed, [2]int{m[4], m[5]})
			add(i, namespace, window[m[4]:m[5]], line)
			// The rest of a list belongs to the same namespace word.
			for rest, at := window[m[1]:], m[1]; ; {
				tail := listedVersion.FindStringSubmatchIndex(rest)
				if tail == nil {
					break
				}
				claimed = append(claimed, [2]int{at + tail[2], at + tail[3]})
				add(i, namespace, rest[tail[2]:tail[3]], line)
				rest, at = rest[tail[1]:], at+tail[1]
			}
		}
		// A bare filename names no namespace, so the union is the honest pool.
		for _, m := range bareFilename.FindAllStringSubmatchIndex(line, -1) {
			if versionAlreadyClaimed(claimed, m[2]) {
				continue
			}
			add(i, "migration", line[m[2]:m[3]], line)
		}
	}
	return found
}

// versionAlreadyClaimed reports whether a namespaced match already consumed the
// version starting at this offset.
func versionAlreadyClaimed(claimed [][2]int, start int) bool {
	for _, span := range claimed {
		if start >= span[0] && start < span[1] {
			return true
		}
	}
	return false
}

// registerEntryPath is the path half of a register key, which is everything
// before the namespace and version it ends with.
func registerEntryPath(key string) string {
	if i := strings.LastIndex(key, " "); i > 0 {
		if j := strings.LastIndex(key[:i], " "); j > 0 {
			return key[:j]
		}
	}
	return key
}

// carriesVersion reports whether the cited version exists in the namespace the
// citation named. An UNQUALIFIED citation names none, so the union is the honest
// pool — narrowing it to one namespace would invent a claim the text never made.
func carriesVersion(known map[string]map[string]bool, namespace, version string) bool {
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

// knownMigrationVersions collects the versions every EMBEDDED namespace carries,
// read from the index rather than the working directory.
//
// From git, so an untracked leftover `NNNN_x.up.sql` cannot make a version
// "known" locally and green while CI, on a clean checkout, goes red.
func knownMigrationVersions(t *testing.T) map[string]map[string]bool {
	t.Helper()
	known := map[string]map[string]bool{}
	for _, ns := range embeddedNamespaces(t) {
		known[ns] = map[string]bool{}
	}
	prefix := "backend/" + migrationsRoot + "/"
	for _, f := range trackedFiles(t) {
		if !strings.HasPrefix(f.path, prefix) || !strings.HasSuffix(f.path, ".up.sql") {
			continue
		}
		ns, name, ok := strings.Cut(strings.TrimPrefix(f.path, prefix), "/")
		if !ok {
			continue
		}
		versions, embedded := known[ns]
		if !embedded {
			continue
		}
		version, _, ok := strings.Cut(name, "_")
		if !ok {
			t.Fatalf("%s: want <version>_<name>.up.sql", f.path)
		}
		versions[version] = true
	}
	for ns, versions := range known {
		if len(versions) == 0 {
			t.Fatalf("namespace %q is embedded by %s but carries no .up.sql in the index — the "+
				"derivation has stopped resolving, and every citation of it would report as "+
				"dangling", ns, namespaceOwner)
		}
	}
	return known
}

// embeddedNamespaces reads the //go:embed line that declares them.
func embeddedNamespaces(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(namespaceOwner)
	if err != nil {
		t.Fatalf("reading %s: %v", namespaceOwner, err)
	}
	m := embedLine.FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatalf("%s carries no //go:embed line, so the namespace set cannot be derived from the "+
			"product's own declaration", namespaceOwner)
	}
	return strings.Fields(m[1])
}

// readCitationRegister reads the recorded backlog as a set.
func readCitationRegister(t *testing.T, path string) map[string]bool {
	t.Helper()
	recorded := map[string]bool{}
	for _, e := range registerEntries(t, path) {
		recorded[e] = true
	}
	return recorded
}

// registerEntries reads the register's lines, in file order.
//
// Deliberately NOT shared with uniquenessclaims_test.go's reader, which is the
// same handful of lines over a different file. One helper in front of two
// registers would couple formats that are free to diverge — this one is three
// space-separated fields, that one is a prose key — and the coupling would be
// discovered the first time either changes shape. Two readers, one reason,
// stated here rather than in a pull request.
func registerEntries(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var entries []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		entries = append(entries, trimmed)
	}
	return entries
}

// pruneCitationRegister rewrites the register with the entries the tree still
// has, and names every line it removed.
func pruneCitationRegister(t *testing.T, recorded, present, unread map[string]bool) {
	t.Helper()
	var keep, removed []string
	for key := range recorded {
		// Kept when the citation is still there, and ALSO when its file could
		// not be read: an unread file has not stopped citing anything, and a
		// prune run mid-rebase would otherwise delete entries wholesale.
		if present[key] || unread[registerEntryPath(key)] {
			keep = append(keep, key)
			continue
		}
		removed = append(removed, key)
	}
	sort.Strings(keep)
	sort.Strings(removed)

	var b strings.Builder
	b.WriteString(citationRegisterHeader)
	for _, k := range keep {
		b.WriteString(k)
		b.WriteString("\n")
	}
	if err := os.WriteFile(citationRegisterPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writing %s: %v", citationRegisterPath, err)
	}
	for _, r := range removed {
		t.Logf("pruned: %s", r)
	}
	t.Logf("%s: %d entries kept, %d pruned", citationRegisterPath, len(keep), len(removed))
}

// citationRegisterHeader is what the register explains about itself. It is
// written on every prune rather than preserved from the file, so a rewrite
// cannot silently drop the part that stops the next reader treating the list as
// a permission.
const citationRegisterHeader = `# THE DANGLING-CITATION REGISTER — a record, not a permission.
#
# Every line is a file that cites a migration version no namespace carries, in
# the form: <path> <namespace> <version>. A namespace of "migration" means the
# citation named none, so it was checked against every namespace at once.
#
# The migrations these name were mostly deleted by the 2026-08 baseline
# consolidation, and most still resolve through git history — so they are stale
# prose rather than defects, and rewriting them mechanically would cost more
# than it buys: a citation is usually the only thing in a sentence saying where
# the reasoning lives.
#
# What this holds is narrower than "the backlog cannot grow", and the difference
# matters: a file acquires no citation of a version it does not ALREADY cite. A
# second citation of a version already listed for that file is invisible.
#
# Per FILE rather than a total, because a total lets a deletion anywhere fund a
# new dead citation elsewhere: the two net and the gate reports green over a
# tree that got worse.
#
# Line numbers are deliberately absent: editing a file above a citation is not a
# new citation, and a register that churned on every edit would be ignored.
#
# A test harness that WRITES fixture migration paths lands here too — the string
# is citation-shaped and this gate cannot tell it from prose. Those entries are
# noise rather than debt, and they are left in rather than skipped by name: a
# skip-list in front of a census is the thing that makes one quietly go short.
#
# TO FIX ONE: state the invariant so the sentence stands alone, or say the
# migration is in git history rather than in the tree — then prune the line.
#
# TO PRUNE, from backend/:
#   go test . -run TestEveryCitedMigrationExists -update-citations
#
# That command only REMOVES entries. Adding one is a hand edit on purpose:
# regenerating to absorb a citation you just wrote is not a fix, and the added
# line has to be one a reviewer agreed to.
`

// citationExcerpt trims a cited line to something readable. Prose lines in this
// tree run long, and one full paragraph per finding buries the list this output
// exists to be read as. Runes, not bytes: these lines carry em-dashes, and
// slicing mid-rune prints replacement characters.
func citationExcerpt(line string) string {
	const width = 100
	trimmed := strings.TrimSpace(line)
	if utf8.RuneCountInString(trimmed) <= width {
		return trimmed
	}
	return string([]rune(trimmed)[:width]) + "…"
}
