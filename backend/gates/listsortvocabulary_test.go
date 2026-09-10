// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// A sort the list OFFERS is a sort the server ACCEPTS.
//
// Tagged `!integration` with the sibling reference census, whose AST helpers it
// shares: reaching for them from a build they are not in is a typecheck failure
// the merge gate finds and the default `go test` does not.
//
// A column header carries the field name it will ask for. The server keeps a
// per-resource vocabulary and refuses anything outside it, so the two are one
// invariant spelled on both sides of a wire — and the drift is silent until a
// reader clicks, because nothing renders differently until the request is made.
//
// This is #2090's own last ask: "a column that declares a sort the server does
// not accept should fail a test, not silently render a dead header." What that
// issue found was the other direction — columns with no sort at all — and the
// ruling behind it (any column shown is sortable) is answered column by column
// as the sort model grows. THIS direction needs no ruling and no waiver: a
// header offering a field the resource does not have is a refusal in front of a
// user, whatever anybody decides about the columns that offer nothing.
//
// The Go side is read with its CONSTANTS RESOLVED. Several vocabularies spell
// their keys as package constants (`ownerIDColumn`, `personNameColumn`), so a
// scan reading string literals finds an EMPTY vocabulary and agrees with every
// sort a screen could possibly offer. That is the failure direction a census
// must not have, and this one had it until the constants were followed.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// listSurface pairs a screen with the resource it lists.
//
// Declared because nothing in either tree names the other: a screen fetches a
// path and the vocabulary belongs to a store, and no import, type or route
// connects the two. Every entry is checked to resolve on both sides, so a
// renamed file or a retired vocabulary fails here rather than dropping a
// surface out of the census quietly.
var listSurfaces = map[string]struct{ vocabulary, source string }{
	"deals.tsx":          {"dealListFields", "internal/modules/deals/deal_read.go"},
	"contacts.tsx":       {"personListFields", "internal/modules/people/person_list.go"},
	"leads.list.tsx":     {"leadListFields", "internal/modules/people/lead_list.go"},
	"organizations.tsx":  {"organizationListFields", "internal/modules/people/organization_list.go"},
	"projects.tsx":       {"projectListFields", "internal/modules/projects/read.go"},
	"products.tsx":       {"productListFields", "internal/modules/deals/product.go"},
	"offertemplates.tsx": {"offerTemplateListFields", "internal/modules/deals/offer_template.go"},
}

const screenDir = "../frontend/src/screens/"

// sharedColumnSource holds the column helpers several screens draw from. A sort
// one of them offers is offered by every screen that calls it, so it is read
// per screen rather than once.
const sharedColumnSource = screenDir + "recordlist.tsx"

var (
	// tsSortLiteral reads one `sort: "field"` off a column.
	tsSortLiteral = regexp.MustCompile(`\bsort:\s*"([a-z_0-9]+)"`)
	// tsColumnHelper reads one exported column helper's name.
	tsColumnHelper = regexp.MustCompile(`export function ([a-zA-Z]+Column)<`)
)

func TestEverySortAListOffersIsOneItsResourceAccepts(t *testing.T) {
	t.Parallel()

	helpers := sharedColumnSorts(t)
	if len(helpers) == 0 {
		t.Fatal("read no shared column helpers at all — every screen's borrowed sorts would then go unchecked")
	}

	screens := make([]string, 0, len(listSurfaces))
	for name := range listSurfaces {
		screens = append(screens, name)
	}
	sort.Strings(screens)

	for _, name := range screens {
		surface := listSurfaces[name]
		accepted := goSortVocabulary(t, surface.source, surface.vocabulary)
		if len(accepted) == 0 {
			t.Errorf("%s resolves to an EMPTY vocabulary — an empty one accepts nothing and this gate would agree with everything; the keys are probably constants this scan did not follow",
				surface.vocabulary)
			continue
		}
		screen := readSource(t, screenDir+name)

		offered := map[string]string{}
		for _, m := range tsSortLiteral.FindAllStringSubmatch(screen, -1) {
			offered[m[1]] = "its own column"
		}
		// The sorts this screen borrows. A helper's header is this screen's
		// header once it is drawn here, so its field is this resource's to
		// accept.
		for helper, field := range helpers {
			if strings.Contains(screen, helper+"<") {
				offered[field] = helper + "()"
			}
		}

		fields := make([]string, 0, len(offered))
		for field := range offered {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			if !slices.Contains(accepted, field) {
				t.Errorf("%s offers sort %q (from %s) and %s does not accept it — the header renders, the reader clicks, and the request is refused",
					name, field, offered[field], surface.vocabulary)
			}
		}
	}
}

// sharedColumnSorts maps each shared helper to the sort it offers, skipping the
// ones that offer none.
func sharedColumnSorts(t *testing.T) map[string]string {
	t.Helper()
	body := readSource(t, sharedColumnSource)
	names := tsColumnHelper.FindAllStringSubmatchIndex(body, -1)
	out := map[string]string{}
	for i, at := range names {
		end := len(body)
		if i+1 < len(names) {
			end = names[i+1][0]
		}
		helper, definition := body[at[2]:at[3]], body[at[0]:end]
		if m := tsSortLiteral.FindStringSubmatch(definition); m != nil {
			out[helper] = m[1]
			continue
		}
		// A helper that MENTIONS a sort this scan cannot read is the failure
		// direction that matters: the field goes unattributed, every screen
		// drawing it goes unchecked, and the gate reports PASS over exactly the
		// header it exists to catch. A `sort:` computed from a flag did this
		// once already — the mutation that should have failed came back green.
		if strings.Contains(definition, "sort:") {
			t.Fatalf("%s writes a sort this gate cannot read as a literal — a computed one leaves every screen that draws it unchecked, which reads as agreement. Give the column a plain `sort: \"field\"`, or split the cell out for the caller that must not offer one",
				helper)
		}
	}
	return out
}

// goSortVocabulary reads a vocabulary map's keys, resolving a key spelled as a
// package constant to the string it holds.
func goSortVocabulary(t *testing.T, source, name string) []string {
	t.Helper()
	constants := packageStringConstants(t, filepath.Dir(source))
	file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}
	var out []string
	for _, spec := range valueSpecsNamed(file, name) {
		lit, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			t.Fatalf("%s is not a composite literal; this gate can only read one", name)
		}
		for _, entry := range lit.Elts {
			kv, ok := entry.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			switch key := kv.Key.(type) {
			case *ast.BasicLit:
				unquoted, err := strconv.Unquote(key.Value)
				if err != nil {
					t.Fatalf("unquoting %s in %s: %v", key.Value, source, err)
				}
				out = append(out, unquoted)
			case *ast.Ident:
				value, declared := constants[key.Name]
				if !declared {
					t.Fatalf("%s names %s, which its package declares no string constant for — an unresolved key is a field this gate cannot see, and it would then agree with a screen offering it",
						name, key.Name)
				}
				out = append(out, value)
			default:
				t.Fatalf("%s has a key this gate cannot read (%T)", name, kv.Key)
			}
		}
	}
	return out
}

// packageStringConstants reads every string constant one package declares.
func packageStringConstants(t *testing.T, dir string) map[string]string {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				if lit, ok := value.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					unquoted, err := strconv.Unquote(lit.Value)
					if err != nil {
						t.Fatalf("unquoting %s in %s: %v", lit.Value, source, err)
					}
					out[value.Names[0].Name] = unquoted
				}
			}
		}
	}
	return out
}
