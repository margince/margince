// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"go/ast"
	"go/parser"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func problemSourceUnit(t *testing.T, source string) *goUnit {
	t.Helper()
	file, err := parser.ParseFile(gatekit.SourceFileSet(), "internal/problemsample/sample.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return &goUnit{path: "internal/problemsample/sample.go", dir: "internal/problemsample", file: file, imports: localImports(file)}
}

// TestProblemStatusPairsKeepEachServerTableRowTogether rejects a cross product of unrelated rows.
func TestProblemStatusPairsKeepEachServerTableRowTogether(t *testing.T) {
	t.Parallel()
	unit := problemSourceUnit(t, `package sample
 import "github.com/margince/margince/backend/internal/platform/httperr"
 func faults(){for _,row:=range []struct{Status int;Code string}{{401,"unauthorized"},{404,"not_found"}}{_=httperr.DetailedError{Status:row.Status,Code:row.Code}}}`)
	codes := newEmittedCensus(t, []*goUnit{unit})
	codes.fieldCodes = true
	pairs := pairCensus{codes: codes, known: map[string]string{"unauthorized": "sample", "not_found": "sample"}, pairs: map[problemStatusPair]bool{}}
	for _, decl := range unit.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			status, hasStatus := fieldValue(lit, nil, "Status")
			code, hasCode := fieldValue(lit, nil, "Code")
			if hasStatus && hasCode {
				pairs.read(status, code, unit, fn, 0)
			}
			return true
		})
	}
	if !pairs.pairs[problemStatusPair{401, "unauthorized"}] || !pairs.pairs[problemStatusPair{404, "not_found"}] || pairs.pairs[problemStatusPair{404, "unauthorized"}] || pairs.pairs[problemStatusPair{401, "not_found"}] {
		t.Fatalf("mixed server rows: %v (faults %v)", pairs.pairs, pairs.faults)
	}
}

// TestAMessageFaultCanChooseItsStatusThroughIs follows the same sentinel as errors.Is.
func TestAMessageFaultCanChooseItsStatusThroughIs(t *testing.T) {
	t.Parallel()
	unit := problemSourceUnit(t, `package sample
 import "github.com/margince/margince/backend/internal/shared/apperrors"
 type heldError struct{}
 func (*heldError) MessageFault()(string,string){return "held_sample","Held"}
 func (*heldError) Is(target error)bool{return target==apperrors.ErrConflict}`)
	units := append(readGoUnits(t, "internal"), unit)
	pairs := pairCensus{codes: newEmittedCensus(t, units)}
	for _, decl := range unit.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "MessageFault" {
			continue
		}
		got := pairs.messageMethodStatuses(fn, unit, []int{422})
		if !slices.Equal(got, []int{409}) {
			t.Fatalf("Is(conflict) statuses=%v, want only409", got)
		}
	}
}

// TestAValidationBodyHelperKeepsItsFieldCode reads helpers whose callers supply the status.
func TestAValidationBodyHelperKeepsItsFieldCode(t *testing.T) {
	t.Parallel()
	source := `function validation(field:string,code:string){return {code:"validation_error",title:"Unprocessable Entity",detail:"No",details:{errors:[{field,code}]}};} Response.json(validation("name","too_long"),{status:422});`
	tree, err := parseTS(source, "body-helper.test.ts")
	if err != nil {
		t.Fatal(err)
	}
	scan := scanProblemFile("body-helper.test.ts", source, tree, map[string]bool{})
	found := false
	for _, mock := range scan.mocks {
		found = found || (mock.field && mock.code == "too_long")
	}
	if !found {
		t.Fatalf("body helper lost field code: %+v", scan.mocks)
	}
}

// TestAFieldFaultCanBeSpreadFromATypedTable follows each actual field code.
func TestAFieldFaultCanBeSpreadFromATypedTable(t *testing.T) {
	t.Parallel()
	source := `function refused(fault:Fault){return {status:422,code:"validation_error",details:{errors:[{...fault,message:"No"}]}};} const cases:readonly [Fault,string][]=[[{field:"name",code:"too_long"},"long"],[{field:"name",code:"required"},"empty"]];for(const [fault,sentence] of cases){refused(fault);}`
	tree, err := parseTS(source, "spread.test.ts")
	if err != nil {
		t.Fatal(err)
	}
	scan := scanProblemFile("spread.test.ts", source, tree, map[string]bool{})
	found := map[string]bool{}
	for _, mock := range scan.mocks {
		if mock.field {
			found[mock.code] = true
		}
	}
	if !found["too_long"] || !found["required"] {
		t.Fatalf("spread table lost codes: %v", found)
	}
}
