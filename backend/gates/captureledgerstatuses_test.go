// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The disposition ledger's status vocabulary has ONE definition, and it is the
// column's own constraint.
//
// Two readers have to account for every state a sender's question can end in:
// the capture-activity funnel decides which bucket each settled verdict counts
// under, and the message ladder names each one to a member. A status added to
// the column alone reaches neither — it is not open, so the ladder reports that
// it cannot tell, and it is not folded, so the counters go on saying the sender
// is waiting for a verdict that landed. That is the exact failure the funnel
// fold was written to end, one status later.
//
// Derived from the migration catalog rather than listed here, so this gate is
// not a third copy of the vocabulary it exists to keep singular. It reads the
// CHECK the head catalog records, which is what a fresh installation gets.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// statusCheck is the constraint line, and the quoted literals inside it.
const statusCheck = "capture_pending_counterparty.capture_pending_counterparty_status_check"

var checkedLiteral = regexp.MustCompile(`'([a-z_]+)'::text`)

func TestTheLedgerStatusSetMatchesItsConstraint(t *testing.T) {
	t.Parallel()

	constrained, err := ledgerStatusesFromCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(constrained) == 0 {
		t.Fatal("the head catalog records no status constraint for the disposition ledger — either " +
			"the constraint was dropped, in which case the column now accepts anything and this " +
			"gate is the least of it, or this scan is reading the wrong file. A parity check with " +
			"nothing on one side cannot report a pass.")
	}
	declared, err := declaredLedgerStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(declared) == 0 {
		t.Fatal("capture.PendingStatuses() reads as empty — either it was renamed and this scan " +
			"now parses nothing, or the list really is empty and no reader classifies anything.")
	}
	slices.Sort(declared)
	if !slices.Equal(declared, constrained) {
		t.Errorf("capture.PendingStatuses() is %v and the column accepts %v.\n\n"+
			"Every reader that must account for all of them — the funnel fold in tracestore.go, the "+
			"verdict rung in compose/pipelinetrace — reads the Go list. A state the database accepts "+
			"and the list omits is one no reader classifies: the ladder reports that it cannot tell, "+
			"and the counters go on saying the sender is waiting for a verdict that has landed.",
			declared, constrained)
	}
}

// ledgerStatusesFromCatalog reads the accepted values off the recorded CHECK.
func ledgerStatusesFromCatalog() ([]string, error) {
	path := filepath.Join(repoRoot, "backend", "migrations", "testdata", "head_catalog.txt")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the head catalog: %w", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, statusCheck) {
			continue
		}
		var out []string
		for _, match := range checkedLiteral.FindAllStringSubmatch(line, -1) {
			out = append(out, match[1])
		}
		slices.Sort(out)
		return out, nil
	}
	return nil, nil
}

// declaredLedgerStatuses reads the values capture.PendingStatuses() returns.
//
// By SOURCE rather than by calling it: a gate may not import a module, and the
// rule is about what that list says rather than about running it. The constants
// and the list live in one file, so the two halves are read together.
func declaredLedgerStatuses() ([]string, error) {
	path := filepath.Join(repoRoot, "backend", "internal", "modules", "capture", "pending.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing the ledger's constants: %w", err)
	}
	values := map[string]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		literal, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		unquoted, err := strconv.Unquote(literal.Value)
		if err != nil {
			return true
		}
		values[spec.Names[0].Name] = unquoted
		return true
	})
	var out []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "PendingStatuses" || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if value, declared := values[ident.Name]; declared {
				out = append(out, value)
			}
			return true
		})
	}
	return out, nil
}
