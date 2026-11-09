// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// The small-group floor of "Show individual performance" is compared in one
// function, belowFloor, and nowhere else.
//
// A second comparison is a second floor, which can drift from the first. The
// corpus is every hand-written Go file of the backend. A second floor reads
// fewestColleagues, or compares a count of members or seats with 2 or 3.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	floorConstant = "fewestColleagues"
	floorFunction = "belowFloor"
)

// floorCount names a count of the seats a total holds.
var floorCount = regexp.MustCompile(`(?i)(member|colleague|contributor|seat)`)

// notAFloor ratifies small-count comparisons that count something other than
// the the seats behind a figure.
var notAFloor = gatekit.Waive(map[string]string{
	"sameDestination":          "counts worklist rows to fold, not the seats behind a figure",
	"requireCaseSelection":     "a contact case merges at least two contacts; it counts records, not the seats behind a figure",
	"WithContactCaseAdmission": "a case admission names at least two contacts to merge; it counts records, not the seats behind a figure",
})

// floorComparison is one place a file compares against the floor.
type floorComparison struct {
	owner string
	pos   token.Pos
	what  string
}

// floorComparisons lists every read of fewestColleagues and every count of
// seats compared with a small literal in file, outside belowFloor. permitted
// counts belowFloor's own reads, so the census can prove it reached them.
func floorComparisons(file *ast.File) (found []floorComparison, permitted int) {
	for _, decl := range file.Decls {
		owner, isFunc := "", false
		if fn, ok := decl.(*ast.FuncDecl); ok {
			owner, isFunc = fn.Name.Name, true
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				if !isFunc && len(node.Names) == 1 && node.Names[0].Name == floorConstant {
					return false
				}
			case *ast.Ident:
				if node.Name != floorConstant {
					return true
				}
				if owner == floorFunction {
					permitted++
					return true
				}
				found = append(found, floorComparison{owner, node.Pos(), "reads " + floorConstant})
			case *ast.BinaryExpr:
				if smallCountComparison(node) {
					found = append(found, floorComparison{owner, node.OpPos, "compares a count of seats with a small number"})
				}
			}
			return true
		})
	}
	return found, permitted
}

func smallCountComparison(cmp *ast.BinaryExpr) bool {
	switch cmp.Op {
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return false
	}
	return (smallLiteral(cmp.X) && namesACount(cmp.Y)) || (smallLiteral(cmp.Y) && namesACount(cmp.X))
}

func smallLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && (lit.Value == "2" || lit.Value == "3")
}

func namesACount(e ast.Expr) bool {
	named := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && floorCount.MatchString(id.Name) {
			named = true
		}
		return !named
	})
	return named
}

func TestTheIndividualPerformanceFloorIsComparedInOneFunction(t *testing.T) {
	t.Parallel()
	defer notAFloor.AssertAllMatched(t)
	fset := gatekit.SourceFileSet()
	permitted := 0
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		found, reads := floorComparisons(file)
		permitted += reads
		for _, f := range found {
			if f.what != "reads "+floorConstant && notAFloor.Waived(t, f.owner) {
				continue
			}
			t.Errorf("%s: %s %s. The floor is compared only in %s, which every surface "+
				"reaches through thinGroups; ask thinGroups instead.", fset.Position(f.pos), describeOwner(f.owner), f.what, floorFunction)
		}
	}
	if permitted == 0 {
		t.Fatalf("the census never reached %s's own read of %s, so it has stopped reading the floor it guards",
			floorFunction, floorConstant)
	}
}

// A floor in another module, spelled with a bare number, is found too.
func TestASecondFloorWithoutTheConstantIsFound(t *testing.T) {
	t.Parallel()
	const second = `package reporting
func thin(members []int) bool { return len(members) < 2 }
func fine(rows []int) bool { return len(rows) < 2 }
func also(counted int) bool { return counted < fewestColleagues }
`
	file, err := parser.ParseFile(token.NewFileSet(), "second.go", second, 0)
	if err != nil {
		t.Fatal(err)
	}
	found, _ := floorComparisons(file)
	var owners []string
	for _, f := range found {
		owners = append(owners, f.owner)
	}
	if strings.Join(owners, ",") != "thin,also" {
		t.Errorf("found second floors in %v, want thin and also", owners)
	}
}

func describeOwner(owner string) string {
	if owner == "" {
		return "a package-level declaration"
	}
	return owner
}
