// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit_test

// An unaliased import binds the package's OWN name.
//
// Go binds the `package` clause, not the last segment of the directory. Every
// gate that locates calls asks gatekit which qualifier a file spells, and for
// the path most of them care about the two answers differ:
//
//	directory   backend/internal/contracts
//	package     crmcontracts
//
// A gate given `contracts` hunts `contracts.Foo` through a file that says
// `crmcontracts.Foo`, finds nothing, and reports a clean package — a census
// failing short through its helper rather than through its own reader, which
// is the one way it may not fail.

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const contractsPath = "github.com/margince/margince/backend/internal/contracts"

func TestDeclaredPackageNameReadsThePackageClauseNotTheDirectory(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		what, importPath, want string
	}{
		{
			what:       "a module package whose name differs from its directory",
			importPath: contractsPath,
			want:       "crmcontracts",
		},
		{
			what:       "a module package whose name matches its directory",
			importPath: "github.com/margince/margince/backend/internal/shared/gatekit",
			want:       "gatekit",
		},
		// Outside the module there are no sources to read, and the directory
		// name is the right answer — which is why falling back is safe.
		{what: "the standard library", importPath: "net/http", want: "http"},
		{what: "a nested standard library path", importPath: "go/ast", want: "ast"},
		{what: "a path that resolves to nothing", importPath: "example.com/nope/nowhere", want: "nowhere"},
	} {
		if got := gatekit.DeclaredPackageName(c.importPath); got != c.want {
			t.Errorf("%s: DeclaredPackageName(%q) = %q, want %q",
				c.what, c.importPath, got, c.want)
		}
	}
}

// And ImportedAs answers with it, which is what every gate actually calls.
//
// The aliased and dot-imported arms are asserted beside it because the fix
// touches the unaliased one only, and a change that resolved the declared name
// at the cost of ignoring an alias would trade one blindness for another.
func TestImportedAsAnswersTheDeclaredNameForAnUnaliasedImport(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		what, source, wantQualifier string
		wantDot                     bool
	}{
		{
			what:          "unaliased: the package clause, not the directory",
			source:        "package p\nimport \"" + contractsPath + "\"\n",
			wantQualifier: "crmcontracts",
		},
		{
			what:          "aliased: the alias the file chose",
			source:        "package p\nimport api \"" + contractsPath + "\"\n",
			wantQualifier: "api",
		},
		{
			what:    "dot-imported: no qualifier at all",
			source:  "package p\nimport . \"" + contractsPath + "\"\n",
			wantDot: true,
		},
		{
			what:   "blank: imported for effect, and not callable through",
			source: "package p\nimport _ \"" + contractsPath + "\"\n",
		},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "probe.go", c.source, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: parsing the probe: %v", c.what, err)
		}
		qualifier, dot := gatekit.ImportedAs(file, contractsPath)
		if qualifier != c.wantQualifier || dot != c.wantDot {
			t.Errorf("%s: ImportedAs = (%q, %v), want (%q, %v)",
				c.what, qualifier, dot, c.wantQualifier, c.wantDot)
		}
	}
}
