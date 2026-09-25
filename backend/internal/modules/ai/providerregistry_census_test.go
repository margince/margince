// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A per-provider fact written anywhere but the registry is a second list, and a
// second list is how a provider comes to be accepted by one question and
// unknown to the next. This census reads the package's own source: a map entry
// keyed by a provider, or a switch case on one, outside providerregistry.go
// fails it.
//
// Two functions legitimately switch on a provider word and are named in
// providerSwitchAllowed with the reason, so a third has to argue its way into
// that list in a diff:
//   - selectBrainOn builds each adapter: a construction recipe is code, not a
//     fact, and TestLocalOnlyMatchesLocalProvidersForEveryProvider already
//     fails a registry row with no recipe.
//   - NewPublicProfile switches on the RUNTIME STATE "fake", which shares a
//     spelling with the fake provider and is not a provider at all.
//
// One map is keyed by a vendor word by contract and is not a table across
// providers: model.Response.ProviderMetadata, where an adapter files its own
// vendor-only outputs under its own namespace. A literal assigned to it is
// skipped; the same literal assigned anywhere else is not.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var providerSwitchAllowed = map[string]string{
	"selectBrainOn":    "builds each adapter; a recipe per provider is the switch's job",
	"NewPublicProfile": `switches on the runtime state "fake", not on a provider`,
}

func TestProviderFactsLiveOnlyInTheRegistry(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed[name] = f
	}
	idents := providerConstIdents(parsed)
	// Every provider must be recognisable by its constant, or a table keyed by
	// that constant reads as clean. A second constant sharing a provider's
	// spelling (a ProviderOptions namespace) is read as a provider too: that
	// errs toward a finding, never toward silence.
	for _, provider := range KnownProviders() {
		if !slices.Contains(slices.Collect(maps.Values(idents)), provider) {
			t.Fatalf("no constant spells provider %q — the census would read a smaller tree than it claims", provider)
		}
	}
	for name, f := range parsed {
		if name == "providerregistry.go" {
			continue
		}
		for _, hit := range providerFactSites(f, idents) {
			t.Errorf("%s:%d: %s — declare it as a providerDescriptor field and project it", name, fset.Position(hit.pos).Line, hit.what)
		}
	}
}

// The detector must fire on every shape it claims, or the census above reads
// green over the list it exists to stop.
func TestTheProviderCensusFiresOnEachShape(t *testing.T) {
	idents := map[string]string{"providerGemini": providerGemini}
	cases := map[string]string{
		"map key by constant": `package ai; var m = map[string]int{providerGemini: 1}`,
		"map key by literal":  `package ai; var m = map[string]int{"gemini": 1}`,
		"case by constant":    `package ai; func f(p string) { switch p { case providerGemini: } }`,
		"case by literal":     `package ai; func f(p string) { switch p { case "gemini": } }`,
	}
	for label, src := range cases {
		f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(providerFactSites(f, idents)) == 0 {
			t.Errorf("%s: census found nothing in %q", label, src)
		}
	}
	cases["map key assigned to a field that is not ProviderMetadata"] = `package ai; func f(r *R) { r.Other = map[string]int{"gemini": 1} }`
	quiet := `package ai; func selectBrainOn(p string) { switch p { case providerGemini: } }; var m = map[string]int{"other": 1}
func g(r *R, meta []byte) { r.ProviderMetadata = map[string][]byte{"gemini": meta} }`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", quiet, 0)
	if err != nil {
		t.Fatal(err)
	}
	if hits := providerFactSites(f, idents); len(hits) != 0 {
		t.Errorf("census fired on an allowed switch or an unrelated key: %v", hits)
	}
}

type providerFactSite struct {
	pos  token.Pos
	what string
}

// providerNameSet is the provider words as a set, read from the registry so the
// census recognises a provider the day its row is added.
func providerNameSet() map[string]bool {
	names := map[string]bool{}
	for _, p := range KnownProviders() {
		names[p] = true
	}
	return names
}

// providerConstIdents maps each constant whose value is a provider name to that
// name, found in the source rather than listed here, so a new provider constant
// is read the day it is declared.
func providerConstIdents(files map[string]*ast.File) map[string]string {
	names := providerNameSet()
	out := map[string]string{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, v := range spec.Values {
				if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil && names[s] {
						out[spec.Names[i].Name] = s
					}
				}
			}
			return true
		})
	}
	return out
}

func providerFactSites(f *ast.File, idents map[string]string) []providerFactSite {
	names := providerNameSet()
	isProvider := func(e ast.Expr) bool {
		switch v := e.(type) {
		case *ast.Ident:
			return idents[v.Name] != ""
		case *ast.BasicLit:
			s, err := strconv.Unquote(v.Value)
			return v.Kind == token.STRING && err == nil && names[s]
		default:
			return false
		}
	}
	var hits []providerFactSite
	scan := func(root ast.Node, fn string) {
		ast.Inspect(root, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.AssignStmt:
				return !assignsProviderMetadata(v)
			case *ast.KeyValueExpr:
				if isProvider(v.Key) {
					hits = append(hits, providerFactSite{v.Pos(), "a table keyed by provider"})
				}
			case *ast.CaseClause:
				if _, allowed := providerSwitchAllowed[fn]; allowed {
					return true
				}
				for _, e := range v.List {
					if isProvider(e) {
						hits = append(hits, providerFactSite{e.Pos(), "a switch case on a provider"})
					}
				}
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			scan(fd, fd.Name.Name)
			continue
		}
		scan(decl, "")
	}
	return hits
}

// assignsProviderMetadata reports whether stmt writes model.Response's
// ProviderMetadata, whose keys are the writing adapter's own vendor namespace.
func assignsProviderMetadata(stmt *ast.AssignStmt) bool {
	for _, lhs := range stmt.Lhs {
		if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "ProviderMetadata" {
			return true
		}
	}
	return false
}
