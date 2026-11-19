// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"testing"
)

// A projected activity row carries its direction as a pointer, and compose
// reads that row only to decide who wrote it. Direction names the transport:
// a colleague's mail can arrive inbound and a contact's can leave from here.
// So compose asks activities.WroteItOurSide or WriterSide, and a pointer read
// of a Direction field (a dereference or a nil check) is the shape of a reader
// that decides authorship by hand.
func TestComposeAsksTheActivityVoiceWhoWroteARow(t *testing.T) {
	t.Parallel()
	fset, files := parseGoFilesUnder(t, filepath.Join("internal", "compose"))
	if len(files) == 0 {
		t.Fatal("found no compose source to read")
	}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.StarExpr:
				if isDirectionField(n.X) {
					t.Errorf("%s dereferences an activity's direction; ask activities.WriterSide who wrote it", fset.Position(n.Pos()))
				}
			case *ast.BinaryExpr:
				if (n.Op == token.EQL || n.Op == token.NEQ) && (isNilCheckOfDirection(n.X, n.Y) || isNilCheckOfDirection(n.Y, n.X)) {
					t.Errorf("%s checks an activity's direction for nil; ask activities.WroteItOurSide who wrote it", fset.Position(n.Pos()))
				}
			}
			return true
		})
	}
}

func isDirectionField(expr ast.Expr) bool {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		expr = paren.X
	}
	selector, ok := expr.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "Direction"
}

func isNilCheckOfDirection(field, other ast.Expr) bool {
	ident, ok := other.(*ast.Ident)
	return ok && ident.Name == "nil" && isDirectionField(field)
}
