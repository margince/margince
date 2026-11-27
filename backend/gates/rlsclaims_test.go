// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// No source comment or prose may name row-level security as a control on what
// a read reaches, because no table carries it.
//
// No table in any schema carries row-level security, and no policy exists to
// read. What a statement reaches is bounded by the predicate it writes for
// itself and by the row-scope clauses in platform/auth — so a source comment
// saying RLS scopes, bounds, confines or gates a read names a control the
// database does not apply, and the next reader trusts it instead of checking.
// Nineteen statements whose scope had been the DATABASE's were found the hard
// way, two of them data loss and one a cross-tenant disclosure; every one was
// found by a failing test, because nothing gates the class. This is the cheap
// half of that gate: it cannot tell whether a query is scoped, but it can stop
// the tree from claiming a retired mechanism scopes it.
//
// One walk reads the whole repository: Go, and the prose a reader meets
// first. That is the published contract, the front door, an agent's brief and
// a comment shipped into the database catalog.
//
// It bans the CLAIM, not the word. Three spellings stay legal because they
// name something real: the `// rls-exempt:` waiver marker that
// scripts/check-rls-store-path.sh reads, the BYPASSRLS/NOBYPASSRLS role
// attributes (a live cluster property AssertRuntimeRole still refuses), and
// prose that names the retirement itself.

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// rlsClaim matches RLS asserted as a live scoping control. The verb list is
// the set of spellings this tree actually grew before the gate existed, in
// both orders (`RLS scopes it` and `scoped by RLS`) plus the hyphenated
// adjective form, which is how most of them were written.
// The suffix group carries `d` as well as `ed` because the stems are whole
// words: "scoped" is scope+d, not scope+ed. Leaving it out is what the pinning
// test below caught — the gate had swept the tree clean of `RLS-scoped` while
// being unable to see it. "bound" is listed separately from the verb group
// because it is bind's irregular past participle, not bind+ed; the `already`
// alternative allows up to two words between RLS and "already" because a
// review sweep found `RLS GUC already` sitting between the two; and "forced"
// gets its own alternative because it names the same live-control claim as
// the verb group without appearing as one of its stems ("RLS is forced",
// "RLS-forced").
var rlsClaim = regexp.MustCompile(`(?i)\bRLS[ -](?:scope|bound|confine|restrict|isolate|enforce|govern|gate|bind|keep|constrain|protect|guard)(?:s|d|es|ed)?\b` +
	`|\bRLS(?:[ -]\w+){0,2}[ -]already\b` +
	`|\b(?:scoped|bounded|bound|confined|restricted|isolated|gated|governed|protected|filtered|guarded)[ -]by[ -]RLS\b` +
	`|\bFORCE RLS (?:doesn't|does not|do not|don't)\b` +
	`|\bRLS[ -](?:is[ -](?:on and[ -])?)?forced\b`)

// TestTheRLSClaimPatternCatchesTheSpellingsThisTreeGrew pins the pattern
// against the real phrasings the 2026-08 sweep removed, and against the
// three that must survive it. A gate whose pattern silently stopped matching
// would read exactly like a clean tree.
func TestTheRLSClaimPatternCatchesTheSpellingsThisTreeGrew(t *testing.T) {
	t.Parallel()
	mustMatch := []string{
		"// EmailSuppressed reports whether an address belongs to an erased subject in the current workspace (RLS scopes the read).",
		"// members (row-scoped by RLS), optionally filtered by in.Q.",
		"// comms_outbound is RLS-scoped, so every read the dispatcher makes",
		"// The workspace predicate duplicates what RLS already enforces",
		"// for the RLS-governed catalog insert and audit write.",
		"// workspace-scoped read (RLS confines it to the tenant)",
		"// superuser, so FORCE RLS doesn't bite behaviorally",
		"// workspace-bound by RLS through the GUC the binding above set.",
		"// app_user is RLS-guarded like every tenant table",
		"// the signal table, which is RLS-forced and has no handler",
		"// transaction the RLS GUC already bounds what can be locked",
	}
	for _, line := range mustMatch {
		if !rlsClaim.MatchString(line) {
			t.Errorf("the pattern no longer catches a claim it was written for:\n\t%s", line)
		}
	}
	mustNotMatch := []string{
		"\t// rls-exempt: fleet enumeration — the workspace table is not workspace-scoped",
		"// the fixture owner role holds neither rolsuper nor rolbypassrls",
		"// margince_owner is NOSUPERUSER NOBYPASSRLS in db-bootstrap.sql",
		"// core 0217 (ADR-0091) retired every tenant-isolation policy",
		"// the query's own workspace predicate scopes the read",
	}
	for _, line := range mustNotMatch {
		if rlsClaim.MatchString(line) {
			t.Errorf("the pattern flags a legitimate mention:\n\t%s", line)
		}
	}
}

// proseRLSClaim is the spellings prose grew that Go comments did not. "Row-scoped
// by workspace RLS" is the contract's, and "FORCE RLS plus a workspace-bound
// policy" a shipped migration's. "Bound by row-level security" spells the
// acronym out.
//
// Kept as a second pattern because a denial in the same line answers for it,
// and nothing answers for a line rlsClaim matches.
var proseRLSClaim = regexp.MustCompile(`(?i)` +
	`\b(?:row-)?scoped[ -]by[ -](?:workspace[ -])?RLS\b` +
	`|\bFORCE (?:RLS|ROW LEVEL SECURITY)\b` +
	`|\bENABLE ROW LEVEL SECURITY\b` +
	`|\b(?:bound|bounded|scoped|confined|isolated|governed|protected)[ -]by[ -]row.level security\b` +
	`|\brow.level security[ -](?:scope|bind|bound|confine|isolate|enforce|govern|protect|applies|appl)(?:s|d|es|ed|y)?\b` +
	`|\bRLS[ -](?:scope|bind|bound|confine|isolate|enforce|govern|protect|guard|applies|bypass)(?:s|d|es|ed)?\b` +
	`|\bRLS[ -](?:polic|proof|runtime)\w*\b` +
	`|\bRLS does bind\b` +
	`|\bdespite RLS\b` +
	// A lane, target or heading named after the control: "integration (RLS +
	// erasure)" tells a reader the lane proves something it does not.
	`|\bRLS \+` +
	`|\bthe RLS\b`)

// proseScanned are the extensions besides `.go` that a reader reads a claim in.
var proseScanned = map[string]bool{
	".md": true, ".yaml": true, ".yml": true, ".sh": true,
	".sql": true, ".ts": true, ".tsx": true, ".txt": true,
	// .example, because an operator reads the environment template first and it
	// once bounded the app role by a policy that is not there. Makefile,
	// because `make help` prints a target's help text, and two credited it.
	".example": true, ".mk": true,
}

// ignoredTrees answers which directories are not this repository's prose, by
// asking git rather than keeping a list of names. A list is a second copy of
// .gitignore. An ignored agent worktree once made it report hundreds of claims
// out of a stale copy of the tree, while CI stayed green.
//
// It fails rather than falling back. Without the answer the walk reads ignored
// trees again, and a census that widens cannot be told from one that narrows.
func ignoredTrees(t *testing.T) map[string]bool {
	t.Helper()
	// --directory collapses a wholly-ignored directory to one entry with a
	// trailing slash, so the walk can skip the tree instead of every file in it.
	out, err := exec.Command("git", "-C", "..",
		"ls-files", "--others", "--ignored", "--exclude-standard", "--directory").Output()
	if err != nil {
		t.Fatalf("asking git which trees it ignores: %v", err)
	}
	ignored := map[string]bool{
		// git never reports its own directory, and it is full of prose from
		// every branch that ever mentioned the control.
		"../.git": true,
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if !strings.HasSuffix(line, "/") {
			continue
		}
		ignored["../"+strings.TrimSuffix(line, "/")] = true
	}
	return ignored
}

// proseExempt are claims that cannot be fixed by editing the line they sit on.
//
// Keyed by path and text, so an entry answers for its own sentence and a new
// claim in the same file is still reported. All three are in shipped
// migrations. An applied version never re-runs, so editing one would leave
// every deployed catalog with the old text. The correction is a new migration,
// which each reason names.
var proseExempt = gatekit.Waive(map[string]string{
	"../backend/migrations/core/0001_baseline.up.sql: COMMENT ON COLUMN org_dossier.user_id IS 'The reader this assembly was generated for. Written into every read explicitly — RLS binds the workspace, not the reader.';": "an APPLIED version, which never re-runs — editing it would change what a fresh installation is told while every deployed catalog kept this sentence. Corrected by migration 1788751821_a_dossier_names_its_reader_not_a_policy",

	"../backend/migrations/core/0001_baseline.up.sql: COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. Tenant isolation is FORCE RLS plus a workspace-bound policy per table, NOT ownership — a per-unit ext_<name> owner role exists only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';": "the same applied baseline, and this comment was already corrected by migration 1787905400_the_ext_schema_says_what_bounds_a_unit",

	"../backend/migrations/core/1787905400_the_ext_schema_says_what_bounds_a_unit.down.sql: COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. Tenant isolation is FORCE RLS plus a workspace-bound policy per table, NOT ownership — a per-unit ext_<name> owner role exists only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';": "the DOWN half of the migration that corrected the claim above: restoring the prior text is what a rollback IS",
})

// deniesTheControl spots a sentence that names the control to say it is not
// there. Those are the corrections this gate exists to produce, and failing
// them would make it unsatisfiable: three down migrations say audit_log
// carries no policy.
func deniesTheControl(line string) bool {
	lowered := strings.ToLower(line)
	// A line that denies the control in one clause and asserts it in another
	// is a claim. Checked first, or one denial phrase would exempt the line.
	for _, assertion := range []string{
		"bound by row-level security", "scoped by rls", "rls binds", "rls scopes",
		"rls enforces", "protected by rls",
	} {
		if strings.Contains(lowered, assertion) {
			return false
		}
	}
	for _, denial := range []string{
		"no rls", "no row-level security", "no row level security",
		"rls=false", "not workspace-scoped", "carries neither",
		"assumed force rls", "does not apply to a superuser",
		"nobypassrls", "rolbypassrls", "rls-exempt", "rls store-path", "rls-store-path",
	} {
		if strings.Contains(lowered, denial) {
			return true
		}
	}
	return false
}

// historicalRecord reports whether a page opens by declaring itself a record of
// what was once true. The marker has to be near the top, where a reader meets
// it before the prose it qualifies.
func historicalRecord(b []byte) bool {
	head := string(b)
	if len(head) > 2000 {
		head = head[:2000]
	}
	return strings.Contains(head, "**Historical record")
}

// rlsClaimIn reports whether a line credits RLS with a control. A line
// rlsClaim matches is a claim whatever else it says. A denial beside it
// answers the prose pattern, which reads no Go. A Go test plants the control
// in SQL to prove a gate refuses it.
func rlsClaimIn(line string, isGo bool) bool {
	if !mayHoldFolded(line, "rls", "level security") {
		return false
	}
	return rlsClaim.MatchString(line) || (!isGo && proseRLSClaim.MatchString(line) && !deniesTheControl(line))
}

// rlsClaimsSelf is this file, which states the banned spellings to pin them.
// The exemption is by path: any other file naming itself the same way is read.
const rlsClaimsSelf = "../backend/" + gateDir + "/rlsclaims_test.go"

func TestTheProseRLSPatternCatchesWhatProseGrew(t *testing.T) {
	t.Parallel()
	// The pattern must bite. Each of these stood in the tree when this gate was
	// written, and each is a spelling the Go gate's pattern does not catch.
	for _, line := range []string{
		"Excludes archived teams. Row-scoped by workspace RLS.",
		"Tenant isolation is FORCE RLS plus a workspace-bound policy per table",
		"a migration that forgets `FORCE RLS`, an erasure that misses a PII table",
		"Extension runtime code is bound by row-level security exactly as core code is",
		"RLS binds the workspace, not the reader.",
		"despite RLS and the composite same-workspace foreign keys",
		// Found still standing after the first sweep, in files this gate did
		// not read and in phrasings its pattern did not have.
		"[required] Postgres DSN for the application role; row-level security applies.",
		"MARGINCE_DSN names margince_app, which RLS does bind",
		"DB-free floor under the RLS runtime proof",
		"name: integration (RLS + erasure + HTTP e2e)",
		"addresses the superuser pool directly (RLS bypass)",
		"ALTER TABLE contact ENABLE ROW LEVEL SECURITY;",
		"Extension runtime code is bound by row-level security exactly as core code is",
	} {
		if !rlsClaimIn(line, false) {
			t.Errorf("the pattern does not catch a claim this gate was written for:\n\t%s", line)
		}
	}
	// And it must not bite the sentences that say the opposite, or the gate
	// makes the tree's own corrections unwritable.
	for _, line := range []string{
		"No table has row-level security. Isolation is SQL predicates.",
		"A unit table must carry no workspace column, no row-level security and no policy",
		"the fixture owner role holds neither rolsuper nor rolbypassrls",
		"the committed schema records it `rls=false force=false` and no migration enables it",
		"An earlier reading of this gap assumed FORCE RLS made the probe blind; it does not exist",
		"`make rls-store-path` — no module statement addresses the superuser pool",
		"River persists args verbatim in a table with no workspace column and no RLS",
		// The denial is the whole sentence here. A line with a denial and an
		// assertion is a claim, which deniesTheControl checks first.
		"the owner is a superuser, and FORCE row-level security does not apply to a superuser",
	} {
		if rlsClaimIn(line, false) {
			t.Errorf("the pattern flags a sentence that states the truth:\n\t%s", line)
		}
	}
	// Prose is held to the Go pattern too, and a denial does not answer it.
	// The first matches no prose spelling; the second carries a denial phrase.
	for _, line := range []string{
		"the queue is RLS-gated like every tenant table",
		"RLS already bounds the read, as the no rls tables never did",
	} {
		if !rlsClaimIn(line, false) || !rlsClaimIn(line, true) {
			t.Errorf("a claim the Go pattern sees passes in prose or Go:\n\t%s", line)
		}
	}
	// And Go is not held to the prose pattern: a Go test plants the control in
	// SQL to prove a gate refuses it.
	if planted := "ALTER TABLE webhook_delivery ENABLE ROW LEVEL SECURITY"; rlsClaimIn(planted, true) || !rlsClaimIn(planted, false) {
		t.Errorf("the prose pattern reads Go, or no longer reads prose:\n\t%s", planted)
	}
}

// TestNoTextClaimsRLSStillScopesARead walks the repository, skipping only what
// git ignores, so a new file is enrolled the moment it exists. It counts Go
// and prose apart, so losing either half fails on its own floor.
func TestNoTextClaimsRLSStillScopesARead(t *testing.T) {
	t.Parallel()
	// An exemption that stopped matching is one nobody removed.
	defer proseExempt.AssertAllMatched(t)

	skipped := ignoredTrees(t)
	var claims []string
	goRead, proseRead := 0, 0
	err := gatekit.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipped[filepath.ToSlash(path)] || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		isGo := strings.HasSuffix(path, ".go")
		if !isGo && !proseScanned[strings.ToLower(filepath.Ext(path))] && filepath.Base(path) != "Makefile" {
			return nil
		}
		slashed := filepath.ToSlash(path)
		if !d.Type().IsRegular() || slashed == rlsClaimsSelf {
			return nil
		}
		b, err := gatekit.ReadFile(path) // #nosec G304 G122 -- a text file from walking the trusted source tree
		if err != nil {
			return err
		}
		// A page that opens by saying it describes code no longer in the tree
		// is a record of what was true. Correcting its tense falsifies it.
		if !isGo && historicalRecord(b) {
			return nil
		}
		if isGo {
			goRead++
		} else {
			proseRead++
		}
		if !mayHoldFolded(string(b), "rls", "level security") {
			return nil
		}
		for i, line := range strings.Split(string(b), "\n") {
			// Asked about the offence and not the file, keyed on the line's
			// text, so an exemption covers its own claim and no new one.
			if !rlsClaimIn(line, isGo) || proseExempt.Waived(t, slashed+": "+strings.TrimSpace(line)) {
				continue
			}
			claims = append(claims, slashed+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}

	// A sweep that read nothing passes like a clean tree. The prose floor sits
	// near the real count, so losing one directory fails.
	if goRead < 1000 || proseRead < 400 {
		t.Fatalf("read only %d Go and %d prose files: the walk has lost part of the tree and this gate is "+
			"no longer a census", goRead, proseRead)
	}
	if len(claims) > 0 {
		t.Errorf("%d line(s) credit row-level security with a control this tree does not carry. "+
			"Core has no policy and no workspace column; say what actually bounds the thing: the "+
			"statement's own predicate, the row-scope clauses in platform/auth, ADR-0061's single company, "+
			"or the single installation. If the honest answer is \"nothing does\", that is a defect to fix "+
			"rather than a sentence to reword. A claim in a SHIPPED migration is corrected by a new "+
			"migration and listed in proseExempt, never by editing the applied one:\n\t%s",
			len(claims), strings.Join(claims, "\n\t"))
	}
}
