// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Leads and contacts share the LinkedIn profile as an identity key. So every
// value written to lead.linkedin_url or contact_social passes the one normaliser.
// The corpus is each production statement or patch that writes those columns.
// Its function reaches the normaliser through its callees, or all its callers
// do. A patch copying another row's stored value passes; it was normalised once.

import (
	"go/ast"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// writesLinkedIn matches a statement that writes a LinkedIn value: any write of
// a contact_social handle, and a lead write naming linkedin_url.
var writesLinkedIn = map[string]*regexp.Regexp{
	"contact_social insert": regexp.MustCompile(`(?is)INSERT\s+INTO\s+contact_social[\s(]`),
	"contact_social update": regexp.MustCompile(`(?is)UPDATE\s+contact_social\s+SET\b[^;]*\bhandle\b`),
	"lead insert":           regexp.MustCompile(`(?is)INSERT\s+INTO\s+lead\s*\([^)]*\blinkedin_url\b`),
	"lead update":           regexp.MustCompile(`(?is)UPDATE\s+lead\s+SET\b[^;]*\blinkedin_url\b`),
}

// linkedInNormalizers are the functions whose output a writer may store. The
// company page normaliser counts only for a patch, whose column name a company
// shares with a lead.
const (
	profileNormalizer     = "NormalizeContactLinkedInURL"
	companyPageNormalizer = "NormalizeCompanyLinkedInURL"
)

const linkedInColumn = "linkedin_url"

// linkedInWriter is one function holding a LinkedIn write.
type linkedInWriter struct {
	file, kind string
	fn         *ast.FuncDecl
}

func TestEveryLinkedInWriteGoesThroughTheOneNormalizer(t *testing.T) {
	t.Parallel()
	packages := map[string][]*ast.FuncDecl{}
	fileOf := map[*ast.FuncDecl]string{}
	constants := map[string]map[string]string{}
	var writers []linkedInWriter
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || strings.HasPrefix(where, "internal/contracts/") ||
			isIntegrationTagged(path) || !strings.HasPrefix(where, "internal/") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		dir := filepath.Dir(where)
		if constants[dir] == nil {
			constants[dir] = map[string]string{}
		}
		collectStringConstants(file, constants[dir])
		for _, decl := range file.Decls {
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Body != nil {
				packages[dir] = append(packages[dir], fn)
				fileOf[fn] = where
			}
		}
	}
	for dir, funcs := range packages {
		for _, fn := range funcs {
			writers = append(writers, linkedInWritesOf(fileOf[fn], fn, constants[dir])...)
		}
	}

	// A kind with no writer found means the census read too little.
	seen := map[string]bool{}
	for _, w := range writers {
		seen[w.kind] = true
	}
	for _, kind := range []string{"contact_social insert", "lead insert", "patch"} {
		if !seen[kind] {
			t.Fatalf("found no production %s of a LinkedIn value: the pattern has stopped matching", kind)
		}
	}

	sort.Slice(writers, func(i, j int) bool {
		return writers[i].file+writers[i].fn.Name.Name < writers[j].file+writers[j].fn.Name.Name
	})
	for _, w := range writers {
		allowed := map[string]bool{profileNormalizer: true}
		if w.kind == "patch" {
			allowed[companyPageNormalizer] = true
		}
		reach := normalizerReach{funcs: packages[filepath.Dir(w.file)], allowed: allowed}
		if !reach.guarded(w.fn, map[*ast.FuncDecl]bool{}) {
			t.Errorf("%s: %s writes a LinkedIn value (%s) and no path to it calls contacts.%s.\n"+
				"Normalise the value before the write, so a lead and a contact holding one profile "+
				"hold it in one spelling and the identity check finds both.", w.file, w.fn.Name.Name, w.kind, profileNormalizer)
		}
	}
}

// linkedInWritesOf lists the LinkedIn writes one function holds. Those are its
// SQL statements and its patch sets of linkedin_url, less copies of a stored row.
func linkedInWritesOf(where string, fn *ast.FuncDecl, constants map[string]string) []linkedInWriter {
	var out []linkedInWriter
	for _, statement := range gatekit.SQLStatementsOf(fn) {
		for kind, pattern := range writesLinkedIn {
			if pattern.MatchString(statement) && (!strings.HasSuffix(kind, "update") || !clearsOnly(statement)) {
				out = append(out, linkedInWriter{file: where, kind: kind, fn: fn})
			}
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		column, value := patchColumnAndValue(call)
		if column == nil || copiesStoredLinkedIn(value) {
			return true
		}
		if name, known := resolveString(column, constants); !known || name != linkedInColumn {
			return true
		}
		out = append(out, linkedInWriter{file: where, kind: "patch", fn: fn})
		return true
	})
	return out
}

// linkedInAssignment is one SET of a LinkedIn column in an UPDATE.
var linkedInAssignment = regexp.MustCompile(`(?i)\b(?:linkedin_url|handle)\s*=\s*([^,\s]+)`)

// clearsOnly reports an UPDATE whose every LinkedIn assignment is NULL, as an
// anonymisation writes: it stores no profile, so it has nothing to normalise.
func clearsOnly(statement string) bool {
	assignments := linkedInAssignment.FindAllStringSubmatch(statement, -1)
	for _, assignment := range assignments {
		if !strings.EqualFold(assignment[1], "NULL") {
			return false
		}
	}
	return len(assignments) > 0
}

// patchColumnAndValue answers the column and new value of a storekit.Patch Set
// (p.Set(column, old, new)) or of the merge's fillString(p, column, target, source).
func patchColumnAndValue(call *ast.CallExpr) (ast.Expr, ast.Expr) {
	switch calleeName(call) {
	case "Set":
		if len(call.Args) == 3 {
			return call.Args[0], call.Args[2]
		}
	case "fillString":
		if len(call.Args) == 4 {
			return call.Args[1], call.Args[3]
		}
	}
	return nil, nil
}

// copiesStoredLinkedIn reports a value read off another row's linkedin_url,
// such as a merge copying the source record's profile onto the survivor.
func copiesStoredLinkedIn(value ast.Expr) bool {
	selector, ok := value.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "LinkedinUrl"
}

// normalizerReach answers, within one package, whether a function reaches the
// normaliser through what it calls, or is only ever called from functions that
// do.
type normalizerReach struct {
	funcs   []*ast.FuncDecl
	allowed map[string]bool
}

func (r normalizerReach) guarded(fn *ast.FuncDecl, visiting map[*ast.FuncDecl]bool) bool {
	if r.reaches(fn, map[*ast.FuncDecl]bool{}) {
		return true
	}
	if fn.Name.IsExported() || visiting[fn] {
		return false
	}
	visiting[fn] = true
	defer delete(visiting, fn)
	callers := 0
	for _, caller := range r.funcs {
		if caller == fn || !callsName(caller, fn.Name.Name) {
			continue
		}
		callers++
		if !r.guarded(caller, visiting) {
			return false
		}
	}
	return callers > 0
}

// reaches reports whether fn calls a normaliser, itself or through a function
// of its package. Calls match by name, so a shared method name can only make a
// writer pass that calls the normaliser through either method.
func (r normalizerReach) reaches(fn *ast.FuncDecl, visited map[*ast.FuncDecl]bool) bool {
	if visited[fn] {
		return false
	}
	visited[fn] = true
	callees := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall {
			callees[calleeName(call)] = true
		}
		return true
	})
	for name := range r.allowed {
		if callees[name] {
			return true
		}
	}
	for _, callee := range r.funcs {
		if callees[callee.Name.Name] && r.reaches(callee, visited) {
			return true
		}
	}
	return false
}
