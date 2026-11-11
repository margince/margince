// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"fmt"
	"go/ast"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A seat revoked between session resolution and the write refuses the buyer
// with the same 401 the session check gives, never a 500.
func TestASeatRevokedAfterTheSessionResolvedAnswers401(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want int
	}{
		"revoked seat":     {fmt.Errorf("reply as buyer: %w", ErrSessionRefused), http.StatusUnauthorized},
		"withdrawn record": {apperrors.ErrNotFound, http.StatusNotFound},
	} {
		w := httptest.NewRecorder()
		writeBuyerFault(w, httptest.NewRequest(http.MethodPost, "/", nil), tc.err)
		if w.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", name, w.Code, tc.want)
		}
	}
}

// Every handler that serves a resolved session answers its errors through
// writeBuyerFault, so a later seat refusal cannot reach httperr.Write.
func TestBuyerHandlersAnswerErrorsThroughWriteBuyerFault(t *testing.T) {
	f := parseGoFile(t, publicHandlersFile)
	served := 0
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !calls(fn.Body, "", "SessionFrom") {
			continue
		}
		served++
		if calls(fn.Body, "httperr", "Write") {
			t.Errorf("%s answers an error with httperr.Write; use writeBuyerFault", fn.Name.Name)
		}
	}
	if served == 0 {
		t.Fatal("no handler in handlers_public.go reads SessionFrom; the census is empty")
	}
}

// calls says whether body calls pkg.name, or the bare name when pkg is "".
func calls(body *ast.BlockStmt, pkg, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return !found
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			found = found || (pkg == "" && fun.Name == name)
		case *ast.SelectorExpr:
			x, ok := fun.X.(*ast.Ident)
			found = found || (ok && x.Name == pkg && fun.Sel.Name == name)
		}
		return !found
	})
	return found
}
