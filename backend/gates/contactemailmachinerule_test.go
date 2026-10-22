// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every product statement that inserts into contact_email sits in a function
// that asks refuseMachineAddress first. An address whose local part names a
// sending system (noreply@, notifications@) names no contact. A writer that
// skips the question puts one on a record whatever the other writers do.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var contactEmailInsert = regexp.MustCompile(`(?i)INSERT\s+INTO\s+contact_email\b`)

// contactEmailRefusal is the contacts store's one check for a machine address.
const contactEmailRefusal = "refuseMachineAddress"

// contactEmailScope names the refusal only because the walk needs a name.
// It is not inside the owning package, so no call is claimed.
var contactEmailScope = helperScope{names: map[string]bool{contactEmailRefusal: true}}

func TestEveryContactEmailInsertAsksTheMachineAddressRule(t *testing.T) {
	t.Parallel()
	var findings []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		inserts, unasked := contactEmailInsertsUnasked(file)
		judged += inserts
		for _, name := range unasked {
			findings = append(findings, path+": "+name)
		}
	}
	// Two writers exist today: the child-row writer every create and update
	// reaches, and the bought-address fill. Fewer means the walk went blind.
	if judged < 2 {
		t.Fatalf("only %d statement(s) inserting into contact_email were found, want at least 2; the census walked too little of the tree", judged)
	}
	if len(findings) > 0 {
		t.Errorf("these functions insert into contact_email without asking %s:\n  %s\n\n"+
			"An address whose local part names a sending system is refused on every path into a contact. "+
			"Call %s on each address before the insert.",
			contactEmailRefusal, strings.Join(findings, "\n  "), contactEmailRefusal)
	}
}

// contactEmailInsertsUnasked counts the contact_email inserts in a file and
// names each function holding one that never calls the refusal.
func contactEmailInsertsUnasked(file *ast.File) (int, []string) {
	inserts := 0
	var unasked []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		found := 0
		seen := map[ast.Node]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if seen[n] {
				return false
			}
			if text, ok := flattenSQL(n, seen, contactEmailScope); ok && contactEmailInsert.MatchString(text) {
				found++
			}
			return true
		})
		asked := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == contactEmailRefusal {
					asked = true
				}
			}
			return !asked
		})
		inserts += found
		if found > 0 && !asked {
			unasked = append(unasked, fn.Name.Name)
		}
	}
	return inserts, unasked
}

func TestTheContactEmailCensusSeesEveryShapeOfInsert(t *testing.T) {
	t.Parallel()
	for _, probe := range []struct {
		name    string
		src     string
		inserts int
		unasked bool
	}{
		{"an insert with the refusal asked", "func w() { refuseMachineAddress(e); q(`INSERT INTO contact_email (email) VALUES ($1)`) }", 1, false},
		{"an insert with no refusal", "func w() { q(`INSERT INTO contact_email (email) VALUES ($1)`) }", 1, true},
		{"lower case, wrapped after the table", "func w() { q(`insert into contact_email\n (email) values ($1)`) }", 1, true},
		{"split across a concatenation", "func w() { q(`INSERT INTO ` + `contact_email (email) VALUES ($1)`) }", 1, true},
		{"a read of the table", "func r() { q(`SELECT email FROM contact_email WHERE contact_id = $1`) }", 0, false},
		{"a different table with the same prefix", "func w() { q(`INSERT INTO contact_email_history (email) VALUES ($1)`) }", 0, false},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "probe.go", "package p\n"+probe.src, 0)
		if err != nil {
			t.Fatalf("%s: %v", probe.name, err)
		}
		inserts, unasked := contactEmailInsertsUnasked(file)
		if inserts != probe.inserts || (len(unasked) > 0) != probe.unasked {
			t.Errorf("%s: inserts = %d, unasked = %v; want %d, unasked %v", probe.name, inserts, unasked, probe.inserts, probe.unasked)
		}
	}
}
