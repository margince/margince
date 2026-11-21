// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every writer of a child set audits the set.
//
// Put back reads a contact's emails and phones and a company's domains from the
// audit trail. The latest later entry that names the key holds the value.
// A writer that leaves the key out is invisible, so replaying an earlier entry
// drops its rows.
//
// Writers come from the table-ownership census. A writer is covered when its
// call tree, or that of every caller above it, reaches a function marked
// `//gate:child-set-audit-door`. A function that inserts the contact or company
// row ends a path, because its create entry is the first on the record.
// A path that ends without a door is an offender, fixed or ratified below.
//
// The call graph is by name inside one owning directory. A shared name merges
// two declarations and can only hide an offender. The census floors fail the
// gate when the walk finds fewer writers or doors than the tree holds.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const childSetDoorDirective = "//gate:child-set-audit-door"

// childSetTables are the tables whose live rows a Put back reads from the
// trail instead of from the record's own columns.
var childSetTables = []string{"contact_email", "contact_phone", "company_domain"}

// childSetUnaudited ratifies the entry points that change a child set without
// auditing the key, keyed "owning-dir:function".
const (
	archiveReason = "An archive hides a record's child rows with the record, stamped with the archive's own time, and restore brings back exactly the rows that stamp names. The set after a restore is the set before the archive, so no row is left for a later entry to disagree about."
	erasureReason = "Erasure and retention destroy the child rows with the subject's data and redact the entity's audit history, so no earlier entry is replayed over them afterwards."
	fillReason    = "The audit image of an enrichment fill names the fields it filled and never their values, and its own undo retracts the exact row through a receipt. A later Put back of an earlier emails or phones entry can still drop such a row; recording the set here is a product decision on what an enrichment entry may show."
)

var childSetUnaudited = gatekit.Waive(map[string]string{
	"internal/modules/contacts:ApplySignatureFields":                fillReason,
	"internal/modules/contacts:ApplyStoredProviderClaims":           fillReason,
	"internal/modules/contacts:Archive":                             archiveReason,
	"internal/modules/contacts:ArchiveCompanyTx":                    archiveReason,
	"internal/modules/contacts:ArchiveContactTx":                    archiveReason,
	"internal/modules/contacts:ArchiveSeatContactsTx":               archiveReason,
	"internal/modules/contacts:DemoteLead":                          archiveReason,
	"internal/modules/contacts:FillSignatureTitle":                  fillReason,
	"internal/modules/contacts:RejectCompany":                       archiveReason,
	"internal/modules/contacts:RestoreCompanyTx":                    archiveReason,
	"internal/modules/contacts:RestoreContactProfileField":          fillReason,
	"internal/modules/contacts:RestoreContactTx":                    archiveReason,
	"internal/modules/contacts:RetractCaptureOnlyContactTx":         archiveReason,
	"internal/modules/contacts:RetractFill":                         fillReason,
	"internal/modules/contacts:RetractMisattributedSignatureFields": fillReason,
	"internal/modules/contacts:RetractSignatureFieldsTx":            fillReason,
	"internal/modules/contacts:RetractUnsupportedSignaturePhonesTx": fillReason,
	"internal/modules/privacy:AnonymiseContacts":                    erasureReason,
	"internal/modules/privacy:CreateRetentionPolicy":                erasureReason,
	"internal/modules/privacy:EvaluateInstallation":                 erasureReason,
	"internal/modules/privacy:ReapplySuppressions":                  erasureReason,
	"internal/modules/privacy:UpdateRetentionPolicy":                erasureReason,
})

// mintsRecord matches the statement that creates the record a child set hangs
// off. The word boundary keeps contact_email and company_domain out.
var mintsRecord = regexp.MustCompile(`(?i)INSERT\s+INTO\s+(contact|company)\s*\(`)

type childSetFunc struct {
	door    bool
	mints   bool
	refs    map[string]bool
	callers map[string]bool
}

// childSetGraph reads every non-test file under dir into a name graph of what
// each function or constant declaration references.
func childSetGraph(t *testing.T, dir string) (graph map[string]*childSetFunc, doors []string) {
	t.Helper()
	graph = map[string]*childSetFunc{}
	node := func(name string) *childSetFunc {
		if graph[name] == nil {
			graph[name] = &childSetFunc{refs: map[string]bool{}, callers: map[string]bool{}}
		}
		return graph[name]
	}
	err := gatekit.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || isIntegrationTagged(path) {
			return err
		}
		file, err := gatekit.ParseFile(filepath.ToSlash(path), parser.ParseComments)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			name, body := childSetDeclName(decl)
			if name == "" {
				continue
			}
			n := node(name)
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Doc != nil {
				for _, c := range fn.Doc.List {
					if strings.TrimSpace(c.Text) == childSetDoorDirective {
						n.door = true
						doors = append(doors, name)
					}
				}
			}
			ast.Inspect(body, func(x ast.Node) bool {
				switch v := x.(type) {
				case *ast.BasicLit:
					if text, err := strconv.Unquote(v.Value); v.Kind == token.STRING && err == nil && mintsRecord.MatchString(text) {
						n.mints = true
					}
				case *ast.Ident:
					n.refs[v.Name] = true
				case *ast.SelectorExpr:
					n.refs[v.Sel.Name] = true
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, fn := range graph {
		for ref := range fn.refs {
			if target := graph[ref]; target != nil && ref != name {
				target.callers[name] = true
			}
		}
	}
	return graph, doors
}

// childSetDeclName names a declaration the graph tracks: a function, a method
// by its bare name, or a package-level value (SQL held in a constant).
func childSetDeclName(decl ast.Decl) (string, ast.Node) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Body == nil {
			return "", nil
		}
		return d.Name.Name, d.Body
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok && len(vs.Names) > 0 && len(vs.Values) > 0 {
				return vs.Names[0].Name, vs
			}
		}
	}
	return "", nil
}

// uncoveredEntries are the functions at the top of a call path that never
// reached a door, starting from one writer.
func uncoveredEntries(graph map[string]*childSetFunc, writer string, seen map[string]bool) []string {
	fn := graph[writer]
	if fn == nil || fn.door || fn.mints || seen[writer] {
		return nil
	}
	if reachesDoor(writer, graph, map[string]bool{}) {
		return nil
	}
	seen[writer] = true
	if len(fn.callers) == 0 {
		return []string{writer}
	}
	var out []string
	for caller := range fn.callers {
		out = append(out, uncoveredEntries(graph, caller, seen)...)
	}
	return out
}

// reachesDoor reports whether the function or anything it references is a
// door. An operation that writes a set and audits it in one call tree is covered.
func reachesDoor(name string, graph map[string]*childSetFunc, seen map[string]bool) bool {
	fn := graph[name]
	if fn == nil || seen[name] {
		return false
	}
	if fn.door {
		return true
	}
	seen[name] = true
	for ref := range fn.refs {
		if reachesDoor(ref, graph, seen) {
			return true
		}
	}
	return false
}

func TestEveryWriterOfAChildSetAuditsItsKey(t *testing.T) {
	t.Parallel()
	writes := collectTableWrites(t)
	wanted := map[string]bool{}
	for _, table := range childSetTables {
		wanted[table] = false
	}
	graphs := map[string]map[string]*childSetFunc{}
	doorCount := 0
	offenders := map[string][]string{} // owner:entry → write positions
	for owner, list := range writes {
		for _, w := range list {
			if _, ok := wanted[w.table]; !ok {
				continue
			}
			wanted[w.table] = true
			if graphs[owner] == nil {
				var doors []string
				graphs[owner], doors = childSetGraph(t, owner)
				doorCount += len(doors)
			}
			_, writer, _ := strings.Cut(w.site, ":")
			writer = writer[strings.LastIndex(writer, ".")+1:]
			for _, entry := range uncoveredEntries(graphs[owner], writer, map[string]bool{}) {
				key := owner + ":" + entry
				offenders[key] = append(offenders[key], w.pos+" "+w.verb+" "+w.table)
			}
		}
	}
	for table, found := range wanted {
		if !found {
			t.Errorf("the census found no write to %s: the walk no longer sees its writers", table)
		}
	}
	if doorCount < 3 {
		t.Errorf("found %d child-set audit doors, want the three that write the emails, phones and domains keys", doorCount)
	}
	keys := make([]string, 0, len(offenders))
	for key := range offenders {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if childSetUnaudited.Waived(t, key) {
			continue
		}
		pos := offenders[key]
		slices.Sort(pos)
		t.Errorf("%s changes a contact's emails or phones or a company's domains and no caller path reaches a "+
			"child-set audit door, so Put back cannot see the change (first write: %s). Record the set through "+
			"recordEmails, recordPhones or recordDomains in the audit image it writes", key, pos[0])
	}
	childSetUnaudited.AssertAllMatched(t)
}
