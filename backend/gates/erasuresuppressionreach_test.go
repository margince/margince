// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Every writer of an address or channel account asks the erasure list. The
// corpus is each production INSERT into contact_email, lead or
// contact_channel_identity. Its function must call a storekit probe, or every
// caller of it in its package must; an exported writer needs its own. The
// order of probe and insert is not checked.

import (
	"go/ast"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// storesIdentifier matches an INSERT into a table that holds an identifier the
// suppression list names. The trailing [\s(] keeps `lead_routing_rule` and
// other child tables out.
var storesIdentifier = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(contact_email|lead|contact_channel_identity)[\s(]`)

// suppressionProbes are storekit's questions to the erasure list.
var suppressionProbes = map[string]bool{
	"EmailSuppressed":             true,
	"ChannelIdentitySuppressed":   true,
	"RefuseErasedEmail":           true,
	"RefuseErasedChannelIdentity": true,
}

// identifierWriter is one function holding an identifier INSERT.
type identifierWriter struct {
	file  string
	fn    *ast.FuncDecl
	table string
}

func TestEveryIdentifierWriterReachesTheSuppressionProbe(t *testing.T) {
	t.Parallel()
	packages := map[string][]*ast.FuncDecl{}
	var writers []identifierWriter
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
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			packages[dir] = append(packages[dir], fn)
			for _, statement := range gatekit.SQLStatementsOf(fn) {
				if m := storesIdentifier.FindStringSubmatch(statement); m != nil {
					writers = append(writers, identifierWriter{file: where, fn: fn, table: strings.ToLower(m[1])})
					break
				}
			}
		}
	}

	// A table with no writer found means the census read too little.
	seen := map[string]bool{}
	for _, w := range writers {
		seen[w.table] = true
	}
	for _, table := range []string{"contact_email", "lead", "contact_channel_identity"} {
		if !seen[table] {
			t.Fatalf("found no production INSERT INTO %s: the pattern has stopped matching", table)
		}
	}

	sort.Slice(writers, func(i, j int) bool {
		return writers[i].file+writers[i].fn.Name.Name < writers[j].file+writers[j].fn.Name.Name
	})
	for _, w := range writers {
		reach := probeReach{funcs: packages[filepath.Dir(w.file)], visiting: map[*ast.FuncDecl]bool{}}
		if !reach.guarded(w.fn) {
			t.Errorf("%s: %s inserts into %s and no path to it asks the erasure list.\n"+
				"Call storekit.RefuseErasedEmail or storekit.RefuseErasedChannelIdentity in the "+
				"same transaction before the insert, or route the write through a writer that does "+
				"(contacts.insertContactEmails, contacts.insertLeadRow, "+
				"contacts.ResolveOrCreateChannelIdentity).", w.file, w.fn.Name.Name, w.table)
		}
	}
}

// probeReach answers, within one package, whether a function asks the erasure
// list itself or is only ever called from functions that do.
type probeReach struct {
	funcs    []*ast.FuncDecl
	visiting map[*ast.FuncDecl]bool
}

func (r probeReach) guarded(fn *ast.FuncDecl) bool {
	if callsProbe(fn) {
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

func callsProbe(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall && suppressionProbes[calleeName(call)] {
			found = true
		}
		return !found
	})
	return found
}

// callsName matches by name, so two methods sharing one name both count as
// callers: that can only make the check stricter.
func callsName(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, isCall := n.(*ast.CallExpr); isCall && calleeName(call) == name {
			found = true
		}
		return !found
	})
	return found
}
