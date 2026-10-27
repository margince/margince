// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Nobody is a lead and a contact at once. Every production writer of an
// identity key asks who already holds it. The function must call
// storekit.LockIdentityHolders, or every caller in its package must. A writer
// that reaches the question another way states that way here.

import (
	"go/ast"
	"maps"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// insertsIdentityKey matches an INSERT that gives a lead or a contact an
// identity key. The trailing [\s(] keeps lead_* and contact_email_* child
// tables out.
var insertsIdentityKey = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(contact_email|lead)[\s(]`)

// updatesIdentityTable finds an UPDATE of a table that holds identity keys, and
// setsIdentityKey a SET clause that gives a key or revives one. A key set to
// NULL is an erasure, which gives nothing.
var (
	updatesIdentityTable = regexp.MustCompile(`(?is)UPDATE\s+(lead|contact_email)\s+(\w+\s+)?SET\s`)
	setsIdentityKey      = regexp.MustCompile(`(?is)\b(email|linkedin_url)\s*=\s*(\$|lower|btrim|coalesce|'|\w+\.)|\barchived_at\s*=\s*NULL\b`)
	identityWhere        = regexp.MustCompile(`(?is)\bWHERE\b`)
)

// identityKeyStatement answers how a statement writes an identity key, if it does.
func identityKeyStatement(statement string) (string, bool) {
	if m := insertsIdentityKey.FindString(statement); m != "" {
		return "writes " + strings.Join(strings.Fields(m), " "), true
	}
	loc := updatesIdentityTable.FindStringIndex(statement)
	if loc == nil {
		return "", false
	}
	set := statement[loc[1]:]
	if where := identityWhere.FindStringIndex(set); where != nil {
		set = set[:where[0]]
	}
	if key := setsIdentityKey.FindString(set); key != "" {
		return "writes " + strings.Join(strings.Fields(statement[loc[0]:loc[1]]), " ") + " " + strings.TrimSpace(key), true
	}
	return "", false
}

// identityKeyFields are the lead columns a storekit patch may set by name.
var identityKeyFields = map[string]bool{"leadEmailColumn": true, "linkedinURLField": true}

// identityProbe is the one question the census asks a writer to reach.
const identityProbe = "LockIdentityHolders"

// identityReachVerdicts are the writers that reach the question another way.
// Keyed file:function.
var identityReachVerdicts = gatekit.Waive(map[string]string{
	"modules/contacts/contact_children.go:insertContactEmails": "the one contact address writer is the backstop: " +
		"refuseLeadHeldEmails asks storekit.IdentityHoldersOf under the subject locks RefuseErasedEmails took just before, " +
		"and every typed door asked LockIdentityHolders first",
	"modules/contacts/providerclaimfill.go:fillEmail": "a provider fill asks storekit.IdentityHoldersOf (leadHolds) " +
		"under the subject lock its erasure check took, and leaves a live lead's address alone",
	"modules/contacts/leadmerge.go:buildLeadSurvivorshipPatch": "a lead merge moves a key from the lead it retires " +
		"to the survivor: one lead stays one lead, and no contact gains a key",
})

// identityKeyWriter is one function that writes an identity key.
type identityKeyWriter struct {
	file string
	fn   *ast.FuncDecl
	how  string
}

func TestEveryIdentityKeyWriterAsksWhoHoldsIt(t *testing.T) {
	t.Parallel()
	packages := map[string][]*ast.FuncDecl{}
	shared := map[string]map[string]string{}
	files := map[string]*ast.File{}
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
		files[where] = file
		dir := filepath.Dir(where)
		if shared[dir] == nil {
			shared[dir] = map[string]string{}
		}
		maps.Copy(shared[dir], sharedIdentityWrites(file))
	}
	var writers []identityKeyWriter
	for where, file := range files {
		dir := filepath.Dir(where)
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			packages[dir] = append(packages[dir], fn)
			if how, writes := identityKeyWrite(fn, shared[dir]); writes {
				writers = append(writers, identityKeyWriter{file: where, fn: fn, how: how})
			}
		}
	}
	requireIdentityCensus(t, writers)

	sort.Slice(writers, func(i, j int) bool {
		return writers[i].file+writers[i].fn.Name.Name < writers[j].file+writers[j].fn.Name.Name
	})
	defer identityReachVerdicts.AssertAllMatched(t)
	for _, w := range writers {
		key := strings.TrimPrefix(w.file, "internal/") + ":" + w.fn.Name.Name
		reach := callReach{funcs: packages[filepath.Dir(w.file)], probe: identityProbe, visiting: map[*ast.FuncDecl]bool{}}
		if !reach.guarded(w.fn) && !identityReachVerdicts.Waived(t, key) {
			t.Errorf("%s: %s %s and no path to it asks storekit.LockIdentityHolders who holds the key.\n"+
				"Ask it in the same transaction before the write, or state here how the writer reaches it.",
				key, w.fn.Name.Name, w.how)
		}
	}
}

// identityKeyWrite answers whether fn writes an identity key, and how. shared
// are the package-level statements fn may name.
func identityKeyWrite(fn *ast.FuncDecl, shared map[string]string) (string, bool) {
	for _, statement := range gatekit.SQLStatementsOf(fn) {
		if how, writes := identityKeyStatement(statement); writes {
			return how, true
		}
	}
	how, found := "", false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if write, named := shared[node.Name]; named {
				how, found = "names "+node.Name+", which "+write, true
			}
		case *ast.CallExpr:
			if calleeName(node) != "Set" || len(node.Args) == 0 {
				return !found
			}
			if field, ok := node.Args[0].(*ast.Ident); ok && identityKeyFields[field.Name] {
				how, found = "patches "+field.Name, true
			}
		}
		return !found
	})
	return how, found
}

// sharedIdentityWrites are the package-level declarations whose statements
// write an identity key, by name, so a function that names one is a writer.
func sharedIdentityWrites(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for _, statement := range gatekit.SQLStatementsOf(value) {
				if how, writes := identityKeyStatement(statement); writes {
					for _, name := range value.Names {
						out[name.Name] = how
					}
				}
			}
		}
	}
	return out
}

// requireIdentityCensus fails a census that reads too little: each way of
// writing a key must have been found at least once.
func requireIdentityCensus(t *testing.T, writers []identityKeyWriter) {
	t.Helper()
	for _, want := range []string{
		"INSERT INTO contact_email", "INSERT INTO lead", "UPDATE lead", "UPDATE contact_email",
		"patches leadEmailColumn", "patches linkedinURLField",
	} {
		found := false
		for _, w := range writers {
			found = found || strings.Contains(strings.ToLower(w.how), strings.ToLower(want))
		}
		if !found {
			t.Fatalf("found no identity key writer that %s: the census has stopped matching", want)
		}
	}
}

// callReach answers, within one package, whether a function asks the probe:
// itself, through a function it calls, or because every caller of it does.
type callReach struct {
	funcs    []*ast.FuncDecl
	probe    string
	visiting map[*ast.FuncDecl]bool
}

func (r callReach) guarded(fn *ast.FuncDecl) bool {
	if r.asks(fn, map[*ast.FuncDecl]bool{}) {
		return true
	}
	if fn.Name.IsExported() || r.visiting[fn] {
		return false
	}
	r.visiting[fn] = true
	defer delete(r.visiting, fn)
	callers := 0
	for _, caller := range r.funcs {
		if caller == fn || !callsName(caller, fn.Name.Name) {
			continue
		}
		callers++
		if !r.guarded(caller) {
			return false
		}
	}
	return callers > 0
}

// asks reports whether fn calls the probe, or calls a function of its package
// that does. A name two methods share counts for both.
func (r callReach) asks(fn *ast.FuncDecl, seen map[*ast.FuncDecl]bool) bool {
	if callsName(fn, r.probe) {
		return true
	}
	seen[fn] = true
	for _, callee := range r.funcs {
		if !seen[callee] && callsName(fn, callee.Name.Name) && r.asks(callee, seen) {
			return true
		}
	}
	return false
}
