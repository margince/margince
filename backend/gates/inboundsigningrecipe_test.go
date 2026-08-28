// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates_test

// The signing scope is ONE invariant spelled on both sides of a wire.
//
// The verifier reads it from extension.ScopeInbound; the member reads it from a
// `curl` the connector's screen generates and pastes into a terminal. Neither
// side can see the other, and the failure when they disagree is the worst shape
// available at this edge: every refusal here is one opaque 401 by design, so a
// member whose command silently stopped verifying gets an answer with nothing
// in it to say why — not a wrong scope, not a wrong signature, nothing. They
// would blame the secret, mint a new one, and get the same 401 again.
//
// Held in BOTH directions, because either half moving alone breaks it: a scope
// bumped in Go leaves every published recipe minting signatures nothing
// accepts, and a scope edited in the screen leaves a command that never worked
// in the one place a member is told to copy from.
//
// Ordinary version bumps are expected — that is what a scope is for. This gate
// does not refuse one; it refuses HALF of one.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// recipeFile is the screen the member copies the command from.
const recipeFile = "../extensions/openchannel/frontend/recipe.tsx"

// scopeInFrontend reads the exported constant rather than any of the prose
// around it, so a comment that mentions the old value does not hold the gate
// green after the code has moved on.
var scopeInFrontend = regexp.MustCompile(`export const SCOPE_INBOUND = "([^"]+)"`)

func TestTheSigningScopeIsSpelledTheSameOnBothSidesOfTheWire(t *testing.T) {
	source, err := os.ReadFile(recipeFile)
	if err != nil {
		t.Fatalf("reading %s: %v", recipeFile, err)
	}
	match := scopeInFrontend.FindSubmatch(source)
	if match == nil {
		t.Fatalf("%s no longer exports SCOPE_INBOUND as a string literal — this gate reads that "+
			"shape, so a different one leaves the two sides of the signature unheld", recipeFile)
	}
	if got, want := string(match[1]), string(extension.ScopeInbound); got != want {
		t.Errorf("the screen signs under scope %q and the verifier accepts %q — the command a member "+
			"is told to paste mints a signature this installation refuses, and the refusal is the same "+
			"opaque 401 as a wrong secret, so nothing in the answer says which", got, want)
	}
}

// THE SCOPE IS ONE OF SIX FIELDS, and holding only it would leave five ways for
// the two sides to disagree that look identical from either one. Field order, the
// number of separators and whether the body is followed by one are each spelled
// twice — once in SigningPayload's format, once in the shell the screen emits —
// and each produces the same opaque 401 when they drift.
//
// So the shapes are compared rather than the scope alone. The Go format carries
// the five fields before the body and ends with the separator that introduces
// it; the shell format carries all six. Anything else — a field added on one
// side, a separator changed, a trailing newline after the body — makes these two
// disagree here instead of in a member's terminal.
//
// What this does NOT hold is that the two field ORDERS match, which no static
// comparison of a Go format against a shell format can see. The screen's own
// test pins the emitted argument list verbatim, which is where that belongs:
// it asserts against what curlRecipe RETURNS, not against the file's bytes.
func TestTheGeneratedCommandSignsTheSameShapeTheVerifierBuilds(t *testing.T) {
	goFormat := signingPayloadFormat(t)
	shellFormat := recipePrintfFormat(t)

	goFields := strings.Count(goFormat, "%")
	shellFields := strings.Count(shellFormat, "%")
	if goFields+1 != shellFields {
		t.Errorf("the verifier builds %d fields before the body and the screen's command signs %d in total — "+
			"one side names a field the other does not, and the disagreement reaches a member as an opaque 401",
			goFields, shellFields)
	}
	if !strings.HasSuffix(goFormat, `\n`) {
		t.Errorf("SigningPayload's format %q does not end with the separator that introduces the body — "+
			"the last field and the body would run together", goFormat)
	}
	if strings.HasSuffix(shellFormat, `\n`) {
		t.Errorf("the screen's printf format %q ends with a separator, so the command signs a trailing "+
			"newline the verifier does not", shellFormat)
	}
	for _, format := range []string{strings.TrimSuffix(goFormat, `\n`), shellFormat} {
		if strings.Contains(strings.ReplaceAll(format, `\n`, ""), `\`) {
			t.Errorf("format %q joins its fields with something other than a newline", format)
		}
	}
}

// signingPayloadFormat reads the format literal SigningPayload builds its head
// from, so this gate quotes the verifier rather than a copy of it.
func signingPayloadFormat(t *testing.T) string {
	t.Helper()
	const seamFile = "pkg/extension/inbound.go"
	file, err := parser.ParseFile(token.NewFileSet(), seamFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", seamFile, err)
	}
	var format string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "SigningPayload" {
			return true
		}
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Sprintf" {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				format = strings.Trim(lit.Value, "\"")
			}
			return false
		})
		return false
	})
	if format == "" {
		t.Fatalf("%s no longer builds SigningPayload's head with one Sprintf format — this gate reads that "+
			"shape, so a different one leaves the two sides of the signature unheld", seamFile)
	}
	return format
}

// headerConstants names the three inbound header constants the recipe must
// copy verbatim: extension.InboundHeaderTimestamp, InboundHeaderNonce and
// InboundHeaderSignature.
var headerConstants = []string{"InboundHeaderTimestamp", "InboundHeaderNonce", "InboundHeaderSignature"}

// TestTheSignedHeaderNamesAreSpelledTheSameOnBothSidesOfTheWire holds the
// third field this signature depends on both sides agreeing about: not just
// the scope and the payload shape, but the three header NAMES a member's
// pasted command sets and the core reads back. A rename on the Go side, with
// the recipe left as free-standing string literals, leaves every copied
// command signing headers the core never looks for — refused with the same
// opaque 401 as a wrong secret, and nothing in it to say why.
func TestTheSignedHeaderNamesAreSpelledTheSameOnBothSidesOfTheWire(t *testing.T) {
	values := headerConstantValues(t)
	if len(values) != len(headerConstants) {
		t.Fatalf("read %d of %d header constants from %s — the gate scanned a smaller source than it protects",
			len(values), len(headerConstants), inboundHeaderFile)
	}

	source, err := os.ReadFile(recipeFile)
	if err != nil {
		t.Fatalf("reading %s: %v", recipeFile, err)
	}
	for _, name := range headerConstants {
		value := values[name]
		if !strings.Contains(string(source), value) {
			t.Errorf("%s does not contain %q, the value of extension.%s — a rename on the Go side "+
				"leaves the recipe minting headers the core never reads, refused with the same opaque "+
				"401 as a wrong secret", recipeFile, value, name)
		}
	}
}

// inboundHeaderFile is where the three signed header names are published.
const inboundHeaderFile = "pkg/extension/inbound.go"

// headerConstantValues reads headerConstants' string values from
// inboundHeaderFile by AST rather than a hand-copied list, so a changed value
// is read here rather than assumed.
func headerConstantValues(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), inboundHeaderFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", inboundHeaderFile, err)
	}
	want := make(map[string]bool, len(headerConstants))
	for _, name := range headerConstants {
		want[name] = true
	}
	values := make(map[string]string)
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, ident := range spec.Names {
			if !want[ident.Name] || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok {
				continue
			}
			values[ident.Name] = strings.Trim(lit.Value, `"`)
		}
		return true
	})
	return values
}

// recipePrintfFormat reads the format the screen's generated command signs with.
func recipePrintfFormat(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile(recipeFile)
	if err != nil {
		t.Fatalf("reading %s: %v", recipeFile, err)
	}
	match := regexp.MustCompile(`printf '([^']+)'`).FindSubmatch(source)
	if match == nil {
		t.Fatalf("%s no longer emits a `printf '<format>'` — this gate reads that shape, so a different "+
			"one leaves the command's signed material unheld", recipeFile)
	}
	// The .tsx source escapes the separator for the template literal it sits in.
	return strings.ReplaceAll(string(match[1]), `\\n`, `\n`)
}
