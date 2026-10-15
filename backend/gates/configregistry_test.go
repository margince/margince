// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every setting declared in the tree is one some program loads. A setting no
// registry holds is read by nothing: an operator sets it, and it does nothing.
// The census parses the source rather than asking the registries, because a
// declaration the registries miss is what it exists to find.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	configImport   = "github.com/margince/margince/backend/internal/platform/config"
	settingsImport = "github.com/margince/margince/backend/internal/platform/settings"
	cliflagsImport = "github.com/margince/margince/backend/internal/platform/cliflags"
)

// settingsDefineFiles is how many files still declare a stored setting with
// settings.Define. It only goes down: each module that moves to
// config.DefineStored lowers it, and 0 retires the rule's pin.
const settingsDefineFiles = 28

// declaredSetting is one package-level var a config.Define or DefineStored
// call initialises.
type declaredSetting struct{ pos, name, key string }

type configCensus struct {
	settings      []declaredSetting
	defineFiles   []string
	cliflagsUsers []string
	files         int
}

func TestEverySettingIsInSomeProgram(t *testing.T) {
	t.Parallel()
	census := takeConfigCensus(t)
	if census.files == 0 || len(census.settings) == 0 {
		t.Fatalf("read %d files and found %d settings — a census that finds nothing passes like a clean tree", census.files, len(census.settings))
	}
	registered := map[string]bool{}
	for _, reg := range compose.ProgramRegistries() {
		for _, s := range reg.All() {
			registered[s.Key()] = true
		}
	}
	for _, s := range census.settings {
		switch {
		case s.key == "":
			t.Errorf("%s: %s declares its Key with an expression; write it as a string literal so this census can read it", s.pos, s.name)
		case !registered[s.key]:
			t.Errorf("%s: %s (%s) is declared but no program registers it", s.pos, s.name, s.key)
		}
	}
	for _, pos := range census.cliflagsUsers {
		t.Errorf("%s: imports platform/cliflags, which is retired: declare a platform/config setting instead", pos)
	}
}

// settings.Define is internal to platform/config; a module declares a stored
// setting with config.DefineStored. Pinned while modules move, so the rule
// fails the day a file is added and asks for the pin to drop the day one goes.
func TestSettingsDefineStaysInsidePlatformConfig(t *testing.T) {
	t.Parallel()
	census := takeConfigCensus(t)
	switch got := len(census.defineFiles); {
	case got > settingsDefineFiles:
		t.Errorf("%d files call settings.Define, %d allowed. settings.Define is internal; declare with config.DefineStored:\n\t%s",
			got, settingsDefineFiles, strings.Join(census.defineFiles, "\n\t"))
	case got < settingsDefineFiles:
		t.Errorf("%d files call settings.Define and the pin allows %d: lower settingsDefineFiles to %d", got, settingsDefineFiles, got)
	}
}

func takeConfigCensus(t *testing.T) configCensus {
	t.Helper()
	var census configCensus
	fset := gatekit.SourceFileSet()
	walkHandWrittenGoFiles(t, ".", func(path, text string) {
		if strings.HasSuffix(path, "_test.go") || strings.HasPrefix(path, "internal/contracts/") {
			return
		}
		file, err := parser.ParseFile(fset, path, text, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		census.files++
		names := importNames(t, file)
		if _, ok := names[cliflagsImport]; ok {
			census.cliflagsUsers = append(census.cliflagsUsers, path)
		}
		census.settings = append(census.settings, packageSettings(file, names[configImport], fset.Position)...)
		if settingsName, ok := names[settingsImport]; ok && !strings.HasPrefix(path, "internal/platform/config/") && callsDefine(file, settingsName) {
			census.defineFiles = append(census.defineFiles, path)
		}
	})
	slices.Sort(census.defineFiles)
	return census
}

// importNames maps each import path to the name the file uses for it.
func importNames(t *testing.T, file *ast.File) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("an import path the parser accepted does not unquote: %v", err)
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		out[path] = name
	}
	return out
}

// packageSettings are the file's package-level vars whose value is a call to
// pkg.Define or pkg.DefineStored, with the Key their Spec literal spells.
func packageSettings(file *ast.File, pkg string, position func(p token.Pos) token.Position) []declaredSetting {
	if pkg == "" {
		return nil
	}
	var out []declaredSetting
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, expr := range value.Values {
				call, ok := expr.(*ast.CallExpr)
				if !ok || !callsFunc(call.Fun, pkg, "Define", "DefineStored") || len(call.Args) == 0 || i >= len(value.Names) {
					continue
				}
				out = append(out, declaredSetting{
					pos: position(value.Names[i].Pos()).String(), name: value.Names[i].Name, key: specKey(call.Args[0]),
				})
			}
		}
	}
	return out
}

// callsFunc matches pkg.Name and the instantiated pkg.Name[T].
func callsFunc(fun ast.Expr, pkg string, names ...string) bool {
	switch typed := fun.(type) {
	case *ast.IndexExpr:
		fun = typed.X
	case *ast.IndexListExpr:
		fun = typed.X
	}
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && slices.Contains(names, sel.Sel.Name)
}

// specKey is the Key a Spec composite literal spells, or "" when it is not a
// string literal.
func specKey(arg ast.Expr) string {
	lit, ok := arg.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	for _, element := range lit.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := field.Key.(*ast.Ident); !ok || key.Name != "Key" {
			continue
		}
		if basic, ok := field.Value.(*ast.BasicLit); ok && basic.Kind == token.STRING {
			value, err := strconv.Unquote(basic.Value)
			if err == nil {
				return value
			}
		}
	}
	return ""
}

func callsDefine(file *ast.File, settingsName string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && callsFunc(call.Fun, settingsName, "Define") {
			found = true
		}
		return !found
	})
	return found
}
