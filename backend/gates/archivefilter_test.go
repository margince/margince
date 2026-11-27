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
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// archiveFilteredCall is where a storekit row writer takes its table and,
// when the caller chooses one, its archive filter. A filter position of -1
// means the call writes through LiveOnly.
type archiveFilteredCall struct{ table, filter int }

const storekitDir = "internal/platform/database/storekit"

// storekitWriterFloor is the vacuity floor on the derived writers: storekit
// had nine when this gate was written.
const storekitWriterFloor = 8

// storekitArchiveFilteredCalls derives the writers from storekit itself: each
// exported function taking `table string` and either an ArchivedFilter or a
// body that names LiveOnly.
func storekitArchiveFilteredCalls(t *testing.T) map[string]archiveFilteredCall {
	t.Helper()
	entries, err := gatekit.ReadDir(storekitDir)
	if err != nil {
		t.Fatalf("reading %s: %v", storekitDir, err)
	}
	calls := map[string]archiveFilteredCall{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(storekitDir, entry.Name()), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Body == nil {
				continue
			}
			if at, ok := archiveFilterPositions(fn); ok {
				calls[fn.Name.Name] = at
			}
		}
	}
	if len(calls) < storekitWriterFloor {
		t.Fatalf("derived %d storekit row writers, under the floor of %d; the reader has stopped "+
			"matching storekit's signatures", len(calls), storekitWriterFloor)
	}
	return calls
}

// archiveFilterPositions reads one storekit function's table and filter
// argument positions, and reports whether it is a row writer at all.
func archiveFilterPositions(fn *ast.FuncDecl) (archiveFilteredCall, bool) {
	at := archiveFilteredCall{table: -1, filter: -1}
	position := 0
	for _, field := range fn.Type.Params.List {
		typeName := ""
		if ident, ok := field.Type.(*ast.Ident); ok {
			typeName = ident.Name
		}
		for _, name := range field.Names {
			switch {
			case name.Name == "table" && typeName == "string":
				at.table = position
			case typeName == "ArchivedFilter":
				at.filter = position
			}
			position++
		}
	}
	if at.table < 0 {
		return at, false
	}
	if at.filter >= 0 {
		return at, true
	}
	namesLiveOnly := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == "LiveOnly" {
			namesLiveOnly = true
		}
		return !namesLiveOnly
	})
	return at, namesLiveOnly
}

// archiveFilterCallFloor is the vacuity floor: the tree held 297 such
// calls on a catalog table when this gate was written.
const archiveFilterCallFloor = 200

// archiveFilterUse is one call the gate judges: the table it names and the
// filter it resolves rows through.
type archiveFilterUse struct {
	at, call, table, filter string
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

// archiveFilterUses reads every storekit row writer in one Go file. A table or
// filter the reader cannot resolve is left empty, for the caller to report.
func archiveFilterUses(
	t *testing.T, path string, calls map[string]archiveFilteredCall, packageConstants map[string]map[string]string,
) []archiveFilterUse {
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
		name := calledName(call)
		at, ok := calls[name]
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
			filter = ""
			if selector, ok := call.Args[at.filter].(*ast.SelectorExpr); ok {
				filter = selector.Sel.Name
			}
		}
		line := gatekit.SourceFileSet().Position(call.Pos()).Line
		uses = append(uses, archiveFilterUse{at: path + ":" + strconv.Itoa(line), call: name, table: table, filter: filter})
		return true
	})
	return uses
}

// archiveFilterForwarders names each call, as file:writer, that hands storekit
// a table or filter its own caller chose, with why that caller is judged instead.
var archiveFilterForwarders = gatekit.Waive(map[string]string{
	"internal/compose/csvreviewguard.go:EnsureIfMatch": "lockReviewedImportRow answers company, contact, lead, " +
		"deal or activity, and each carries archived_at.",
	"internal/modules/activities/lifecycle.go:ApplyGuardedIn": "activityArchivedFilter answers LiveOnly or " +
		"IncludeArchived, and activity carries archived_at.",
	"internal/modules/contacts/bulkchange.go:LockRowReading": "holdForBulk is handed contact, company or lead, " +
		"and each carries archived_at.",
	"internal/modules/contacts/claim.go:ClaimOwnership": "claimableTables admits contact, company and lead, " +
		"and each carries archived_at.",
	"internal/modules/contacts/leadcontactlock.go:LockRow": "the table is contact, and each caller passes " +
		"LiveOnly or IncludeArchived, which contact answers.",
	"internal/modules/contacts/mergesurvivorshippreview.go:EnsureIfMatch": "the preview admits a contact or " +
		"company kind only, and both carry archived_at.",
	"internal/modules/contracts/contract_write.go:ApplyGuardedIn": "the table is contract, and every caller passes " +
		"LiveOnly or IncludeArchived, which contract answers.",
})

func TestAStorekitRowWriteNamesAnArchiveFilterItsTableAnswers(t *testing.T) {
	t.Parallel()
	defer archiveFilterForwarders.AssertAllMatched(t)
	schema := parseHeadCatalog(t)
	archived := schema.tablesWithColumn("archived_at", "")
	calls := storekitArchiveFilteredCalls(t)
	packageConstants := map[string]map[string]string{}
	judged := 0
	for _, root := range []string{"internal", "pkg", "../extensions"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || path == storekitDir {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
				return nil
			}
			for _, use := range archiveFilterUses(t, path, calls, packageConstants) {
				if use.table == "" || use.filter == "" {
					if !archiveFilterForwarders.Waived(t, use.at[:strings.LastIndex(use.at, ":")]+":"+use.call) {
						t.Errorf("%s: %s names its table or archive filter through a value this gate cannot read; "+
							"pass a literal, a package constant and a storekit filter, or waive the call in archiveFilterForwarders",
							use.at, use.call)
					}
					continue
				}
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
	cases := []struct {
		filter               string
		hasArchive, mismatch bool
	}{
		{"LiveOnly", false, true}, {"IncludeArchived", false, true}, {"NoArchiveColumn", false, false},
		{"LiveOnly", true, false}, {"IncludeArchived", true, false}, {"NoArchiveColumn", true, true},
	}
	for _, c := range cases {
		if got := archiveFilterMismatch(c.filter, c.hasArchive) != ""; got != c.mismatch {
			t.Errorf("%s with archived_at=%v: mismatch %v, want %v", c.filter, c.hasArchive, got, c.mismatch)
		}
	}
}
