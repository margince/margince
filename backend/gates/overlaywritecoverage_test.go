// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H2

package gates

// Every field a caller may PATCH either projects onto the incumbent or says why
// it does not.
//
// THE DEFECT THIS REPLACES. A canonical field absent from an overlay write
// projection is not refused — it is accepted by the door and never sent. The
// next overlay read returns the old value, so to a user the edit looks like
// somebody else reverted it.
//
// It happened three times over on organization: legal_name, linkedin_url and
// description were each simply never added when their column landed. Three
// column changes failing to notice one obligation is one missing rule, not
// three mistakes — which is why this is a rule and not three entries. Measured
// while writing it, the gap was wider than the report: ten of organization's
// twelve writable fields were unanswered for, and person and lead had never
// been asked at all.
//
// THE CORPUS IS THE CONTRACT, not the schema. `organization` carries 65 columns
// and most are ours alone — geocode state, logo keys, legal_hold. What makes a
// field's absence a DEFECT is that the published contract invites a caller to
// write it, so each update operation's own request schema is the set that has
// to be answered for, and a field the contract adds joins the obligation by
// being added.
//
// It lives HERE rather than beside the mapping because the overlay component
// may not import the generated contract package (go-arch-lint refuses it), and
// the contract is the half that makes this a promise rather than a preference.
//
// The writable set is read off internal/contracts' GENERATED request structs —
// their json tags are what the door decodes, so they are the same promise
// crm.yaml makes, in the form the server actually applies. Reading the YAML
// instead would mean a second parser for a fact the generator already resolved.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// writeProjections pairs each canonical class's update operation with the two
// declarations in mapwrite.go that must together answer for it.
//
// A table rather than three tests, because the three fail the same way — and a
// class missing from it is the shape that let organization drift while person
// and lead were never asked.
var writeProjections = []struct {
	canonical, projects, deferred string
	// request is the generated struct the door decodes an update into. Its
	// json tags ARE the writable set.
	request any
}{
	{"organization", "organizationWriteFields", "deferredOrganizationWrites", crmcontracts.UpdateOrganizationRequest{}},
	{"person", "personWriteFields", "deferredPersonWrites", crmcontracts.UpdatePersonRequest{}},
	{"lead", "leadWriteFields", "deferredLeadWrites", crmcontracts.UpdateLeadRequest{}},
}

const mapwritePath = "internal/modules/overlay/hubspot/mapwrite.go"

func TestEveryOverlayWritableFieldProjectsOrSaysWhyNot(t *testing.T) {
	t.Parallel()

	file := parseMapWrite(t)
	consts := hubspotStringConsts(t)
	for _, p := range writeProjections {
		t.Run(p.canonical, func(t *testing.T) {
			projected := projectedCanonicals(t, file, consts, p.projects)
			deferred := deferredCanonicals(t, file, consts, p.deferred)
			if len(projected) == 0 && len(deferred) == 0 {
				t.Fatalf("neither %s nor %s was found in %s — this class is judged against nothing",
					p.projects, p.deferred, mapwritePath)
			}
			writable := writableFieldsOf(t, p.canonical, p.request)
			for _, field := range writable {
				reason, isDeferred := deferred[field]
				switch {
				case projected[field] && isDeferred:
					t.Errorf("%s.%s is both projected and deferred — one of the two is wrong, and a "+
						"reader cannot tell which describes what the write does", p.canonical, field)
				case projected[field]:
				case isDeferred && reason != "":
				case isDeferred:
					t.Errorf("%s.%s is deferred with an empty reason, which says no more than leaving "+
						"it out did", p.canonical, field)
				default:
					t.Errorf("%s.%s can be PATCHed through the public contract, and the overlay write "+
						"projection neither carries it nor says why not. In overlay mode that write is "+
						"accepted and never reaches the incumbent, so the next read returns the old "+
						"value and the edit reads as somebody else's revert. Add it to %s, or to %s "+
						"with the reason it cannot be the inverse of its read",
						p.canonical, field, p.projects, p.deferred)
				}
			}
			// The other direction: a deferral naming a field the contract no
			// longer offers is a reason nobody needs, and the next author reads
			// it as covering something that is handled.
			for field := range deferred {
				if !sortedContains(writable, field) {
					t.Errorf("%s defers %q and the update request declares no such writable field — "+
						"the contract moved and this reason outlived it", p.deferred, field)
				}
			}
		})
	}
}

// projectedCanonicals reads the Canonical values out of one []directWriteField
// literal.
func projectedCanonicals(t *testing.T, file *ast.File, consts map[string]string, varName string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	forEachVarLiteral(file, varName, func(lit *ast.CompositeLit) {
		for _, elt := range lit.Elts {
			entry, ok := elt.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, field := range entry.Elts {
				kv, ok := field.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "Canonical" {
					if name, ok := gatekit.StringExpr(kv.Value, consts, gatekit.FoldStrict); ok {
						out[name] = true
					}
				}
			}
		}
	})
	return out
}

// deferredCanonicals reads one map[string]string literal's keys and the reason
// each carries.
func deferredCanonicals(t *testing.T, file *ast.File, consts map[string]string, varName string) map[string]string {
	t.Helper()
	out := map[string]string{}
	forEachVarLiteral(file, varName, func(lit *ast.CompositeLit) {
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, keyed := gatekit.StringExpr(kv.Key, consts, gatekit.FoldStrict)
			if !keyed {
				continue
			}
			reason, _ := gatekit.StringExpr(kv.Value, consts, gatekit.FoldTotal)
			out[key] = reason
		}
	})
	return out
}

// forEachVarLiteral visits the composite literal a named package-level var is
// assigned.
func forEachVarLiteral(file *ast.File, varName string, visit func(*ast.CompositeLit)) {
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || spec.Names[0].Name != varName || len(spec.Values) != 1 {
			return true
		}
		if lit, ok := spec.Values[0].(*ast.CompositeLit); ok {
			visit(lit)
		}
		return true
	})
}

func parseMapWrite(t *testing.T) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repoRoot, "backend", mapwritePath), nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", mapwritePath, err)
	}
	return file
}

// writableFieldsOf reads the json tags off one update request struct, which is
// the set of fields the door will decode and therefore the set where an
// unprojected field turns into a silent drop.
//
//craft:ignore naked-any the three request structs are unrelated generated types with no common interface — a constraint would have to name all three, which is the list this gate exists to avoid keeping
func writableFieldsOf(t *testing.T, canonical string, request any) []string {
	t.Helper()
	rt := reflect.TypeOf(request)
	fields := make([]string, 0, rt.NumField())
	for i := range rt.NumField() {
		tag, ok := rt.Field(i).Tag.Lookup("json")
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			fields = append(fields, name)
		}
	}
	sort.Strings(fields)
	if len(fields) < 2 {
		t.Fatalf("%s's update request declares %d writable field(s) — too few for the projection to be "+
			"a meaningful subset of anything, so this class is answered for by asking nothing",
			canonical, len(fields))
	}
	return fields
}

func sortedContains(haystack []string, needle string) bool {
	i := sort.SearchStrings(haystack, needle)
	return i < len(haystack) && haystack[i] == needle
}

// hubspotStringConsts indexes the string constants the hubspot package
// declares, by name.
//
// The projections key several entries on a constant rather than a literal —
// `{Canonical: industryField}`, `canonicalOwnerID:` — so a reader that took
// only literals would silently see a shorter list than the code declares, and
// report the fields it could not read as unanswered. Resolving them is what
// makes this gate judge the mapping rather than its spelling.
func hubspotStringConsts(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot, "backend", "internal", "modules", "overlay", "hubspot")
	fset := token.NewFileSet()
	var files []*ast.File
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		parsed, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		files = append(files, parsed)
		return nil
	}); err != nil {
		t.Fatalf("parsing the hubspot package: %v", err)
	}
	consts := map[string]string{}
	{
		for _, file := range files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
						continue
					}
					if text, isString := gatekit.LiteralText(value.Values[0]); isString {
						consts[value.Names[0].Name] = text
					}
				}
			}
		}
	}
	if len(consts) == 0 {
		t.Fatal("the hubspot package declares no string constant — every constant-keyed entry below " +
			"would read as absent, and the gate would report fields as unanswered that are answered")
	}
	return consts
}
