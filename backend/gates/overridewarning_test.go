// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// The words behind a recorded acknowledgement cannot change without the version
// changing.
//
// An instruction records that a named human read a particular warning before
// overruling the engine, and it records that as a VERSION. The whole claim
// rests on the version identifying one fixed text: edit the sentence and leave
// the version alone, and every past acknowledgement silently becomes a record
// of words nobody was shown.
//
// Nothing about the Go compiler stops that edit, and nothing in a review
// reliably catches it — the version is thirty lines from the text and looks
// like a constant nobody needs to touch. So the text is pinned by its digest
// here, and the digest is what a change has to come and edit deliberately.
//
// WHEN THE WARNING CHANGES: move the version constant, put the new digest
// below, and say in the commit message why the wording moved. The old version
// stays valid for the instructions that name it — this gate is about what the
// CURRENT version means, not about retiring old ones.

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// The warning lives in consent, which this package may not import: gates read
// the tree rather than linking it, so a gate cannot be satisfied by a build
// that compiles. The text and the version are parsed out of the source.
const overrideWarningFile = "internal/modules/consent/overridewarning.go"

// warningDigests pins the exact text each served version stands for.
//
// A version missing here fails too: an unpinned version is one whose words can
// be edited freely, which is the state this gate exists to end.
// gatekit:fixture the digest each served warning version stands for — expected
// data, not a cost
var warningDigests = map[string]string{
	"override-v1": "de2de8c051f8eed3177d2583e13ef5f72d77d0db20d4f32b27ddea0e6d494379",
}

func TestTheOverrideWarningTextMatchesItsVersion(t *testing.T) {
	t.Parallel()
	version, text := servedWarning(t)

	pinned, known := warningDigests[version]
	if !known {
		t.Fatalf("the served warning version %q is not pinned to any text.\n\n"+
			"An unpinned version is one whose words can be edited without anything noticing, and "+
			"every instruction naming it would then record an acknowledgement of words nobody was "+
			"shown. Add it to warningDigests with the digest below.\n\n\tdigest: %s",
			version, digestOf(text))
	}
	if got := digestOf(text); got != pinned {
		t.Errorf("the warning text for %q changed and the version did not.\n\n"+
			"Every instruction already naming this version claims somebody read the OLD words. "+
			"Move the version constant, pin the new digest, and say in the commit why the wording "+
			"moved — the old version stays valid for the records that name it.\n\n"+
			"\twas:  %s\n\tnow:  %s",
			version, pinned, got)
	}

	// The text is asserted to be real before its digest is believed: an empty
	// string has a digest too, and a warning that says nothing would pass a
	// pure digest comparison while telling a director nothing at all.
	if len(text) < 100 {
		t.Errorf("the override warning is %d characters, which is too short to say what it has "+
			"to say: that the refusal stands, that the decision is recorded under their name, and "+
			"that it covers this message alone", len(text))
	}
}

// servedWarning reads the version and the text out of the source, so this gate
// judges what the tree says rather than what a build happens to link.
//
// PARSED WITH go/ast, not with a regex over the bytes. A regex that scanned for
// quoted pieces could be fooled by three gofmt-valid edits that leave the
// digest untouched: a raw string appended to the text, a `+` continuation past
// a blank line, and a quoted fragment moved into a line comment. Each made the
// gate read a different string than the server serves, so it agreed with itself
// while the warning had changed — which is the one failure it exists to
// prevent.
//
// The parser evaluates the constant the compiler would: every operand of the
// concatenation, in order, whatever quoting each uses.
func servedWarning(t *testing.T) (version, text string) {
	t.Helper()
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, overrideWarningFile, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("reading %s: %v", overrideWarningFile, err)
	}
	for _, decl := range parsed.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			switch value.Names[0].Name {
			case "OverrideWarningVersion":
				version = constantString(t, value.Values[0])
			case "overrideWarningText":
				text = constantString(t, value.Values[0])
			}
		}
	}
	if version == "" {
		t.Fatalf("%s declares no OverrideWarningVersion — this gate cannot see the version it "+
			"exists to pin", overrideWarningFile)
	}
	if text == "" {
		t.Fatalf("%s declares no overrideWarningText this gate could read", overrideWarningFile)
	}
	return version, text
}

// constantString evaluates a string constant expression the way the compiler
// does: a literal, or any concatenation of them.
//
// ANYTHING ELSE FAILS rather than being skipped. A constant built from a
// helper call or another identifier is one this gate cannot follow, and
// quietly reading part of it would be the silent disagreement the whole file
// is about.
func constantString(t *testing.T, expr ast.Expr) string {
	t.Helper()
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			t.Fatalf("%s: expected a string constant, found %s", overrideWarningFile, node.Kind)
		}
		return unquote(t, node.Value)
	case *ast.BinaryExpr:
		if node.Op != token.ADD {
			t.Fatalf("%s: a string constant is built with + and nothing else, found %s",
				overrideWarningFile, node.Op)
		}
		return constantString(t, node.X) + constantString(t, node.Y)
	default:
		t.Fatalf("%s: this gate reads string literals and their concatenation, and the constant "+
			"is neither — it cannot judge text it cannot evaluate", overrideWarningFile)
		return ""
	}
}

func unquote(t *testing.T, quoted string) string {
	t.Helper()
	value, err := strconv.Unquote(quoted)
	if err != nil {
		t.Fatalf("reading %s: %v", quoted, err)
	}
	return value
}

func digestOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
