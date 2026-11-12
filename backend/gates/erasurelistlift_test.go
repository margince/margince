// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every statement on the erasure list names lifted_at.
//
// A lifted row stays on erasure_suppression, so a reader that forgets the
// column treats a person who asked to be stored again as erased, and a writer
// that forgets it keeps a lift through a second erasure. Naming the column is
// the check a pattern can make: a reader filters on it or carries it, and a
// writer clears or carries it. The corpus is every hand-written Go file.

import (
	"go/parser"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// erasureListStatement matches SQL that reads or writes the erasure list as a
// relation. A literal naming it as a string value, such as an
// information_schema lookup, is not a statement on its rows.
var erasureListStatement = regexp.MustCompile(`(?i)\b(FROM|JOIN|INTO|UPDATE)\s+erasure_suppression(\s|$|[,;()])`)

var namesLift = regexp.MustCompile(`(?i)\blifted_at\b`)

// erasureListLiftBlind holds the statements that rightly see every row, each
// by file, function and first line.
var erasureListLiftBlind = gatekit.Waive(map[string]string{
	"internal/platform/database/storekit/identifierkey.go:RefuseForeignSuppressionKey:SELECT …":                                   "a lifted row is still hashed under its key and must be re-keyed with the rest, or a restore could not find it",
	"internal/modules/privacy/identifierrekey.go:CountUnkeyedSuppressions:SELECT 1 FROM erasure_suppression WHERE key_id IS NULL": "counts the rows the re-key has left, lifted ones included, because the re-key carries every row",
	"internal/modules/privacy/identifierrekey.go:RekeySuppressionListPageTx:DELETE FROM erasure_suppression s …":                  "drops the unkeyed twin only after the INSERT above wrote the keyed row with the same lift",
})

func TestEveryErasureListStatementNamesTheLift(t *testing.T) {
	t.Parallel()
	defer erasureListLiftBlind.AssertAllMatched(t)
	statements := 0
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || strings.HasPrefix(where, "gates/") || isIntegrationTagged(path) {
			continue
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		for _, decl := range file.Decls {
			for _, read := range gatekit.DeclReads(decl, erasureListStatement) {
				statements++
				if namesLift.MatchString(read.SQL) {
					continue
				}
				site := where + ":" + read.Function + ":" + gatekit.FirstLineOf(read.SQL)
				if !erasureListLiftBlind.Waived(t, site) {
					t.Errorf("%s works on erasure_suppression without naming lifted_at. A reader must "+
						"filter on lifted_at IS NULL or carry the column; a writer must clear or carry it", site)
				}
			}
		}
	}
	// The storekit probes, the three readers in privacy and the two writers
	// make six; fewer means the pattern stopped seeing the tree.
	if statements < 6 {
		t.Fatalf("found %d statements on erasure_suppression; the pattern no longer matches this tree's SQL", statements)
	}
}

func TestTheErasureListCensusSeesEachKindOfStatement(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{
		"SELECT 1 FROM erasure_suppression WHERE kind = 'email'",
		"SELECT s.value_hash FROM contact c JOIN erasure_suppression s ON true",
		"INSERT INTO erasure_suppression (kind, value_hash) VALUES ($1, $2)",
		"UPDATE erasure_suppression SET key_id = $1",
		"DELETE FROM erasure_suppression\nWHERE kind = $1",
	} {
		if !erasureListStatement.MatchString(sql) || namesLift.MatchString(sql) {
			t.Errorf("the census would not report %q", sql)
		}
	}
	if erasureListStatement.MatchString("WHERE table_name = 'erasure_suppression'") {
		t.Error("the census counts a catalog lookup as a statement on the list")
	}
}
