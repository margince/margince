// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type identifierFold struct {
	call   *ast.CallExpr
	value  ast.Expr
	domain bool
}

var identifierWords = regexp.MustCompile(`(?i)email|address|\baddr\b|domain|\bhost\b|counterparty|recipient|mailbox|localpart|bounced`)

func foldNodeText(node ast.Node) string {
	var out bytes.Buffer
	if err := format.Node(&out, token.NewFileSet(), node); err != nil {
		return ""
	}
	return out.String()
}

func foldBoundValue(expr ast.Expr, depth int) ast.Expr {
	if depth > 8 {
		return expr
	}
	name, ok := expr.(*ast.Ident)
	if !ok || name.Obj == nil {
		return expr
	}
	switch d := name.Obj.Decl.(type) {
	case *ast.AssignStmt:
		for i, left := range d.Lhs {
			if foldNodeText(left) != name.Name {
				continue
			}
			if i < len(d.Rhs) {
				return foldBoundValue(d.Rhs[i], depth+1)
			}
		}
	case *ast.ValueSpec:
		for i, left := range d.Names {
			if left.Name == name.Name && i < len(d.Values) {
				return foldBoundValue(d.Values[i], depth+1)
			}
		}
	}
	return expr
}

func foldStringCall(expr ast.Expr, pkg, method string) (*ast.CallExpr, bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != method {
		return nil, false
	}
	owner, ok := sel.X.(*ast.Ident)
	return call, ok && owner.Name == pkg
}

func manualFoldPair(call *ast.CallExpr, pkg string) (ast.Expr, bool) {
	for _, pair := range [][2]string{{"ToLower", "TrimSpace"}, {"TrimSpace", "ToLower"}} {
		outer, ok := foldStringCall(call, pkg, pair[0])
		if !ok {
			continue
		}
		inner, ok := foldStringCall(foldBoundValue(outer.Args[0], 0), pkg, pair[1])
		if ok {
			if _, inline := outer.Args[0].(*ast.CallExpr); !inline {
				return outer.Args[0], true
			}
			return inner.Args[0], true
		}
	}
	return nil, false
}

func manualIdentifierFolds(file *ast.File) []identifierFold {
	pkg := ""
	for _, imp := range file.Imports {
		name, err := strconv.Unquote(imp.Path.Value)
		if err == nil && name == "strings" {
			pkg = "strings"
			if imp.Name != nil {
				pkg = imp.Name.Name
			}
		}
	}
	if pkg == "" {
		return nil
	}
	var out []identifierFold
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		context := fn.Name.Name
		var parents []string
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if node == nil {
				context = parents[len(parents)-1]
				parents = parents[:len(parents)-1]
				return false
			}
			parents = append(parents, context)
			words := []string{context}
			switch n := node.(type) {
			case *ast.AssignStmt:
				for _, left := range n.Lhs {
					words = append(words, foldNodeText(left))
				}
			case *ast.ValueSpec:
				for _, left := range n.Names {
					words = append(words, left.Name)
				}
			case *ast.CaseClause:
				for _, value := range n.List {
					words = append(words, foldNodeText(value))
				}
			}
			context = strings.Join(words, " ")
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			value, ok := manualFoldPair(call, pkg)
			if !ok {
				return true
			}
			text := foldNodeText(foldBoundValue(value, 0))
			address := identifierWords.MatchString(text + " " + context)
			if name, ok := value.(*ast.Ident); ok && name.Name == "owner" {
				address = true
			}
			if name, ok := value.(*ast.Ident); ok {
				switch name.Name {
				case "record", "body", "subject", "contentType", "name", "firstName", "lastName", "recipientName", "country":
					address = false
				}
			}
			if address {
				out = append(out, identifierFold{call: call, value: value, domain: strings.Contains(strings.ToLower(text), "domain") || strings.Contains(strings.ToLower(text), "host")})
			}
			return true
		})
	}
	return out
}

func TestAddressAndDomainFoldsUseTheirSharedOwner(t *testing.T) {
	t.Parallel()
	root := filepath.Join(gatekit.ModuleRoot(t), "internal")
	files := 0
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		files++
		if strings.Contains(filepath.ToSlash(path), "/shared/kernel/values/") || strings.Contains(filepath.ToSlash(path), "/shared/kernel/correspondence/") {
			return nil
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			return err
		}
		for _, fold := range manualIdentifierFolds(file) {
			t.Errorf("%s: hand-written address/domain fold; use correspondence.Fold or owndomain.Fold, or values.LegacyIdentifierKey for an explicitly retained legacy lookup", gatekit.SourceFileSet().Position(fold.call.Pos()))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 1000 {
		t.Fatalf("the address-fold census read only %d source files", files)
	}
}

func TestTheAddressFoldCensusReadsAliasesAndSplitBindings(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		body string
		want int
	}{
		{`func emailKey(email string)string{return s.ToLower(s.TrimSpace(email))}`, 1},
		{`func key(address string)string{trimmed:=s.TrimSpace(address);return s.ToLower(trimmed)}`, 1},
		{`func key(domain string)string{return s.TrimSpace(s.ToLower(domain))}`, 1},
		{`func key(value string)string{switch "email"{case "email":return s.ToLower(s.TrimSpace(value))};return value}`, 1},
		{`func key(country string)string{return s.ToLower(s.TrimSpace(country))}`, 0},
		{`func key(name string)string{return s.ToLower(s.TrimSpace(name))}`, 0},
		{`func key(contentType string)string{return s.ToLower(s.TrimSpace(contentType))}`, 0},
	} {
		t.Run(c.body, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "case.go", "package x;import s \"strings\";"+c.body, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(manualIdentifierFolds(file)); got != c.want {
				t.Fatalf("found %d folds, want %d", got, c.want)
			}
		})
	}
}
