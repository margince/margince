// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestJobRegistrationMarkersComeOnlyFromTheCatalog(t *testing.T) {
	t.Parallel()
	root := filepath.Join("internal", "compose")
	generated := filepath.Join(root, "jobkinds_gen.go")
	file, err := gatekit.ParseFile(generated, 0)
	if err != nil {
		t.Fatal(err)
	}
	marker := jobRegistrationMarker(t, file)
	declared := map[string]bool{}
	for _, spec := range jobs.Declared() {
		declared[spec.GoType] = true
	}
	matched := map[string]int{}
	// The unexported marker can be implemented only in compose, including its tests.
	paths, err := filepath.Glob(filepath.Join(root, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		parsed, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		if path != generated && hasBorrowedJobRegistration(parsed, marker) {
			t.Errorf("%s: registration markers and constraint embeddings must be generated", path)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != marker {
				continue
			}
			if fault := jobRegistrationMarkerFault(fn, path, generated, declared); fault != "" {
				t.Errorf("%s: %s", path, fault)
				continue
			}
			matched[promptReceiverType(fn)]++
		}
	}
	if len(declared) == 0 {
		t.Fatal("job catalog contains no marker subjects")
	}
	for name := range declared {
		if matched[name] != 1 {
			t.Errorf("%s has %d generated registration markers, want one", name, matched[name])
		}
	}
}

func jobRegistrationMarker(t *testing.T, file *ast.File) string {
	t.Helper()
	var names []string
	for _, decl := range file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			named, ok := spec.(*ast.TypeSpec)
			if !ok || named.Name.Name != "declaredJobArgs" {
				continue
			}
			constraint, ok := named.Type.(*ast.InterfaceType)
			if !ok {
				t.Fatal("declared job constraint is not an interface")
			}
			for _, method := range constraint.Methods.List {
				for _, name := range method.Names {
					names = append(names, name.Name)
				}
			}
		}
	}
	if len(names) != 1 {
		t.Fatalf("declared job constraint has %d marker methods, want one", len(names))
	}
	return names[0]
}

func jobRegistrationMarkerFault(fn *ast.FuncDecl, path, generated string, declared map[string]bool) string {
	if path != generated {
		return "registration markers must be generated from api/jobs.yaml"
	}
	receiver := promptReceiverType(fn)
	if !declared[receiver] {
		return "registration marker names an undeclared args type"
	}
	if len(fn.Type.Params.List) != 1 {
		return "registration marker must name its exact args type"
	}
	self, ok := fn.Type.Params.List[0].Type.(*ast.Ident)
	if !ok || self.Name != receiver {
		return "registration marker must name its exact args type"
	}
	return ""
}

func TestJobRegistrationRefusesHandwrittenAndBorrowedMarkers(t *testing.T) {
	t.Parallel()
	file, err := gatekit.ParseFile(filepath.Join("internal", "compose", "jobkinds_gen.go"), 0)
	if err != nil {
		t.Fatal(err)
	}
	marker := jobRegistrationMarker(t, file)
	for _, test := range []struct {
		path, receiver, parameter string
		valid                     bool
	}{
		{"generated", "DeclaredArgs", "DeclaredArgs", true},
		{"handwritten", "DeclaredArgs", "DeclaredArgs", false},
		{"generated", "ExtraArgs", "ExtraArgs", false},
		{"generated", "DeclaredArgs", "ExtraArgs", false},
	} {
		source := "package fixture\nfunc (" + test.receiver + ") " + marker + "(" + test.parameter + ") {}"
		parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		fn, ok := parsed.Decls[0].(*ast.FuncDecl)
		if !ok {
			t.Fatal("fixture has no marker declaration")
		}
		fault := jobRegistrationMarkerFault(fn, test.path, "generated", map[string]bool{"DeclaredArgs": true})
		if (fault == "") != test.valid {
			t.Errorf("%+v: fault = %q", test, fault)
		}
	}
}

func hasBorrowedJobRegistration(file *ast.File, marker string) bool {
	allowed := map[*ast.Ident]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Type.TypeParams == nil {
			continue
		}
		for _, field := range fn.Type.TypeParams.List {
			ast.Inspect(field.Type, func(node ast.Node) bool {
				if name, ok := node.(*ast.Ident); ok && name.Name == "declaredJobArgs" {
					allowed[name] = true
				}
				return true
			})
		}
	}
	forbidden := false
	ast.Inspect(file, func(node ast.Node) bool {
		if name, ok := node.(*ast.Ident); ok && (name.Name == marker || name.Name == "declaredJobArgs" && !allowed[name]) {
			forbidden = true
		}
		return true
	})
	return forbidden
}

func TestJobRegistrationRefusesPromotedAndAliasedMarkers(t *testing.T) {
	t.Parallel()
	file, err := gatekit.ParseFile(filepath.Join("internal", "compose", "jobkinds_gen.go"), 0)
	if err != nil {
		t.Fatal(err)
	}
	marker := jobRegistrationMarker(t, file)
	for _, declaration := range []string{
		"type markFoo interface { " + marker + "(Foo) }; type Foo struct { markFoo }",
		"type Foo struct { declaredJobArgs[Foo] }",
		"type Foo struct { *declaredJobArgs[Foo] }",
		"type borrowed = declaredJobArgs[Foo]; type Foo struct { borrowed }",
		"func (*Foo) " + marker + "(Foo) {}",
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; "+declaration, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !hasBorrowedJobRegistration(parsed, marker) {
			t.Errorf("accepted %s", declaration)
		}
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func register[A declaredJobArgs[A]](a A) {}", 0)
	if err != nil {
		t.Fatal(err)
	}
	if hasBorrowedJobRegistration(parsed, marker) {
		t.Fatal("refused a generic registration constraint")
	}
}
