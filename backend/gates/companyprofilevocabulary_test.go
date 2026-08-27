// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The company-profile vocabulary is spelled in eight places, and this gate is
// what makes widening seven of them a failure instead of a silent half-job.
//
// `organization_profile_field.field` names what a company profile may state.
// The same list is restated by the contract enum (four times, because four
// schemas inline it), by two Go constant blocks that key the same rows, by the
// extraction allowlist that tells the model what to look for, and by the
// writer registry that decides where a value lands. Nothing held them
// together: adding a field meant eight edits with no failing test for a miss.
//
// The failure that motivates it is quiet in both directions. A field admitted
// by the CHECK but absent from the extraction allowlist is never read off a
// page — the column exists, the model is never told to fill it, and the
// feature looks shipped. A field the writer registry omits is accepted at the
// API and then silently never written. Neither breaks a build.
//
// The CHECK is the authority because it is what the database enforces: a value
// it refuses cannot be stored whatever any Go file believes.
//
// WHAT IT CANNOT SEE, two things:
//
// Whether a field is offered to a HUMAN. The company form's typed properties
// in the contract are hand-listed prose, and a field missing there is still
// reachable through the generic `fields` map, so its absence is a smaller
// defect than the ones above and not one this gate can name.
//
// The OTHER direction — a Go name the CHECK would refuse. It is checked one
// way only, deliberately: the const blocks this reads also hold source names
// ("human", "site_read") and other unrelated values, so demanding every string
// be a vocabulary member would fail on correct code. The database refuses such
// a write at insert, which is a loud failure rather than a silent one, and
// that is the direction that can afford to be caught late.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// theProfileVocabulary is the column whose CHECK this gate treats as the
// authority.
const theProfileVocabulary = "organization_profile_field.field"

// profileVocabularyMirrors are the Go declarations that must each carry the
// whole vocabulary. Every one keys rows in that column, so a name it lacks is
// a row it can neither write nor read back.
var profileVocabularyMirrors = []struct {
	// file is the source holding the declaration.
	file string
	// decl is the var or const name whose string values are the mirror.
	decl string
	// why says what breaks when this mirror falls short, so a failure tells
	// the reader the consequence rather than only the diff.
	why string
}{
	{
		file: "internal/compose/enrichextract.go",
		decl: "extractionFieldNames",
		why:  "the model is never told to look for it, so a site read silently never fills it",
	},
	{
		file: "internal/compose/company.go",
		decl: "fieldDisplayName",
		why:  "the company form can neither write nor read it back",
	},
	{
		file: "internal/modules/people/company.go",
		decl: "fieldOfferSummary",
		why:  "the store has no constant for it",
	},
	{
		file: "internal/modules/people/company.go",
		decl: "companyFields",
		why:  "the value is accepted at the API and then never written",
	},
}

// displayName is the company's own name, which the CHECK admits as a profile
// field but the store writes through the organization row instead
// (SaveCompany sets it directly). So the two people-module mirrors legitimately
// omit it, and this gate would otherwise demand a constant nothing should use.
const displayName = "display_name"

// mirrorsExemptFromDisplayName are the declarations that may omit it, by the
// file and declaration name they are listed under above.
var mirrorsExemptFromDisplayName = map[string]bool{
	"internal/modules/people/company.go:fieldOfferSummary": true,
	"internal/modules/people/company.go:companyFields":     true,
}

func TestTheCompanyProfileVocabularyIsSpelledOnceEverywhere(t *testing.T) {
	t.Parallel()
	admitted := tableCheckSets(t)[theProfileVocabulary]
	// A census that judged nothing certifies nothing. This column has carried
	// a CHECK since the baseline, so an empty set means the derivation stopped
	// working rather than the vocabulary having emptied.
	if len(admitted) < 10 {
		t.Fatalf("only %d value(s) derived for %s — the migration scan has stopped reading it",
			len(admitted), theProfileVocabulary)
	}
	want := map[string]bool{}
	for _, value := range admitted {
		want[value] = true
	}

	contractNames := constValuesIn(t, "internal/contracts/api_gen.go")
	for _, mirror := range profileVocabularyMirrors {
		// An identifier in a mirror resolves against that file's own consts
		// first, and against the generated contract constants for the
		// `string(crmcontracts.…)` form.
		known := constValuesIn(t, mirror.file)
		for name, value := range contractNames {
			if _, shadowed := known[name]; !shadowed {
				known[name] = value
			}
		}
		got := stringValuesOfDecl(t, mirror.file, mirror.decl, known)
		if len(got) == 0 {
			t.Errorf("%s: found no string values for %s — the extractor has stopped reading it",
				mirror.file, mirror.decl)
			continue
		}
		exempt := mirrorsExemptFromDisplayName[mirror.file+":"+mirror.decl]
		var missing []string
		for value := range want {
			if value == displayName && exempt {
				continue
			}
			if !got[value] {
				missing = append(missing, value)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("%s (%s) omits %s, which %s admits.\n"+
				"  Consequence: %s.\n"+
				"  Add the value here, or take it out of the CHECK if the product does not have it.",
				mirror.decl, mirror.file, strings.Join(missing, ", "), theProfileVocabulary, mirror.why)
		}
	}
}

// stringValuesOfDecl collects the vocabulary members a declaration carries.
//
// The mirrors spell a member three ways, and the walk resolves all three
// because picking only literals would read two of the four as empty — which
// is under-recognition, the one way a census must not fail:
//
//	"legal_name"                                  a literal
//	string(crmcontracts.ColdStartFieldFieldLegalName)  a conversion of a
//	                                              contract constant
//	{name: fieldLegalName}                        an identifier declared
//	                                              elsewhere in the same file
//
// The last two are resolved through `known`, the file's own const values,
// and a contract constant is resolved by its trailing name — the generated
// constants are named for their value, so ColdStartFieldFieldLegalName IS
// legal_name spelled in Go. An identifier that resolves to nothing is
// ignored rather than guessed at.
func stringValuesOfDecl(t *testing.T, file, decl string, known map[string]string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(file), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	values := map[string]bool{}
	add := func(value string) {
		if value != "" {
			values[value] = true
		}
	}
	for _, node := range parsed.Decls {
		general, isGeneral := node.(*ast.GenDecl)
		if !isGeneral || !declMentions(general, decl) {
			continue
		}
		ast.Inspect(general, func(n ast.Node) bool {
			switch typed := n.(type) {
			case *ast.BasicLit:
				if typed.Kind == token.STRING {
					if unquoted, err := strconv.Unquote(typed.Value); err == nil {
						add(unquoted)
					}
				}
			case *ast.Ident:
				add(known[typed.Name])
			case *ast.SelectorExpr:
				add(known[typed.Sel.Name])
			}
			return true
		})
	}
	return values
}

// constValuesIn reads a file's own `name = "value"` const declarations, so an
// identifier used in a mirror resolves to what it stands for.
func constValuesIn(t *testing.T, file string) map[string]string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(file), nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	values := map[string]string{}
	for _, node := range parsed.Decls {
		general, isGeneral := node.(*ast.GenDecl)
		if !isGeneral || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			literal, isLiteral := value.Values[0].(*ast.BasicLit)
			if !isLiteral || literal.Kind != token.STRING {
				continue
			}
			if unquoted, err := strconv.Unquote(literal.Value); err == nil {
				values[value.Names[0].Name] = unquoted
			}
		}
	}
	return values
}

// declMentions reports whether this declaration is the one named — either
// because a spec is called that, or because the const group contains a member
// with that name.
func declMentions(decl *ast.GenDecl, name string) bool {
	for _, spec := range decl.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		for _, ident := range value.Names {
			if ident.Name == name {
				return true
			}
		}
	}
	return false
}
