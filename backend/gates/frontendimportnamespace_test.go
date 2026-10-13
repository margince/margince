// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser parses the CSV importer's source stamp to say a record was
// imported, and from which upload.
//
// The three parts of that grammar it reads are declared mirrors of the server's:
// IMPORT_NAMESPACE of provenance.ReservedSourceSystemPrefix, IMPORT_CONNECTOR
// of migration's ConnectorCSV, IMPORT_OBJECTS of migration's ImportOrder. A drift
// in either direction fails, and so does a second hand-typed namespace.

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const (
	frontendSrc           = "../frontend/src"
	frontendImportGrammar = "../frontend/src/design-system/provenanceof.ts"
	// The importer's owner, read as source: gates may not import a module.
	migrationModuleDir = "internal/modules/migration"
)

// importerGrammar reads ConnectorCSV and the ImportOrder slice out of the
// migration module's own files, so the gate compares against what it ships.
func importerGrammar(t *testing.T) (connector string, objects []string) {
	t.Helper()
	fset := token.NewFileSet()
	files := parsePackageDir(t, fset, migrationModuleDir)
	consts := stringConsts(t, fset, files)
	connector, found := consts["ConnectorCSV"]
	if !found || connector == "" {
		t.Fatalf("%s declares no string constant ConnectorCSV: the importer's connector moved, so check the browser's IMPORT_CONNECTOR moved with it", migrationModuleDir)
	}
	objects, err := stringSliceVar(files, "ImportOrder", consts)
	if err != nil {
		t.Fatalf("%s: %v", migrationModuleDir, err)
	}
	return connector, objects
}

// stringSliceVar resolves `var <name> = []string{...}`, each element a string
// literal or a string constant. An element it cannot resolve, or an empty
// list, is an error: a short census would compare against fewer objects.
func stringSliceVar(files []*ast.File, name string, consts map[string]string) ([]string, error) {
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
					continue
				}
				return stringSliceElements(value.Values[0], name, consts)
			}
		}
	}
	return nil, fmt.Errorf("no `var %s = []string{...}` found", name)
}

func stringSliceElements(expr ast.Expr, name string, consts map[string]string) ([]string, error) {
	lit, isLit := expr.(*ast.CompositeLit)
	if !isLit {
		return nil, fmt.Errorf("%s is not a slice literal", name)
	}
	var values []string
	for _, elt := range lit.Elts {
		switch e := elt.(type) {
		case *ast.Ident:
			value, found := consts[e.Name]
			if !found {
				return nil, fmt.Errorf("%s names %s, which is no string constant of the package", name, e.Name)
			}
			values = append(values, value)
		case *ast.BasicLit:
			value, err := strconv.Unquote(e.Value)
			if err != nil || e.Kind != token.STRING {
				return nil, fmt.Errorf("%s holds %s, which is not a string", name, e.Value)
			}
			values = append(values, value)
		default:
			return nil, fmt.Errorf("%s holds an element this gate cannot resolve", name)
		}
	}
	if len(values) == 0 {
		return nil, errors.New(name + " is empty: a census of nothing agrees with everything")
	}
	return values, nil
}

// tsStringConst reads `const <name> = "<value>"` in any quote a string literal
// admits, so a re-quoted declaration is still read instead of counted as gone.
func tsStringConst(source, name string) []string {
	decl := regexp.MustCompile(`const ` + name + "\\s*=\\s*[\"'`]([^\"'`]*)[\"'`]")
	var values []string
	for _, m := range decl.FindAllStringSubmatch(tsComment.ReplaceAllString(source, " "), -1) {
		values = append(values, m[1])
	}
	return values
}

var (
	tsImportObjects = regexp.MustCompile(`const IMPORT_OBJECTS\s*=\s*new Set\(\[([^\]]*)\]\)`)
	tsQuoted        = regexp.MustCompile("[\"'`]([^\"'`]*)[\"'`]")
)

// tsImportObjectSet reads the Set's members. Anything in the brackets that is
// not a quoted word (a spread, a constant) is refused. A member this reader
// cannot see is one the comparison would silently agree with.
func tsImportObjectSet(source string) ([]string, error) {
	matches := tsImportObjects.FindAllStringSubmatch(tsComment.ReplaceAllString(source, " "), -1)
	if len(matches) != 1 {
		return nil, fmt.Errorf("IMPORT_OBJECTS declared %d times as new Set([...]), want once", len(matches))
	}
	body := matches[0][1]
	var members []string
	for _, m := range tsQuoted.FindAllStringSubmatch(body, -1) {
		members = append(members, m[1])
	}
	if residue := strings.Trim(tsQuoted.ReplaceAllString(body, ""), " ,\n\t"); residue != "" {
		return nil, fmt.Errorf("IMPORT_OBJECTS holds %q, which is not a quoted object name", residue)
	}
	return members, nil
}

// namespaceCopies counts literals that open with the namespace word: a string,
// a template, a regexp source or a regexp literal, anchored or not. Each is a
// predicate on the stamp the declared mirror no longer governs.
func namespaceCopies(source, prefix string) int {
	word := regexp.QuoteMeta(strings.TrimSuffix(prefix, ":"))
	copyOf := regexp.MustCompile("[\"'`/]\\^?" + word + "(?:" + regexp.QuoteMeta(":") + "|[\"'`/])")
	return len(copyOf.FindAllStringIndex(tsComment.ReplaceAllString(source, " "), -1))
}

// isProductTypeScript keeps tests and stories out of the census. Both may hold
// a stamp as data (leadreadings.stories.tsx renders a `source_system` value)
// and neither decides what a reader of the product sees.
func isProductTypeScript(path string) bool {
	return (strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) &&
		!strings.Contains(path, ".test.") && !strings.Contains(path, ".stories.")
}

func TestTheFrontendImportGrammarMirrorsTheImporter(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(frontendImportGrammar)
	if err != nil {
		t.Fatalf("read %s: %v", frontendImportGrammar, err)
	}
	source := string(raw)
	connector, importOrder := importerGrammar(t)
	for _, mirror := range []struct{ name, server, cost string }{
		{"IMPORT_NAMESPACE", provenance.ReservedSourceSystemPrefix, "an imported record would read as typed by the administrator who ran the import"},
		{"IMPORT_CONNECTOR", connector, "every imported record would lose its upload's name"},
	} {
		got := tsStringConst(source, mirror.name)
		if len(got) != 1 {
			t.Errorf("%s declares %s %d times, want once", frontendImportGrammar, mirror.name, len(got))
			continue
		}
		if got[0] != mirror.server {
			t.Errorf("%s disagrees: server %q, browser %q — %s", mirror.name, mirror.server, got[0], mirror.cost)
		}
	}
	objects, err := tsImportObjectSet(source)
	if err != nil {
		t.Fatalf("%s: %v", frontendImportGrammar, err)
	}
	for _, object := range importOrder {
		if !slices.Contains(objects, object) {
			t.Errorf("IMPORT_OBJECTS lacks %q from ImportOrder: a record of it would lose its upload's name", object)
		}
	}
	for _, object := range objects {
		if !slices.Contains(importOrder, object) {
			t.Errorf("IMPORT_OBJECTS holds %q, which ImportOrder does not import: the browser reads a stamp no importer writes", object)
		}
	}
}

func TestTheImportNamespaceIsTypedOnceInTheFrontend(t *testing.T) {
	t.Parallel()
	copies := map[string]int{}
	walked := 0
	walkErr := filepath.WalkDir(frontendSrc, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isProductTypeScript(path) {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		walked++
		if n := namespaceCopies(string(body), provenance.ReservedSourceSystemPrefix); n > 0 {
			copies[filepath.ToSlash(path)] = n
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %s: %v", frontendSrc, walkErr)
	}
	if walked == 0 {
		t.Fatalf("no TypeScript read under %s — a census of nothing agrees with everything", frontendSrc)
	}
	if copies[frontendImportGrammar] != 1 || len(copies) != 1 {
		t.Errorf("the namespace %q must be typed once in frontend/src product code, as IMPORT_NAMESPACE in %s; found %v. Call importProvenance instead of testing the stamp again",
			provenance.ReservedSourceSystemPrefix, frontendImportGrammar, copies)
	}
}

func TestTheImportGrammarReaderSeesQuotedDeclarationsAndPredicateCopies(t *testing.T) {
	t.Parallel()
	prefix := provenance.ReservedSourceSystemPrefix
	cases := []struct {
		name     string
		source   string
		declared int
		copies   int
	}{
		{"double-quoted declaration", `const IMPORT_NAMESPACE = "` + prefix + `";`, 1, 1},
		{"single-quoted declaration", `const IMPORT_NAMESPACE = '` + prefix + `';`, 1, 1},
		{"backtick declaration", "const IMPORT_NAMESPACE = `" + prefix + "`;", 1, 1},
		{"declaration only in a comment", "// const IMPORT_NAMESPACE = \"" + prefix + "\";\n/* \"" + prefix + "\" */", 0, 0},
		{"startsWith on a longer stamp", `s.startsWith("` + prefix + `csv:")`, 0, 1},
		{"anchored regexp literal", `/^` + prefix + `/.test(s)`, 0, 1},
		{"regexp built from a string", `new RegExp("^` + prefix + `csv")`, 0, 1},
		{"template literal", "s === `" + prefix + "csv:${name}`", 0, 1},
		{"the word without its colon", `s.split(":")[0] === "` + strings.TrimSuffix(prefix, ":") + `"`, 0, 1},
		{"a longer word is not the namespace", `const label = "` + strings.TrimSuffix(prefix, ":") + `ed";`, 0, 0},
	}
	for _, c := range cases {
		if got := len(tsStringConst(c.source, "IMPORT_NAMESPACE")); got != c.declared {
			t.Errorf("%s: read %d declarations, want %d", c.name, got, c.declared)
		}
		if got := namespaceCopies(c.source, prefix); got != c.copies {
			t.Errorf("%s: counted %d copies, want %d", c.name, got, c.copies)
		}
	}
	if got, err := tsImportObjectSet(`const IMPORT_OBJECTS = new Set(["company", 'lead']);`); err != nil || !slices.Equal(got, []string{"company", "lead"}) {
		t.Errorf("quoted members: read %v, %v", got, err)
	}
	if _, err := tsImportObjectSet(`const IMPORT_OBJECTS = new Set(["company", ...MORE]);`); err == nil {
		t.Error("a spread member was read as nothing instead of refused")
	}
	for _, c := range []struct{ name, source, err string }{
		{"constants and literals", `package m; const A = "a"; var Order = []string{A, "b"}`, ""},
		{"an unknown constant", `package m; var Order = []string{Missing}`, "no string constant"},
		{"an empty list", `package m; var Order = []string{}`, "empty"},
		{"no such var", `package m; var Other = []string{"a"}`, "no `var Order"},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), c.name+".go", c.source, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, err := stringSliceVar([]*ast.File{file}, "Order", map[string]string{"A": "a"})
		if c.err == "" && (err != nil || !slices.Equal(got, []string{"a", "b"})) {
			t.Errorf("%s: read %v, %v", c.name, got, err)
		}
		if c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: error %v, want one naming %q", c.name, err, c.err)
		}
	}
	for path, want := range map[string]bool{"a/b.ts": true, "a/b.tsx": true, "a/b.stories.tsx": false, "a/b.test.ts": false, "a/b.md": false} {
		if got := isProductTypeScript(path); got != want {
			t.Errorf("isProductTypeScript(%q) = %v, want %v", path, got, want)
		}
	}
}
