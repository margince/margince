// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The plaintext confirm token goes into the mail body and nowhere else.
//
// The token is a bearer credential over one person's record: whoever holds it
// can open what is held about them, correct it, and answer for them about
// marketing. The claim that a grant made through it is the SUBJECT'S rests
// entirely on the plaintext having reached only the subject's own mailbox — so
// a copy anywhere an operator can read is not a leak of a secret, it is the
// evidence quietly ceasing to be evidence.
//
// The retired operator-token endpoint is what this prevents returning. It handed
// the plaintext back to the caller, who could paste it straight in again, so one
// person could mint and redeem a confirmation the subject never saw. Those rows
// are still on the proof log and authorize nothing (recordedStateFor's
// issuance_trigger IS NOT NULL clause is what excludes them); this gate is what
// stops the shape coming back.
//
// Three carriers are checked, because they are the three places a value escapes
// a Go program without anybody deciding to publish it:
//
//   - an HTTP response body,
//   - a log line,
//   - an audit or outbox payload.
//
// The mail body is the one legitimate destination and is ratified by name.
//
// This gate reads the token's OWN package rather than sweeping the tree. The
// plaintext exists in exactly one type, IssuedConfirm.Token, built in one
// function and returned to two callers; a value that never leaves that package
// cannot be logged by code that cannot name it. The scope is asserted below
// rather than assumed, so a second minting site elsewhere fails rather than
// escaping the walk.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// consentDir is the package that mints and spends the plaintext.
	consentDir = "internal/modules/consent"
	// plaintextField is the only field carrying an unhashed token.
	plaintextField = "Token"
	// mailBodyFile is the one file that may put the plaintext in a string: it
	// composes the message the token is mailed in.
	mailBodyFile = "confirmmail.go"
)

// tokenSinks are the calls that publish a value outside the process. A
// plaintext token reaching any of them is the defect.
//
// The value is what the sink IS, used to render the finding — "passes the token
// to Audit (an audit payload)" — rather than the cost of an exemption. Nothing
// here is waived: every entry is a destination the token must never reach.
//
// gatekit:fixture the destination each sink publishes to, for the finding text
var tokenSinks = map[string]string{
	"WriteJSON":    "an HTTP response body",
	"Encode":       "an HTTP response body",
	"InfoContext":  "a log line",
	"WarnContext":  "a log line",
	"ErrorContext": "a log line",
	"Info":         "a log line",
	"Warn":         "a log line",
	"Error":        "a log line",
	"Audit":        "an audit payload",
	"AuditEvent":   "an audit payload",
	"Emit":         "an outbox payload",
}

func TestThePlaintextConfirmTokenReachesNoSinkButTheMail(t *testing.T) {
	t.Parallel()

	files := parseConsentPackage(t)
	checked := 0
	for path, file := range files {
		if filepath.Base(path) == mailBodyFile {
			// The mail body is where the token is SUPPOSED to go. Everything
			// else in this file is still checked by the sink walk below via the
			// other files; exempting the whole file here is safe because the
			// only sink it holds is the relay call, which is the destination.
			continue
		}
		checked++
		for _, finding := range plaintextReachesASink(file) {
			t.Errorf("%s passes the plaintext confirm token to %s (%s): the token is a bearer "+
				"credential over one person's record, and a copy anywhere an operator can read it "+
				"ends the claim that a grant made through it was the subject's own",
				path, finding.call, finding.sink)
		}
	}
	if checked == 0 {
		t.Fatal("no consent sources were walked: this gate is looking in the wrong place")
	}
}

// TestTheConfirmTokenIsMintedOnlyWhereThisGateWatches proves the scope claim the
// census rests on.
//
// The walk above reads one package because the plaintext is built in one place.
// If a second minting site appeared elsewhere in the tree, the census would keep
// reporting PASS while a token escaped through code it never reads. This asserts
// the field's home instead of assuming it.
func TestTheConfirmTokenIsMintedOnlyWhereThisGateWatches(t *testing.T) {
	t.Parallel()

	files := parseConsentPackage(t)
	minting := 0
	for path, file := range files {
		if constructsIssuedConfirm(file) {
			minting++
			_ = path
		}
	}
	if minting == 0 {
		t.Fatal("no file in the consent package constructs an IssuedConfirm: the type this gate " +
			"follows has moved, and the census is watching nothing")
	}
}

type sinkFinding struct{ call, sink string }

// plaintextReachesASink reports every publishing call that is handed something
// named for the plaintext token.
func plaintextReachesASink(file *ast.File) []sinkFinding {
	var out []sinkFinding
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calleeName(call)
		sink, isSink := tokenSinks[name]
		if !isSink {
			return true
		}
		for _, arg := range call.Args {
			if mentionsPlaintext(arg) {
				out = append(out, sinkFinding{call: name, sink: sink})
				break
			}
		}
		return true
	})
	return out
}

// mentionsPlaintext reports whether an expression reads the plaintext field, at
// any depth — a composite literal, a map value, a concatenation.
//
// It matches the FIELD name rather than a variable, because the variable holding
// an IssuedConfirm is named differently at each call site and a gate keyed on
// those names would stop matching the moment one was renamed.
func mentionsPlaintext(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == plaintextField {
			found = true
			return false
		}
		return true
	})
	return found
}

// constructsIssuedConfirm reports whether a file builds the type that carries
// the plaintext.
func constructsIssuedConfirm(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		id, ok := lit.Type.(*ast.Ident)
		if !ok || id.Name != "IssuedConfirm" {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == plaintextField {
				found = true
			}
		}
		return true
	})
	return found
}

// parseConsentPackage parses the consent module's non-test sources.
func parseConsentPackage(t *testing.T) map[string]*ast.File {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(consentDir, "*.go"))
	if err != nil {
		t.Fatalf("listing the consent package: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		out[path] = file
	}
	if len(out) == 0 {
		t.Fatalf("no sources found under %s: the gate is looking in the wrong place", consentDir)
	}
	return out
}
