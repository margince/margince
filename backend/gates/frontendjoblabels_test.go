// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// History names the system pass behind a change by its words, never by its
// key. A row carries the principal the pass acted as, which is not always its
// job kind, so there is one `systemJob.<name>` message per bound principal.
// A principal without one reads as a bare "System task", and a message no
// principal earns is dead, so both directions fail.

import (
	"go/ast"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendCatalogue = "../frontend/src/i18n/en.ts"

// tsJobLabelKey reads one `"systemJob.<name>":` key out of the English
// catalogue. Comments are stripped first (tsComment), so a line that only
// mentions a key cannot stand in for one that was deleted.
var tsJobLabelKey = regexp.MustCompile(`"systemJob\.([a-z0-9_]+)"\s*:`)

// systemPrincipalText is a string that is a whole system principal.
var systemPrincipalText = regexp.MustCompile(`^system:[a-z0-9_.-]+$`)

func TestEverySystemPrincipalHasAFrontendLabel(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendCatalogue)
	if err != nil {
		t.Fatalf("reading the frontend catalogue: %v", err)
	}
	labelled := map[string]bool{}
	for _, m := range tsJobLabelKey.FindAllStringSubmatch(tsComment.ReplaceAllString(string(source), " "), -1) {
		labelled[m[1]] = true
	}

	bound := boundSystemPrincipals(t)
	if len(bound) == 0 || len(labelled) == 0 {
		t.Fatalf("read %d system principals and %d labels — a census that reads nothing agrees with everything", len(bound), len(labelled))
	}
	for _, name := range sortedKeys(bound) {
		if !labelled[name] {
			t.Errorf("%s binds system principal %q, and %s has no \"systemJob.%s\" message (add it to de.ts and vi.ts too), so its changes read as a bare \"System task\" in History", bound[name], name, frontendCatalogue, name)
		}
	}
	for _, name := range sortedKeys(labelled) {
		if _, ok := bound[name]; !ok {
			t.Errorf("%s labels systemJob.%s, which no system principal in the backend binds — delete the message or bind the principal", frontendCatalogue, name)
		}
	}
}

// labelKey is the frontend's reading of a principal's name (systemJobLabel in
// provenance.tsx): the prefix dropped, and hyphens read as underscores.
func labelKey(principalName string) string {
	return strings.ReplaceAll(strings.TrimPrefix(principalName, "system:"), "-", "_")
}

// boundSystemPrincipals reads the named system principals the backend can
// stamp on a row, keyed by label key, valued by a file that binds it. Two readings
// make it: every string that is a whole `system:<name>`, and every name handed
// to principal.SystemActing, which also takes names without the prefix.
func boundSystemPrincipals(t *testing.T) map[string]string {
	t.Helper()
	files := gatekit.Scope{
		Roots:   []string{"internal", "cmd"},
		Subject: namesASystemPrincipal,
	}.Files(t)
	found := map[string]string{}
	consts := packageConsts{t: t, byDir: map[string]map[string]string{}}
	for _, parsed := range files {
		ast.Inspect(parsed.File, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				if text, err := strconv.Unquote(lit.Value); err == nil && systemPrincipalText.MatchString(text) {
					found[labelKey(text)] = parsed.Path
				}
			}
			return true
		})
		for _, name := range systemActingNames(t, parsed, consts) {
			// A bare "system" names no pass, and "agent:…" is not a system
			// principal whatever helper bound it.
			if name != "system" && !strings.Contains(strings.TrimPrefix(name, "system:"), ":") {
				found[labelKey(name)] = parsed.Path
			}
		}
	}
	return found
}

func namesASystemPrincipal(_ string, file *ast.File) bool {
	if gatekit.References(file, principalPath, systemActingHelper) {
		return true
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok {
			text, err := strconv.Unquote(lit.Value)
			found = found || (err == nil && systemPrincipalText.MatchString(text))
		}
		return !found
	})
	return found
}

// systemActingNames reads the name each principal.SystemActing call in the
// file binds. A name handed through a helper's parameter is read at every call
// of that helper in its package. Any other name this cannot read fails: a
// principal the census cannot see is one History cannot name.
func systemActingNames(t *testing.T, parsed gatekit.ParsedFile, consts packageConsts) []string {
	t.Helper()
	qualifier, _ := gatekit.ImportedAs(parsed.File, principalPath)
	if qualifier == "" {
		return nil
	}
	var names []string
	for _, fn := range functionsIn(parsed.File) {
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !selects(call.Fun, qualifier, systemActingHelper) || len(call.Args) != 2 {
				return true
			}
			if name, ok := consts.read(parsed, call.Args[1]); ok {
				names = append(names, name)
				return true
			}
			passed, ok := parameterIndex(fn, call.Args[1])
			if !ok {
				t.Errorf("%s: %s binds a system principal this census cannot read; name it with a string constant", parsed.Path, fn.Name.Name)
				return true
			}
			names = append(names, helperArguments(t, parsed, consts, fn.Name.Name, passed)...)
			return true
		})
	}
	return names
}

// helperArguments reads argument `index` of every call to helper in the
// package beside parsed.
func helperArguments(t *testing.T, parsed gatekit.ParsedFile, consts packageConsts, helper string, index int) []string {
	t.Helper()
	dir := packageDir(parsed)
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	var names []string
	calls := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(source, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		sibling := gatekit.ParsedFile{Path: parsed.Path, File: file}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !selects(call.Fun, "", helper) || len(call.Args) <= index {
				return true
			}
			calls++
			if name, ok := consts.read(sibling, call.Args[index]); ok {
				names = append(names, name)
			} else {
				t.Errorf("%s: a call to %s hands it a system principal this census cannot read; name it with a string constant", source, helper)
			}
			return true
		})
	}
	if calls == 0 {
		t.Errorf("%s passes its parameter to principal.SystemActing and nothing in its package calls it, so the census read no principal from it", helper)
	}
	return names
}

func parameterIndex(fn *ast.FuncDecl, arg ast.Expr) (int, bool) {
	ident, ok := arg.(*ast.Ident)
	if !ok {
		return 0, false
	}
	index := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == ident.Name {
				return index, true
			}
			index++
		}
	}
	return 0, false
}

func packageDir(parsed gatekit.ParsedFile) string {
	return filepath.FromSlash(path.Dir(parsed.Path))
}

// packageConsts reads a string expression through the constants of its own
// package, or of the module package a selector names.
type packageConsts struct {
	t     *testing.T
	byDir map[string]map[string]string
}

// of reads every string constant in dir, including one spelled as a
// concatenation of others (`"agent:" + verdictReason`), which
// gatekit.PackageStringConstants leaves out.
func (c packageConsts) of(dir string) map[string]string {
	if table, ok := c.byDir[dir]; ok {
		return table
	}
	table := gatekit.PackageStringConstants(c.t, dir)
	pending := constExprs(c.t, dir)
	for progress := true; progress; {
		progress = false
		for name, expr := range pending {
			if text, ok := gatekit.StringExpr(expr, table, gatekit.FoldStrict); ok {
				table[name] = text
				delete(pending, name)
				progress = true
			}
		}
	}
	c.byDir[dir] = table
	return table
}

func constExprs(t *testing.T, dir string) map[string]ast.Expr {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	out := map[string]ast.Expr{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(source, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		for _, decl := range file.Decls {
			general, isGeneral := decl.(*ast.GenDecl)
			if !isGeneral || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok && len(value.Names) == 1 && len(value.Values) == 1 {
					out[value.Names[0].Name] = value.Values[0]
				}
			}
		}
	}
	return out
}

func (c packageConsts) read(parsed gatekit.ParsedFile, expr ast.Expr) (string, bool) {
	sel, isSelector := expr.(*ast.SelectorExpr)
	if !isSelector {
		return gatekit.StringExpr(expr, c.of(packageDir(parsed)), gatekit.FoldStrict)
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return "", false
	}
	dir, found := moduleImportDir(parsed.File, pkg.Name)
	if !found {
		return "", false
	}
	text, known := c.of(dir)[sel.Sel.Name]
	return text, known
}

// moduleImportDir maps the package a file imports as `name` onto its
// directory, for an import inside this module.
func moduleImportDir(file *ast.File, name string) (string, bool) {
	const module = "github.com/margince/margince/backend/"
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(importPath, module) {
			continue
		}
		asName := gatekit.DeclaredPackageName(importPath)
		if spec.Name != nil {
			asName = spec.Name.Name
		}
		if asName == name {
			return filepath.FromSlash(strings.TrimPrefix(importPath, module)), true
		}
	}
	return "", false
}
