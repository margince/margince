// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// How far into the diary a meeting still says a relationship is live, spelled
// twice in two modules that may not import each other, and held equal here.
//
// The two answer the same question about the same rows and are read one after
// the other in the life of one partner: capture's copy decides whether the
// guest on a meeting BECOMES a contact, and consent's decides whether writing
// to that contact is lawful. Drift is silent and lands in the worst possible
// shape — widen capture's alone and the product creates a record it will then
// refuse to mail, widen consent's alone and the lawful window covers contacts no
// record exists for. Neither fails anything.
//
// Two writers of one invariant either share a helper or say why they do not
// (AGENTS.md). They cannot share one: a module never imports a sibling, and the
// value is a Postgres interval literal interpolated into each module's own SQL,
// so a shared constant would have to live in a lower tier that neither owns.
// This gate is the "say why" — and it fails from either side.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The two copies, by the PACKAGE that holds each — module-relative, because
// TestMain chdirs to the backend root. Both declare the same constant name,
// which is deliberate: a reader who greps the name finds both.
//
// The package and not the file. A constant keyed to the file it sits in today
// reports a horizon of "" the moment somebody splits that file, which reads as
// a drift between the two modules rather than as a gate that lost its subject —
// a failure that fires when nothing is wrong, and names the wrong thing while
// it does.
const (
	meetingHorizonCapturePkg = "internal/modules/capture"
	meetingHorizonConsentPkg = "internal/modules/consent"
	meetingHorizonConstName  = "meetingHorizonInterval"
)

func TestOneSpellingOfTheMeetingHorizon(t *testing.T) {
	t.Parallel()

	capture := meetingHorizonIn(t, meetingHorizonCapturePkg)
	consent := meetingHorizonIn(t, meetingHorizonConsentPkg)

	if capture != consent {
		t.Fatalf("the meeting horizon is %q in capture and %q in consent — one decides whether a "+
			"meeting's guest becomes a contact and the other whether writing to them is lawful, so "+
			"a difference either creates records the product then refuses to mail, or opens a "+
			"lawful window over contacts no record exists for",
			capture, consent)
	}
	// A horizon nobody can read is not a horizon. Postgres would reject an
	// unparseable interval at query time, on a path that runs inside the capture
	// transaction, so an empty or renamed constant must fail here instead.
	if capture == "" {
		t.Fatalf("neither copy declares %s — the gate is comparing nothing, which is how a "+
			"census fails short and reports PASS", meetingHorizonConstName)
	}
}

// meetingHorizonIn reads the constant's value out of one package, or "" when no
// file in it declares the constant.
//
// Parsed rather than grepped: a comment or a test fixture mentioning the name
// would satisfy a text search, and a gate that can be satisfied by prose is a
// gate that stops applying the moment somebody writes about it.
func meetingHorizonIn(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var found string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if value := meetingHorizonInFile(t, filepath.Join(dir, name)); value != "" {
			found = value
		}
	}
	return found
}

// meetingHorizonInFile is the parse of one file, answering "" when it declares
// no such constant.
func meetingHorizonInFile(t *testing.T, path string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var found string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if name.Name != meetingHorizonConstName || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Fatalf("%s in %s is not a string literal — this gate compares the interval "+
					"Postgres is handed, and it can only read one that is written out",
					meetingHorizonConstName, path)
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("unquoting %s in %s: %v", meetingHorizonConstName, path, err)
			}
			found = value
		}
		return true
	})
	return found
}
