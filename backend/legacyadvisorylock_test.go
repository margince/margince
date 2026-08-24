// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind prohibition H1

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
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// legacyAdvisoryLockKeys is every workspace-qualified advisory-lock statement
// the previous release took, normalized to single-spaced text. Frozen: these
// are not ours to edit while an older build may still be running.
//
// Each is that release's statement BYTE FOR BYTE, and the inconsistencies
// between them are deliberately preserved. Under an UNSET GUC the eight behave
// three different ways — take no lock, raise, or lock an empty-suffix key — and
// each entry below says which it is rather than the count being tallied here,
// because a tally in prose is one edit away from disagreeing with the list it
// describes. Tidying any of that would change the key, or change when the
// statement fails, and the only thing this text has to do is hash to what the
// old build hashes. What serializes the CURRENT build is the bare key each site
// takes first.
//
// When #2528 removes the legacy half, this whole file goes with it.
//
// Held by: TestLegacyAdvisoryLockKeysStillMatchThePreviousRelease
// (backend/legacyadvisorylock_test.go) — the claim that this is EVERY such
// statement is the gate's own subject, and it is derived from the tree rather
// than trusted, so a site added or dropped fails here.
var legacyAdvisoryLockKeys = []string{
	// compose/bootscope.go — coalesce: locks the empty-suffix key
	`pg_advisory_xact_lock( hashtext($1 || coalesce(current_setting('app.workspace_id', true), ''))::bigint)`,
	// platform/extsecrets/scope.go — missing_ok, no coalesce: takes NO lock
	`pg_advisory_xact_lock(hashtext( 'margince:extsecrets:' || current_setting('app.workspace_id', true) || ':' || $1 || ':' || $2 || ':' || $3 || ':' || $4)::bigint)`,
	// modules/identity/lastadmin.go — missing_ok, no coalesce: takes NO lock
	`pg_advisory_xact_lock(hashtext('margince:admin-guard:' || current_setting('app.workspace_id', true))::bigint)`,
	// modules/capture/pendingcap.go — missing_ok, no coalesce: takes NO lock
	`pg_advisory_xact_lock(hashtext('margince:capture-deferrals:' || current_setting('app.workspace_id', true))::bigint)`,
	// modules/capture/freemaildomain.go — missing_ok, no coalesce: takes NO lock
	`pg_advisory_xact_lock(hashtext('margince:consumer-mail:' || current_setting('app.workspace_id', true) || ':' || $1)::bigint)`,
	// modules/overlay/visibility.go — no missing_ok: RAISES
	`pg_advisory_xact_lock(hashtext('margince:overlay-visibility:' || current_setting('app.workspace_id'))::bigint)`,
	// storekit.LockWriteIdentity — coalesce: locks the empty-suffix key
	`pg_advisory_xact_lock(hashtextextended( $1 || ':' || coalesce(current_setting('app.workspace_id', true), '') || ':' || $2, 0))`,
	// storekit/suppression.go — coalesce: locks the empty-suffix key
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
			stmts, err := advisoryLockStatements(path)
			if err != nil {
				return err
			}
			for _, stmt := range stmts {
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

// advisoryLockStatements returns every pg_advisory_xact_lock(…) call written
// in one Go source file's STRING LITERALS, normalized to single-spaced text so
// that reindenting a statement is not reported as changing its key.
//
// It parses rather than scanning the raw file, because a comment is allowed to
// discuss these statements and several of them do — including the doc this gate
// is named in. A text scan cannot tell a comment's mention from a real
// statement, so one prose example would either be counted as a ninth key or,
// with an unbalanced parenthesis, swallow the real statement that follows it.
// Either way the census reports a number that is not the tree's, which is the
// one failure mode a gate must not have.
//
// Within a literal it matches the CALL and balances its parentheses rather than
// reading lines: these statements wrap across three and four lines, and a
// line-wise scan would see fragments of the key it is meant to be pinning.
func advisoryLockStatements(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		out = append(out, advisoryLockCalls(text)...)
		return true
	})
	return out, nil
}

// advisoryLockCalls pulls the balanced pg_advisory_xact_lock(…) calls out of
// one string literal's text.
func advisoryLockCalls(src string) []string {
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
			// An unbalanced call means the scan lost the shape of the literal,
			// and reporting what it found so far would under-count silently.
			return append(out, "UNPARSEABLE pg_advisory_xact_lock call")
		}
		out = append(out, strings.Join(strings.Fields(src[start:end+1]), " "))
		i = end + 1
	}
}
