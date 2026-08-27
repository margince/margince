// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// Every list envelope carries its rows in a field the writer can find.
//
// The contract declares each list response `required: [data, page]` with `data`
// of `type: array`. `null` is not an array, so an envelope carrying a nil Go
// slice breaks its own contract on the wire — and Go makes that the EASY
// mistake rather than an unusual one: a nil slice IS the idiomatic empty list,
// `append` works on it, `len` and `range` work, and the only place it behaves
// differently is encoding/json.
//
// The cost falls on the client. Generated TypeScript reads `data: Person[]`,
// because the contract says so, and gives a caller no reason to guard — 36
// reads across 18 files call `.map` on it directly, and each takes down the
// screen rendering it (issue #1606). No compiler on either side can see it: the
// Go type is a slice, the TS type is an array, and only the encoder disagrees.
//
// httperr.WriteJSON fixes it for all of them at once, by replacing a nil slice
// in the body's `Data` field with an empty one. THAT IS WHAT THIS CENSUS HOLDS:
// the writer finds the rows by the field NAME, so an envelope whose rows live
// under a different name would silently stop being normalised — correct on
// every list that has rows, and `null` on exactly the empty ones nobody tests.
//
// The envelopes are read out of the generated contract, so one added by
// `make gen` is judged the day it lands and there is no list here to go short.
// The mechanism itself — that a nil Data becomes `[]`, through a value and
// through a pointer — is held by httperr's own tests, because it is a property
// of the writer rather than of the contract.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// The field WriteJSON reflects for. Spelled here because this census exists to
// hold the two together: a rename on either side and the normalisation stops.
const rowsField = "Data"

func TestEveryListEnvelopeCarriesItsRowsWhereTheWriterLooks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(repoRoot, "backend", "internal", "contracts", "api_gen.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the generated contract: %v", err)
	}

	envelopes, findings := judgeEnvelopes(file)
	for _, finding := range findings {
		t.Error(finding)
	}

	// The generated contract carries dozens. A census that found a handful is
	// one whose discovery broke, and it would report the same green as a tree
	// where every envelope is correct.
	if envelopes < 20 {
		t.Fatalf("found %d list envelope(s) in the generated contract, which is too few to be "+
			"the whole set — the walk is broken, or the generated shape changed", envelopes)
	}
	t.Logf("%d list envelope(s) carry their rows in %s", envelopes, rowsField)
}

// structsIn maps every struct type declared in the file to its fields.
func structsIn(file *ast.File) map[string]map[string]ast.Expr {
	out := map[string]map[string]ast.Expr{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Fields == nil {
				continue
			}
			fields := map[string]ast.Expr{}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					fields[name.Name] = field.Type
				}
			}
			out[typeSpec.Name.Name] = fields
		}
	}
	return out
}

// judgeEnvelopes returns how many list envelopes the file declares and what is
// wrong with them.
//
// Split out so the cases below can drive it over sources this tree does not
// contain. The mutation that matters — an envelope whose rows are named
// something else — cannot be made in the generated contract, because renaming
// the field stops every handler compiling. So the compiler prevents the bug and
// prevents proving the gate would catch it, which is exactly when a synthetic
// case earns its place.
func judgeEnvelopes(file *ast.File) (int, []string) {
	envelopes := 0
	var findings []string
	for name, fields := range structsIn(file) {
		// A list envelope is one that IS a page: it carries a Page by value.
		// Never identified by whether it has a `Data` field — a census that
		// required `Data` to recognise an envelope would SKIP the one thing it
		// exists to catch, a renamed rows field, and report the same green as a
		// tree where every envelope is correct.
		//
		// By VALUE is the discriminator, and it is the contract's own: a `Page
		// PageInfo` says this response IS a page of something, while a `Page
		// *PageInfo` says a response may INCLUDE one — LeadScoreExplanation is
		// a record that optionally carries its score history. The second kind
		// has no problem to fix: its rows are a `*[]T` under `omitempty`, so an
		// absent history is absent rather than null.
		if !pagesRows(fields["Page"]) {
			continue
		}
		envelopes++
		rows := fields[rowsField]
		if rows == nil {
			findings = append(findings, name+" pages rows and has no "+rowsField+
				" field, so httperr.WriteJSON cannot find them — an empty one goes out as JSON "+
				"null, which the contract declares an array")
			continue
		}
		if _, isSlice := rows.(*ast.ArrayType); !isSlice {
			findings = append(findings, name+" carries "+rowsField+" but not as a slice, so "+
				"httperr.WriteJSON cannot normalise it and an empty one goes out as JSON null")
		}
	}
	return envelopes, findings
}

// pagesRows reports a `Page PageInfo` — the field that says this response IS a
// page of something.
//
// The TYPE as well as the shape. A non-pointer `Page` alone admits any struct
// with a field of that name — `Page string` on a record about a printed page —
// which the census would then count as an envelope and report for having no
// rows. Two wrong answers from one loose test: a type nobody paginates named as
// a defect, and the count that guards against a broken walk quietly inflated by
// it.
//
// A POINTER is the other half and is not this: `Page *PageInfo` says a response
// may INCLUDE a page, which is what a record carrying optional history does.
// Those have no problem to fix — their rows are a `*[]T` under `omitempty`, so
// an absent list is absent rather than null.
func pagesRows(page ast.Expr) bool {
	ident, isValue := page.(*ast.Ident)
	return isValue && ident.Name == "PageInfo"
}

// What the census must and must not report, over sources the generated contract
// cannot hold.
func TestTheEnvelopeCensusJudgesTheRightStructs(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		source    string
		envelopes int
		findings  int
	}{
		"a list envelope": {
			source: `package p
type PageInfo struct{ HasMore bool }
type PersonListResponse struct {
	Data []int
	Page PageInfo
}`,
			envelopes: 1,
		},
		// THE case. The writer finds rows by field name, so an envelope that
		// pages them under another name is correct on every list that has rows
		// and null on exactly the empty ones nobody tests.
		"an envelope whose rows are named something else": {
			source: `package p
type PageInfo struct{ HasMore bool }
type PersonListResponse struct {
	Rows []int
	Page PageInfo
}`,
			envelopes: 1,
			findings:  1,
		},
		"an envelope whose rows are not a slice": {
			source: `package p
type PageInfo struct{ HasMore bool }
type PersonListResponse struct {
	Data *[]int
	Page PageInfo
}`,
			envelopes: 1,
			findings:  1,
		},
		// A record that may INCLUDE a page is not a page. Its rows are optional
		// and omitted when absent, so there is no null to fix.
		"a record that optionally carries a page": {
			source: `package p
type PageInfo struct{ HasMore bool }
type LeadScoreExplanation struct {
	History *[]int
	Page    *PageInfo
	Score   int
}`,
			envelopes: 0,
		},
		// A field named Page that is not a page. Counted as an envelope by a
		// test that looks only at the name, and then reported for having no
		// rows — a defect invented out of a type nobody paginates.
		"a record whose Page is not a PageInfo": {
			source: `package p
type Leaflet struct {
	Page  string
	Title string
}`,
			envelopes: 0,
		},
		"a record with no page at all": {
			source: `package p
type Person struct{ Name string }`,
			envelopes: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			envelopes, findings := judgeEnvelopes(file)
			if envelopes != tc.envelopes {
				t.Errorf("counted %d envelope(s), want %d", envelopes, tc.envelopes)
			}
			if len(findings) != tc.findings {
				t.Errorf("reported %d finding(s) %v, want %d", len(findings), findings, tc.findings)
			}
		})
	}
}
