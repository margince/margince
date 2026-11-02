// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestRestoreApprovalIncludesNewCrossPackageCallersAndShapes(t *testing.T) {
	t.Parallel()
	owner, _ := threadCatalogFixture(t, `package storekit;type UnarchiveShape struct{Lock string};func Unarchive(){}`)
	first, firstFile := threadCatalogFixture(t, `package one;import restore "github.com/margince/margince/backend/internal/platform/database/storekit";var shape=restore.UnarchiveShape{Lock:"SELECT id FROM company"};func Restore(){restore.Unarchive(shape)}`)
	index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{threadRestoreOwner: owner, "internal/one": first}, restoreCatalogLoaded: true}
	index.restoreDeclarations = threadRestoreSites("internal/one", firstFile)
	original, _ := index.fingerprint(threadRestoreOwner, "Unarchive")
	second, secondFile := threadCatalogFixture(t, `package two;import renamed "github.com/margince/margince/backend/internal/platform/database/storekit";var extra=renamed.UnarchiveShape{Lock:"SELECT id FROM activity"};func Restore(){renamed.Unarchive(extra)}`)
	index.packages["internal/two"] = second
	index.restoreDeclarations = append(index.restoreDeclarations, threadRestoreSites("internal/two", secondFile)...)
	changed, _ := index.fingerprint(threadRestoreOwner, "Unarchive")
	if original == changed {
		t.Fatal("new cross-package caller/catalog retained the old approval")
	}
	if len(threadRestoreSites("internal/two", secondFile)) != 2 {
		t.Fatal("renamed canonical import lost a caller or catalog shape")
	}
}

func TestRestoreApprovalDetectsEachExternalCorpusExpansion(t *testing.T) {
	t.Parallel()
	owner, _ := threadCatalogFixture(t, `package storekit;type UnarchiveShape struct{Lock string};func Unarchive(){}`)
	for name, source := range map[string]string{
		"caller": `package two;import p "github.com/margince/margince/backend/internal/platform/database/storekit";func Restore(shape p.UnarchiveShape){p.Unarchive(shape)}`,
		"shape":  `package two;import p "github.com/margince/margince/backend/internal/platform/database/storekit";var shape=p.UnarchiveShape{Lock:"SELECT id FROM activity"}`,
	} {
		t.Run(name, func(t *testing.T) {
			index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{threadRestoreOwner: owner}, restoreCatalogLoaded: true}
			before, _ := index.fingerprint(threadRestoreOwner, "Unarchive")
			declarations, file := threadCatalogFixture(t, source)
			index.packages["internal/two"] = declarations
			index.restoreDeclarations = threadRestoreSites("internal/two", file)
			if len(index.restoreDeclarations) != 1 {
				t.Fatalf("derived corpus=%v", index.restoreDeclarations)
			}
			after, _ := index.fingerprint(threadRestoreOwner, "Unarchive")
			if before == after {
				t.Fatal("external corpus expansion retained the previous approval")
			}
		})
	}
}

func threadCatalogFixture(t *testing.T, source string) (map[string]threadProducerDeclaration, *ast.File) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[string]string{}
	for _, spec := range file.Imports {
		module, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		name := module[strings.LastIndex(module, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = strings.TrimPrefix(module, "github.com/margince/margince/backend/")
	}
	declarations := map[string]threadProducerDeclaration{}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			declarations[declaration.Name.Name] = threadProducerDeclaration{node: declaration, imports: imports}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						declarations[name.Name] = threadProducerDeclaration{node: spec, imports: imports}
					}
				case *ast.TypeSpec:
					declarations[spec.Name.Name] = threadProducerDeclaration{node: spec, imports: imports}
				}
			}
		}
	}
	return declarations, file
}

func TestGenericForwardersBindTheirActualCatalogAndCallback(t *testing.T) {
	t.Parallel()
	source := `package fixture
 type Row struct{}
 type Spec struct { SQL func() string }
 func producer[T any](tx DB,ctx Context,spec Spec) { tx.Query(ctx,spec.SQL()) }
 func wrapper[T any](tx DB,ctx Context,spec Spec) { producer[T](tx,ctx,spec) }
 func caller() { wrapper[Row](nil,nil,catalog()) }
 func catalog() Spec { return Spec{SQL:clause} }
 func clause() string { return "SELECT id FROM company" }
 `
	declarations, _ := threadCatalogFixture(t, source)
	index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": declarations}}
	original, names := index.fingerprint("fixture", "producer")
	for _, required := range []string{"fixture:catalog", "fixture:clause", "caller:fixture:caller"} {
		found := false
		for _, name := range names {
			if name == required {
				found = true
			}
		}
		if !found {
			t.Fatalf("forwarded producer omitted %s", required)
		}
	}
	mutated, _ := threadCatalogFixture(t, strings.ReplaceAll(source, "SELECT id FROM company", "SELECT subject FROM activity"))
	changed := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": mutated}}
	fingerprint, _ := changed.fingerprint("fixture", "producer")
	if original == fingerprint {
		t.Fatal("changed callback behind generic forwarder retained its approval")
	}
}

func TestRenamedGenericCatalogFieldsStillBindTheirFactory(t *testing.T) {
	t.Parallel()
	source := `package fixture;type Row struct{};type Spec struct{Unusual string};func producer[T any](tx DB,ctx Context,spec Spec){tx.Query(ctx,spec.Unusual)};func wrapper[T any](tx DB,ctx Context,spec Spec){producer[T](tx,ctx,spec)};func caller(){wrapper[Row](nil,nil,catalog())};func catalog()Spec{return Spec{Unusual:"SELECT id FROM company"}}`
	original, _ := threadCatalogFixture(t, source)
	index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": original}}
	before, names := index.fingerprint("fixture", "producer")
	found := false
	for _, name := range names {
		if name == "fixture:catalog" {
			found = true
		}
	}
	if !found {
		t.Fatal("renamed SQL catalog field hid its forwarded factory")
	}
	mutated, _ := threadCatalogFixture(t, strings.ReplaceAll(source, "SELECT id FROM company", "SELECT subject FROM activity"))
	changed := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": mutated}}
	after, _ := changed.fingerprint("fixture", "producer")
	if before == after {
		t.Fatal("changed factory behind renamed generic catalog field retained approval")
	}
}

func TestBoundValuesDoNotExpandTheSQLProducerCallerClosure(t *testing.T) {
	t.Parallel()
	source := `package fixture;func producer(tx DB,ctx Context,email string){tx.Query(ctx,"SELECT id FROM company WHERE domain=$1",email)};func wrapper(tx DB,ctx Context,email string){producer(tx,ctx,email)};func caller(){wrapper(nil,nil,value())};func value()string{return "one.example"}`
	declarations, _ := threadCatalogFixture(t, source)
	index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": declarations}}
	before, _ := index.fingerprint("fixture", "producer")
	changed, _ := threadCatalogFixture(t, strings.ReplaceAll(source, "one.example", "two.example"))
	mutated := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": changed}}
	after, _ := mutated.fingerprint("fixture", "producer")
	if before != after {
		t.Fatal("a bound data value expanded the SQL/catalog caller proof")
	}
}

func TestProducerFingerprintsDoNotDependOnRootTraversalOrder(t *testing.T) {
	t.Parallel()
	source := `package fixture
func first(tx DB,ctx Context,sql string){second(tx,ctx,sql)}
func second(tx DB,ctx Context,sql string){first(tx,ctx,sql);tx.Query(ctx,sql)}
func outer(tx DB,ctx Context,sql string){first(tx,ctx,sql)}
func caller(){outer(nil,nil,catalog())}
func catalog()string{return "SELECT id FROM activity"}`
	declarations, _ := threadCatalogFixture(t, source)
	orders := [][]string{{"first", "second", "outer"}, {"outer", "second", "first"}, {"second", "first", "outer"}}
	expected := map[string]string{}
	for _, order := range orders {
		index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": declarations}}
		for _, root := range order {
			fingerprint, names := index.fingerprint("fixture", root)
			proof := fingerprint + strings.Join(names, "\n")
			if previous, exists := expected[root]; exists && previous != proof {
				t.Fatalf("root order changed %s fingerprint or dependency membership", root)
			}
			expected[root] = proof
			if !index.sqlParameterSlots("fixture", root, nil)[2] {
				t.Fatalf("recursive SQL forwarding omitted %s SQL argument", root)
			}
		}
	}
}
