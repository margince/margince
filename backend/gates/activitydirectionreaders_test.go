// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Direction names the transport, never the writer: a colleague's mail can
// arrive inbound and a contact's can leave from here. A projected activity
// carries it as a pointer, so a dereference or a nil check of a Direction field
// in compose is a reader deciding who wrote the row by hand. It asks
// activities.WroteItOurSide or WriterSide instead.
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

// selectedDirection is an activity's direction read as a column: aliased and
// followed by a comma, or behind coalesce. A WHERE test reads it otherwise.
var selectedDirection = regexp.MustCompile(`(?i)\b[a-z_]+\.direction\s*,|coalesce\(\s*([a-z_]+\.)?direction\b`)

// The compose statements that select the transport itself, each with why.
var transportDirectionReaders = gatekit.Waive(map[string]string{
	"internal/compose/signalscan.go":    "asks whether we sent the newest message; directionowner_test holds it to the direction rule",
	"internal/compose/extsystemmail.go": "hands an extension the stored transport fact of a sending system's mail",
})

// A statement that hands a model who wrote a row selects
// activities.WriterSideSQL, never the stored direction.
func TestComposeSelectsTheWriterSideNotTheDirection(t *testing.T) {
	t.Parallel()
	defer transportDirectionReaders.AssertAllMatched(t)
	fset, files := parseGoFilesUnder(t, filepath.Join("internal", "compose"))
	for _, file := range files {
		name := filepath.ToSlash(fset.Position(file.Pos()).Filename)
		ast.Inspect(file, func(node ast.Node) bool {
			expr, ok := node.(ast.Expr)
			if !ok {
				return true
			}
			text, isLiteral := gatekit.LiteralText(expr)
			if !isLiteral || !selectedDirection.MatchString(text) {
				return true
			}
			if !transportDirectionReaders.Waived(t, name) {
				t.Errorf("%s selects an activity's direction; select activities.WriterSideSQL for who wrote it", fset.Position(expr.Pos()))
			}
			return true
		})
	}
}
