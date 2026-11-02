// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestThreadProducerApprovalBindsCallerHelperAndCatalog(t *testing.T) {
	t.Parallel()
	source := `package fixture
var tables = []string{"activity"}
func helper() string { return tables[0] }
func producer(callback func() string) { query(helper()); callback() }
func caller() { producer(func() string { return "safe" }) }
`
	original := threadProducerFixtureFingerprint(t, source)
	cases := map[string]string{
		"unsafe sibling":   source + `func sibling() { producer(func() string { return "unsafe" }) }`,
		"changed helper":   strings.ReplaceAll(source, `return tables[0]`, `return "unguarded"`),
		"changed catalog":  strings.ReplaceAll(source, `[]string{"activity"}`, `[]string{"activity","other"}`),
		"changed callback": strings.ReplaceAll(source, `return "safe"`, `return "unsafe"`),
		"changed producer": strings.ReplaceAll(source, `query(helper())`, `query(helper()); query("unsafe")`),
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if threadProducerFixtureFingerprint(t, source) == original {
				t.Fatal("changed producer authority retained its reviewed fingerprint")
			}
		})
	}
}

func threadProducerFixtureFingerprint(t *testing.T, source string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	declarations := map[string]threadProducerDeclaration{}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			declarations[declaration.Name.Name] = threadProducerDeclaration{node: declaration}
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				if binding, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range binding.Names {
						declarations[name.Name] = threadProducerDeclaration{node: binding}
					}
				}
			}
		}
	}
	index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": declarations}}
	fingerprint, _ := index.fingerprint("fixture", "producer")
	return fingerprint
}

func TestThreadProducerApprovalBindsImportedHelper(t *testing.T) {
	t.Parallel()
	render := func(body string) string {
		file, err := parser.ParseFile(token.NewFileSet(), "helper.go", "package helper; func Clause() string { return "+body+" }", 0)
		if err != nil {
			t.Fatal(err)
		}
		root, err := parser.ParseFile(token.NewFileSet(), "root.go", "package fixture; func producer() { query(helper.Clause()) }", 0)
		if err != nil {
			t.Fatal(err)
		}
		index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{
			"fixture": {"producer": {node: root.Decls[0], imports: map[string]string{"helper": "helper"}}},
			"helper":  {"Clause": {node: file.Decls[0]}},
		}}
		fingerprint, _ := index.fingerprint("fixture", "producer")
		return fingerprint
	}
	if render(`"guarded"`) == render(`"unguarded"`) {
		t.Fatal("changed imported helper retained its producer approval")
	}
}

func TestOpaqueImportedPredicateCannotHideANewActivitySource(t *testing.T) {
	t.Parallel()
	check := func(body string) int {
		helperFile, err := parser.ParseFile(token.NewFileSet(), "helper.go", "package helper; func Clause() string { return "+body+" }", 0)
		if err != nil {
			t.Fatal(err)
		}
		rootFile, err := parser.ParseFile(token.NewFileSet(), "root.go", `package fixture; import "github.com/margince/margince/backend/internal/helper"; func read(tx DB,ctx Context) { tx.Query(ctx,"SELECT id FROM company WHERE "+helper.Clause()) }`, 0)
		if err != nil {
			t.Fatal(err)
		}
		helper := threadSQLPackageOf([]*ast.File{helperFile})
		helper.directory = "internal/helper"
		root := threadSQLPackageOf([]*ast.File{rootFile})
		root.directory = "fixture"
		root.resolvePackage = func(directory string) threadSQLPackage {
			if directory != "internal/helper" {
				t.Fatalf("unexpected producer %s", directory)
			}
			return helper
		}
		return len(root.unguardedQueries(root.functions["read"]))
	}
	if check(`"archived_at IS NULL"`) != 0 {
		t.Fatal("closed nonactivity predicate became an activity subject")
	}
	if check(`"EXISTS(SELECT 1 FROM public.\"activity\")"`) == 0 {
		t.Fatal("imported activity subquery escaped the derived census")
	}
}

func TestImportedQueryFieldsFollowTheirActualSQLProducer(t *testing.T) {
	t.Parallel()
	findings := func(table string) int {
		helperFile, err := parser.ParseFile(token.NewFileSet(), "helper.go", `package helper; type Plan struct { SQL string };func Query() Plan { return Plan{SQL:"SELECT id FROM `+table+`"} }`, 0)
		if err != nil {
			t.Fatal(err)
		}
		rootFile, err := parser.ParseFile(token.NewFileSet(), "root.go", `package fixture; import "github.com/margince/margince/backend/internal/helper"; func read(tx DB,ctx Context) { tx.Query(ctx,helper.Query().SQL) }`, 0)
		if err != nil {
			t.Fatal(err)
		}
		helper := threadSQLPackageOf([]*ast.File{helperFile})
		helper.directory = "internal/helper"
		root := threadSQLPackageOf([]*ast.File{rootFile})
		root.directory = "fixture"
		root.resolvePackage = func(directory string) threadSQLPackage { return helper }
		return len(root.unguardedQueries(root.functions["read"]))
	}
	if findings("company") != 0 {
		t.Fatal("closed nonactivity query was escalated")
	}
	if findings("activity") == 0 {
		t.Fatal("imported query field hid its activity source")
	}
}

func TestMissingProducerDeclarationsCannotKeepAnApproval(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture; func producer() {}", 0)
	if err != nil {
		t.Fatal(err)
	}
	declarations := map[string]threadProducerDeclaration{"producer": {node: file.Decls[0]}}
	index := threadProducerIndex{t: t, packages: map[string]map[string]threadProducerDeclaration{"fixture": declarations}}
	if _, err := index.requiredRoots("fixture", "producer"); err != nil {
		t.Fatal(err)
	}
	delete(declarations, "producer")
	if _, err := index.requiredRoots("fixture", "producer"); err == nil {
		t.Fatal("deleted producer declaration silently disappeared from its approved closure")
	}
	if _, err := index.requiredRoots("fixture", "typo"); err == nil {
		t.Fatal("unknown producer declaration was accepted")
	}
}
