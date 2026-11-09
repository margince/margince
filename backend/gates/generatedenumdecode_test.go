// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// No generated Go client refuses an enum value it does not know while
// decoding.
//
// The contract's response enums are open, so a client must keep a response
// that carries a value added after it was built. In the generated packages an
// enum is a string type with a Valid method, and json decoding into it never
// checks membership. This gate holds that true: no enum type gains its own
// decoder, and no generated decoder calls Valid. A server checks a request
// value with Valid itself, where it handles the request.

import (
	"go/ast"
	"go/parser"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// generatedClients are the packages generated from api/crm.yaml.
var generatedClients = []string{"internal/contracts", "pkg/extension/crm"}

func TestNoGeneratedClientRefusesAnUnknownEnumValueOnDecode(t *testing.T) {
	t.Parallel()
	enums := 0
	for _, dir := range generatedClients {
		files, err := filepath.Glob(filepath.Join(dir, "*_gen.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no generated file under %s (%v), so this gate read nothing", dir, err)
		}
		var parsed []*ast.File
		for _, path := range files {
			file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			parsed = append(parsed, file)
		}
		valid, decoders := methodsByReceiver(parsed)
		for typ := range valid {
			enums++
			for _, decoder := range []string{"UnmarshalJSON", "UnmarshalText"} {
				if decoders[typ][decoder] {
					t.Errorf("%s: the enum %s has its own %s, which can refuse a value a newer server sends",
						dir, typ, decoder)
				}
			}
		}
		for _, file := range parsed {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && strings.HasPrefix(fn.Name.Name, "Unmarshal") && callsValid(fn) {
					t.Errorf("%s: %s checks an enum while decoding, so an unknown value fails the response",
						dir, fn.Name.Name)
				}
			}
		}
	}
	if enums == 0 {
		t.Fatal("no generated enum type found, so this gate read nothing")
	}
}

// methodsByReceiver answers the types with a Valid method, and every method
// name by receiver type.
func methodsByReceiver(files []*ast.File) (map[string]bool, map[string]map[string]bool) {
	valid := map[string]bool{}
	methods := map[string]map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			typ := ""
			if ok {
				typ = receiverTypeName(fn)
			}
			if typ == "" {
				continue
			}
			if methods[typ] == nil {
				methods[typ] = map[string]bool{}
			}
			methods[typ][fn.Name.Name] = true
			if fn.Name.Name == "Valid" {
				valid[typ] = true
			}
		}
	}
	return valid, methods
}

// callsValid reports whether a function calls a method named exactly Valid.
func callsValid(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Valid" {
				found = true
			}
		}
		return !found
	})
	return found
}
