// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The prose says what bounds a read, and it is not row-level security.
//
// rlsclaims_test.go beside this holds the same rule for Go comments and reads
// `.go` and nothing else. That left the claim standing in the places a reader
// is most likely to meet it first: the published API contract, the repository's
// front door, the brief a security-review agent is handed, and a comment shipped
// into the live database catalog. Four of those said row-level security scoped a
// read; core carries none, and has not since the tenant column was retired.
//
// A census that can fail short has already failed, so this one walks the whole
// repository rather than a list of files somebody remembered to add.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// proseRLSClaim is rlsClaim plus the spellings prose grew that Go comments did
// not. "Row-scoped by workspace RLS" is the contract's; "FORCE RLS plus a
// workspace-bound policy" is a shipped migration's; "bound by row-level
// security" spells out what the acronym abbreviates.
//
// Kept as a separate pattern rather than widened in place, because the Go gate's
// own pinning test states what its pattern is for, and a phrase that only ever
// appeared in prose would be dead weight there.
var proseRLSClaim = regexp.MustCompile(`(?i)` +
	`\b(?:row-)?scoped[ -]by[ -](?:workspace[ -])?RLS\b` +
	`|\bFORCE (?:RLS|ROW LEVEL SECURITY)\b` +
	`|\b(?:bound|bounded|scoped|confined|isolated|governed|protected)[ -]by[ -]row.level security\b` +
	`|\brow.level security[ -](?:scope|bind|bound|confine|isolate|enforce|govern|protect)(?:s|d|es|ed)?\b` +
	`|\bRLS[ -](?:scope|bind|bound|confine|isolate|enforce|govern|protect|guard)(?:s|d|es|ed)?\b` +
	`|\bdespite RLS\b`)

// proseScanned are the extensions a reader actually reads a claim in. `.go` is
// absent: rlsclaims_test.go owns it, and scanning it twice would report every
// finding twice.
var proseScanned = map[string]bool{
	".md": true, ".yaml": true, ".yml": true, ".sh": true,
	".sql": true, ".ts": true, ".tsx": true, ".txt": true,
}

// proseSkippedDirs are trees that are not this repository's prose: vendored
// code, build output, and the generated client whose text comes from the
// contract (fixing crm.yaml fixes it, and it regenerates).
var proseSkippedDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	".build": true, ".tmp": true, "coverage": true,
}

// proseExempt are CLAIMS that cannot be fixed by editing the line they sit on.
//
// Keyed by path AND text, so an entry answers for the sentence it was written
// for and a NEW claim in the same file is still reported. Three are in shipped
// migrations: an applied version never re-runs, so editing one changes what a
// fresh installation is told while every deployed catalog keeps the old text,
// and the two then disagree silently. The correction goes in a new migration,
// which each reason names. The fourth is a changelog entry about the release
// that genuinely had the control.
var proseExempt = gatekit.Waive(map[string]string{
	"../backend/migrations/core/0001_baseline.up.sql: COMMENT ON COLUMN org_dossier.user_id IS 'The reader this assembly was generated for. Written into every read explicitly — RLS binds the workspace, not the reader.';": "an APPLIED version, which never re-runs — editing it would change what a fresh installation is told while every deployed catalog kept this sentence. Corrected by migration 1788751821_a_dossier_names_its_reader_not_a_policy",

	"../backend/migrations/core/0001_baseline.up.sql: COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. Tenant isolation is FORCE RLS plus a workspace-bound policy per table, NOT ownership — a per-unit ext_<name> owner role exists only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';": "the same applied baseline, and this comment was already corrected by migration 1787905400_the_ext_schema_says_what_bounds_a_unit",

	"../backend/migrations/core/1787905400_the_ext_schema_says_what_bounds_a_unit.down.sql: COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. Tenant isolation is FORCE RLS plus a workspace-bound policy per table, NOT ownership — a per-unit ext_<name> owner role exists only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';": "the DOWN half of the migration that corrected the claim above: restoring the prior text is what a rollback IS",

	"../CHANGELOG.md: `FORCE ROW LEVEL SECURITY` requires, and a runbook": "a record of what shipped in a release that HAD row-level security — correcting its tense would falsify the record",
})

// deniesTheControl spots a sentence that names the control in order to say it
// is NOT there. Those are the corrections this gate exists to produce, and a
// census that failed them would make itself unsatisfiable — three down
// migrations say audit_log carries no policy and that an earlier reading
// wrongly assumed one.
func deniesTheControl(line string) bool {
	lowered := strings.ToLower(line)
	for _, denial := range []string{
		"no rls", "no row-level security", "no row level security",
		"rls=false", "not workspace-scoped", "carries neither",
		"does not exist", "it does not", "assumed force rls",
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

func TestNoProseClaimsRLSStillScopesARead(t *testing.T) {
	t.Parallel()
	// An exemption that stopped matching is an exemption nobody removed. The
	// two shipped migrations are the ones that would go stale, and quietly.
	defer proseExempt.AssertAllMatched(t)

	// The pattern must bite. Each of these stood in the tree when this gate was
	// written, and each is a spelling the Go gate's pattern does not catch.
	for _, line := range []string{
		"Excludes archived teams. Row-scoped by workspace RLS.",
		"Tenant isolation is FORCE RLS plus a workspace-bound policy per table",
		"a migration that forgets `FORCE RLS`, an erasure that misses a PII table",
		"Extension runtime code is bound by row-level security exactly as core code is",
		"RLS binds the workspace, not the reader.",
		"despite RLS and the composite same-workspace foreign keys",
	} {
		if !proseRLSClaim.MatchString(line) {
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
	} {
		if proseRLSClaim.MatchString(line) && !deniesTheControl(line) {
			t.Errorf("the pattern flags a sentence that states the truth:\n\t%s", line)
		}
	}

	var claims []string
	checked := 0
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if proseSkippedDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !proseScanned[strings.ToLower(filepath.Ext(path))] || !d.Type().IsRegular() {
			return nil
		}
		slashed := filepath.ToSlash(path)
		// This file states the banned spellings in order to pin them.
		if slashed == "../backend/"+gateDir+"/rlsclaimsprose_test.go" || strings.HasSuffix(slashed, "/rlsclaims_test.go") {
			return nil
		}
		b, err := os.ReadFile(path) // #nosec G304 G122 -- a text file from walking the trusted source tree
		if err != nil {
			return err
		}
		// A page that opens by saying it describes code no longer in the tree
		// is a record of what WAS true. Correcting its tense would falsify the
		// record; the marker is what keeps it out of a reader's way.
		if historicalRecord(b) {
			return nil
		}
		checked++
		for i, line := range strings.Split(string(b), "\n") {
			if !proseRLSClaim.MatchString(line) || deniesTheControl(line) {
				continue
			}
			// Asked about the OFFENCE and not the file, so an exemption covers
			// the claim it was written for and a new one in the same file is
			// still reported. Keyed on the line's text rather than its number,
			// which moves whenever anything above it does.
			if proseExempt.Waived(t, slashed+": "+strings.TrimSpace(line)) {
				continue
			}
			claims = append(claims, slashed+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}

	// A sweep that read nothing passes exactly like a clean tree. The floor is
	// near the real count on purpose: losing one directory has to fail here.
	if checked < 400 {
		t.Fatalf("read only %d prose files — the walk has lost part of the tree and this gate is no longer a census", checked)
	}
	if len(claims) > 0 {
		t.Errorf("%d line(s) credit row-level security with a control this tree does not carry. "+
			"Core has no policy and no workspace column; say what actually bounds the thing — the "+
			"statement's own predicate, the row-scope clauses in platform/auth, or the single "+
			"installation — and if the honest answer is \"nothing does\", that is a defect to fix "+
			"rather than a sentence to reword. A claim in a SHIPPED migration is corrected by a new "+
			"migration and listed in proseExempt, never by editing the applied one:\n\t%s",
			len(claims), strings.Join(claims, "\n\t"))
	}
}
