// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A storekit row write names the archive filter its table can answer.
//
// LiveOnly renders `archived_at IS NULL`, so on a table without that column the
// lock fails at run time with an undefined column. IncludeArchived on such a
// table claims to reach rows that cannot exist. NoArchiveColumn on a table that
// has the column lets an archived row be written without saying so.

import (
	"go/ast"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// archiveFilteredCall is where a storekit row writer takes its table and,
// when the caller chooses one, its archive filter. A filter position of -1
// means the call always resolves live rows.
type archiveFilteredCall struct{ table, filter int }

var archiveFilteredCalls = map[string]archiveFilteredCall{
	"LockRow": {2, 4}, "ApplyGuardedIn": {2, 5}, "EnsureIfMatchIn": {2, 5},
	"ApplyGuarded": {2, -1}, "ApplyWithVersion": {2, -1}, "EnsureIfMatch": {2, -1},
	"ClaimOwnership": {2, -1}, "LockPair": {2, -1},
}

// archiveFilterCallFloor is the vacuity floor: the tree held 296 such
// calls on a catalog table when this gate was written.
const archiveFilterCallFloor = 200

// archiveFilterUse is one call the gate judges: the table it names and the
// filter it resolves rows through.
type archiveFilterUse struct {
	at, table, filter string
}

// archiveFilterMismatch says what is wrong with a filter on a table, or "".
func archiveFilterMismatch(filter string, hasArchive bool) string {
	switch {
	case !hasArchive && filter == "LiveOnly":
		return "resolves live rows through archived_at, which the table does not have; pass storekit.NoArchiveColumn"
	case !hasArchive && filter == "IncludeArchived":
		return "declares it reaches archived rows of a table that has no archived_at; pass storekit.NoArchiveColumn"
	case hasArchive && filter == "NoArchiveColumn":
		return "says the table has no archived_at, but it has; pass storekit.LiveOnly or storekit.IncludeArchived"
	}
	return ""
}

// archiveFilterUses reads every storekit row writer in one Go file whose table
// is a literal or a package constant.
func archiveFilterUses(t *testing.T, path string, packageConstants map[string]map[string]string) []archiveFilterUse {
	t.Helper()
	file, err := gatekit.ParseFile(path, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	constants := packageStringConstants(t, filepath.Dir(path), packageConstants)
	var uses []archiveFilterUse
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		at, ok := archiveFilteredCalls[calledName(call)]
		if !ok || len(call.Args) <= max(at.table, at.filter) {
			return true
		}
		table := ""
		switch arg := call.Args[at.table].(type) {
		case *ast.BasicLit:
			table, _ = strconv.Unquote(arg.Value)
		case *ast.Ident:
			table = constants[arg.Name]
		}
		filter := "LiveOnly"
		if at.filter >= 0 {
			selector, ok := call.Args[at.filter].(*ast.SelectorExpr)
			if !ok {
				return true
			}
			filter = selector.Sel.Name
		}
		if table != "" {
			uses = append(uses, archiveFilterUse{at: path, table: table, filter: filter})
		}
		return true
	})
	return uses
}

func TestAStorekitRowWriteNamesAnArchiveFilterItsTableAnswers(t *testing.T) {
	t.Parallel()
	schema := parseHeadCatalog(t)
	archived := schema.tablesWithColumn("archived_at", "")
	packageConstants := map[string]map[string]string{}
	judged := 0
	for _, root := range []string{"internal", "pkg", "../extensions"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
				return nil
			}
			for _, use := range archiveFilterUses(t, path, packageConstants) {
				if _, known := schema.tables[use.table]; !known {
					continue
				}
				judged++
				if why := archiveFilterMismatch(use.filter, archived[use.table]); why != "" {
					t.Errorf("%s: a %s write of %s %s", use.at, use.filter, use.table, why)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if judged < archiveFilterCallFloor {
		t.Fatalf("judged %d storekit row writes on catalog tables, under the floor of %d; "+
			"the reader has stopped matching the calls", judged, archiveFilterCallFloor)
	}
}

func TestTheArchiveFilterRuleFiresOnEveryMismatch(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"LiveOnly/false": true, "IncludeArchived/false": true, "NoArchiveColumn/false": false,
		"LiveOnly/true": false, "IncludeArchived/true": false, "NoArchiveColumn/true": true,
	}
	for _, key := range slices.Sorted(maps.Keys(cases)) {
		filter, column, _ := strings.Cut(key, "/")
		if got := archiveFilterMismatch(filter, column == "true") != ""; got != cases[key] {
			t.Errorf("%s with archived_at=%s: mismatch %v, want %v", filter, column, got, cases[key])
		}
	}
}
