// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser knows every way the server says a contact was obtained, and no other.
//
// The privacy notice and the disclosure-duty queue label a kind only when
// frontend/src/format/acquisitionkinds.ts lists it. A kind missing there shows a
// stranger the general unknown sentence and an officer the raw token.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var frontendAcquisitionKinds = filepath.Join(repoRoot, "frontend", "src", "format", "acquisitionkinds.ts")

var tsCommentInAcquisitionKinds = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

// Any quote style counts, so an entry spelled another way is still read.
var tsQuotedEntry = regexp.MustCompile("\"([^\"]*)\"|'([^']*)'|`([^`]*)`")

func TestTheFrontendAcquisitionKindsMatchTheGoVocabulary(t *testing.T) {
	t.Parallel()
	inGo := goAcquisitionKinds(t)
	inTS := tsAcquisitionKinds(t)
	for kind := range inGo {
		if !inTS[kind] {
			t.Errorf("contacts can record the acquisition kind %q and %s does not list it, so the screens print it raw", kind, frontendAcquisitionKinds)
		}
	}
	for kind := range inTS {
		if !inGo[kind] {
			t.Errorf("%s lists %q, which contacts never records: a dead label says the list is not maintained", frontendAcquisitionKinds, kind)
		}
	}
}

// goAcquisitionKinds reads every string in the const block that declares
// AcquiredUnknownLegacy, so a kind added there under any name is counted.
func goAcquisitionKinds(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join(repoRoot, "backend", "internal", "modules", "contacts", "acquisition.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if isGen && gen.Tok == token.CONST && constBlockDeclares(gen, "AcquiredUnknownLegacy") {
			return stringValuesOf(t, gen)
		}
	}
	t.Fatalf("%s no longer declares AcquiredUnknownLegacy in a const block: point this gate at the vocabulary's new home", path)
	return nil
}

func constBlockDeclares(gen *ast.GenDecl, name string) bool {
	for _, spec := range gen.Specs {
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

func stringValuesOf(t *testing.T, gen *ast.GenDecl) map[string]bool {
	t.Helper()
	kinds := map[string]bool{}
	for _, spec := range gen.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		for _, expr := range value.Values {
			literal, isLiteral := expr.(*ast.BasicLit)
			if !isLiteral || literal.Kind != token.STRING {
				t.Fatalf("an acquisition kind is not a string literal (%T): this gate cannot read it", expr)
			}
			kind, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("unquoting %s: %v", literal.Value, err)
			}
			kinds[kind] = true
		}
	}
	return kinds
}

// tsAcquisitionKinds reads the ACQUISITION_KINDS array. Anything left once the
// quoted entries and commas are gone is an entry this parser cannot see.
func tsAcquisitionKinds(t *testing.T) map[string]bool {
	t.Helper()
	source, err := os.ReadFile(frontendAcquisitionKinds)
	if err != nil {
		t.Fatalf("reading the frontend acquisition kinds: %v", err)
	}
	const opener = "export const ACQUISITION_KINDS = ["
	start := strings.Index(string(source), opener)
	if start < 0 {
		t.Fatalf("%s no longer declares %s: this gate is reading a shape that is gone", frontendAcquisitionKinds, opener)
	}
	body := string(source)[start+len(opener):]
	end := strings.Index(body, "] as const")
	if end < 0 {
		t.Fatalf("%s: ACQUISITION_KINDS is not closed by `] as const`", frontendAcquisitionKinds)
	}
	body = tsCommentInAcquisitionKinds.ReplaceAllString(body[:end], " ")
	kinds := map[string]bool{}
	for _, match := range tsQuotedEntry.FindAllStringSubmatch(body, -1) {
		kinds[match[1]+match[2]+match[3]] = true
	}
	if rest := strings.Trim(tsQuotedEntry.ReplaceAllString(body, ""), ", \r\n\t"); rest != "" {
		t.Fatalf("%s: ACQUISITION_KINDS holds %q, which is not a quoted kind this gate can read", frontendAcquisitionKinds, rest)
	}
	if len(kinds) == 0 {
		t.Fatal("read no kinds out of ACQUISITION_KINDS: a gate that reads nothing agrees with everything")
	}
	return kinds
}
