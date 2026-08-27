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

	envelopes := 0
	for name, fields := range structsIn(file) {
		rows, page := fields[rowsField], fields["Page"]
		// A list envelope is one that pages ROWS: it has a page, and a slice of
		// something to put on it. Identified by shape rather than by a name
		// ending in "ListResponse", so a renamed envelope is still judged and a
		// type that merely reads like one is not.
		if page == nil || rows == nil {
			continue
		}
		if _, isSlice := rows.(*ast.ArrayType); !isSlice {
			t.Errorf("%s pages something that is not a slice, so httperr.WriteJSON cannot "+
				"normalise it and an empty one goes out as JSON null", name)
			continue
		}
		envelopes++
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
