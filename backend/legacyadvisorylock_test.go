// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// ADR-0091 §5 took the workspace out of this tree's advisory-lock identities.
// A lock identity is not a private detail, though — it is a rendezvous between
// PROCESSES, and a rolling deploy runs two builds at once. So every site that
// takes a bare key also takes the workspace-qualified key the previous release
// took, and the two builds keep meeting on it until #2528 removes the legacy
// half.
//
// That only works while each legacy statement hashes to what the previous
// release hashed. Nothing else in the tree would notice if it stopped: a
// reformat inside the SQL literal, a reordered concatenation, or a swap of
// hashtext for hashtextextended leaves both builds taking two locks each and
// simply not meeting on one of them. Every test still passes — including the
// contention suite, which observes one build. The failure surfaces as the race
// the lock exists to prevent, in production, during a deploy.
//
// So the statements are frozen here, and the corpus is derived from the tree
// rather than listed: a new dual-locking site has to enroll its legacy key, and
// a site that quietly drops one fails rather than going unnoticed.

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// legacyAdvisoryLockKeys is every workspace-qualified advisory-lock statement
// the previous release took, normalized to single-spaced text. Frozen: these
// are not ours to edit while an older build may still be running.
//
// Each was checked against its origin/main original at the time §5 landed. They
// differ from it in one way only — a coalesce around current_setting — which
// changes the key exclusively when the GUC is UNSET, and there the previous
// release passed NULL to a STRICT function and took no lock at all. Whenever
// the GUC is set, and that is the only case in which two builds have to meet,
// the text below computes what the previous release computed.
//
// When #2528 removes the legacy half, this whole file goes with it.
//
// Held by: TestLegacyAdvisoryLockKeysStillMatchThePreviousRelease
// (backend/legacyadvisorylock_test.go) — the claim that this is EVERY such
// statement is the gate's own subject, and it is derived from the tree rather
// than trusted, so a site added or dropped fails here.
var legacyAdvisoryLockKeys = []string{
	`pg_advisory_xact_lock( hashtext($1 || coalesce(current_setting('app.workspace_id', true), ''))::bigint)`,
	`pg_advisory_xact_lock(hashtext( 'margince:extsecrets:' || coalesce(current_setting('app.workspace_id', true), '') || ':' || $1 || ':' || $2 || ':' || $3 || ':' || $4)::bigint)`,
	`pg_advisory_xact_lock(hashtext('margince:admin-guard:' || coalesce(current_setting('app.workspace_id', true), ''))::bigint)`,
	`pg_advisory_xact_lock(hashtext('margince:capture-deferrals:' || coalesce(current_setting('app.workspace_id', true), ''))::bigint)`,
	`pg_advisory_xact_lock(hashtext('margince:consumer-mail:' || coalesce(current_setting('app.workspace_id', true), '') || ':' || $1)::bigint)`,
	`pg_advisory_xact_lock(hashtext('margince:overlay-visibility:' || coalesce(current_setting('app.workspace_id', true), ''))::bigint)`,
	`pg_advisory_xact_lock(hashtextextended( $1 || ':' || coalesce(current_setting('app.workspace_id', true), '') || ':' || $2, 0))`,
	`pg_advisory_xact_lock(hashtextextended( coalesce(current_setting('app.workspace_id', true), '') || ':' || $1, 0))`,
}

func TestLegacyAdvisoryLockKeysStillMatchThePreviousRelease(t *testing.T) {
	var found []string
	// The same hand-written Go corpus the licence notice covers, for the same
	// reason: extensions/ and fixtures/ are separate modules that a `./...`
	// from here never reaches, and a lock taken in one is as much a rendezvous
	// as a lock taken in the backend. Sharing the list keeps a new tree from
	// being enrolled in one sweep and forgotten by the other.
	for _, tree := range licensedTrees {
		err := filepath.WalkDir(tree.root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "node_modules" {
				return fs.SkipDir
			}
			// Test files are excluded because a test may legitimately spell a
			// key out to assert which locks a transaction holds — that is a
			// deliberate mirror of production, and freezing it here would make
			// this gate assert against itself.
			if d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || !d.Type().IsRegular() {
				return nil
			}
			b, err := os.ReadFile(path) // #nosec G304 G122 -- path is a *.go file from walking the trusted source tree
			if err != nil {
				return err
			}
			for _, stmt := range advisoryLockStatements(string(b)) {
				if strings.Contains(stmt, "app.workspace_id") {
					found = append(found, stmt)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", tree.root, err)
		}
	}
	sort.Strings(found)

	frozen := append([]string(nil), legacyAdvisoryLockKeys...)
	sort.Strings(frozen)
	if len(found) != len(frozen) {
		t.Fatalf("the tree holds %d workspace-qualified advisory-lock statement(s), the frozen set has %d.\n"+
			"A new dual-locking site must add its legacy key to legacyAdvisoryLockKeys; a site that dropped one "+
			"has left an old build with nothing to contend against.\nfound:\n\t%s\nfrozen:\n\t%s",
			len(found), len(frozen), strings.Join(found, "\n\t"), strings.Join(frozen, "\n\t"))
	}
	for i := range found {
		if found[i] != frozen[i] {
			t.Errorf("a legacy advisory-lock key changed. It is a rendezvous with the binaries of the previous "+
				"release, so it must stay byte-identical until #2528 removes it — revert the edit rather than "+
				"updating this expectation.\n  now:    %s\n  frozen: %s", found[i], frozen[i])
		}
	}
}

// advisoryLockStatements returns every pg_advisory_xact_lock(…) call in one Go
// source file, normalized to single-spaced text so that reindenting a statement
// is not reported as changing its key.
//
// It matches the CALL and balances its parentheses rather than reading lines:
// these statements wrap across three and four lines, and a line-wise scan would
// see fragments of the key it is meant to be pinning.
func advisoryLockStatements(src string) []string {
	const open = "pg_advisory_xact_lock("
	var out []string
	for i := 0; ; {
		j := strings.Index(src[i:], open)
		if j < 0 {
			return out
		}
		start := i + j
		depth := 0
		end := -1
		for k := start + len(open) - 1; k < len(src); k++ {
			switch src[k] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = k
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			// An unbalanced call means the scan lost the shape of the file, and
			// reporting what it found so far would under-count silently.
			return append(out, "UNPARSEABLE pg_advisory_xact_lock call")
		}
		out = append(out, strings.Join(strings.Fields(src[start:end+1]), " "))
		i = end + 1
	}
}
