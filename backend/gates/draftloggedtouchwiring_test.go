// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// draftWiringRoot is the tree whose construction of the draft services is read.
const draftWiringRoot = "internal/compose"

// draftEngines are the import paths of the drafts that classify a conversation
// through draftcore.ClassifyWithLogged, and so read the newest logged touch.
var draftEngines = map[string]bool{
	"github.com/margince/margince/backend/internal/compose/contactdraft": true,
	"github.com/margince/margince/backend/internal/compose/accountdraft": true,
	"github.com/margince/margince/backend/internal/compose/leaddraft":    true,
}

// Every draft service built in compose binds the logged-touch read. Without it
// the draft sees only the timeline page. A contact met once behind a page of
// newer activity is then written to as a stranger.
func TestEveryDraftServiceBindsTheLoggedTouchRead(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	built := map[string]int{}
	err := gatekit.WalkDir(draftWiringRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		body, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(fset, path, body, 0)
		if err != nil {
			return err
		}
		for _, missing := range unboundDraftServices(file, built) {
			t.Errorf("%s: a %s service is built without WithLoggedTouches", fset.Position(missing.Pos()), filepath.Base(missing.engine))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for engine := range draftEngines {
		if built[engine] == 0 {
			t.Errorf("no %s service is built in %s; the census is short or the engine moved", filepath.Base(engine), draftWiringRoot)
		}
	}
}

type unboundService struct {
	ast.Node
	engine string
}

// unboundDraftServices counts each engine's NewService calls in file into
// built, and answers those whose call chain never reaches WithLoggedTouches.
func unboundDraftServices(file *ast.File, built map[string]int) []unboundService {
	engines := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !draftEngines[path] {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		engines[name] = path
	}
	if len(engines) == 0 {
		return nil
	}
	var missing []unboundService
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		stack = append(stack, n)
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewService" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || engines[pkg.Name] == "" {
			return true
		}
		built[engines[pkg.Name]]++
		if !chainReaches(stack, "WithLoggedTouches") {
			missing = append(missing, unboundService{Node: call, engine: engines[pkg.Name]})
		}
		return true
	})
	return missing
}

// chainReaches climbs from the call on top of stack through the method calls
// chained onto it, and reports one named method.
func chainReaches(stack []ast.Node, method string) bool {
	for i := len(stack) - 1; i > 0; i-- {
		parent := stack[i-1]
		switch p := parent.(type) {
		case *ast.SelectorExpr:
			if p.X != stack[i] {
				return false
			}
			if p.Sel.Name == method {
				return true
			}
		case *ast.CallExpr:
			if p.Fun != stack[i] {
				return false
			}
		default:
			return false
		}
	}
	return false
}
