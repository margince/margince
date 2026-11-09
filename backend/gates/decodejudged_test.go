// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"go/ast"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// decodeJudgedCallers are the handlers that take number bounds out of the
// shared decode (httperr.DecodeJudged). Each says which check holds those
// bounds instead, so no bound the contract states goes unheld.
var decodeJudgedCallers = gatekit.Waive(map[string]string{
	"internal/compose/handlers_ai_taskoverrides.go:ReplaceAiTaskOverrides": "ai.validateTaskOverrides, run by every save, holds both timeout bounds and lists every refused path in one 422; the settings screen shows them field by field",
	"internal/modules/deals/handlers_deal.go:CreateDeal":                   "the deals store refuses a negative amount_minor as amount_negative (NegativeAmountError), the code the contract and the create form name",
	"internal/modules/deals/handlers_deal.go:UpdateDeal":                   "the deals store refuses a negative amount_minor as amount_negative (NegativeAmountError), the code the contract and the create form name",
	"internal/modules/deals/handlers_suggestions.go:AcceptDealSuggestion":  "the deals store refuses a negative amount_minor as amount_negative (NegativeAmountError), the code the contract and the create form name",
	"internal/compose/handlers_ai_taskoverrides.go:PreviewAiTaskOverrides": "the preview lists an out-of-range timeout among its errors from ai.validateTaskOverrides, so a decode refusal would turn the preview into a 422",
})

// Every handler that leaves number bounds to its own check is named above,
// with that check. A new caller fails until it says who holds its bounds.
func TestEveryDecodeJudgedCallerNamesTheCheckThatHoldsItsBounds(t *testing.T) {
	t.Parallel()
	defer decodeJudgedCallers.AssertAllMatched(t)
	found := 0
	for _, root := range []string{"internal", "../extensions"} {
		fset, files := parseGoFilesUnder(t, root)
		for _, file := range files {
			path, err := filepath.Rel(".", fset.Position(file.Pos()).Filename)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || !callsDecodeJudged(fn.Body) {
					continue
				}
				found++
				if site := path + ":" + fn.Name.Name; !decodeJudgedCallers.Waived(t, site) {
					t.Errorf("%s leaves number bounds to its own check; name that check in decodeJudgedCallers, or call httperr.Decode", site)
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("found no call of httperr.DecodeJudged; the scan no longer reads the tree")
	}
}

func callsDecodeJudged(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "DecodeJudged" || sel.Sel.Name == "DecodeJudgedOrRefusal") {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "httperr" {
				found = true
			}
		}
		return !found
	})
	return found
}
